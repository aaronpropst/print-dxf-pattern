package main

import (
	"fmt"
	"math"
)

// ── Page / unit helpers ───────────────────────────────────────────────────────

// PageSizeMM returns (width, height) in mm for a named page size.
func PageSizeMM(name string) (float64, float64) {
	switch name {
	case "a4":
		return 210.0, 297.0
	default: // letter
		return 215.9, 279.4
	}
}

// ScaleToMM returns the factor that converts DXF world units to millimetres.
func ScaleToMM(dxfUnits string) float64 {
	if dxfUnits == "mm" {
		return 1.0
	}
	return 25.4 // inch → mm
}

// ── Coordinate transform ──────────────────────────────────────────────────────

// WorldToPage transforms DXF world coordinates to fpdf page coordinates (mm).
//
// DXF uses a Y-up convention (origin at bottom-left).
// fpdf uses a Y-down convention (origin at top-left).
// W2P performs the Y-flip so geometry renders right-side-up on the page.
type WorldToPage struct {
	Scale       float64 // world units → mm
	WorldX0     float64 // world X of the printable area's left edge
	WorldY0     float64 // world Y of the printable area's bottom edge (DXF Y-up)
	PageX0      float64 // fpdf X of the printable area's left edge (mm from left)
	PageBottomY float64 // fpdf Y of the printable area's bottom edge (mm from top)
}

// W2P converts a world-unit point to fpdf mm coordinates.
func (t WorldToPage) W2P(x, y float64) (float64, float64) {
	px := t.PageX0 + (x-t.WorldX0)*t.Scale
	py := t.PageBottomY - (y-t.WorldY0)*t.Scale // Y-flip
	return px, py
}

// ── Bounding boxes ────────────────────────────────────────────────────────────

// EntityBBox returns the axis-aligned bounding box of e in world units.
// ok is false for empty or unsupported entities.
func EntityBBox(e Entity) (minX, minY, maxX, maxY float64, ok bool) {
	switch e.Kind {
	case KindLine:
		return math.Min(e.X1, e.X2), math.Min(e.Y1, e.Y2),
			math.Max(e.X1, e.X2), math.Max(e.Y1, e.Y2), true

	case KindPolyline:
		if len(e.Points) == 0 {
			return 0, 0, 0, 0, false
		}
		minX, minY = e.Points[0].X, e.Points[0].Y
		maxX, maxY = minX, minY
		for _, p := range e.Points[1:] {
			if p.X < minX {
				minX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
			if p.X > maxX {
				maxX = p.X
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
		return minX, minY, maxX, maxY, true

	case KindCircle, KindArc:
		// Conservative: full circle bounds.
		return e.CX - e.Radius, e.CY - e.Radius,
			e.CX + e.Radius, e.CY + e.Radius, true

	case KindPoint:
		return e.PX, e.PY, e.PX, e.PY, true
	}
	return 0, 0, 0, 0, false
}

// DrawingBBox returns the union bounding box of all entities in world units.
func DrawingBBox(entities []Entity) (minX, minY, maxX, maxY float64, err error) {
	first := true
	for _, e := range entities {
		x0, y0, x1, y1, ok := EntityBBox(e)
		if !ok {
			continue
		}
		if first {
			minX, minY, maxX, maxY = x0, y0, x1, y1
			first = false
			continue
		}
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	if first {
		return 0, 0, 0, 0, fmt.Errorf("no supported geometry found in DXF")
	}
	return minX, minY, maxX, maxY, nil
}

// ── Tiling ────────────────────────────────────────────────────────────────────

// Tile describes one page of a tiled layout.
type Tile struct {
	Col, Row         int     // zero-based grid position (Col=X, Row=Y)
	WorldX0, WorldY0 float64 // world coords of the tile's printable-area origin (bottom-left)
}

// ComputeTiles divides a drawing bounding box into overlapping page-sized tiles.
// All length arguments are in DXF world units.
func ComputeTiles(minX, minY, maxX, maxY, printableW, printableH, overlap float64) ([]Tile, int, int, error) {
	stepW := printableW - overlap
	stepH := printableH - overlap
	if stepW <= 0 || stepH <= 0 {
		return nil, 0, 0, fmt.Errorf("overlap %.3f exceeds printable area (%.3f × %.3f)", overlap, printableW, printableH)
	}

	totalW := maxX - minX
	totalH := maxY - minY

	nx := atLeast1(int(math.Ceil((totalW - overlap) / stepW)))
	ny := atLeast1(int(math.Ceil((totalH - overlap) / stepH)))

	tiles := make([]Tile, 0, nx*ny)
	for row := 0; row < ny; row++ {
		for col := 0; col < nx; col++ {
			tiles = append(tiles, Tile{
				Col:     col,
				Row:     row,
				WorldX0: minX + float64(col)*stepW,
				WorldY0: minY + float64(row)*stepH,
			})
		}
	}
	return tiles, nx, ny, nil
}

func atLeast1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
