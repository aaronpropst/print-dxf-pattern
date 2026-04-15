package main

import (
	"math"
	"os"
	"testing"

	"github.com/go-pdf/fpdf"
)

// ── dashPatternMM ─────────────────────────────────────────────────────────────

func TestDashPatternMM_nil(t *testing.T) {
	if p := dashPatternMM(nil, 25.4); p != nil {
		t.Errorf("nil pattern → %v, want nil", p)
	}
	if p := dashPatternMM([]float64{}, 25.4); p != nil {
		t.Errorf("empty pattern → %v, want nil", p)
	}
}

func TestDashPatternMM_values(t *testing.T) {
	// DXF DASHED: [0.5, -0.25] in inches, scale=25.4 mm/in
	p := dashPatternMM([]float64{0.5, -0.25}, 25.4)
	if len(p) != 2 {
		t.Fatalf("len = %d, want 2", len(p))
	}
	// 0.5 * 25.4 * 0.3 = 3.81 mm
	wantDash := 0.5 * 25.4 * dashScale
	wantGap := 0.25 * 25.4 * dashScale
	if !approxEq(p[0], wantDash, 1e-6) {
		t.Errorf("dash = %.4f, want %.4f", p[0], wantDash)
	}
	if !approxEq(p[1], wantGap, 1e-6) {
		t.Errorf("gap = %.4f, want %.4f", p[1], wantGap)
	}
}

func TestDashPatternMM_allPositive(t *testing.T) {
	// All output values must be positive (gap elements are negated in source).
	p := dashPatternMM([]float64{1.0, -0.5, 0.25, -0.125}, 25.4)
	for i, v := range p {
		if v <= 0 {
			t.Errorf("p[%d] = %v, want > 0", i, v)
		}
	}
}

func TestDashPatternMM_oddLengthDoubled(t *testing.T) {
	// Odd-length patterns must be doubled so fpdf gets even-length array.
	p := dashPatternMM([]float64{1.0, -0.5, 0.25}, 25.4)
	if len(p)%2 != 0 {
		t.Errorf("len = %d, want even", len(p))
	}
	if len(p) != 6 {
		t.Errorf("len = %d, want 6 (3 doubled)", len(p))
	}
}

func TestDashPatternMM_dotMinimum(t *testing.T) {
	// A zero element should become dotMM, not zero.
	p := dashPatternMM([]float64{0, -0}, 25.4)
	for i, v := range p {
		if v < dotMM-1e-9 {
			t.Errorf("p[%d] = %v, want >= dotMM (%.3f)", i, v, dotMM)
		}
	}
}

// ── arcPoints ─────────────────────────────────────────────────────────────────

func TestArcPoints_quarterCircle(t *testing.T) {
	// 0° to 90°, r=1 → from (1,0) to (0,1)
	pts := arcPoints(0, 0, 1, 0, 90, 16)
	if len(pts) != 17 {
		t.Fatalf("len = %d, want 17", len(pts))
	}
	first, last := pts[0], pts[16]
	if !approxEq(first.X, 1, 1e-6) || !approxEq(first.Y, 0, 1e-6) {
		t.Errorf("first = (%.4f, %.4f), want (1, 0)", first.X, first.Y)
	}
	if !approxEq(last.X, 0, 1e-6) || !approxEq(last.Y, 1, 1e-6) {
		t.Errorf("last = (%.4f, %.4f), want (0, 1)", last.X, last.Y)
	}
}

func TestArcPoints_wraparound(t *testing.T) {
	// 270° to 90° wraps: should sweep CCW through 0° (endDeg normalised to 450°).
	pts := arcPoints(0, 0, 1, 270, 90, 8)
	if len(pts) == 0 {
		t.Fatal("no points returned")
	}
	first, last := pts[0], pts[len(pts)-1]
	// First point at 270° = (0, -1)
	if !approxEq(first.X, 0, 1e-6) || !approxEq(first.Y, -1, 1e-6) {
		t.Errorf("first = (%.4f, %.4f), want (0, -1)", first.X, first.Y)
	}
	// Last point at 90° = (0, 1)
	if !approxEq(last.X, 0, 1e-6) || !approxEq(last.Y, 1, 1e-6) {
		t.Errorf("last = (%.4f, %.4f), want (0, 1)", last.X, last.Y)
	}
}

func TestArcPoints_allOnCircle(t *testing.T) {
	pts := arcPoints(3, 4, 5, 0, 180, 32)
	for i, p := range pts {
		d := math.Sqrt((p.X-3)*(p.X-3) + (p.Y-4)*(p.Y-4))
		if !approxEq(d, 5, 1e-6) {
			t.Errorf("pts[%d] distance from center = %.6f, want 5.0", i, d)
		}
	}
}

