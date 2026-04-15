package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// ── Data types ────────────────────────────────────────────────────────────────

// DXFPoint is a 2-D point in DXF world coordinates.
type DXFPoint struct{ X, Y float64 }

// Linetype holds a parsed DXF linetype definition.
// Pattern elements follow group 49: positive = dash length, negative = gap length.
type Linetype struct {
	Name    string
	Pattern []float64
}

// IsContinuous reports whether this linetype renders as a solid line.
func (lt Linetype) IsContinuous() bool {
	n := strings.ToUpper(strings.TrimSpace(lt.Name))
	return n == "" || n == "CONTINUOUS" || n == "CONT"
}

// Layer holds a parsed DXF layer definition.
type Layer struct {
	Name     string
	Linetype string // linetype name as stored in the DXF
}

// EntityKind discriminates Entity variants.
type EntityKind int

const (
	KindLine     EntityKind = iota // LINE
	KindPolyline                   // LWPOLYLINE, POLYLINE, flattened SPLINE/ELLIPSE
	KindCircle                     // CIRCLE
	KindArc                        // ARC
	KindPoint                      // POINT
)

// Entity is a flattened representation of a DXF drawing entity.
type Entity struct {
	Kind     EntityKind
	Layer    string
	Linetype string // explicit linetype name, or "" which means BYLAYER

	// KindLine
	X1, Y1, X2, Y2 float64

	// KindPolyline
	Points []DXFPoint
	Closed bool

	// KindCircle / KindArc
	CX, CY, Radius     float64
	StartAngle, EndAngle float64 // degrees, ARC only

	// KindPoint
	PX, PY float64
}

// Drawing is the parsed result of a DXF file.
type Drawing struct {
	Linetypes map[string]Linetype // uppercase name → linetype
	Layers    map[string]Layer    // name → layer
	Entities  []Entity
	InsUnits  int // $INSUNITS value (1=inch, 4=mm, etc.)
}

// ── Low-level reader ──────────────────────────────────────────────────────────

type dxfPair struct {
	code int
	val  string
}

func readGroups(filename string) ([]dxfPair, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var pairs []dxfPair
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		codeLine := strings.TrimSpace(sc.Text())
		if !sc.Scan() {
			break
		}
		valLine := strings.TrimSpace(sc.Text())
		code, err := strconv.Atoi(codeLine)
		if err != nil {
			continue // skip malformed lines
		}
		pairs = append(pairs, dxfPair{code, valLine})
	}
	return pairs, sc.Err()
}

// ── Top-level parser ──────────────────────────────────────────────────────────

// ParseDXF reads a DXF file and returns the parsed drawing.
func ParseDXF(filename string) (*Drawing, error) {
	pairs, err := readGroups(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}

	d := &Drawing{
		Linetypes: make(map[string]Linetype),
		Layers:    make(map[string]Layer),
	}

	fval := func(i int) float64 {
		v, _ := strconv.ParseFloat(pairs[i].val, 64)
		return v
	}
	ival := func(i int) int {
		v, _ := strconv.Atoi(pairs[i].val)
		return v
	}

	var curSection string
	i, n := 0, len(pairs)
	for i < n {
		p := pairs[i]
		if p.code == 0 {
			switch p.val {
			case "SECTION":
				i++
				if i < n && pairs[i].code == 2 {
					curSection = pairs[i].val
				}
				i++
				continue
			case "ENDSEC":
				curSection = ""
				i++
				continue
			case "EOF":
				return d, nil
			}
		}

		switch curSection {
		case "HEADER":
			i = parseHeaderPair(pairs, i, n, d, ival)
		case "TABLES":
			i = parseTablesPair(pairs, i, n, d, fval, ival)
		case "ENTITIES", "BLOCKS":
			i = parseEntityPair(pairs, i, n, d, fval, ival)
		default:
			i++
		}
	}
	return d, nil
}

// ── HEADER section ────────────────────────────────────────────────────────────

