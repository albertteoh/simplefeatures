package geom

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"io"
	"sort"
	"unsafe"
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

// NewMultiPolygon creates a MultiPolygon from its constituent Polygons. It
// gives an error if any of the MultiPolygon assertions are not maintained.
func NewMultiPolygon(polys []Polygon, opts ...ConstructorOption) (MultiPolygon, error) {
	if !doExpensiveValidations(opts) {
		return MultiPolygon{polys}, nil
	}

	type interval struct {
		minX, maxX float64
	}
	intervals := make([]interval, len(polys))
	for i := range intervals {
		env, ok := polys[i].Envelope()
		if !ok {
			return MultiPolygon{}, errors.New("polygon in multiploygon not allowed to be empty")
		}
		intervals[i].minX = env.Min().X
		intervals[i].maxX = env.Max().X
	}
	indexes := seq(len(polys))
	sort.Slice(indexes, func(i, j int) bool {
		xi := intervals[indexes[i]].minX
		xj := intervals[indexes[j]].minX
		return xi < xj
	})

	active := intHeap{less: func(i, j int) bool {
		xi := intervals[i].maxX
		xj := intervals[j].maxX
		return xi < xj
	}}

	for _, i := range indexes {
		currentX := intervals[i].minX
		for len(active.data) > 0 && intervals[active.data[0]].maxX < currentX {
			active.pop()
		}
		for _, j := range active.data {
			bound1 := polys[i].Boundary()
			bound2 := polys[j].Boundary()
			inter := mustIntersection(bound1, bound2)
			if inter.Dimension() > 0 {
				return MultiPolygon{}, errors.New("the boundaries of the polygon elements of multipolygons must only intersect at points")
			}
			if polyInteriorsIntersect(polys[i], polys[j]) {
				return MultiPolygon{}, errors.New("polygon interiors must not intersect")
			}
		}
		active.push(i)
	}

	return MultiPolygon{polys}, nil
}

// NewMultiPolygonC creates a new MultiPolygon from its constituent Coordinate values.
func NewMultiPolygonC(coords [][][]Coordinates, opts ...ConstructorOption) (MultiPolygon, error) {
	var polys []Polygon
	for _, c := range coords {
		if len(c) == 0 {
			continue
		}
		poly, err := NewPolygonC(c, opts...)
		if err != nil {
			return MultiPolygon{}, err
		}
		polys = append(polys, poly)
	}
	return NewMultiPolygon(polys, opts...)
}

