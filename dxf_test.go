package main

import (
	"math"
	"testing"
)

const sampleDXF = "pouch-v5-38-sa.dxf"

// ── ParseDXF ──────────────────────────────────────────────────────────────────

func TestParseDXF_noError(t *testing.T) {
	_, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatalf("ParseDXF(%q): %v", sampleDXF, err)
	}
}

func TestParseDXF_missingFile(t *testing.T) {
	_, err := ParseDXF("nonexistent.dxf")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseDXF_insUnits(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	if d.InsUnits != 1 {
		t.Errorf("InsUnits = %d, want 1 (inch)", d.InsUnits)
	}
}

// ── Entity counts ─────────────────────────────────────────────────────────────

func TestParseDXF_entityCounts(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	var lines, points int
	for _, e := range d.Entities {
		switch e.Kind {
		case KindLine:
			lines++
		case KindPoint:
			points++
		}
	}

	if lines != 39 {
		t.Errorf("LINE count = %d, want 39", lines)
	}
	if points != 3 {
		t.Errorf("POINT count = %d, want 3", points)
	}
}

// ── Linetype table ────────────────────────────────────────────────────────────

func TestParseDXF_linetypeDASHED(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	lt, ok := d.Linetypes["DASHED"]
	if !ok {
		t.Fatalf("DASHED linetype not found; have: %v", linetypeNames(d))
	}
	if len(lt.Pattern) != 2 {
		t.Fatalf("DASHED pattern length = %d, want 2; pattern = %v", len(lt.Pattern), lt.Pattern)
	}
	if !approxEq(lt.Pattern[0], 0.5, 1e-6) {
		t.Errorf("DASHED pattern[0] = %v, want 0.5 (dash)", lt.Pattern[0])
	}
	if !approxEq(lt.Pattern[1], -0.25, 1e-6) {
		t.Errorf("DASHED pattern[1] = %v, want -0.25 (gap)", lt.Pattern[1])
	}
}

func TestParseDXF_linetypeContinuousFlag(t *testing.T) {
	lt := Linetype{Name: "CONTINUOUS"}
	if !lt.IsContinuous() {
		t.Error("CONTINUOUS should be continuous")
	}
	lt2 := Linetype{Name: "DASHED"}
	if lt2.IsContinuous() {
		t.Error("DASHED should not be continuous")
	}
}

// ── Layer table ───────────────────────────────────────────────────────────────

func TestParseDXF_layerZero(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	layer, ok := d.Layers["0"]
	if !ok {
		t.Fatalf("layer '0' not found; have: %v", layerNames(d))
	}
	if layer.Linetype != "CONTINUOUS" {
		t.Errorf("layer '0' linetype = %q, want %q", layer.Linetype, "CONTINUOUS")
	}
}

// ── Linetype resolution ───────────────────────────────────────────────────────

func TestEffectiveLinetypeName_explicit(t *testing.T) {
	d := &Drawing{
		Linetypes: map[string]Linetype{
			"DASHED": {Name: "DASHED", Pattern: []float64{0.5, -0.25}},
		},
		Layers: map[string]Layer{
			"0": {Name: "0", Linetype: "CONTINUOUS"},
		},
	}

	e := Entity{Kind: KindLine, Layer: "0", Linetype: "DASHED"}
	if got := d.EffectiveLinetypeName(e); got != "DASHED" {
		t.Errorf("EffectiveLinetypeName = %q, want %q", got, "DASHED")
	}
}

func TestEffectiveLinetypeName_byLayer(t *testing.T) {
	d := &Drawing{
		Layers: map[string]Layer{
			"cut": {Name: "cut", Linetype: "DASHED"},
		},
	}

	e := Entity{Kind: KindLine, Layer: "cut", Linetype: ""}
	if got := d.EffectiveLinetypeName(e); got != "DASHED" {
		t.Errorf("EffectiveLinetypeName = %q, want DASHED", got)
	}
}

func TestEffectiveLinetypeName_byLayerFallback(t *testing.T) {
	d := &Drawing{
		Layers: map[string]Layer{
			"0": {Name: "0", Linetype: "CONTINUOUS"},
		},
	}
	e := Entity{Kind: KindLine, Layer: "0", Linetype: "BYLAYER"}
	if got := d.EffectiveLinetypeName(e); got != "CONTINUOUS" {
		t.Errorf("EffectiveLinetypeName = %q, want CONTINUOUS", got)
	}
}

func TestEffectiveLinetypeName_unknownLayer(t *testing.T) {
	d := &Drawing{Layers: map[string]Layer{}}
	e := Entity{Kind: KindLine, Layer: "missing"}
	if got := d.EffectiveLinetypeName(e); got != "CONTINUOUS" {
		t.Errorf("EffectiveLinetypeName = %q, want CONTINUOUS for missing layer", got)
	}
}

func TestEffectiveLinetypeName_sampleFile(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	var dashedCount int
	for _, e := range d.Entities {
		if d.EffectiveLinetypeName(e) == "DASHED" {
			dashedCount++
		}
	}
	// 23 LINE entities carry an explicit DASHED linetype group; layer "0" is CONTINUOUS
	if dashedCount != 23 {
		t.Errorf("DASHED entity count = %d, want 23", dashedCount)
	}
}

// ── DashPattern ───────────────────────────────────────────────────────────────

func TestDashPattern_continuous(t *testing.T) {
	d := &Drawing{Linetypes: map[string]Linetype{}}
	if p := d.DashPattern("CONTINUOUS"); p != nil {
		t.Errorf("DashPattern(CONTINUOUS) = %v, want nil", p)
	}
	if p := d.DashPattern(""); p != nil {
		t.Errorf("DashPattern(\"\") = %v, want nil", p)
	}
}

func TestDashPattern_dashed(t *testing.T) {
	d := &Drawing{
		Linetypes: map[string]Linetype{
			"DASHED": {Name: "DASHED", Pattern: []float64{0.5, -0.25}},
		},
	}
	p := d.DashPattern("DASHED")
	if p == nil {
		t.Fatal("DashPattern(DASHED) = nil, want non-nil")
	}
	if len(p) != 2 {
		t.Fatalf("len = %d, want 2", len(p))
	}
}

// ── Curve flattening (smoke tests) ────────────────────────────────────────────

func TestFlattenEllipse_fullCircle(t *testing.T) {
	// A circle is an ellipse with ratio=1, major axis along X with length r.
	r := 5.0
	pts := flattenEllipse(0, 0, r, 0, 1.0, 0, 2*math.Pi, 64)
	if len(pts) == 0 {
		t.Fatal("flattenEllipse returned no points")
	}
	// First and last points should be near the same place on the circle.
	first, last := pts[0], pts[len(pts)-1]
	if !approxEq(first.X, r, 1e-6) {
		t.Errorf("first.X = %v, want %v", first.X, r)
	}
	if !approxEq(first.Y, 0, 1e-6) {
		t.Errorf("first.Y = %v, want 0", first.Y)
	}
	// Last point should be just before 2π ≈ first point.
	d := math.Sqrt((last.X-first.X)*(last.X-first.X) + (last.Y-first.Y)*(last.Y-first.Y))
	if d > 0.1 {
		t.Errorf("first/last distance = %v, want near 0 for closed circle", d)
	}
}

func TestFlattenBSpline_line(t *testing.T) {
	// A degree-1 B-spline through two points should be the straight line.
	ctrl := []DXFPoint{{0, 0}, {10, 10}}
	knots := []float64{0, 0, 1, 1}
	pts := flattenBSpline(ctrl, knots, 1, 4)
	if len(pts) == 0 {
		t.Fatal("flattenBSpline returned no points")
	}
	// All points should lie on y=x.
	for _, p := range pts {
		if !approxEq(p.X, p.Y, 1e-6) {
			t.Errorf("point (%v,%v) not on y=x line", p.X, p.Y)
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func approxEq(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func linetypeNames(d *Drawing) []string {
	var names []string
	for k := range d.Linetypes {
		names = append(names, k)
	}
	return names
}

func layerNames(d *Drawing) []string {
	var names []string
	for k := range d.Layers {
		names = append(names, k)
	}
	return names
}
