package main

import (
	"math"
	"testing"
)

// ── PageSizeMM / ScaleToMM ────────────────────────────────────────────────────

func TestPageSizeMM(t *testing.T) {
	w, h := PageSizeMM("letter")
	if !approxEq(w, 215.9, 0.1) || !approxEq(h, 279.4, 0.1) {
		t.Errorf("letter = (%.1f, %.1f), want (215.9, 279.4)", w, h)
	}
	w, h = PageSizeMM("a4")
	if !approxEq(w, 210.0, 0.1) || !approxEq(h, 297.0, 0.1) {
		t.Errorf("a4 = (%.1f, %.1f), want (210.0, 297.0)", w, h)
	}
	// Unknown name falls back to letter.
	w, h = PageSizeMM("unknown")
	if !approxEq(w, 215.9, 0.1) {
		t.Errorf("unknown page = %.1f wide, want letter width 215.9", w)
	}
}

func TestScaleToMM(t *testing.T) {
	if !approxEq(ScaleToMM("mm"), 1.0, 1e-9) {
		t.Errorf("ScaleToMM(mm) = %v, want 1.0", ScaleToMM("mm"))
	}
	if !approxEq(ScaleToMM("inch"), 25.4, 1e-9) {
		t.Errorf("ScaleToMM(inch) = %v, want 25.4", ScaleToMM("inch"))
	}
}

// ── WorldToPage / W2P ─────────────────────────────────────────────────────────

// letterXform builds a WorldToPage for a letter page (10mm margin) with the
// drawing origin at (worldX0, worldY0) in world units (inches).
func letterXform(worldX0, worldY0 float64) WorldToPage {
	_, pageH := PageSizeMM("letter")
	margin := 10.0
	return WorldToPage{
		Scale:       ScaleToMM("inch"),
		WorldX0:     worldX0,
		WorldY0:     worldY0,
		PageX0:      margin,
		PageBottomY: pageH - margin,
	}
}

func TestW2P_originMapsToMargin(t *testing.T) {
	xf := letterXform(0, 0)
	pageW, pageH := PageSizeMM("letter")
	margin := 10.0

	px, py := xf.W2P(0, 0)
	// World (0,0) → printable bottom-left → fpdf (margin, pageH-margin)
	if !approxEq(px, margin, 1e-6) {
		t.Errorf("px = %.4f, want %.4f (left margin)", px, margin)
	}
	if !approxEq(py, pageH-margin, 1e-6) {
		t.Errorf("py = %.4f, want %.4f (bottom margin in fpdf Y-down)", py, pageH-margin)
	}
	_ = pageW
}

func TestW2P_topEdgeMapsToTopMargin(t *testing.T) {
	_, pageH := PageSizeMM("letter")
	margin := 10.0
	scale := ScaleToMM("inch")
	printableH_wu := (pageH - 2*margin) / scale

	xf := letterXform(0, 0)
	_, py := xf.W2P(0, printableH_wu)
	// World top of printable area → fpdf y = margin (top of printable in Y-down)
	if !approxEq(py, margin, 1e-6) {
		t.Errorf("py at top = %.4f, want %.4f (top margin)", py, margin)
	}
}

func TestW2P_yFlip(t *testing.T) {
	// A point higher in world space must have a smaller fpdf Y (closer to top).
	xf := letterXform(0, 0)
	_, y1 := xf.W2P(0, 1)
	_, y2 := xf.W2P(0, 2)
	if y2 >= y1 {
		t.Errorf("Y-flip broken: W2P(0,2).py=%.4f should be < W2P(0,1).py=%.4f", y2, y1)
	}
}

func TestW2P_xMonotone(t *testing.T) {
	xf := letterXform(0, 0)
	x1, _ := xf.W2P(1, 0)
	x2, _ := xf.W2P(2, 0)
	if x2 <= x1 {
		t.Errorf("X not monotone: W2P(2,0).px=%.4f should be > W2P(1,0).px=%.4f", x2, x1)
	}
}