func parseHeaderPair(pairs []dxfPair, i, n int, d *Drawing, ival func(int) int) int {
	p := pairs[i]
	if p.code == 9 && p.val == "$INSUNITS" {
		i++
		for i < n && pairs[i].code != 9 && pairs[i].code != 0 {
			if pairs[i].code == 70 {
				d.InsUnits = ival(i)
			}
			i++
		}
		return i
	}
	return i + 1
}

// ── TABLES section ────────────────────────────────────────────────────────────

func parseTablesPair(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64, ival func(int) int) int {
	p := pairs[i]
	if p.code == 0 {
		switch p.val {
		case "LTYPE":
			return parseLtype(pairs, i, n, d, fval)
		case "LAYER":
			return parseLayer(pairs, i, n, d)
		}
	}
	return i + 1
}

func parseLtype(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	lt := Linetype{}
	i++ // consume "0 / LTYPE"
	for i < n {
		p := pairs[i]
		if p.code == 0 {
			break
		}
		if p.code != 100 { // skip subclass markers
			switch p.code {
			case 2:
				lt.Name = p.val
			case 49:
				lt.Pattern = append(lt.Pattern, fval(i))
			}
		}
		i++
	}
	if lt.Name != "" {
		d.Linetypes[strings.ToUpper(lt.Name)] = lt
	}
	return i
}

func parseLayer(pairs []dxfPair, i, n int, d *Drawing) int {
	layer := Layer{}
	i++ // consume "0 / LAYER"
	for i < n {
		p := pairs[i]
		if p.code == 0 {
			break
		}
		if p.code != 100 {
			switch p.code {
			case 2:
				layer.Name = p.val
			case 6:
				layer.Linetype = p.val
			}
		}
		i++
	}
	if layer.Name != "" {
		d.Layers[layer.Name] = layer
	}
	return i
}

// ── ENTITIES section ──────────────────────────────────────────────────────────

func parseEntityPair(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64, ival func(int) int) int {
	p := pairs[i]
	if p.code != 0 {
		return i + 1
	}
	switch p.val {
	case "LINE":
		return parseLine(pairs, i, n, d, fval)
	case "LWPOLYLINE":
		return parseLwPolyline(pairs, i, n, d, fval, ival)
	case "POLYLINE":
		return parsePolyline(pairs, i, n, d, fval, ival)
	case "CIRCLE":
		return parseCircle(pairs, i, n, d, fval)
	case "ARC":
		return parseArc(pairs, i, n, d, fval)
	case "POINT":
		return parsePoint(pairs, i, n, d, fval)
	case "SPLINE":
		return parseSpline(pairs, i, n, d, fval, ival)
	case "ELLIPSE":
		return parseEllipse(pairs, i, n, d, fval)
	default:
		return i + 1
	}
}

// entityCommon reads shared fields (layer, linetype) until next group-0.
// Returns an entity pre-populated with those fields and the new index.
func entityCommon(pairs []dxfPair, i, n int, fval func(int) float64, ival func(int) int,
	perGroup func(p dxfPair, i int),
) int {
	i++ // consume the "0 / TYPE" pair
	for i < n {
		p := pairs[i]
		if p.code == 0 {
			break
		}
		if p.code != 100 { // skip subclass markers
			perGroup(p, i)
		}
		i++
	}
	return i
}

func parseLine(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	e := Entity{Kind: KindLine}
	i = entityCommon(pairs, i, n, fval, nil, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 10:
			e.X1 = fval(idx)
		case 20:
			e.Y1 = fval(idx)
		case 11:
			e.X2 = fval(idx)
		case 21:
			e.Y2 = fval(idx)
		}
	})
	d.Entities = append(d.Entities, e)
	return i
}

func parseLwPolyline(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64, ival func(int) int) int {
	e := Entity{Kind: KindPolyline}
	var cur DXFPoint
	hasX := false
	i = entityCommon(pairs, i, n, fval, ival, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 70:
			e.Closed = ival(idx)&1 != 0
		case 10:
			if hasX {
				e.Points = append(e.Points, cur)
			}
			cur = DXFPoint{X: fval(idx)}
			hasX = true
		case 20:
			cur.Y = fval(idx)
		}
	})
	if hasX {
		e.Points = append(e.Points, cur)
	}
	d.Entities = append(d.Entities, e)
	return i
}