// NewMultiPolygonXY creates a new MultiPolygon from its constituent XY values.
func NewMultiPolygonXY(pts [][][]XY, opts ...ConstructorOption) (MultiPolygon, error) {
	return NewMultiPolygonC(threeDimXYToCoords(pts), opts...)
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
		var p2rings []LineString
		allPts := make(map[XY]struct{})
		for _, r1 := range p1.rings() {
			for ln1 := 0; ln1 < r1.NumLines(); ln1++ {
				line1 := r1.LineN(ln1)
				// Collect boundary control points and intersection points.
				linePts := make(map[XY]struct{})
				linePts[line1.a.XY] = struct{}{}
				linePts[line1.b.XY] = struct{}{}
				p2rings = appendRings(p2rings[:0], p2)
				for _, r2 := range p2rings {
					for ln2 := 0; ln2 < r2.NumLines(); ln2++ {
						line2 := r2.LineN(ln2)
						inter := intersectLineWithLineNoAlloc(line1, line2)
						if inter.empty {
							continue
						}
						if inter.ptA != inter.ptB {
							continue
						}
						if inter.ptA != line1.StartPoint().XY() && inter.ptA != line1.EndPoint().XY() {
							linePts[inter.ptA] = struct{}{}
						}
					}
				}
				// Collect midpoints.
				if len(linePts) <= 2 {
					for pt := range linePts {
						allPts[pt] = struct{}{}
					}
				} else {
					linePtsSlice := make([]XY, 0, len(linePts))
					for pt := range linePts {
						linePtsSlice = append(linePtsSlice, pt)
					}
					sort.Slice(linePtsSlice, func(i, j int) bool {
						ptI := linePtsSlice[i]
						ptJ := linePtsSlice[j]
						if ptI.X != ptJ.X {
							return ptI.X < ptJ.X
						}
						return ptI.Y < ptJ.Y
					})
					allPts[linePtsSlice[0]] = struct{}{}
					for i := 0; i+1 < len(linePtsSlice); i++ {
						midpoint := linePtsSlice[i].Midpoint(linePtsSlice[i+1])
						allPts[midpoint] = struct{}{}
						allPts[linePtsSlice[i+1]] = struct{}{}
					}
				}
			}
		}

		// Check to see if any of the points from the boundary from the first
		// polygon are inside the second polygon.
		for pt := range allPts {
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

// AsGeometry converts this MultiPolygon into a Geometry.
func (m MultiPolygon) AsGeometry() Geometry {
	return Geometry{multiPolygonTag, unsafe.Pointer(&m)}
}

// NumPolygons gives the number of Polygon elements in the MultiPolygon.
func (m MultiPolygon) NumPolygons() int {
	return len(m.polys)
}

// PolygonN gives the nth (zero based) Polygon element.
func (m MultiPolygon) PolygonN(n int) Polygon {
	return m.polys[n]
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

func (m MultiPolygon) Intersects(g Geometry) bool {
	return hasIntersection(m.AsGeometry(), g)
}

func (m MultiPolygon) Intersection(g Geometry) (Geometry, error) {
	return intersection(m.AsGeometry(), g)
}

func (m MultiPolygon) IsEmpty() bool {
	return len(m.polys) == 0
}

func (m MultiPolygon) Equals(other Geometry) (bool, error) {
	return equals(m.AsGeometry(), other)
}

func (m MultiPolygon) Envelope() (Envelope, bool) {
	if len(m.polys) == 0 {
		return Envelope{}, false
	}
	env := mustEnv(m.polys[0].Envelope())
	for _, poly := range m.polys[1:] {
		env = env.ExpandToIncludeEnvelope(mustEnv(poly.Envelope()))
	}
	return env, true
}

func (m MultiPolygon) Boundary() Geometry {
	if m.IsEmpty() {
		return m.AsGeometry()
	}
	var bounds []LineString
	for _, poly := range m.polys {
		bounds = append(bounds, poly.outer)
		for _, inner := range poly.holes {
			bounds = append(bounds, inner)
		}
	}
	return NewMultiLineString(bounds).AsGeometry()
}

func (m MultiPolygon) Value() (driver.Value, error) {
	var buf bytes.Buffer
	err := m.AsBinary(&buf)
	return buf.Bytes(), err
}

func (m MultiPolygon) AsBinary(w io.Writer) error {
	marsh := newWKBMarshaller(w)
	marsh.writeByteOrder()
	marsh.writeGeomType(wkbGeomTypeMultiPolygon)
	n := m.NumPolygons()
	marsh.writeCount(n)
	for i := 0; i < n; i++ {
		poly := m.PolygonN(i)
		marsh.setErr(poly.AsBinary(w))
	}
	return marsh.err
}

func (m MultiPolygon) ConvexHull() Geometry {
	return convexHull(m.AsGeometry())
}

func (m MultiPolygon) MarshalJSON() ([]byte, error) {
	return marshalGeoJSON("MultiPolygon", m.Coordinates())
}

// Coordinates returns the coordinates of each constituent Polygon of the
// MultiPolygon.
func (m MultiPolygon) Coordinates() [][][]Coordinates {
	numPolys := m.NumPolygons()
	coords := make([][][]Coordinates, numPolys)
	for i := 0; i < numPolys; i++ {
		rings := m.PolygonN(i).rings()
		coords[i] = make([][]Coordinates, len(rings))
		for j, r := range rings {
			n := r.NumPoints()
			coords[i][j] = make([]Coordinates, n)
			for k := 0; k < n; k++ {
				coords[i][j][k] = r.PointN(k).Coordinates()
			}
		}
	}
	return coords
}

// TransformXY transforms this MultiPolygon into another MultiPolygon according to fn.
func (m MultiPolygon) TransformXY(fn func(XY) XY, opts ...ConstructorOption) (Geometry, error) {
	coords := m.Coordinates()
	transform3dCoords(coords, fn)
	mp, err := NewMultiPolygonC(coords, opts...)
	return mp.AsGeometry(), err
}

// EqualsExact checks if this MultiPolygon is exactly equal to another MultiPolygon.
func (m MultiPolygon) EqualsExact(other Geometry, opts ...EqualsExactOption) bool {
	return other.IsMultiPolygon() &&
		multiPolygonExactEqual(m, other.AsMultiPolygon(), opts)
}

// IsValid checks if this MultiPolygon is valid
func (m MultiPolygon) IsValid() bool {
	_, err := NewMultiPolygonC(m.Coordinates())
	return err == nil
}

// Area gives the area of the multi polygon.
func (m MultiPolygon) Area() float64 {
	var area float64
	n := m.NumPolygons()
	for i := 0; i < n; i++ {
		area += m.PolygonN(i).Area()
	}
	return area
}

// Centroid returns the multi polygon's centroid point. It returns false if the
// multi polygon is empty (in which case, there is no sensible definition for a
// centroid).
func (m MultiPolygon) Centroid() (Point, bool) {
	if m.IsEmpty() {
		return Point{}, false
	}

	n := m.NumPolygons()
	centroids := make([]XY, n)
	areas := make([]float64, n)
	var totalArea float64
	for i := 0; i < n; i++ {
		centroids[i], areas[i] = centroidAndAreaOfPolygon(m.PolygonN(i))
		totalArea += areas[i]
	}
	var avg XY
	for i := range centroids {
		avg = avg.Add(centroids[i].Scale(areas[i]))
	}
	avg = avg.Scale(1.0 / totalArea)
	return NewPointXY(avg), true
}
