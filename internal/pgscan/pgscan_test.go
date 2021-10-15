package pgscan

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/peterstace/simplefeatures/internal/newempty"

	_ "github.com/lib/pq"
)

func TestPostgresScan(t *testing.T) {
	const dbURL = "postgres://postgres:password@postgis:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}

	for i, tc := range []struct {
		wkt      string
		concrete interface{ AsText() string }
	}{
		{"POINT(0 1)", newempty.Point(t)},
		{"MULTIPOINT((0 1))", newempty.MultiPoint(t)},
		{"LINESTRING(0 1,1 0)", newempty.LineString(t)},
		{"MULTILINESTRING((0 1,1 0))", newempty.MultiLineString(t)},
		{"POLYGON((0 0,1 0,0 1,0 0))", newempty.Polygon(t)},
		{"MULTIPOLYGON(((0 0,1 0,0 1,0 0)))", newempty.MultiPolygon(t)},
		{"GEOMETRYCOLLECTION(MULTIPOLYGON(((0 0,1 0,0 1,0 0))))", newempty.GeometryCollection(t)},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if err := db.QueryRow(
				`SELECT ST_AsBinary(ST_GeomFromText($1))`,
				tc.wkt,
			).Scan(tc.concrete); err != nil {
				t.Error(err)
			}
			if got := tc.concrete.AsText(); got != tc.wkt {
				t.Errorf("want=%v got=%v", tc.wkt, got)
			}
		})
	}
}
