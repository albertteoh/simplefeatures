package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/peterstace/simplefeatures/generate"
)

func main() {
	seed := flag.Int64("seed", 0, "seed (0 will cause the current unix nano epoch to be used)")
	geomType := flag.String("type", "", "geometry type (point, line, linestring, "+
		"polygon, multipoint, multilinestring, multipolygon, geometrycollection")
	count := flag.Int("count", 1, "the number of geometries to generate")
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	log.Printf("seed: %d", *seed)
	rnd := rand.New(rand.NewSource(*seed))

	switch *geomType {
	case "point":
		generatePoints(rnd, *count)
	case "line":
		generateLines(rnd, *count)
	case "linestring":
		generateLineStrings(rnd, *count)
	case "polygon":
		generatePolygons(rnd, *count)
	case "multipoint":
		fallthrough
	case "multilinestring":
		fallthrough
	case "multipolygon":
		fallthrough
	case "geometrycollection":
		log.Fatal("geometry type not supported yet")
	default:
		log.Fatal("unknown geometry type")
	}
}

func generatePoints(rnd *rand.Rand, count int) {
	for i := 0; i < count; i++ {
		fmt.Println(generate.RandomPoint(rnd).AsText())
	}
}

func generateLines(rnd *rand.Rand, count int) {
	for i := 0; i < count; i++ {
		fmt.Println(generate.RandomLine(rnd).AsText())
	}
}

func generateLineStrings(rnd *rand.Rand, count int) {
	for i := 0; i < count; i++ {
		ls := generate.RandomLineStringRandomWalk(rnd, generate.LineStringSpec{
			NumPoints: 50,
			IsClosed:  true,
			IsSimple:  true,
		})
		fmt.Println(ls.AsText())
	}
}

func generatePolygons(rnd *rand.Rand, count int) {
	for i := 0; i < count; i++ {
		wkt := generate.RandomPolygon(rnd, generate.PolygonSpec{
			Valid:      true,
			RingPoints: []int{20, 10},
		})
		fmt.Println(wkt)
	}
}