// ── DrawEntity / DrawEntities (integration against real DXF) ─────────────────

// newLetterPDF creates a bare fpdf instance matching the test page setup.
func newLetterPDF() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()
	pdf.SetLineWidth(0.3)
	return pdf
}

// sampleXform builds the WorldToPage for tile (0,0) of the sample DXF on letter paper.
func sampleXform() (WorldToPage, error) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		return WorldToPage{}, err
	}
	minX, minY, _, _, err := DrawingBBox(d.Entities)
	if err != nil {
		return WorldToPage{}, err
	}
	_, pageH := PageSizeMM("letter")
	margin := 10.0
	scale := ScaleToMM("inch")
	return WorldToPage{
		Scale:       scale,
		WorldX0:     minX,
		WorldY0:     minY,
		PageX0:      margin,
		PageBottomY: pageH - margin,
	}, nil
}

func TestDrawEntities_producesFile(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	xf, err := sampleXform()
	if err != nil {
		t.Fatal(err)
	}

	pdf := newLetterPDF()
	DrawEntities(pdf, d.Entities, xf, d, false)

	out := t.TempDir() + "/all.pdf"
	if err := pdf.OutputFileAndClose(out); err != nil {
		t.Fatalf("OutputFileAndClose: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output PDF is empty")
	}
	t.Logf("all-entities PDF: %d bytes", info.Size())
}

func TestDrawEntities_noDashedSmallerOrEqual(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	xf, err := sampleXform()
	if err != nil {
		t.Fatal(err)
	}

	writeEntities := func(noDashed bool) int64 {
		pdf := newLetterPDF()
		DrawEntities(pdf, d.Entities, xf, d, noDashed)
		out := t.TempDir() + "/out.pdf"
		if err := pdf.OutputFileAndClose(out); err != nil {
			t.Fatalf("OutputFileAndClose: %v", err)
		}
		info, _ := os.Stat(out)
		return info.Size()
	}

	sizeAll := writeEntities(false)
	sizeNoDash := writeEntities(true)

	if sizeAll == 0 || sizeNoDash == 0 {
		t.Error("one of the PDFs is empty")
	}
	if sizeNoDash > sizeAll {
		t.Errorf("noDashed PDF (%d bytes) is larger than full PDF (%d bytes)", sizeNoDash, sizeAll)
	}
	t.Logf("full=%d bytes  noDashed=%d bytes  (skipped %d bytes of dashed content)",
		sizeAll, sizeNoDash, sizeAll-sizeNoDash)
}

func TestDrawEntities_noPanic_allKinds(t *testing.T) {
	// Synthetic entities covering every Kind — just verify no panic.
	d := &Drawing{
		Linetypes: map[string]Linetype{
			"DASHED": {Name: "DASHED", Pattern: []float64{0.5, -0.25}},
		},
		Layers: map[string]Layer{
			"0": {Name: "0", Linetype: "CONTINUOUS"},
		},
	}
	entities := []Entity{
		{Kind: KindLine, Layer: "0", X1: 0, Y1: 0, X2: 10, Y2: 10},
		{Kind: KindLine, Layer: "0", Linetype: "DASHED", X1: 0, Y1: 5, X2: 10, Y2: 5},
		{Kind: KindPolyline, Layer: "0", Points: []DXFPoint{{0, 0}, {5, 5}, {10, 0}}, Closed: false},
		{Kind: KindPolyline, Layer: "0", Points: []DXFPoint{{0, 0}, {5, 5}, {10, 0}}, Closed: true},
		{Kind: KindCircle, Layer: "0", CX: 5, CY: 5, Radius: 3},
		{Kind: KindArc, Layer: "0", CX: 5, CY: 5, Radius: 3, StartAngle: 0, EndAngle: 90},
		{Kind: KindPoint, Layer: "0", PX: 5, PY: 5},
		{Kind: KindPolyline, Layer: "0"}, // empty — should not panic
	}

	_, pageH := PageSizeMM("letter")
	margin := 10.0
	xf := WorldToPage{
		Scale:       25.4,
		WorldX0:     0,
		WorldY0:     0,
		PageX0:      margin,
		PageBottomY: pageH - margin,
	}

	pdf := newLetterPDF()
	DrawEntities(pdf, entities, xf, d, false)

	out := t.TempDir() + "/kinds.pdf"
	if err := pdf.OutputFileAndClose(out); err != nil {
		t.Fatalf("OutputFileAndClose: %v", err)
	}
}
