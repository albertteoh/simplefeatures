package geom

import (
	"database/sql/driver"
	"fmt"
	"math"

	"github.com/peterstace/simplefeatures/rtree"
)

// LineString is a linear geometry defined by linear interpolation between a
// finite set of points. Its zero value is the empty line string. It is
// immutable after creation.
//
// A LineString must consist of either zero points (i.e. it is the empty line
// string), or it must have at least 2 points with distinct XY values.
type LineString interface {
	Geometryer

	Coordinates() Sequence
	IsClosed() bool
	ForceCoordinatesType(newCType CoordinatesType) LineString
	StartPoint() Point
	EndPoint() Point
	Force2D() LineString
	Boundary() MultiPoint
	Reverse() LineString
	IsRing() bool
	AsMultiLineString() MultiLineString

	appendWKTBody(dst []byte) []byte
	getSeq() Sequence
}

type lineString struct {
	seq Sequence
}

// NewLineString creates a new LineString from a Sequence of points. The
// sequence must contain exactly 0 points, or at least 2 points with distinct
// XY values (otherwise an error is returned).
func NewLineString(seq Sequence, opts ...ConstructorOption) (LineString, error) {
	n := seq.Length()
	ctorOpts := newOptionSet(opts)
	if ctorOpts.skipValidations || n == 0 {
		return lineString{seq}, nil
	}

	// Valid non-empty LineStrings must have at least 2 *distinct* points.
	if !hasAtLeast2DistinctPointsInSeq(seq) {
		if ctorOpts.omitInvalid {
			return lineString{}, nil
		}
		return lineString{}, validationError{
			"non-empty linestring contains only one distinct XY value"}
	}

	// All XY values must be valid.
	if err := seq.validate(); err != nil {
		if ctorOpts.omitInvalid {
			return lineString{}, nil
		}
		return lineString{}, validationError{err.Error()}
	}

	return lineString{seq}, nil
}

func hasAtLeast2DistinctPointsInSeq(seq Sequence) bool {
	n := seq.Length()
	if n == 0 {
		return false
	}
	first := seq.GetXY(0)
	for i := 1; i < n; i++ {
		if seq.GetXY(i) != first {
			return true
		}
	}
	return false
}

func (s lineString) reverse() Geometryer {
	return s.Reverse()
}

// Type returns the GeometryType for a LineString
func (s lineString) Type() GeometryType {
	return TypeLineString
}

// AsGeometry converts this LineString into a Geometry.
func (s lineString) AsGeometry() Geometry {
	return Geometry{s}
}

// StartPoint gives the first point of the LineString. If the LineString is
// empty then it returns the empty Point.
func (s lineString) StartPoint() Point {
	if s.IsEmpty() {
		return NewEmptyPoint(s.CoordinatesType())
	}
	c := s.seq.Get(0)
	return newUncheckedPoint(c)
}

// EndPoint gives the last point of the LineString. If the LineString is empty
// then it returns the empty Point.
func (s lineString) EndPoint() Point {
	if s.IsEmpty() {
		return NewEmptyPoint(s.CoordinatesType())
	}
	end := s.seq.Length() - 1
	c := s.seq.Get(end)
	return newUncheckedPoint(c)
}

// AsText returns the WKT (Well Known Text) representation of this geometry.
func (s lineString) AsText() string {
	return string(s.AppendWKT(nil))
}

// AppendWKT appends the WKT (Well Known Text) representation of this geometry
// to the input byte slice.
func (s lineString) AppendWKT(dst []byte) []byte {
	dst = appendWKTHeader(dst, "LINESTRING", s.CoordinatesType())
	return s.appendWKTBody(dst)
}

func (s lineString) appendWKTBody(dst []byte) []byte {
	if s.IsEmpty() {
		return appendWKTEmpty(dst)
	}
	return appendWKTSequence(dst, s.seq, false)
}

