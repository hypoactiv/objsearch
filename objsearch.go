package objsearch

import (
	"fmt"
	"image"
	"io"
	"math"
	"sort"
	"sync"

	"github.com/hypoactiv/imutil"
)

type objSearchContext struct {
	Field, Object *image.RGBA
	SearchRect    image.Rectangle
	Tolerance     float64
	VerboseOut    io.Writer
	MinDist       int
}

// Color processing mode
type ColorMode int

// Per-channel result combination mode
type CombineMode int

const (
	COLORMODE_GRAY = iota // convert field and image to grayscale before searching

	COMBINEMODE_MAX // combine results by taking the per-pixel maximum over all channels
)

// Returns a slice of Hits indicating detected occurences of 'object' in 'field'
// Hits are at the top-left corner of the detected object.
// Hits returned have scores below tolerance and are at least minDist
// pixels from eachother.
func Search(field, object *image.RGBA, rect image.Rectangle, tolerance float64, minDist int, verboseOut io.Writer, colorMode ColorMode, combineMode CombineMode) []Hit {
	ctx := objSearchContext{
		Field:      field,
		Object:     object,
		SearchRect: rect,
		Tolerance:  tolerance,
		VerboseOut: verboseOut,
	}
	// create intermediate field and object images
	interField, interObject := []*image.Gray{}, []*image.Gray{}
	switch colorMode {
	case COLORMODE_GRAY:
		// generate grayscale intermediate images
		interField = append(interField, imutil.ToGrayscale(field))
		interObject = append(interObject, imutil.ToGrayscale(object))
	default:
		panic("invalid color mode")
	}
	// perform objSearch on each intermediate image pair to get
	// per-channel field-object distances
	if len(interField) != len(interObject) {
		panic("internal error")
	}
	results := make([]objSearchResult, 0, len(interField))
	for i := range interField {
		results = append(results, ctx.objSearch(interField[i], interObject[i]))
		if len(results[i].distances) != len(results[0].distances) {
			// output results inconsistent
			panic("internal error")
		}
	}
	// combine per-channel distances
	combined := make([]float64, len(results[0].distances))
	min := results[0].min
	max := results[0].max
	switch combineMode {
	case COMBINEMODE_MAX:
		for j := range combined {
			combined[j] = results[0].distances[j]
			for i := 1; i < len(results); i++ {
				if combined[j] < results[i].distances[j] {
					// replace with larger distance
					combined[j] = results[i].distances[j]
				}
			}
			if combined[j] < min {
				min = combined[j]
			}
			if combined[j] > max {
				max = combined[j]
			}
		}
	default:
		panic("invalid combine mode")
	}
	return ctx.findHits(combined, 0, max)
}

// The computed object-field distances, and the minimum and maximum distances
// observed
type objSearchResult struct {
	distances []float64
	min, max  float64
}

func (ctx objSearchContext) objSearch(field, object *image.Gray) (res objSearchResult) {
	// convert the pixel at (x,y) in img to a float64
	float := func(img *image.Gray, x, y int) float64 {
		return float64(img.GrayAt(x, y).Y) / 255.0
	}
	wg := sync.WaitGroup{}
	res.distances = make([]float64, ctx.SearchRect.Dx()*ctx.SearchRect.Dy())
	// compute the L1-norm distance between 'object' and and 'object'-sized
	// rectangle of 'field' with top-left corner at (u,v) in 'field'
	//
	// store result in res.distances[offset(u,v)]
	objSearch1 := func(u, v int) {
		result := 0.0
		i := ctx.offset(u, v)
		res.distances[i] = 0
		// Compute L1-norm
		for x := object.Rect.Min.X; x < object.Rect.Max.X; x++ {
			for y := object.Rect.Min.Y; y < object.Rect.Max.Y; y++ {
				result += math.Abs(float(field, u+x, v+y) - float(object, x, y))
			}
		}
		res.distances[i] = result
		wg.Done()
	}
	ctx.verboseOut("\n")
	// choose column of ctx.SearchRect
	for u := ctx.SearchRect.Min.X; u < ctx.SearchRect.Max.X; u++ {
		// launch one gorres.distancesine per row of ctx.SearchRect
		wg.Add(ctx.SearchRect.Size().Y)
		for v := ctx.SearchRect.Min.Y; v < ctx.SearchRect.Max.Y; v++ {
			go objSearch1(u, v)
		}
		// wait for this column to finish
		wg.Wait()
		ctx.verboseOut("\r%.2f%% complete", float64(u-ctx.SearchRect.Min.X)/float64(ctx.SearchRect.Size().X)*100)
		// start next column
	}
	ctx.verboseOut("\n")
	// compute min and res.max of res.distances
	res.min = res.distances[0]
	res.max = res.distances[0]
	for i := range res.distances {
		if res.distances[i] < res.min {
			res.min = res.distances[i]
		}
		if res.distances[i] > res.max {
			res.max = res.distances[i]
		}
	}
	// done
	return
}

// Find L1 distances in d that are below t, and return them as a slice of Hits
// Only hits at least minDist apart are found
//
// d is a slice of distances as returned from objSearch
// Distances in d equal to max are mapped to a Hit score of 1, and distances
// equal to min is mapped to a Hit score of 0.
// t is the L1 distance threshhold to produce a hit.
func (ctx objSearchContext) findHits(d []float64, min, max float64) (hits []Hit) {
	if max <= min {
		panic("max <= min")
	}
	hitChan := make(chan Hit)
	dRange := max - min
	go func() {
		for i := range d {
			// normalize L1 distances into interval [0,1] to compute each
			// pixel's score
			p := (d[i] - min) / dRange
			if p < ctx.Tolerance {
				// pixel's score is within tolerance, create a hit
				x, y := ctx.coords(i)
				hitChan <- Hit{image.Point{x, y}, p}
			}
		}
		close(hitChan)
	}()
nextHit:
	for h := range hitChan {
		for j := range hits {
			if hits[j].Distance(h) < ctx.MinDist {
				// h is too close to hits[j]
				// replace hits[j] if h's score is better, otherwise drop h
				if h.S < hits[j].S {
					hits[j] = h
				}
				continue nextHit
			}
		}
		// h is a new hit
		hits = append(hits, h)
	}
	// sort hits
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].S < hits[j].S
	})
	return
}

////
// Utility functions

// output only if verbose output desired
func (ctx objSearchContext) verboseOut(format string, a ...interface{}) {
	if ctx.VerboseOut != nil {
		fmt.Fprintf(ctx.VerboseOut, format, a...)
	}
}

// return the slice index corresponding to (x,y) in the search rectangle
//
// the inverse of coords
func (ctx objSearchContext) offset(x, y int) int {
	return (x - ctx.SearchRect.Min.X) + ctx.SearchRect.Dx()*(y-ctx.SearchRect.Min.Y)
}

// return the (x,y) coordinates corresponding to slice index i
//
// the inverse of offset
func (ctx objSearchContext) coords(i int) (x, y int) {
	x = ctx.SearchRect.Min.X + (i % ctx.SearchRect.Dx())
	y = ctx.SearchRect.Min.Y + (i / ctx.SearchRect.Dx())
	return
}
