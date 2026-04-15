package main

import (
	"math"

	"github.com/go-pdf/fpdf"
)

const (
	// dashScale shrinks DXF linetype patterns so they look fine on paper.
	// DXF patterns in drawing units map 1:1 to large physical lengths, which
	// look chunky printed. 0.3 matches the Python version's behaviour.
	dashScale = 0.3

	// dotMM is the minimum dash/gap length used in place of a zero pattern element (dot).
	dotMM = 0.2

	// pointDotRadiusMM is the radius of a rendered POINT entity.
	pointDotRadiusMM = 0.6
)

// dashPatternMM converts DXF group-49 pattern values to a fpdf dash array (mm).
// Positive values are dashes, negative are gaps; both become positive lengths.
// Returns nil for empty patterns (caller should reset to solid).
func dashPatternMM(pattern []float64, scaleWUtoMM float64) []float64 {
	if len(pattern) == 0 {
		return nil
	}
	out := make([]float64, len(pattern))
	for i, v := range pattern {
		mm := math.Abs(v) * scaleWUtoMM * dashScale
		if mm < dotMM {
			mm = dotMM
		}
		out[i] = mm
	}
	// fpdf SetDashPattern requires an even-length array (alternating dash/gap).
	if len(out)%2 == 1 {
		out = append(out, out...)
	}
	return out
}

// drawPolylinePoints draws a series of world-space points as a stroked path.
func drawPolylinePoints(pdf *fpdf.Fpdf, pts []DXFPoint, xf WorldToPage, closed bool) {
	if len(pts) < 2 {
		return
	}
	x0, y0 := xf.W2P(pts[0].X, pts[0].Y)
	pdf.MoveTo(x0, y0)
	for _, p := range pts[1:] {
		px, py := xf.W2P(p.X, p.Y)
		pdf.LineTo(px, py)
	}
	if closed {
		pdf.ClosePath()
	}
	pdf.DrawPath("D")
}

// arcPoints returns N+1 world-space points along a DXF arc.
// DXF arcs sweep CCW from startDeg to endDeg (both in degrees).
func arcPoints(cx, cy, r, startDeg, endDeg float64, steps int) []DXFPoint {
	// Ensure CCW sweep: endDeg > startDeg.
	for endDeg < startDeg {
		endDeg += 360
	}
	pts := make([]DXFPoint, 0, steps+1)
	for i := 0; i <= steps; i++ {
		theta := (startDeg + float64(i)/float64(steps)*(endDeg-startDeg)) * math.Pi / 180
		pts = append(pts, DXFPoint{cx + r*math.Cos(theta), cy + r*math.Sin(theta)})
	}
	return pts
}

// DrawEntity renders a single DXF entity onto pdf using xf to map coordinates.
// d is used to resolve linetypes. If noDashed is true, non-continuous entities
// are silently skipped.
func DrawEntity(pdf *fpdf.Fpdf, e Entity, xf WorldToPage, d *Drawing, noDashed bool) {
	ltName := d.EffectiveLinetypeName(e)
	isContinuous := d.IsContinuousEntity(e)

	if noDashed && !isContinuous {
		return
	}

	// Apply dash pattern (or reset to solid).
	rawPattern := d.DashPattern(ltName)
	if len(rawPattern) > 0 {
		pdf.SetDashPattern(dashPatternMM(rawPattern, xf.Scale), 0)
		defer pdf.SetDashPattern([]float64{}, 0)
	}

	switch e.Kind {
	case KindLine:
		x1, y1 := xf.W2P(e.X1, e.Y1)
		x2, y2 := xf.W2P(e.X2, e.Y2)
		pdf.Line(x1, y1, x2, y2)

	case KindPolyline:
		drawPolylinePoints(pdf, e.Points, xf, e.Closed)

	case KindCircle:
		cx, cy := xf.W2P(e.CX, e.CY)
		r := e.Radius * xf.Scale
		pdf.Circle(cx, cy, r, "D")

	case KindArc:
		// Flatten to polyline so the Y-flip is handled uniformly by W2P.
		pts := arcPoints(e.CX, e.CY, e.Radius, e.StartAngle, e.EndAngle, 64)
		drawPolylinePoints(pdf, pts, xf, false)

	case KindPoint:
		px, py := xf.W2P(e.PX, e.PY)
		// Reset to solid for the dot (dash makes no sense for a point).
		pdf.SetDashPattern([]float64{}, 0)
		pdf.Circle(px, py, pointDotRadiusMM, "F")
	}
}

// DrawEntities renders all entities in the slice onto the current fpdf page.
func DrawEntities(pdf *fpdf.Fpdf, entities []Entity, xf WorldToPage, d *Drawing, noDashed bool) {
	for _, e := range entities {
		DrawEntity(pdf, e, xf, d, noDashed)
	}
}
