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

	got, err := geom.UnmarshalWKT(strings.NewReader(wkt))
	if err != nil {
		return err
	}

	if !got.EqualsExact(want) {
		log.Printf("want: %v", want.AsText())
		log.Printf("got:  %v", got.AsText())
		return mismatchErr
	}
	return nil
}

func checkAsBinary(h *Handle, g geom.Geometry, log *log.Logger) error {
	var wantDefined bool
	want, err := h.AsBinary(g)
	if err == nil {
		wantDefined = true
	}
	hasPointEmpty := hasEmptyPoint(g)
	if !wantDefined && !hasPointEmpty {
		return errors.New("AsBinary wasn't defined by libgeos and the test is " +
			"NOT for a geometry containing a POINT EMPTY, which is unexpected",
		)
	}
	if !wantDefined {
		// Skip the test, since we don't have a WKB from libgeos to compare to.
		// This is only for the POINT EMPTY case. Simplefeatures _does_ produce
		// a WKB for POINT EMPTY although this is strictly an extension to the
		// spec.
		return nil
	}

	got := g.AsBinary()
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

	if !want.EqualsExact(got) {
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

	if !want.EqualsExact(got, geom.IgnoreOrder) {
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

	if !want.EqualsExact(got, geom.IgnoreOrder) {
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

	if !want.EqualsExact(got, geom.ToleranceXY(1e-9)) {
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

	if err := checkIntersects(h, g1, g2, log); err != nil {
		return err
	}
	if err := checkEqualsExact(h, g1, g2, log); err != nil {
		return err
	}
	return nil
}

func checkIntersects(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	want, err := h.Intersects(g1, g2)
	if err != nil {
		if err == LibgeosCrashError {
			return nil
		}
		return err
	}
	got := g1.Intersects(g2)

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}

func checkEqualsExact(h *Handle, g1, g2 geom.Geometry, log *log.Logger) error {
	want, err := h.EqualsExact(g1, g2)
	if err != nil {
		return err
	}
	got := g1.EqualsExact(g2)

	if want != got {
		log.Printf("want: %v", want)
		log.Printf("got:  %v", got)
		return mismatchErr
	}
	return nil
}
