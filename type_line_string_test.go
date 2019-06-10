package simplefeatures_test

import (
	"math"
	"strconv"
	"testing"

	. "github.com/peterstace/simplefeatures"
)

func TestLineStringValidation(t *testing.T) {
	xy := func(x, y float64) Coordinates {
		return Coordinates{XY: XY{x, y}}
	}
	for i, pts := range [][]Coordinates{
		{xy(0, 0)},
		{xy(1, 1)},
		{xy(0, 0), xy(0, 0)},
		{xy(1, 1), xy(1, 1)},
		{xy(0, 0), xy(1, math.NaN())},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			_, err := NewLineString(pts)
			if err == nil {
				t.Error("expected error")
			}
			t.Log(err)
		})
	}
}
