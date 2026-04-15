package main

import (
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// fpdfPageSize maps our page name to the string fpdf expects.
func fpdfPageSize(page string) string {
	switch strings.ToLower(page) {
	case "a4":
		return "A4"
	default:
		return "Letter"
	}
}

// filterEntities returns the subset of entities on any of the named layers.
// If layers is empty the full slice is returned unchanged.
func filterEntities(entities []Entity, layers []string) []Entity {
	if len(layers) == 0 {
		return entities
	}
	set := make(map[string]bool, len(layers))
	for _, l := range layers {
		set[l] = true
	}
	out := entities[:0:0]
	for _, e := range entities {
		if set[e.Layer] {
			out = append(out, e)
		}
	}
	return out
}

// drawScaleBar draws a 100 mm scale bar with tick marks and a label.
// (x, y) is the left end of the bar in fpdf mm coordinates.
func drawScaleBar(pdf *fpdf.Fpdf, x, y, lengthMM float64) {
	tickH := 2.0 // half-height of end ticks in mm
	pdf.SetLineWidth(0.5)
	pdf.SetDashPattern([]float64{}, 0)
	pdf.Line(x, y, x+lengthMM, y)
	pdf.Line(x, y-tickH, x, y+tickH)
	pdf.Line(x+lengthMM, y-tickH, x+lengthMM, y+tickH)
	pdf.SetFont("Helvetica", "", 8)
	pdf.Text(x, y-tickH-1, fmt.Sprintf("%.0f mm scale bar", lengthMM))
}

// drawEdgeAlignmentLine draws a sparsely-dotted line across the page showing
// where the adjacent page's paper edge should be placed.
//
//   - If vertical=true, draws a vertical line at x=pos.
//   - If vertical=false, draws a horizontal line at y=pos.
//
// insetMM shortens the line at both ends so it remains visible after taping.
func drawEdgeAlignmentLine(pdf *fpdf.Fpdf, pos, pageW, pageH, insetMM float64, vertical bool) {
	pdf.SetLineWidth(0.8)
	pdf.SetDashPattern([]float64{0.5, 9.5}, 0)
	if vertical {
		pdf.Line(pos, insetMM, pos, pageH-insetMM)
	} else {
		pdf.Line(insetMM, pos, pageW-insetMM, pos)
	}
	pdf.SetDashPattern([]float64{}, 0)
	pdf.SetLineWidth(0.3)
}

// RenderTiledPDF builds the complete tiled PDF from cfg and writes it to disk.
func RenderTiledPDF(cfg *config, d *Drawing) error {
	entities := filterEntities(d.Entities, cfg.layers)
	if len(entities) == 0 {
		return fmt.Errorf("no entities found (check --layers filter and input file)")
	}

	minX, minY, maxX, maxY, err := DrawingBBox(entities)
	if err != nil {
		return err
	}

	scale := ScaleToMM(cfg.dxfUnits)
	pageW, pageH := PageSizeMM(cfg.page)
	marginMM := cfg.marginMM
	overlapMM := cfg.overlapMM
	printableW_mm := pageW - 2*marginMM
	printableH_mm := pageH - 2*marginMM
	printableW_wu := printableW_mm / scale
	printableH_wu := printableH_mm / scale
	overlap_wu := overlapMM / scale
	stepW_mm := printableW_mm - overlapMM
	stepH_mm := printableH_mm - overlapMM

	tiles, nx, ny, err := ComputeTiles(minX, minY, maxX, maxY, printableW_wu, printableH_wu, overlap_wu)
	if err != nil {
		return err
	}

	pdf := fpdf.New("P", "mm", fpdfPageSize(cfg.page), "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	for _, tile := range tiles {
		pdf.AddPage()
		pdf.SetLineWidth(0.3)
		pdf.SetDashPattern([]float64{}, 0)

		xf := WorldToPage{
			Scale:       scale,
			WorldX0:     tile.WorldX0,
			WorldY0:     tile.WorldY0,
			PageX0:      marginMM,
			PageBottomY: pageH - marginMM,
		}

		// ── Header ────────────────────────────────────────────────────────────
		pdf.SetFont("Helvetica", "", 9)
		pdf.Text(marginMM, marginMM-2,
			fmt.Sprintf("col: %d/%d  row: %d/%d", tile.Col+1, nx, tile.Row+1, ny))

		// ── Scale bar (first tile only) ───────────────────────────────────────
		if tile.Col == 0 && tile.Row == 0 {
			drawScaleBar(pdf, marginMM+5, marginMM+10, 100)
		}

		// ── Edge-alignment seam lines ──────────────────────────────────────────
		// Vertical seam: shows where the next column's left paper edge lands.
		if tile.Col < nx-1 {
			drawEdgeAlignmentLine(pdf, marginMM+stepW_mm, pageW, pageH, 10, true)
		}
		// Horizontal seam: shows where the next row's top paper edge lands.
		// In fpdf (Y-down) the next row is UP in DXF space → smaller fpdf Y.
		if tile.Row < ny-1 {
			seamY := pageH - marginMM - stepH_mm
			drawEdgeAlignmentLine(pdf, seamY, pageW, pageH, 10, false)
		}

		// ── Entities ──────────────────────────────────────────────────────────
		pdf.SetLineWidth(0.3)
		DrawEntities(pdf, entities, xf, d, cfg.noDashed)

		// ── Footer ────────────────────────────────────────────────────────────
		pdf.SetDashPattern([]float64{}, 0)
		pdf.SetLineWidth(0.3)
		pdf.SetFont("Helvetica", "", 7)
		pdf.Text(marginMM, pageH-marginMM+4,
			fmt.Sprintf("World origin for tile: (%.2f, %.2f) %s",
				tile.WorldX0, tile.WorldY0, cfg.dxfUnits))
	}

	return pdf.OutputFileAndClose(cfg.outputPDF)
}
