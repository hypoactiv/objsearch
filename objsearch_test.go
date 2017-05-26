package objsearch

import (
	"image"
	"image/draw"
	"math/rand"
	"testing"

	"github.com/hypoactiv/imutil"
)

func TestCoordTransform(t *testing.T) {
	r := image.Rect(10, 20, 50, 60)
	ctx := objSearchContext{
		SearchRect: r,
	}
	// test that coords and offset are inverses
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			x0, y0 := ctx.coords(ctx.offset(x, y))
			if x != x0 || y != y0 {
				t.Fatal("coordinate transform error")
			}
		}
	}
	// check offset bounds
	if ctx.offset(10, 20) != 0 {
		t.Fatal("coordinate transform error")
	}
	if ctx.offset(49, 59) != r.Dx()*r.Dy()-1 {
		t.Fatal("coordinate transform error")
	}
}

func randomGrayImage(w, h int) (r *image.Gray) {
	r = image.NewGray(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			r.Pix[r.PixOffset(x, y)] = uint8(rand.Intn(256))
		}
	}
	return
}

func randomRGBImage(w, h int) (r *image.RGBA) {
	return imutil.CombineRGB(
		randomGrayImage(w, h),
		randomGrayImage(w, h),
		randomGrayImage(w, h),
	)
}

func TestFindHits(t *testing.T) {
	r := image.Rect(0, 0, 10, 10)
	ctx := objSearchContext{
		SearchRect: r,
		Tolerance:  0.2,
	}
	d := make([]float64, 100)
	for i := range d {
		d[i] = 1
	}
	d[ctx.offset(2, 2)] = 0.1
	h := ctx.findHits(d, 0, 2)
	if len(h) != 1 || h[0] != (Hit{image.Point{2, 2}, 0.05}) {
		t.Error(h)
		t.Fatal("findHits error")
	}
}

// generte random field and object images, place the object in the field,
// and test that objSearch can find it
func TestObjSearch(t *testing.T) {
	field := randomRGBImage(100, 100)
	object := randomRGBImage(10, 10)
	// partially obscured object
	draw.Draw(field, object.Bounds().Add(image.Point{20, 30}), object, image.ZP, draw.Src)
	// exact object (obscuring above)
	draw.Draw(field, object.Bounds().Add(image.Point{26, 36}), object, image.ZP, draw.Src)
	h := Search(field, object, field.Bounds(), 0.2, 10, nil, COLORMODE_GRAY, COMBINEMODE_MAX)
	if len(h) != 2 || h[0] != (Hit{image.Point{26, 36}, 0}) || h[1].P.X != 20 || h[1].P.Y != 30 {
		// expect exact hit at 26,36 and approx hit at 20,30, and no others
		t.Error(h)
		t.Fatal("objSearch error")
	}
}
