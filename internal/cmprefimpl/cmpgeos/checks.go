package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/peterstace/simplefeatures/geom"
	"github.com/peterstace/simplefeatures/geos"
)

func unaryChecks(h *Handle, g geom.Geometry, log *log.Logger) error {
	if valid, err := checkIsValid(h, g, log); err != nil {
		return err
	} else if !valid {
		return nil
	}

	log.Println("checking AsBinary")
	if err := checkAsBinary(h, g, log); err != nil {
		return err
	}
	log.Println("checking FromBinary")
	if err := checkFromBinary(h, g, log); err != nil {
		return err
	}
	log.Println("checking AsText")
	if err := checkAsText(h, g, log); err != nil {
		return err
	}
	log.Println("checking FromText")
	if err := checkFromText(h, g, log); err != nil {
		return err
	}
	log.Println("checking IsEmpty")
	if err := checkIsEmpty(h, g, log); err != nil {
		return err
	}
	log.Println("checking Dimension")
	if err := checkDimension(h, g, log); err != nil {
		return err
	}
	log.Println("checking Envelope")
	if err := checkEnvelope(h, g, log); err != nil {
		return err
	}
	log.Println("checking IsSimple")
	if err := checkIsSimple(h, g, log); err != nil {
		return err
	}
	log.Println("checking Boundary")
	if err := checkBoundary(h, g, log); err != nil {
		return err
	}
	log.Println("checking ConvexHull")
	if err := checkConvexHull(h, g, log); err != nil {
		return err
	}
	log.Println("checking IsRing")
	if err := checkIsRing(h, g, log); err != nil {
		return err
	}
	log.Println("checking Length")
	if err := checkLength(h, g, log); err != nil {
		return err
	}
	log.Println("checking Area")
	if err := checkArea(h, g, log); err != nil {
		return err
	}
	log.Println("checking Centroid")
	if err := checkCentroid(h, g, log); err != nil {
		return err
	}
	log.Println("checking PointOnSurface")
	if err := checkPointOnSurface(h, g, log); err != nil {
		return err
	}
	return nil

	// TODO: Reverse isn't checked yet. There is some significant behaviour
	// differences between libgeos and PostGIS.
}

var mismatchErr = errors.New("mismatch")

func checkIsValid(h *Handle, g geom.Geometry, log *log.Logger) (bool, error) {
	wkb := g.AsBinary()
	var validAsPerSimpleFeatures bool
	if _, err := geom.UnmarshalWKB(wkb); err == nil {
		validAsPerSimpleFeatures = true
	}
	log.Printf("Valid as per simplefeatures: %v", validAsPerSimpleFeatures)

	validAsPerLibgeos, err := h.IsValid(g)
	if err != nil {
		// The geometry is _so_ invalid that libgeos can't even tell if it's
		// invalid or not.
		validAsPerLibgeos = false
	}
	log.Printf("Valid as per libgeos: %v", validAsPerLibgeos)

	// libgeos allows empty rings in Polygons, however simplefeatures doesn't
	// (it follows the PostGIS behaviour of disallowing empty rings).
	ignoreMismatch := hasEmptyRing(g)

	if !ignoreMismatch && validAsPerLibgeos != validAsPerSimpleFeatures {
		return false, mismatchErr
	}
	return validAsPerSimpleFeatures, nil
}

