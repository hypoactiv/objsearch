package objsearch

import (
	"image"
)

// A detected occurance of the object image in the field.
type Hit struct {
	// pixel location
	P image.Point
	// score
	S float64
}

// Returns the larger of the X- and Y-distances between Hits p and q
func (p Hit) Distance(q Hit) int {
	dx := p.P.X - q.P.X
	dy := p.P.Y - q.P.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}