func TestW2P_tileOffset(t *testing.T) {
	// Shifting worldX0/worldY0 should shift the projected point by -scale*offset.
	xf0 := letterXform(0, 0)
	xf1 := letterXform(1, 1) // tile offset by (1,1) inch
	scale := ScaleToMM("inch")

	px0, py0 := xf0.W2P(5, 5)
	px1, py1 := xf1.W2P(5, 5)

	if !approxEq(px1-px0, -scale, 1e-6) {
		t.Errorf("tile X shift = %.4f, want %.4f", px1-px0, -scale)
	}
	if !approxEq(py1-py0, scale, 1e-6) { // Y is flipped so positive world shift → positive fpdf shift
		t.Errorf("tile Y shift = %.4f, want %.4f", py1-py0, scale)
	}
}

// ── EntityBBox ────────────────────────────────────────────────────────────────

func TestEntityBBox_line(t *testing.T) {
	e := Entity{Kind: KindLine, X1: 1, Y1: 2, X2: 5, Y2: -3}
	x0, y0, x1, y1, ok := EntityBBox(e)
	if !ok {
		t.Fatal("EntityBBox returned ok=false for LINE")
	}
	check := func(name string, got, want float64) {
		if !approxEq(got, want, 1e-9) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("minX", x0, 1)
	check("minY", y0, -3)
	check("maxX", x1, 5)
	check("maxY", y1, 2)
}

func TestEntityBBox_lineReversed(t *testing.T) {
	e := Entity{Kind: KindLine, X1: 5, Y1: 2, X2: 1, Y2: -3}
	x0, y0, x1, y1, ok := EntityBBox(e)
	if !ok {
		t.Fatal("ok=false")
	}
	if x0 > x1 || y0 > y1 {
		t.Errorf("bbox not normalized: (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

func TestEntityBBox_polyline(t *testing.T) {
	e := Entity{Kind: KindPolyline, Points: []DXFPoint{{0, 0}, {3, 1}, {-1, 5}}}
	x0, y0, x1, y1, ok := EntityBBox(e)
	if !ok {
		t.Fatal("ok=false")
	}
	if !approxEq(x0, -1, 1e-9) || !approxEq(y0, 0, 1e-9) ||
		!approxEq(x1, 3, 1e-9) || !approxEq(y1, 5, 1e-9) {
		t.Errorf("polyline bbox (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

func TestEntityBBox_emptyPolyline(t *testing.T) {
	e := Entity{Kind: KindPolyline}
	_, _, _, _, ok := EntityBBox(e)
	if ok {
		t.Error("empty polyline should return ok=false")
	}
}

func TestEntityBBox_circle(t *testing.T) {
	e := Entity{Kind: KindCircle, CX: 2, CY: 3, Radius: 5}
	x0, y0, x1, y1, ok := EntityBBox(e)
	if !ok {
		t.Fatal("ok=false")
	}
	if !approxEq(x0, -3, 1e-9) || !approxEq(y0, -2, 1e-9) ||
		!approxEq(x1, 7, 1e-9) || !approxEq(y1, 8, 1e-9) {
		t.Errorf("circle bbox (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

func TestEntityBBox_point(t *testing.T) {
	e := Entity{Kind: KindPoint, PX: 7, PY: -2}
	x0, y0, x1, y1, ok := EntityBBox(e)
	if !ok {
		t.Fatal("ok=false")
	}
	if !approxEq(x0, 7, 1e-9) || !approxEq(y0, -2, 1e-9) ||
		!approxEq(x1, 7, 1e-9) || !approxEq(y1, -2, 1e-9) {
		t.Errorf("point bbox (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

// ── DrawingBBox ───────────────────────────────────────────────────────────────

func TestDrawingBBox_union(t *testing.T) {
	entities := []Entity{
		{Kind: KindLine, X1: 0, Y1: 0, X2: 5, Y2: 3},
		{Kind: KindLine, X1: -1, Y1: -2, X2: 4, Y2: 1},
		{Kind: KindPoint, PX: 10, PY: 0},
	}
	x0, y0, x1, y1, err := DrawingBBox(entities)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEq(x0, -1, 1e-9) || !approxEq(y0, -2, 1e-9) ||
		!approxEq(x1, 10, 1e-9) || !approxEq(y1, 3, 1e-9) {
		t.Errorf("union bbox (%v,%v)-(%v,%v), want (-1,-2)-(10,3)", x0, y0, x1, y1)
	}
}

func TestDrawingBBox_empty(t *testing.T) {
	_, _, _, _, err := DrawingBBox(nil)
	if err == nil {
		t.Error("expected error for empty entity list, got nil")
	}
}

func TestDrawingBBox_sampleDXF(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	x0, y0, x1, y1, err := DrawingBBox(d.Entities)
	if err != nil {
		t.Fatal(err)
	}
	if x1 <= x0 || y1 <= y0 {
		t.Errorf("degenerate bbox (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
	// The DXF is in inches; a sewn pouch pattern is on the order of inches, not feet or microns.
	w, h := x1-x0, y1-y0
	if w < 0.1 || w > 100 {
		t.Errorf("suspicious width %.4f inches", w)
	}
	if h < 0.1 || h > 100 {
		t.Errorf("suspicious height %.4f inches", h)
	}
}

// ── ComputeTiles ──────────────────────────────────────────────────────────────

func TestComputeTiles_singleTile(t *testing.T) {
	// Drawing fits in one page.
	tiles, nx, ny, err := ComputeTiles(0, 0, 5, 5, 10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if nx != 1 || ny != 1 {
		t.Errorf("nx=%d ny=%d, want 1×1", nx, ny)
	}
	if len(tiles) != 1 {
		t.Fatalf("len(tiles) = %d, want 1", len(tiles))
	}
	if tiles[0].Col != 0 || tiles[0].Row != 0 {
		t.Errorf("tile[0] col=%d row=%d, want 0,0", tiles[0].Col, tiles[0].Row)
	}
	if !approxEq(tiles[0].WorldX0, 0, 1e-9) || !approxEq(tiles[0].WorldY0, 0, 1e-9) {
		t.Errorf("tile[0] origin (%.4f,%.4f), want (0,0)", tiles[0].WorldX0, tiles[0].WorldY0)
	}
}

func TestComputeTiles_2x2(t *testing.T) {
	// Drawing = 15×15, printable = 10×10, overlap = 1 → step = 9
	// ceil((15-1)/9) = ceil(14/9) = ceil(1.556) = 2 → 2×2
	tiles, nx, ny, err := ComputeTiles(0, 0, 15, 15, 10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if nx != 2 || ny != 2 {
		t.Errorf("nx=%d ny=%d, want 2×2", nx, ny)
	}
	if len(tiles) != 4 {
		t.Fatalf("len(tiles) = %d, want 4", len(tiles))
	}
	// Second column tile should start at step=9.
	if !approxEq(tiles[1].WorldX0, 9, 1e-9) {
		t.Errorf("tiles[1].WorldX0 = %.4f, want 9", tiles[1].WorldX0)
	}
	// Second row tile (tiles[2]) should start at step=9.
	if !approxEq(tiles[2].WorldY0, 9, 1e-9) {
		t.Errorf("tiles[2].WorldY0 = %.4f, want 9", tiles[2].WorldY0)
	}
}

func TestComputeTiles_rowMajorOrder(t *testing.T) {
	// Verify col-major inner, row-major outer ordering (col varies fastest).
	tiles, nx, ny, err := ComputeTiles(0, 0, 20, 30, 10, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if nx != 2 || ny != 3 {
		t.Errorf("nx=%d ny=%d, want 2×3", nx, ny)
	}
	for idx, tile := range tiles {
		wantCol := idx % nx
		wantRow := idx / nx
		if tile.Col != wantCol || tile.Row != wantRow {
			t.Errorf("tiles[%d]: col=%d row=%d, want %d,%d", idx, tile.Col, tile.Row, wantCol, wantRow)
		}
	}
}

func TestComputeTiles_overlapTooLarge(t *testing.T) {
	_, _, _, err := ComputeTiles(0, 0, 10, 10, 5, 5, 6)
	if err == nil {
		t.Error("expected error when overlap > printable, got nil")
	}
}

func TestComputeTiles_nonZeroOrigin(t *testing.T) {
	// Drawing offset from origin.
	tiles, _, _, err := ComputeTiles(100, 200, 110, 210, 10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEq(tiles[0].WorldX0, 100, 1e-9) || !approxEq(tiles[0].WorldY0, 200, 1e-9) {
		t.Errorf("first tile origin (%.4f,%.4f), want (100,200)", tiles[0].WorldX0, tiles[0].WorldY0)
	}
}

func TestComputeTiles_sampleDXF(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	minX, minY, maxX, maxY, err := DrawingBBox(d.Entities)
	if err != nil {
		t.Fatal(err)
	}

	scale := ScaleToMM("inch")
	_, pageH := PageSizeMM("letter")
	marginMM := 10.0
	overlapMM := 10.0
	printableW := (215.9 - 2*marginMM) / scale
	printableH := (pageH - 2*marginMM) / scale
	overlap := overlapMM / scale

	tiles, nx, ny, err := ComputeTiles(minX, minY, maxX, maxY, printableW, printableH, overlap)
	if err != nil {
		t.Fatal(err)
	}
	if len(tiles) == 0 {
		t.Error("expected at least one tile")
	}
	t.Logf("sample DXF tiling: %d×%d = %d tiles (bbox %.3f×%.3f in)",
		nx, ny, len(tiles), maxX-minX, maxY-minY)

	// All tile origins should be within or near the drawing bbox.
	for _, tile := range tiles {
		if tile.WorldX0 < minX-1e-6 || tile.WorldY0 < minY-1e-6 {
			t.Errorf("tile (%d,%d) origin (%.4f,%.4f) outside bbox", tile.Col, tile.Row, tile.WorldX0, tile.WorldY0)
		}
	}

	// Tiles should cover the entire drawing in X and Y.
	stepW := printableW - overlap
	stepH := printableH - overlap
	lastTileMaxX := tiles[len(tiles)-1].WorldX0 + printableW
	lastTileMaxY := tiles[len(tiles)-1].WorldY0 + printableH
	_ = stepW
	_ = stepH
	if lastTileMaxX < maxX {
		t.Errorf("tiles don't cover maxX: last tile reaches %.4f, drawing maxX %.4f", lastTileMaxX, maxX)
	}
	if lastTileMaxY < maxY {
		t.Errorf("tiles don't cover maxY: last tile reaches %.4f, drawing maxY %.4f", lastTileMaxY, maxY)
	}
}

// ── Round-trip: W2P stays within page bounds ──────────────────────────────────

func TestW2P_staysOnPage(t *testing.T) {
	_, pageH := PageSizeMM("letter")
	margin := 10.0
	scale := ScaleToMM("inch")
	printableW_wu := (215.9 - 2*margin) / scale
	printableH_wu := (pageH - 2*margin) / scale

	xf := WorldToPage{
		Scale:       scale,
		WorldX0:     0,
		WorldY0:     0,
		PageX0:      margin,
		PageBottomY: pageH - margin,
	}

	corners := [][2]float64{
		{0, 0},
		{printableW_wu, 0},
		{0, printableH_wu},
		{printableW_wu, printableH_wu},
	}
	for _, c := range corners {
		px, py := xf.W2P(c[0], c[1])
		if px < margin-1e-6 || px > 215.9-margin+1e-6 {
			t.Errorf("W2P(%.3f,%.3f) px=%.4f outside [%.1f, %.1f]", c[0], c[1], px, margin, 215.9-margin)
		}
		if py < margin-1e-6 || py > pageH-margin+1e-6 {
			t.Errorf("W2P(%.3f,%.3f) py=%.4f outside [%.1f, %.1f]", c[0], c[1], py, margin, pageH-margin)
		}
	}
	_ = math.Pi // keep math import used
}