func checkAsText(h *Handle, g geom.Geometry, log *log.Logger) error {
	// Skip any geometries that have a non-empty Point within a MultiPoint.
	// Libgeos erroneously produces WKT with missing parenthesis around each
	// non-empty point.
	if containsNonEmptyPointInMultiPoint(g) {
		return nil
	}

	// Skip any geometries that are collections or contain collections that
	// only contain empty geometries. Libgeos will render WKT for these
	// collections as being EMPTY, however this isn't correct behaviour.
	if containsCollectionWithOnlyEmptyElements(g) {
		return nil
	}

	want, err := h.AsText(g)
	if err != nil {
		return err
	}

	// Account for easy-to-adjust for acceptable spacing differeneces between
	// libgeos and simplefeatures.
	want = strings.ReplaceAll(want, " (", "(")
	want = strings.ReplaceAll(want, ", ", ",")

	got := g.AsText()

	if err := wktsEqual(got, want); err != nil {
		log.Printf("WKTs not equal: %v", err)
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func wktsEqual(wktA, wktB string) error {
	toksA := tokenizeWKT(wktA)
	toksB := tokenizeWKT(wktB)
	if len(toksA) != len(toksB) {
		return fmt.Errorf(
			"token lengths differ: %d vs %d",
			len(toksA), len(toksB),
		)
	}
	for i, tokA := range toksA {
		tokB := toksB[i]
		fA, errA := strconv.ParseFloat(tokA, 64)
		fB, errB := strconv.ParseFloat(tokA, 64)
		var eq bool
		if errA == nil && errB == nil {
			// If this check gives false negatives (e.g. libgeos and
			// simplefeatures may use slightly different precision), then we
			// can always check a relative difference here instead of a strict
			// ==.
			eq = fA == fB
		} else {
			eq = tokA == tokB
		}
		if !eq {
			return fmt.Errorf(
				"tokens at position %d differ: %s vs %s",
				i, tokA, tokB,
			)
		}
	}
	return nil
}

func checkFromText(h *Handle, g geom.Geometry, log *log.Logger) error {
	// libgeos is unable to parse MultiPoints if the *first* Point is empty. It
	// gives the following error: ParseException: Unexpected token: WORD EMPTY.
	// Skip the check in that case.
	if g.IsMultiPoint() &&
		g.AsMultiPoint().NumPoints() > 0 &&
		g.AsMultiPoint().PointN(0).IsEmpty() {
		return nil
	}

	wkt := g.AsText()
	want, err := h.FromText(wkt)
	if err != nil {
		return err
	}

	got, err := geom.UnmarshalWKT(wkt)
	if err != nil {
		return err
	}

	if !geom.ExactEquals(got, want) {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

func checkAsBinary(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.AsBinary(g)
	if err != nil {
		return err
	}

	// GEOS uses a different NaN representation compared to Go (both are valid
	// NaNs). We can account for this by simply updating the WKBs to the Go NaN
	// representation. This could technically cause problems because the
	// replacement is not WKB syntax aware, but hasn't caused any problems so
	// far.
	got := g.AsBinary()
	got = bytes.ReplaceAll(got,
		[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f},
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f},
	)

	if bytes.Compare(want, got) != 0 {
		log.Printf("want:\n%s", hex.Dump(want))
		log.Printf("got:\n%s", hex.Dump(got))
		return mismatchErr
	}
	return nil
}

func checkFromBinary(h *Handle, g geom.Geometry, log *log.Logger) error {
	if containsMultiPolygonWithEmptyPolygon(g) {
		// libgeos omits the empty Polygon, but simplefeatures doesn't.
		return nil
	}

	wkb := g.AsBinary()

	// Skip any MultiPoints that contain empty Points. Libgeos seems has
	// trouble handling these.
	if g.IsMultiPoint() {
		mp := g.AsMultiPoint()
		n := mp.NumPoints()
		for i := 0; i < n; i++ {
			if mp.PointN(i).IsEmpty() {
				return nil
			}
		}
	}

	want, err := h.FromBinary(wkb)
	if err != nil {
		return err
	}

	got, err := geom.UnmarshalWKB(wkb)
	if err != nil {
		return err
	}

	if !geom.ExactEquals(want, got) {
		return mismatchErr
	}
	return nil
}

func checkIsEmpty(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.IsEmpty(g)
	if err != nil {
		return err
	}
	got := g.IsEmpty()

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got: %v", got)
		return mismatchErr
	}
	return nil
}

func checkDimension(h *Handle, g geom.Geometry, log *log.Logger) error {
	var want int
	if !containsOnlyGeometryCollections(g) {
		// Libgeos gives -1 dimension for GeometryCollection trees that only
		// contain other GeometryCollections (all the way to the leaf nodes).
		// This is weird behaviour, and the dimension should actually be zero.
		// So we don't get 'want' from libgeos in that case (and allow want to
		// default to 0).
		var err error
		want, err = h.Dimension(g)
		if err != nil {
			return err
		}
	}
	got := g.Dimension()

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got: %v", got)
		return mismatchErr
	}
	return nil
}

func checkEnvelope(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, wantDefined, err := h.Envelope(g)
	if err != nil {
		return err
	}
	got, gotDefined := g.Envelope()

	if wantDefined != gotDefined {
		log.Println("disagreement about envelope being defined")
		log.Printf("simplefeatures: %v", gotDefined)
		log.Printf("libgeos: %v", wantDefined)
		return mismatchErr
	}

	if !wantDefined {
		return nil
	}
	if want.Min() != got.Min() || want.Max() != got.Max() {
		log.Printf("want: %v", want.AsGeometry().AsText())
		log.Printf("got:  %v", got.AsGeometry().AsText())
		return mismatchErr
	}
	return nil
}

func checkIsSimple(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, wantDefined, err := h.IsSimple(g)
	if err != nil {
		if err == LibgeosCrashError {
			// Skip any tests that would cause libgeos to crash.
			return nil
		}
		return err
	}
	got, gotDefined := g.IsSimple()

	if wantDefined != gotDefined {
		log.Printf("want defined: %v", wantDefined)
		log.Printf("got defined: %v", gotDefined)
		return mismatchErr
	}
	if !gotDefined {
		return nil
	}
	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkBoundary(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, wantDefined, err := h.Boundary(g)
	if err != nil {
		return err
	}

	if !wantDefined && !g.IsGeometryCollection() {
		return errors.New("boundary not defined by libgeos, but " +
			"input is not a geometry collection (this is unexpected)")
	}
	if !wantDefined {
		return nil
	}

	got := g.Boundary()

	// PostGIS and libgeos have different behaviour for Boundary.
	// Simplefeatures currently uses the PostGIS behaviour (the difference in
	// behaviour has to do with the geometry type of empty geometries).
	if got.IsEmpty() && want.IsEmpty() {
		return nil
	}

	if !geom.ExactEquals(want, got, geom.IgnoreOrder) {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

func checkConvexHull(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.ConvexHull(g)
	if err != nil {
		return err
	}
	got := g.ConvexHull()

	// libgeos and PostGIS have slightly different behaviour when the result is
	// empty (different geometry types). Simplefeatures matches PostGIS
	// behaviour right now.
	if got.IsEmpty() && want.IsEmpty() {
		return nil
	}

	if !geom.ExactEquals(want, got, geom.IgnoreOrder) {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

func checkIsRing(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.IsRing(g)
	if err != nil {
		return err
	}
	got := g.IsLineString() && g.AsLineString().IsRing()

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkLength(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.Length(g)
	if err != nil {
		return err
	}
	got := g.Length()

	// libgeos and PostGIS disagree on the definition of length for areal
	// geometries.  PostGIS always gives zero, while libgeos gives the length
	// of the boundary. Simplefeatures follows the PostGIS behaviour.
	if isArealGeometry(g) {
		return nil
	}

	if math.Abs(want-got) > 1e-6 {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func isArealGeometry(g geom.Geometry) bool {
	switch {
	case g.IsPolygon() || g.IsMultiPolygon():
		return true
	case g.IsGeometryCollection():
		gc := g.AsGeometryCollection()
		for i := 0; i < gc.NumGeometries(); i++ {
			if isArealGeometry(gc.GeometryN(i)) {
				return true
			}
		}
	}
	return false
}

func checkArea(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.Area(g)
	if err != nil {
		return err
	}
	got := g.Area()

	if math.Abs(want-got) > 1e-6 {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkCentroid(h *Handle, g geom.Geometry, log *log.Logger) error {
	want, err := h.Centroid(g)
	if err != nil {
		return err
	}
	got := g.Centroid().AsGeometry()

	if !geom.ExactEquals(want, got, geom.ToleranceXY(1e-9)) {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

func checkPointOnSurface(h *Handle, g geom.Geometry, log *log.Logger) error {
	// It's too difficult to perform a direct comparison against GEOS's
	// PointOnSurface, due to numeric stability related issue. This is because
	// there are floating point comparisons to find the "best" point. However,
	// sometimes there are many points that are equally best. Floating point
	// issues mean that it's hard to get the implementations to line up
	// precisely in all cases (and there is no objectively best way to do it).
	// Instead, we check invariants on the result.

	pt := g.PointOnSurface().AsGeometry()

	if pt.IsEmpty() != g.IsEmpty() {
		log.Printf("The geometry's empty status doesn't match the point's empty status")
		log.Printf("g empty:  %v", g.IsEmpty())
		log.Printf("pt empty: %v", pt.IsEmpty())
		return mismatchErr
	}

	if !g.IsEmpty() && !g.IsGeometryCollection() {
		intersects, err := geos.Intersects(pt, g)
		if err != nil {
			return err
		}
		if !intersects {
			log.Printf("the pt doesn't intersect with the input")
			return mismatchErr
		}
	}

	if g.Dimension() == 2 && !g.IsEmpty() && !g.IsGeometryCollection() {
		contains, err := geos.Contains(g, pt)
		if err != nil {
			return err
		}
		if !contains {
			log.Printf("the input doesn't contain the pt")
			return mismatchErr
		}
	}

	return nil
}

func binaryChecks(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	for _, g := range []geom.Geometry{g1, g2} {
		if valid, err := checkIsValid(h, g, log); err != nil {
			return err
		} else if !valid {
			return nil
		}
	}

	log.Println("checking Intersects")
	if err := checkIntersects(h, g1, g2, log); err != nil {
		return err
	}

	log.Println("checking ExactEquals")
	if err := checkExactEquals(h, g1, g2, log); err != nil {
		return err
	}

	log.Println("checking Distance")
	if err := checkDistance(h, g1, g2, log); err != nil {
		return err
	}

	log.Println("checking DCEL operations")
	if err := checkDCELOperations(h, g1, g2, log); err != nil {
		return err
	}

	return nil
}

func checkIntersects(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	skipList := map[string]bool{
		// postgres=# SELECT ST_Intersects(
		//   ST_GeomFromText('LINESTRING(1 0,0.5000000000000001 0.5,0 1)'),
		//   ST_GeomFromText('LINESTRING(0.5 0.5,1.5 1.5)')
		// );
		//  st_intersects
		// ---------------
		//  f # WRONG!!
		// (1 row)
		"LINESTRING(1 0,0.5000000000000001 0.5,0 1)": true,

		// Simplefeatures sometimes gives an incorrect result for this due to
		// numerical precision issues. Would be solved by
		// https://github.com/peterstace/simplefeatures/issues/274
		"LINESTRING(0.5 0,0.5000000000000001 0.5)":                              true,
		"MULTILINESTRING((0 0,2 2.000000000000001),(1 0,-1 2.000000000000001))": true,
	}
	if skipList[g1.AsText()] || skipList[g2.AsText()] {
		// Skipping test because GEOS gives the incorrect result for *some*
		// intersection operations involving this input.
		return nil
	}

	want, err := h.Intersects(g1, g2)
	if err != nil {
		if err == LibgeosCrashError {
			return nil
		}
		return err
	}
	got := geom.Intersects(g1, g2)

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkExactEquals(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	want, err := h.ExactEquals(g1, g2)
	if err != nil {
		return err
	}
	got := geom.ExactEquals(g1, g2)

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkDistance(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	want, err := h.Distance(g1, g2)
	if err != nil {
		if err == LibgeosCrashError {
			// Skip any tests that would cause libgeos to crash.
			return nil
		}
		return err
	}
	got, ok := geom.Distance(g1, g2)
	if !ok {
		// GEOS gives 0 when distance is not defined.
		got = 0
	}

	if math.Abs(want-got) > 1e-12 {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

var skipIntersection = map[string]bool{
	"LINESTRING(0 1,0.3333333333 0.6666666667,1 0)": true,
	"LINESTRING(1 0,0.5000000000000001 0.5,0 1)":    true,
	"MULTILINESTRING((0 0,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 2),(0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0))": true,
	"MULTILINESTRING((0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0))":                                                                 true,
	"MULTILINESTRING((0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0),(0 0,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 2))": true,
	"POLYGON((1 0,0.9807852804032305 -0.19509032201612808,0.923879532511287 -0.3826834323650894,0.8314696123025456 -0.5555702330196017,0.7071067811865481 -0.7071067811865469,0.5555702330196031 -0.8314696123025447,0.38268343236509084 -0.9238795325112863,0.19509032201612964 -0.9807852804032302,0.0000000000000016155445744325867 -1,-0.19509032201612647 -0.9807852804032308,-0.38268343236508784 -0.9238795325112875,-0.5555702330196005 -0.8314696123025463,-0.7071067811865459 -0.7071067811865491,-0.8314696123025438 -0.5555702330196043,-0.9238795325112857 -0.38268343236509234,-0.9807852804032299 -0.19509032201613122,-1 -0.0000000000000032310891488651735,-0.9807852804032311 0.19509032201612486,-0.9238795325112882 0.38268343236508634,-0.8314696123025475 0.555570233019599,-0.7071067811865505 0.7071067811865446,-0.5555702330196058 0.8314696123025428,-0.3826834323650936 0.9238795325112852,-0.19509032201613213 0.9807852804032297,-0.000000000000003736410698672604 1,0.1950903220161248 0.9807852804032311,0.38268343236508673 0.9238795325112881,0.5555702330195996 0.8314696123025469,0.7071067811865455 0.7071067811865496,0.8314696123025438 0.5555702330196044,0.9238795325112859 0.38268343236509206,0.98078528040323 0.19509032201613047,1 0))": true,
	"MULTILINESTRING((0 0,2 2.000000000000001),(1 0,-1 2.000000000000001))":                         true,
	"MULTILINESTRING((0 0,0.5 0.5,1 1,2 2.000000000000001),(1 0,0.5 0.5,0 1,-1 2.000000000000001))": true,
	"POLYGON((1.5 1,1.353553390593274 0.6464466094067265,1.0000000000000009 0.5,0.646446609406727 0.6464466094067254,0.5 0.9999999999999983,0.6464466094067247 1.3535533905932722,0.9999999999999977 1.5,1.3535533905932717 1.3535533905932757,1.5 1))": true,
	"POLYGON((1 0,-0.9 -0.2,-1 -0.0000000000000032310891488651735,-0.9 0.2,1 0))": true,
	"LINESTRING(0.5 0,0.5000000000000001 0.5)":                                    true,
	"LINESTRING(0.5 1,0.5000000000000001 0.5)":                                    true,
}

var skipDifference = map[string]bool{
	"LINESTRING(0 1,0.3333333333 0.6666666667,0.5 0.5,1 0)": true,
	"LINESTRING(0 1,0.3333333333 0.6666666667,1 0)":         true,
	"LINESTRING(1 0,0.5000000000000001 0.5,0 1)":            true,
	"MULTILINESTRING((0 0,0.5 0.5),(0.5 0.5,1 1),(0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,0.5 0.5),(0.5 0.5,1 0))": true,
	"MULTILINESTRING((0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0))":                                               true,

	"MULTIPOLYGON(((1 0,0 1,0.5 1.5,1 1,1.5 1.5,2 1,1 0)),((1.5 1.5,1 2,0.5 1.5,0.333333333333333 1.66666666666667,0 2,1 3,2 2,1.5 1.5)),((3.5 1.5,4 1,3 0,2 1,2.5 1.5,3 1,3.5 1.5)),((3.5 1.5,3 2,2.5 1.5,2 2,3 3,4 2,3.5 1.5)))": true,
	"POLYGON((1 0,-0.9 -0.2,-1 -0.0000000000000032310891488651735,-0.9 0.2,1 0))": true,
	"POLYGON((1 0,0.9807852804032305 -0.19509032201612808,0.923879532511287 -0.3826834323650894,0.8314696123025456 -0.5555702330196017,0.7071067811865481 -0.7071067811865469,0.5555702330196031 -0.8314696123025447,0.38268343236509084 -0.9238795325112863,0.19509032201612964 -0.9807852804032302,0.0000000000000016155445744325867 -1,-0.19509032201612647 -0.9807852804032308,-0.38268343236508784 -0.9238795325112875,-0.5555702330196005 -0.8314696123025463,-0.7071067811865459 -0.7071067811865491,-0.8314696123025438 -0.5555702330196043,-0.9238795325112857 -0.38268343236509234,-0.9807852804032299 -0.19509032201613122,-1 -0.0000000000000032310891488651735,-0.9807852804032311 0.19509032201612486,-0.9238795325112882 0.38268343236508634,-0.8314696123025475 0.555570233019599,-0.7071067811865505 0.7071067811865446,-0.5555702330196058 0.8314696123025428,-0.3826834323650936 0.9238795325112852,-0.19509032201613213 0.9807852804032297,-0.000000000000003736410698672604 1,0.1950903220161248 0.9807852804032311,0.38268343236508673 0.9238795325112881,0.5555702330195996 0.8314696123025469,0.7071067811865455 0.7071067811865496,0.8314696123025438 0.5555702330196044,0.9238795325112859 0.38268343236509206,0.98078528040323 0.19509032201613047,1 0))": true,
	"MULTILINESTRING((0 0,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 2),(0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0))": true,
	"MULTILINESTRING((0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0),(0 0,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 2))": true,
	"MULTILINESTRING((0 0,2 2.000000000000001),(1 0,-1 2.000000000000001))":                                                                            true,
	"MULTILINESTRING((0 0,0.5 0.5,1 1,2 2.000000000000001),(1 0,0.5 0.5,0 1,-1 2.000000000000001))":                                                    true,
	"MULTILINESTRING((0 0,0.5 0.5),(0.5 0.5,1 1),(0 1,0.3333333333 0.6666666667,0.5 0.5),(0.5 0.5,1 0))":                                               true,
	"LINESTRING(0.5 0,0.5000000000000001 0.5)": true,
	"LINESTRING(0.5 1,0.5000000000000001 0.5)": true,
}

var skipSymDiff = map[string]bool{
	"LINESTRING(0 1,0.3333333333 0.6666666667,0.5 0.5,1 0)": true,
	"LINESTRING(0 1,0.3333333333 0.6666666667,1 0)":         true,
	"LINESTRING(1 0,0.5000000000000001 0.5,0 1)":            true,
	"MULTILINESTRING((0 0,0.5 0.5),(0.5 0.5,1 1),(0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,0.5 0.5),(0.5 0.5,1 0))": true,
	"MULTILINESTRING((0 1,0.3333333333 0.6666666667),(0.3333333333 0.6666666667,1 0))":                                               true,
}

func checkDCELOperations(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	// TODO: simplefeatures doesn't support GeometryCollections yet
	if g1.IsGeometryCollection() || g2.IsGeometryCollection() {
		return nil
	}

	for _, op := range []struct {
		name     string
		sfFunc   func(g1, g2 geom.Geometry) (geom.Geometry, error)
		geosFunc func(g1, g2 geom.Geometry) (geom.Geometry, error)
		skip     map[string]bool
	}{
		{
			"Union",
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return geom.Union(g1, g2) },
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return h.Union(g1, g2) },
			nil,
		},
		{
			"Intersection",
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return geom.Intersection(g1, g2) },
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return h.Intersection(g1, g2) },
			skipIntersection,
		},
		{
			"Difference",
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return geom.Difference(g1, g2) },
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return h.Difference(g1, g2) },
			skipDifference,
		},
		{
			"SymmetricDifference",
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return geom.SymmetricDifference(g1, g2) },
			func(g1, g2 geom.Geometry) (geom.Geometry, error) { return h.SymmetricDifference(g1, g2) },
			skipSymDiff,
		},
	} {
		log.Println("checking", op.name)
		err := checkDCELOp(op.sfFunc, op.geosFunc, g1, g2, op.skip, log)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkDCELOp(
	op func(g1, g2 geom.Geometry) (geom.Geometry, error),
	refImpl func(g1, g2 geom.Geometry) (geom.Geometry, error),
	g1, g2 geom.Geometry,
	skip map[string]bool,
	log *log.Logger,
) error {
	// Empty points will cause the reference impl to crash.
	if hasEmptyPoint(g1) || hasEmptyPoint(g2) {
		return nil
	}

	got, err := op(g1, g2)
	if err != nil {
		return err
	}

	// Some geometries give results that are not topologically equivalent to
	// those from GEOS. These have been checked manually, and decided that the
	// difference is acceptable (they typically have to do with different
	// handling of numerically degenerate cases). Note that we bail out of this
	// test _after_ we calculate got. That way we're at least checking that it
	// doesn't crash or give an error.
	if skip[g1.AsText()] || skip[g2.AsText()] {
		return nil
	}

	want, err := refImpl(g1, g2)
	if err != nil {
		if err == ErrInvalidAccordingToGEOS {
			// Because GEOS has given us back an invalid geometry (even according
			// to its own validation routines) we can't trust it for this test
			// case.
			return nil
		}
		if err == ErrInvalidAccordingToSF {
			// GEOS has given back an invalid geometry according to
			// simplefeatures' validation routines (however GEOS thinks that
			// it's valid). This is _probably_ due to slight differences
			// between floating point precision in the validation algorithms
			// between GEOS and SF.
			//
			// We need to look into these cases, however for the time being we
			// can't continue the test here for these cases.
			//
			// TODO: look into these cases.
			return nil
		}
		return err
	}

	if !mantissaTerminatesQuickly(got) || !mantissaTerminatesQuickly(want) {
		// We're not going to be able to compare got and want because of
		// numeric precision issues.
		log.Printf("mantissa doesn't terminate quickly, using area heuristic")
		if err := checkEqualityHeuristic(want, got, log); err != nil {
			return err
		}
		return nil
	}

	if want.IsGeometryCollection() || got.IsGeometryCollection() {
		// We can't use Equals from GEOS on GeometryCollections, so we can't
		// use proper Equals for this case.
		log.Printf("want or got is a geometry collection, using area heuristic")
		if err := checkEqualityHeuristic(want, got, log); err != nil {
			return err
		}
		return nil
	}

	eq, err := geos.Equals(want, got)
	if err != nil {
		return err
	}
	if !eq {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

// checkEqualityHeuristic checks some necessary but not sufficient properties
// of two geometries if they are to be equal.
//
// TODO: we could come up with some smarter heuristics. E.g. distance sampled
// by many random points.
func checkEqualityHeuristic(want, got geom.Geometry, log *log.Logger) error {
	wantArea := want.Area()
	gotArea := got.Area()
	if math.Abs(wantArea-gotArea) > 1e-3 {
		log.Printf("wantWKT: %v\n", want.AsText())
		log.Printf("gotWKT:  %v\n", got.AsText())
		log.Printf("wantArea: %v\n", wantArea)
		log.Printf("gotArea:  %v\n", gotArea)
		return mismatchErr
	}
	return nil
}