func parsePolyline(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64, ival func(int) int) int {
	e := Entity{Kind: KindPolyline}
	i++ // consume "0 / POLYLINE"
	for i < n {
		p := pairs[i]
		if p.code == 0 {
			if p.val == "VERTEX" {
				i++
				var vx, vy float64
				for i < n && pairs[i].code != 0 {
					switch pairs[i].code {
					case 10:
						vx = fval(i)
					case 20:
						vy = fval(i)
					}
					i++
				}
				e.Points = append(e.Points, DXFPoint{vx, vy})
				continue
			}
			if p.val == "SEQEND" {
				i++
				for i < n && pairs[i].code != 0 {
					i++
				}
				break
			}
			break
		}
		if p.code != 100 {
			switch p.code {
			case 8:
				e.Layer = p.val
			case 6:
				e.Linetype = p.val
			case 70:
				e.Closed = ival(i)&1 != 0
			}
		}
		i++
	}
	d.Entities = append(d.Entities, e)
	return i
}

func parseCircle(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	e := Entity{Kind: KindCircle}
	i = entityCommon(pairs, i, n, fval, nil, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 10:
			e.CX = fval(idx)
		case 20:
			e.CY = fval(idx)
		case 40:
			e.Radius = fval(idx)
		}
	})
	d.Entities = append(d.Entities, e)
	return i
}

func parseArc(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	e := Entity{Kind: KindArc}
	i = entityCommon(pairs, i, n, fval, nil, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 10:
			e.CX = fval(idx)
		case 20:
			e.CY = fval(idx)
		case 40:
			e.Radius = fval(idx)
		case 50:
			e.StartAngle = fval(idx)
		case 51:
			e.EndAngle = fval(idx)
		}
	})
	d.Entities = append(d.Entities, e)
	return i
}

func parsePoint(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	e := Entity{Kind: KindPoint}
	i = entityCommon(pairs, i, n, fval, nil, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 10:
			e.PX = fval(idx)
		case 20:
			e.PY = fval(idx)
		}
	})
	d.Entities = append(d.Entities, e)
	return i
}

func parseSpline(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64, ival func(int) int) int {
	e := Entity{Kind: KindPolyline}
	var degree int
	var knots []float64
	var ctrlPts []DXFPoint
	var cur DXFPoint
	hasX := false
	inKnots := true // group-40 values before first group-10 are knots
	i = entityCommon(pairs, i, n, fval, ival, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 71:
			degree = ival(idx)
		case 40:
			if inKnots {
				knots = append(knots, fval(idx))
			}
		case 10:
			inKnots = false
			if hasX {
				ctrlPts = append(ctrlPts, cur)
			}
			cur = DXFPoint{X: fval(idx)}
			hasX = true
		case 20:
			cur.Y = fval(idx)
		}
	})
	if hasX {
		ctrlPts = append(ctrlPts, cur)
	}
	if degree < 1 {
		degree = 3
	}
	e.Points = flattenBSpline(ctrlPts, knots, degree, 64)
	d.Entities = append(d.Entities, e)
	return i
}

func parseEllipse(pairs []dxfPair, i, n int, d *Drawing, fval func(int) float64) int {
	e := Entity{Kind: KindPolyline, Closed: true}
	var cx, cy, mx, my, ratio float64
	startP := 0.0
	endP := 2 * math.Pi
	i = entityCommon(pairs, i, n, fval, nil, func(p dxfPair, idx int) {
		switch p.code {
		case 8:
			e.Layer = p.val
		case 6:
			e.Linetype = p.val
		case 10:
			cx = fval(idx)
		case 20:
			cy = fval(idx)
		case 11:
			mx = fval(idx)
		case 21:
			my = fval(idx)
		case 40:
			ratio = fval(idx)
		case 41:
			startP = fval(idx)
		case 42:
			endP = fval(idx)
		}
	})
	e.Points = flattenEllipse(cx, cy, mx, my, ratio, startP, endP, 128)
	d.Entities = append(d.Entities, e)
	return i
}