// IsSimple returns true if this geometry contains no anomalous geometry
// points, such as self intersection or self tangency. LineStrings are simple
// if and only if the curve defined by the LineString doesn't pass through the
// same point twice (with the except of the two endpoints being coincident).
func (s lineString) IsSimple() bool {
	first, last, ok := firstAndLastLines(s.seq)
	if !ok {
		return true
	}

	n := s.seq.Length()
	items := make([]rtree.BulkItem, 0, n-1)
	for i := 0; i < n; i++ {
		ln, ok := getLine(s.seq, i)
		if !ok {
			continue
		}
		items = append(items, rtree.BulkItem{Box: ln.box(), RecordID: i})
	}
	tree := rtree.BulkLoad(items)

	for i := 0; i < n; i++ {
		ln, ok := getLine(s.seq, i)
		if !ok {
			continue
		}

		prev, ok := previousLine(s.seq, i)
		if !ok {
			prev = -1
		}
		next, ok := nextLine(s.seq, i)
		if !ok {
			next = -1
		}

		simple := true // assume simple until proven otherwise
		tree.RangeSearch(ln.box(), func(j int) error {
			// Skip finding the original line (i == j) or cases where we have
			// already checked that pair (i > j).
			if i >= j {
				return nil
			}

			// Extract the other line from the sequence. We previously were
			// able to access line j (otherwise we wouldn't have been able to
			// put it into the tree). So if we can't access it now, something
			// has gone horribly wrong.
			other, ok := getLine(s.seq, j)
			if !ok {
				panic("couldn't get line")
			}

			inter := ln.intersectLine(other)
			if inter.empty {
				return nil
			}
			if inter.ptA != inter.ptB {
				// Two overlapping line segments.
				simple = false
				return rtree.Stop
			}

			// The dimension must be 1. Since the intersection is between two
			// lines, the intersection must be a *single* point in the cases
			// from this point onwards.

			// Adjacent lines will intersect at a point due to construction, so
			// these cases are okay.
			if j == prev || j == next {
				return nil
			}

			// The first and last segment are allowed to intersect at a point,
			// so long as that point is the start of the first segment and the
			// end of the last segment (i.e. the line string is closed).
			if s.IsClosed() && i == first && j == last {
				return nil
			}

			// Any other point intersection (e.g. looping back on
			// itself at a point) is disallowed for simple linestrings.
			simple = false
			return rtree.Stop
		})

		if !simple {
			return false
		}
	}
	return true
}

// IsClosed returns true if and only if this LineString is not empty and its
// start and end points are coincident.
func (s lineString) IsClosed() bool {
	return !s.IsEmpty() && s.seq.GetXY(0) == s.seq.GetXY(s.seq.Length()-1)
}

// IsEmpty returns true if and only if this LineString is the empty LineString.
// The empty LineString is defined by a zero length coordinates sequence.
func (s lineString) IsEmpty() bool {
	return s.seq.Length() == 0
}

// Envelope returns the Envelope that most tightly surrounds the geometry.
func (s lineString) Envelope() Envelope {
	var env Envelope
	n := s.seq.Length()
	for i := 0; i < n; i++ {
		env = env.uncheckedExtend(s.seq.GetXY(i))
	}
	return env
}

// Boundary returns the spatial boundary of this LineString. For closed
// LineStrings (i.e. LineStrings where the start and end points have the same
// XY value), this is the empty MultiPoint. For non-closed LineStrings, this is
// the MultiPoint containing the two endpoints of the LineString.
func (s lineString) Boundary() MultiPoint {
	if s.IsEmpty() || s.IsClosed() {
		return multiPoint{}
	}
	return NewMultiPoint([]Point{
		s.StartPoint(),
		s.EndPoint(),
	})
}

// Value implements the database/sql/driver.Valuer interface by returning the
// WKB (Well Known Binary) representation of this Geometry.
func (s lineString) Value() (driver.Value, error) {
	return s.AsBinary(), nil
}

// Scan implements the database/sql.Scanner interface by parsing the src value
// as WKB (Well Known Binary).
//
// If the WKB doesn't represent a LineString geometry, then an error is returned.
//
// It constructs the resultant geometry with no ConstructionOptions. If
// ConstructionOptions are needed, then the value should be scanned into a byte
// slice and then UnmarshalWKB called manually (passing in the
// ConstructionOptions as desired).
func (s *lineString) Scan(src interface{}) error {
	return scanAsType(src, s, TypeLineString)
}

// AsBinary returns the WKB (Well Known Text) representation of the geometry.
func (s lineString) AsBinary() []byte {
	return s.AppendWKB(nil)
}

// AppendWKB appends the WKB (Well Known Text) representation of the geometry
// to the input slice.
func (s lineString) AppendWKB(dst []byte) []byte {
	marsh := newWKBMarshaller(dst)
	marsh.writeByteOrder()
	marsh.writeGeomType(TypeLineString, s.CoordinatesType())
	marsh.writeSequence(s.seq)
	return marsh.buf
}

// ConvexHull returns the geometry representing the smallest convex geometry
// that contains this geometry.
func (s lineString) ConvexHull() Geometry {
	return convexHull(s.AsGeometry())
}

