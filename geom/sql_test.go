package geom_test

import (
	"strconv"
	"testing"

	"github.com/peterstace/simplefeatures/internal/newempty"

	. "github.com/peterstace/simplefeatures/geom"
)

func TestSQLValueGeometry(t *testing.T) {
	g := geomFromWKT(t, "POINT(1 2)")
	val, err := g.Value()
	if err != nil {
		t.Fatal(err)
	}
	geom, err := UnmarshalWKB(val.([]byte))
	if err != nil {
		t.Fatal(err)
	}
	expectGeomEq(t, g, geom)
}

func TestSQLScanGeometry(t *testing.T) {
	const wkt = "POINT(2 3)"
	wkb := geomFromWKT(t, wkt).AsBinary()
	var g Geometry
	check := func(t *testing.T, err error) {
		if err != nil {
			t.Fatal(err)
		}
		expectGeomEq(t, g, geomFromWKT(t, wkt))
	}
	t.Run("string", func(t *testing.T) {
		g = Geometry{}
		check(t, g.Scan(string(wkb)))
	})
	t.Run("byte", func(t *testing.T) {
		g = Geometry{}
		check(t, g.Scan(wkb))
	})
}

func TestSQLValueConcrete(t *testing.T) {
	for i, wkt := range []string{
		"POINT EMPTY",
		"POINT(1 2)",
		"LINESTRING(1 2,3 4)",
		"LINESTRING(1 2,3 4,5 6)",
		"POLYGON((0 0,1 0,0 1,0 0))",
		"MULTIPOINT((1 2))",
		"MULTILINESTRING((1 2,3 4,5 6))",
		"MULTIPOLYGON(((0 0,1 0,0 1,0 0)))",
		"GEOMETRYCOLLECTION(POINT(1 2))",
		"GEOMETRYCOLLECTION EMPTY",
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Log(wkt)
			geom := geomFromWKT(t, wkt)
			val, err := geom.Value()
			expectNoErr(t, err)
			g, err := UnmarshalWKB(val.([]byte))
			expectNoErr(t, err)
			expectGeomEq(t, g, geom)
		})
	}
}

func TestSQLScanConcrete(t *testing.T) {
	for _, tc := range []struct {
		wkt      string
		concrete interface {
			AsText() string
			Scan(interface{}) error
		}
	}{
		{"POINT(0 1)", newempty.Point(t)},
		{"MULTIPOINT((0 1))", newempty.MultiPoint(t)},
		{"LINESTRING(0 1,1 0)", newempty.LineString(t)},
		{"MULTILINESTRING((0 1,1 0))", newempty.MultiLineString(t)},
		{"POLYGON((0 0,1 0,0 1,0 0))", newempty.Polygon(t)},
		{"MULTIPOLYGON(((0 0,1 0,0 1,0 0)))", newempty.MultiPolygon(t)},
		{"GEOMETRYCOLLECTION(MULTIPOLYGON(((0 0,1 0,0 1,0 0))))", newempty.GeometryCollection(t)},
	} {
		t.Run(tc.wkt, func(t *testing.T) {
			wkb := geomFromWKT(t, tc.wkt).AsBinary()
			err := tc.concrete.Scan(wkb)
			expectNoErr(t, err)
			expectStringEq(t, tc.concrete.AsText(), tc.wkt)
		})
	}
}
