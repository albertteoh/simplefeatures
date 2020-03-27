package libgeos

/*
#cgo linux CFLAGS: -I/usr/include
#cgo linux LDFLAGS: -L/usr/lib -lgeos_c
#include "geos_c.h"
#include <stdlib.h>
#include <string.h>

void sf_error_handler(const char *message, void *userdata) {
	strncpy(userdata, message, 1024);
}

GEOSContextHandle_t sf_init(void *userdata) {
	GEOSContextHandle_t ctx = GEOS_init_r();
	GEOSContext_setErrorMessageHandler_r(ctx, sf_error_handler, userdata);
	return ctx;
}

*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/peterstace/simplefeatures/geom"
)

// Handle is an opaque handle that can be used to invoke libgeos operations.
// Instances are not threadsafe and thus should not be used in a concurrent
// manner.
type Handle struct {
	context C.GEOSContextHandle_t
	errBuf  [1024]byte
}

// NewHandle creates a new libgeos handle.
func NewHandle() (*Handle, error) {
	h := &Handle{}
	h.context = C.sf_init(unsafe.Pointer(&h.errBuf))
	if h.context == nil {
		return nil, errors.New("could not create libgeos context")
	}
	return h, nil
}

// err gets the last error message reported by libgeos as an error type. It
// always returns a non-nil error. If no error message has been reported, then
// it returns a generic error message.
func (h *Handle) err() error {
	msg := h.errMsg()
	if msg == "" {
		// No error stored, which indicates that the error handler didn't get
		// trigged. The best we can do is give a generic error.
		msg = "libgeos internal error"
	}
	h.errBuf = [1024]byte{} // Reset the buffer for the next error message.
	return errors.New(strings.TrimSpace(msg))
}

// errMsg gets the textual representation of the last error message reported by
// libgeos.
func (h *Handle) errMsg() string {
	// The error message is either NULL terminated, or fills the entire buffer.
	firstZero := len(h.errBuf)
	for i, b := range h.errBuf {
		if b == 0 {
			firstZero = i
			break
		}
	}
	return string(h.errBuf[:firstZero])
}

// Release releases any resources held by the handle. The handle should not be
// used after Release is called.
func (h *Handle) Release() {
	C.GEOS_finish_r(h.context)
}

// createGeometryHandle converts a Geometry object into a libgeos geometry handle.
func (h *Handle) createGeometryHandle(g geom.Geometry) (*C.GEOSGeometry, error) {
	wkbReader := C.GEOSWKBReader_create_r(h.context)
	if wkbReader == nil {
		return nil, h.err()
	}
	defer C.GEOSWKBReader_destroy_r(h.context, wkbReader)

	wkb := new(bytes.Buffer)
	if err := g.AsBinary(wkb); err != nil {
		return nil, err
	}
	gh := C.GEOSWKBReader_read_r(
		h.context,
		wkbReader,
		(*C.uchar)(&wkb.Bytes()[0]),
		C.ulong(wkb.Len()),
	)
	if gh == nil {
		return nil, h.err()
	}
	return gh, nil
}

// ErrGeometryCollectionNotSupported indicates that a GeometryCollection was
// passed to a function that does not support GeometryCollections.
var ErrGeometryCollectionNotSupported = errors.New("GeometryCollection not supported")

// Equals returns true if and only if the input geometries are spatially equal.
func (h *Handle) Equals(g1, g2 geom.Geometry) (bool, error) {
	if g1.IsEmpty() && g2.IsEmpty() {
		// Part of the mask is 'dim(I(a) ∩ I(b)) = T'.  If both inputs are
		// empty, then their interiors will be empty, and thus
		// 'dim(I(a) ∩ I(b) = F'. However, we want to return 'true' for this
		// case. So we just return true manually rather than using DE-9IM.
		return true, nil
	}
	return h.relate(g1, g2, "T*F**FFF*")
}

// Disjoint returns true if and only if the input geometries have no points in
// common.
func (h *Handle) Disjoint(g1, g2 geom.Geometry) (bool, error) {
	return h.relate(g1, g2, "FF*FF****")
}

// coalesce gives the first non-nil error argument, or nil if there are no
// non-nil arguments.
func coalesce(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Touches returns true if and only if the geometries have at least 1 point in
// common, but their interiors don't intersect.
func (h *Handle) Touches(g1, g2 geom.Geometry) (bool, error) {
	r1, err1 := h.relate(g1, g2, "FT*******")
	r2, err2 := h.relate(g1, g2, "F**T*****")
	r3, err3 := h.relate(g1, g2, "F***T****")
	if err := coalesce(err1, err2, err3); err != nil {
		return false, err
	}
	return r1 || r2 || r3, nil
}

// Contains returns true if and only if geometry A contains geometry B.
// Formally, the following two conditions must hold:
//
// 1. No points of B lies on the exterior of geometry A. That is, B must only be
// in the exterior or boundary of A.
//
// 2.At least one point of the interior of B lies on the interior of A. That
// is, they can't *only* intersect at their boundaries.
func (h *Handle) Contains(a, b geom.Geometry) (bool, error) {
	return h.relate(a, b, "T*****FF*")
}

// Covers returns true if and only if geometry A covers geometry B. Formally,
// the following two conditions must hold:
//
// 1. No points of B lies on the exterior of geometry A. That is, B must only be
// in the exterior or boundary of A.
//
// 2. At least one point of B lines on A (either its interor or boundary).
func (h *Handle) Covers(a, b geom.Geometry) (bool, error) {
	r1, err1 := h.relate(a, b, "T*****FF*")
	r2, err2 := h.relate(a, b, "*T****FF*")
	r3, err3 := h.relate(a, b, "***T**FF*")
	r4, err4 := h.relate(a, b, "****T*FF*")
	if err := coalesce(err1, err2, err3, err4); err != nil {
		return false, err
	}
	return r1 || r2 || r3 || r4, nil
}

// relate invokes the libgeos GEOSRelatePattern function, which checks if two
// geometries are related according to a DE-9IM 'relates' mask.
func (h *Handle) relate(g1, g2 geom.Geometry, mask string) (bool, error) {
	if g1.IsGeometryCollection() || g2.IsGeometryCollection() {
		return false, ErrGeometryCollectionNotSupported
	}

	gh1, err := h.createGeometryHandle(g1)
	if err != nil {
		return false, err
	}
	defer C.GEOSGeom_destroy(gh1)

	gh2, err := h.createGeometryHandle(g2)
	if err != nil {
		return false, err
	}

	defer C.GEOSGeom_destroy(gh2)
	cmask := C.CString(mask)
	defer C.free(unsafe.Pointer(cmask))

	switch ret := C.GEOSRelatePattern_r(h.context, gh1, gh2, cmask); ret {
	case 2:
		return false, h.err()
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected return code: %d", ret)
	}
}