// ── Curve flattening ──────────────────────────────────────────────────────────

// flattenBSpline evaluates a clamped B-spline via de Boor's algorithm.
func flattenBSpline(ctrl []DXFPoint, knots []float64, degree, steps int) []DXFPoint {
	nc := len(ctrl)
	if nc == 0 {
		return nil
	}
	p := degree
	if len(knots) < nc+p+1 {
		return ctrl // knot vector too short — fall back to control polygon
	}
	tMin, tMax := knots[p], knots[nc]
	if tMax <= tMin {
		return ctrl
	}
	pts := make([]DXFPoint, 0, steps+1)
	for s := 0; s <= steps; s++ {
		t := tMin + float64(s)/float64(steps)*(tMax-tMin)
		if s == steps {
			t = tMax - 1e-10
		}
		pts = append(pts, deBoor(ctrl, knots, p, t))
	}
	return pts
}

func deBoor(ctrl []DXFPoint, knots []float64, p int, t float64) DXFPoint {
	n := len(ctrl) - 1
	k := p
	for k < n && knots[k+1] <= t {
		k++
	}
	d := make([]DXFPoint, p+1)
	for j := 0; j <= p; j++ {
		idx := clamp(j+k-p, 0, n)
		d[j] = ctrl[idx]
	}
	for r := 1; r <= p; r++ {
		for j := p; j >= r; j-- {
			ki := j + k - p
			denom := knots[ki+p-r+1] - knots[ki]
			var alpha float64
			if denom > 1e-12 {
				alpha = (t - knots[ki]) / denom
			}
			d[j].X = (1-alpha)*d[j-1].X + alpha*d[j].X
			d[j].Y = (1-alpha)*d[j-1].Y + alpha*d[j].Y
		}
	}
	return d[p]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// flattenEllipse evaluates a DXF ELLIPSE parametrically.
func flattenEllipse(cx, cy, mx, my, ratio, startP, endP float64, steps int) []DXFPoint {
	a := math.Sqrt(mx*mx + my*my)
	b := a * ratio
	rot := math.Atan2(my, mx)
	pts := make([]DXFPoint, 0, steps+1)
	for s := 0; s <= steps; s++ {
		t := startP + float64(s)/float64(steps)*(endP-startP)
		lx := a * math.Cos(t)
		ly := b * math.Sin(t)
		rx := lx*math.Cos(rot) - ly*math.Sin(rot)
		ry := lx*math.Sin(rot) + ly*math.Cos(rot)
		pts = append(pts, DXFPoint{cx + rx, cy + ry})
	}
	return pts
}

// ── Linetype resolution ───────────────────────────────────────────────────────

// EffectiveLinetypeName returns the resolved, uppercased linetype name for e.
// It follows BYLAYER → layer linetype → "CONTINUOUS".
func (d *Drawing) EffectiveLinetypeName(e Entity) string {
	name := strings.ToUpper(strings.TrimSpace(e.Linetype))
	if name == "" || name == "BYLAYER" {
		layer, ok := d.Layers[e.Layer]
		if !ok {
			return "CONTINUOUS"
		}
		name = strings.ToUpper(strings.TrimSpace(layer.Linetype))
	}
	if name == "" || name == "BYBLOCK" {
		return "CONTINUOUS"
	}
	return name
}

// IsContinuousEntity reports whether e renders as a solid line.
func (d *Drawing) IsContinuousEntity(e Entity) bool {
	n := d.EffectiveLinetypeName(e)
	return n == "" || n == "CONTINUOUS" || n == "CONT"
}

// DashPattern returns the raw group-49 pattern for the named linetype, or nil
// for continuous linetypes or unknown names.
func (d *Drawing) DashPattern(ltName string) []float64 {
	upper := strings.ToUpper(strings.TrimSpace(ltName))
	if upper == "" || upper == "CONTINUOUS" || upper == "CONT" {
		return nil
	}
	lt, ok := d.Linetypes[upper]
	if !ok {
		return nil
	}
	return lt.Pattern
}
