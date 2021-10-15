package newempty

import (
	"testing"

	"github.com/peterstace/simplefeatures/geom"
)

func Point(_ *testing.T) geom.Point {
	return geom.NewEmptyPoint(geom.DimXY)
}

func MultiPoint(_ *testing.T) geom.MultiPoint {
	return geom.NewMultiPoint([]geom.Point{})
}

func MultiLineString(_ *testing.T) geom.MultiLineString {
	return geom.NewMultiLineString([]geom.LineString{})
}

func GeometryCollection(_ *testing.T) geom.GeometryCollection {
	return geom.NewGeometryCollection([]geom.Geometry{})
}

func LineString(t *testing.T) geom.LineString {
	ls, err := geom.NewLineString(geom.NewSequence([]float64{}, geom.DimXY))
	expectNoErr(t, err)
	return ls
}

func Polygon(t *testing.T) geom.Polygon {
	poly, err := geom.NewPolygon([]geom.LineString{})
	expectNoErr(t, err)
	return poly
}

func MultiPolygon(t *testing.T) geom.MultiPolygon {
	multipoly, err := geom.NewMultiPolygon([]geom.Polygon{})
	expectNoErr(t, err)
	return multipoly
}

func expectNoErr(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
