package simplefeatures

import (
	"errors"
	"sort"
)

// MultiPolygon is a multi surface whose elements are polygons.
//
// Its assertions are:
//
// 1. It must be made up of zero or more valid Polygons.
//
// 2. The interiors of any two polygons must not intersect.
//
// 3. The boundaries of any two polygons may touch only at a finite number of points.
type MultiPolygon struct {
	polys []Polygon
}

func NewMultiPolygon(polys []Polygon) (MultiPolygon, error) {
	for i := 0; i < len(polys); i++ {
		for j := i + 1; j < len(polys); j++ {
			bound1 := polys[i].Boundary()
			bound2 := polys[j].Boundary()
			inter := bound1.Intersection(bound2)
			if inter.Dimension() > 0 {
				return MultiPolygon{}, errors.New("the boundaries of the polygon elements of multipolygons must only intersect at points")
			}
			if polyInteriorsIntersect(polys[i], polys[j]) {
				return MultiPolygon{}, errors.New("polygon interiors must not intersect")
			}
		}
	}
	return MultiPolygon{polys}, nil
}

func polyInteriorsIntersect(p1, p2 Polygon) bool {
	// Run twice, swapping the order of the polygons each time.
	for order := 0; order < 2; order++ {
		p1, p2 = p2, p1

		// Collect points along the boundary of the first polygon. Do this by
		// first breaking the lines in the ring into multiple segments where
		// they are intersected by the rings from the other polygon. Collect
		// the original points in the boundary, plus the intersection points,
		// then each midpoint between those points. These are enough points
		// that one of the points will be inside the other polygon iff the
		// interior of the polygons intersect.
		allPts := newXYSet()
		for _, r1 := range p1.rings() {
			for _, line1 := range r1.ls.lines {
				// Collect boundary control points and intersection points.
				linePts := newXYSet()
				linePts.add(line1.a.XY)
				linePts.add(line1.b.XY)
				for _, r2 := range p2.rings() {
					for _, line2 := range r2.ls.lines {
						env, ok := line1.Intersection(line2).Envelope()
						if !ok {
							continue
						}
						if !env.Min().Equals(env.Max()) {
							continue
						}
						inter := env.Min()
						if !inter.Equals(line1.a.XY) && !inter.Equals(line1.b.XY) {
							linePts.add(inter)
						}
					}
				}
				// Collect midpoints.
				if len(linePts) <= 2 {
					for _, pt := range linePts {
						allPts.add(pt)
					}
				} else {
					linePtsSlice := make([]XY, 0, len(linePts))
					for _, pt := range linePts {
						linePtsSlice = append(linePtsSlice, pt)
					}
					sort.Slice(linePtsSlice, func(i, j int) bool {
						ptI := linePtsSlice[i]
						ptJ := linePtsSlice[j]
						if !ptI.X.Equals(ptJ.X) {
							return ptI.X.LT(ptJ.X)
						}
						return ptI.Y.LT(ptJ.Y)
					})
					allPts.add(linePtsSlice[0])
					for i := 0; i+1 < len(linePtsSlice); i++ {
						midpoint := linePtsSlice[i].Midpoint(linePtsSlice[i+1])
						allPts.add(midpoint)
						allPts.add(linePtsSlice[i+1])
					}
				}
			}
		}

		// Check to see if any of the points from the boundary from the first
		// polygon are inside the second polygon.
		for _, pt := range allPts {
			if isPointInteriorToPolygon(pt, p2) {
				return true
			}
		}
	}
	return false
}

func isPointInteriorToPolygon(pt XY, poly Polygon) bool {
	if pointRingSide(pt, poly.outer) != interior {
		return false
	}
	for _, hole := range poly.holes {
		if pointRingSide(pt, hole) != exterior {
			return false
		}
	}
	return true
}

func NewMultiPolygonFromCoords(coords [][][]Coordinates) (MultiPolygon, error) {
	var polys []Polygon
	for _, c := range coords {
		if len(c) == 0 {
			continue
		}
		poly, err := NewPolygonFromCoords(c)
		if err != nil {
			return MultiPolygon{}, err
		}
		polys = append(polys, poly)
	}
	return NewMultiPolygon(polys)
}

func (m MultiPolygon) AsText() string {
	return string(m.AppendWKT(nil))
}

func (m MultiPolygon) AppendWKT(dst []byte) []byte {
	dst = append(dst, []byte("MULTIPOLYGON")...)
	if len(m.polys) == 0 {
		return append(dst, []byte(" EMPTY")...)
	}
	dst = append(dst, '(')
	for i, poly := range m.polys {
		dst = poly.appendWKTBody(dst)
		if i != len(m.polys)-1 {
			dst = append(dst, ',')
		}
	}
	return append(dst, ')')
}

// IsSimple returns true. All MultiPolygons are simple by definition.
func (m MultiPolygon) IsSimple() bool {
	return true
}

func (m MultiPolygon) Intersection(g Geometry) Geometry {
	return intersection(m, g)
}

func (m MultiPolygon) IsEmpty() bool {
	return len(m.polys) == 0
}

func (m MultiPolygon) Dimension() int {
	if m.IsEmpty() {
		return 0
	}
	return 2
}

func (m MultiPolygon) Equals(other Geometry) bool {
	return equals(m, other)
}

func (m MultiPolygon) Envelope() (Envelope, bool) {
	if len(m.polys) == 0 {
		return Envelope{}, false
	}
	env := mustEnvelope(m.polys[0])
	for _, poly := range m.polys[1:] {
		env = env.Union(mustEnvelope(poly))
	}
	return env, true
}

func (m MultiPolygon) Boundary() Geometry {
	if m.IsEmpty() {
		return m
	}
	var bounds []LineString
	for _, poly := range m.polys {
		bounds = append(bounds, poly.outer.ls)
		for _, inner := range poly.holes {
			bounds = append(bounds, inner.ls)
		}
	}
	return NewMultiLineString(bounds)
}