// MarshalJSON implements the encoding/json.Marshaller interface by encoding
// this geometry as a GeoJSON geometry object.
func (s lineString) MarshalJSON() ([]byte, error) {
	var dst []byte
	dst = append(dst, `{"type":"LineString","coordinates":`...)
	dst = appendGeoJSONSequence(dst, s.seq)
	dst = append(dst, '}')
	return dst, nil
}

// Coordinates returns the coordinates of each point along the LineString.
func (s lineString) Coordinates() Sequence {
	return s.seq
}

// TransformXY transforms this LineString into another LineString according to fn.
func (s lineString) TransformXY(fn func(XY) XY, opts ...ConstructorOption) (LineString, error) {
	transformed := transformSequence(s.seq, fn)
	ls, err := NewLineString(transformed, opts...)
	return ls, wrapTransformed(err)
}

// IsRing returns true iff this LineString is both simple and closed (i.e. is a
// linear ring).
func (s lineString) IsRing() bool {
	return s.IsClosed() && s.IsSimple()
}

// Length gives the length of the line string.
func (s lineString) Length() float64 {
	var sum float64
	n := s.seq.Length()
	for i := 0; i+1 < n; i++ {
		xyA := s.seq.GetXY(i)
		xyB := s.seq.GetXY(i + 1)
		delta := xyA.Sub(xyB)
		sum += math.Sqrt(delta.Dot(delta))
	}
	return sum
}
func (s lineString) getSeq() Sequence {
	return s.seq
}

// Centroid gives the centroid of the coordinates of the line string.
func (s lineString) Centroid() Point {
	sumXY, sumLength := sumCentroidAndLengthOfLineString(s)
	if sumLength == 0 {
		return NewEmptyPoint(DimXY)
	}
	return sumXY.Scale(1.0 / sumLength).asUncheckedPoint()
}

func sumCentroidAndLengthOfLineString(s LineString) (sumXY XY, sumLength float64) {
	seq := s.Coordinates()
	for i := 0; i < seq.Length(); i++ {
		ln, ok := getLine(seq, i)
		if !ok {
			continue
		}
		length := ln.length()
		cent := ln.centroid()
		sumXY = sumXY.Add(cent.Scale(length))
		sumLength += length
	}
	return sumXY, sumLength
}

// AsMultiLineString is a convenience function that converts this LineString
// into a MultiLineString.
func (s lineString) AsMultiLineString() MultiLineString {
	return NewMultiLineString([]LineString{s})
}

// Reverse in the case of LineString outputs the coordinates in reverse order.
func (s lineString) Reverse() LineString {
	return lineString{s.seq.Reverse()}
}

// CoordinatesType returns the CoordinatesType used to represent points making
// up the geometry.
func (s lineString) CoordinatesType() CoordinatesType {
	return s.seq.CoordinatesType()
}

// ForceCoordinatesType returns a new LineString with a different CoordinatesType. If a
// dimension is added, then new values are populated with 0.
func (s lineString) ForceCoordinatesType(newCType CoordinatesType) LineString {
	return lineString{s.seq.ForceCoordinatesType(newCType)}
}

// Force2D returns a copy of the LineString with Z and M values removed.
func (s lineString) Force2D() LineString {
	return s.ForceCoordinatesType(DimXY)
}

func (s lineString) asLines() []line {
	n := s.seq.Length()
	lines := make([]line, 0, max(0, n-1))
	for i := 0; i < n; i++ {
		ln, ok := getLine(s.seq, i)
		if ok {
			lines = append(lines, ln)
		}
	}
	return lines
}

// PointOnSurface returns a Point on the LineString.
func (s lineString) PointOnSurface() Point {
	// Look for the control point on the LineString (other than the first and
	// last point) that is closest to the centroid.
	n := s.seq.Length()
	nearest := newNearestPointAccumulator(s.Centroid())
	for i := 1; i < n-1; i++ {
		candidate := s.seq.GetXY(i).asUncheckedPoint()
		nearest.consider(candidate)
	}
	if !nearest.point.IsEmpty() {
		return nearest.point
	}

	// Consider the star/end points if we don't have anything yet.
	nearest.consider(s.StartPoint().Force2D())
	nearest.consider(s.EndPoint().Force2D())
	return nearest.point
}

// Summary returns a text summary of the LineString following a similar format to https://postgis.net/docs/ST_Summary.html.
func (s lineString) Summary() string {
	return fmt.Sprintf("%s[%s] with %d points", s.Type(), s.CoordinatesType(), s.Coordinates().Length())
}

// String returns the string representation of the LineString.
func (s lineString) String() string {
	return s.Summary()
}
