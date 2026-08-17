package novelty

import (
	"io"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Figure is a box of a stated height that the caller paints itself, the way out
// of the document's vocabulary and onto the canvas. It is a [piudoc.Drawer], so
// a story adds it like any other element.
//
// Paint is given the page, the frame it may use and the y of the box's top
// edge; y runs up the page, so the box is the band from yTop down to yTop-H.
// A figure never splits: if it does not fit, it moves whole to the next page.
type Figure struct {
	H     float64
	Paint func(c *canvas.Canvas, f piudoc.Frame, yTop float64)
}

// Draw places the box, moving to the next page if this one has no room left.
func (fig Figure) Draw(dst []canvas.Canvas, f piudoc.Frame, yTop float64) (adv int, yEnd float64, err error) {
	y := yTop
	if y-fig.H < f.Bottom && y < f.Top {
		adv++
		if adv >= len(dst) {
			return adv, y, io.ErrShortBuffer
		}
		y = f.Top
	}
	if fig.Paint != nil {
		fig.Paint(&dst[adv], f, y)
	}
	return adv, y - fig.H, nil
}
