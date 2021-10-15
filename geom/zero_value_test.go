package geom_test

import (
	"testing"

	"github.com/peterstace/simplefeatures/internal/newempty"

	. "github.com/peterstace/simplefeatures/geom"
)

func TestZeroValueGeometries(t *testing.T) {
	t.Run("Point", func(t *testing.T) {
		pt := newempty.Point(t)
		expectBoolEq(t, pt.IsEmpty(), true)
		expectCoordinatesTypeEq(t, pt.CoordinatesType(), DimXY)
	})
	t.Run("LineString", func(t *testing.T) {
		ls := newempty.LineString(t)
		expectIntEq(t, ls.Coordinates().Length(), 0)
		expectCoordinatesTypeEq(t, ls.CoordinatesType(), DimXY)
	})
	t.Run("Polygon", func(t *testing.T) {
		p := newempty.Polygon(t)
		expectBoolEq(t, p.IsEmpty(), true)
		expectCoordinatesTypeEq(t, p.CoordinatesType(), DimXY)
	})
	t.Run("MultiPoint", func(t *testing.T) {
		mp := newempty.MultiPoint(t)
		expectIntEq(t, mp.NumPoints(), 0)
		expectCoordinatesTypeEq(t, mp.CoordinatesType(), DimXY)
	})
	t.Run("MultiLineString", func(t *testing.T) {
		mls := newempty.MultiLineString(t)
		expectIntEq(t, mls.NumLineStrings(), 0)
		expectCoordinatesTypeEq(t, mls.CoordinatesType(), DimXY)
	})
	t.Run("MultiPolygon", func(t *testing.T) {
		mp := newempty.MultiPolygon(t)
		expectIntEq(t, mp.NumPolygons(), 0)
		expectCoordinatesTypeEq(t, mp.CoordinatesType(), DimXY)
	})
	t.Run("GeometryCollection", func(t *testing.T) {
		gc := newempty.GeometryCollection(t)
		expectIntEq(t, gc.NumGeometries(), 0)
		expectCoordinatesTypeEq(t, gc.CoordinatesType(), DimXY)
	})
}

func TestEmptySliceConstructors(t *testing.T) {
	t.Run("Polygon", func(t *testing.T) {
		p, err := NewPolygon(nil)
		expectNoErr(t, err)
		expectBoolEq(t, p.IsEmpty(), true)
		expectCoordinatesTypeEq(t, p.CoordinatesType(), DimXY)
	})
	t.Run("MultiPoint", func(t *testing.T) {
		mp := NewMultiPoint(nil)
		expectIntEq(t, mp.NumPoints(), 0)
		expectCoordinatesTypeEq(t, mp.CoordinatesType(), DimXY)
	})
	t.Run("MultiLineString", func(t *testing.T) {
		mls := NewMultiLineString(nil)
		expectIntEq(t, mls.NumLineStrings(), 0)
		expectCoordinatesTypeEq(t, mls.CoordinatesType(), DimXY)
	})
	t.Run("MultiPolygon", func(t *testing.T) {
		mp, err := NewMultiPolygon(nil)
		expectNoErr(t, err)
		expectIntEq(t, mp.NumPolygons(), 0)
		expectCoordinatesTypeEq(t, mp.CoordinatesType(), DimXY)
	})
	t.Run("GeometryCollection", func(t *testing.T) {
		gc := NewGeometryCollection(nil)
		expectIntEq(t, gc.NumGeometries(), 0)
		expectCoordinatesTypeEq(t, gc.CoordinatesType(), DimXY)
	})
}
