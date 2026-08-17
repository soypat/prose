package novelty

import (
	"image"
	"image/color"
	"math"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Dither is the error-diffusion filter an image is screened with. The zero
// value is [Stucki].
//
// Every one of these takes the error a cell makes by being rounded to ink or to
// paper and hands it to the cells not yet decided, so that a run of cells
// averages out to the tone the image asked for. They differ in how far they
// spread it: the wider the filter, the less the eye finds a pattern in the
// result and the softer the edges get.
type Dither uint8

const (
	// Stucki spreads the error over twelve neighbours in a five-wide filter.
	// It is the one to reach for on a photograph: wide enough that no texture
	// of its own survives in flat tone, and cheaper than [Jarvis] for a result
	// that is a shade crisper.
	Stucki Dither = iota
	// Jarvis is the twelve-tap filter Stucki was a refinement of. It is the
	// softest of these, and the safest on an image with banded skies.
	Jarvis
	// FloydSteinberg spreads over four neighbours. It is the fastest and the
	// sharpest, and the likeliest to show its own diagonal weave in flat tone.
	FloydSteinberg
	// Atkinson passes on only three quarters of the error, which throws away
	// tone at both ends and gives the airy, blown-out look of a 1984 Macintosh.
	Atkinson
)

// tap is one neighbour a cell's error reaches, weighted out of the filter's
// divisor. dx is mirrored on the rows that are scanned right to left.
type tap struct {
	dx, dy int8
	w      int16
}

func (d Dither) filter() (taps []tap, div int32) {
	switch d {
	case Jarvis:
		return []tap{
			{1, 0, 7}, {2, 0, 5},
			{-2, 1, 3}, {-1, 1, 5}, {0, 1, 7}, {1, 1, 5}, {2, 1, 3},
			{-2, 2, 1}, {-1, 2, 3}, {0, 2, 5}, {1, 2, 3}, {2, 2, 1},
		}, 48
	case FloydSteinberg:
		return []tap{{1, 0, 7}, {-1, 1, 3}, {0, 1, 5}, {1, 1, 1}}, 16
	case Atkinson:
		return []tap{{1, 0, 1}, {2, 0, 1}, {-1, 1, 1}, {0, 1, 1}, {1, 1, 1}, {0, 2, 1}}, 8
	default: // Stucki
		return []tap{
			{1, 0, 8}, {2, 0, 4},
			{-2, 1, 2}, {-1, 1, 4}, {0, 1, 8}, {1, 1, 4}, {2, 1, 2},
			{-2, 2, 1}, {-1, 2, 2}, {0, 2, 4}, {1, 2, 2}, {2, 2, 1},
		}, 42
	}
}

// Screening is how an image is reduced to dots.
type Screening struct {
	// Cols is the width of the screen in cells, which is the whole of its
	// resolution: the drawn size decides how big a cell lands on the page, not
	// how many there are. 0 means 128. A portrait at 96 reads as a portrait
	// across a page's measure; past about 200 the dots stop being visible as
	// dots, which is usually the point of drawing one this way.
	Cols int
	// Dither is the filter, [Stucki] by default.
	Dither Dither
	// Gamma bends the tone before screening: above 1 lightens, below 1
	// darkens. 0 and 1 both mean no bend.
	Gamma float64
	// Invert screens the negative, for a light image on a dark ground.
	Invert bool
}

// Halftone is an image reduced to one ink dot per cell, ready to be drawn at
// any size. The screening is done once, when it is made; drawing it only emits
// the dots that survived, so a document may draw one many times for what the
// last one cost.
//
// The zero Halftone draws nothing.
type Halftone struct {
	// Dot is the diameter of a dot as a multiple of the cell it sits in. 0
	// means 1.15, a shade over the 1.128 at which a square grid of circles
	// first covers its ground completely -- at exactly 1 a solid black area
	// would come out 79% covered and read as grey.
	Dot float64
	// Color is the ink. nil means black.
	Color color.Color

	cols, rows int
	dots       int
	on         []bool
}

// Screen reduces img to dots. It is the expensive half and is meant to be done
// once, at the time a document is built rather than each time it is drawn.
func Screen(img image.Image, s Screening) *Halftone {
	cols := s.Cols
	if cols <= 0 {
		cols = 128
	}
	b := img.Bounds()
	if b.Empty() {
		return &Halftone{}
	}
	rows := int(math.Round(float64(cols) * float64(b.Dy()) / float64(b.Dx())))
	rows = max(rows, 1)

	h := &Halftone{cols: cols, rows: rows}
	tone := resample(img, cols, rows, s.Gamma, s.Invert)
	h.on, h.dots = diffuse(tone, cols, rows, s.Dither)
	return h
}

// Cols and Rows are the screen's size in cells, and Dots how many of them
// carry ink -- which is very nearly the cost of drawing it.
func (h *Halftone) Cols() int { return h.cols }
func (h *Halftone) Rows() int { return h.rows }
func (h *Halftone) Dots() int { return h.dots }

// Height is what the halftone stands at when drawn w wide.
func (h *Halftone) Height(w float64) float64 {
	if h.cols == 0 {
		return 0
	}
	return w * float64(h.rows) / float64(h.cols)
}

// Paint draws the halftone w wide with its top left corner at (x, yTop).
//
// Every dot is one zero-length subpath, which PDF paints under a round cap as a
// filled circle. That costs about twenty bytes a dot where the four Béziers of
// an honest circle cost a hundred and fifty, and it lets the whole screen go
// down as a single path and a single stroke.
func (h *Halftone) Paint(c *canvas.Canvas, x, yTop, w float64) {
	if h.dots == 0 {
		return
	}
	cell := w / float64(h.cols)
	dot := h.Dot
	if dot <= 0 {
		dot = 1.15
	}
	ink := color.Color(color.Black)
	if h.Color != nil {
		ink = h.Color
	}

	var p canvas.Path
	p.Reset(make([]byte, 0, 2*h.dots), make([]float64, 0, 4*h.dots))
	for row := range h.rows {
		// Rounded to the hundredth of a point: the shortest form that round
		// trips is what gets written, and no reader can see the difference
		// between a dot placed here and one placed a thousandth away.
		cy := round2(yTop - (float64(row)+0.5)*cell)
		for col := range h.cols {
			if !h.on[row*h.cols+col] {
				continue
			}
			cx := round2(x + (float64(col)+0.5)*cell)
			p.MoveTo(cx, cy)
			p.LineTo(cx, cy)
		}
	}
	c.Stroke(&p, canvas.Pen{Color: ink, Width: dot * cell, Cap: canvas.RoundCap})
}

// Draw fills the frame's width, so a halftone is added to a story like any
// other element. It never splits across pages.
func (h *Halftone) Draw(dst []canvas.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	return Figure{
		H: h.Height(f.Width),
		Paint: func(c *canvas.Canvas, f piudoc.Frame, y float64) {
			h.Paint(c, f.X, y, f.Width)
		},
	}.Draw(dst, f, yTop)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// srgb8 is the linear light a byte of sRGB stands for. Tone has to be averaged
// in linear light or a cell that is half white and half black comes out at the
// 0.5 of the encoding, which is the 0.21 of the light -- the mistake that makes
// a downscaled image darker than the one it came from.
var srgb8 = func() (t [256]float32) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = float32(c / 12.92)
		} else {
			t[i] = float32(math.Pow((c+0.055)/1.055, 2.4))
		}
	}
	return t
}()

// resample box-filters img down onto the cell grid, in linear light, and
// returns the tone of each cell with 1 white and 0 black. Every source pixel
// lands in exactly one cell, so no detail is skipped over the way point
// sampling would skip it.
func resample(img image.Image, cols, rows int, gamma float64, invert bool) []float32 {
	b := img.Bounds()
	sum := make([]float32, cols*rows)
	n := make([]int32, cols*rows)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		gy := (y - b.Min.Y) * rows / b.Dy()
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			// Rec. 709 luminance, the weights the eye actually gives them.
			l := 0.2126*srgb8[r>>8] + 0.7152*srgb8[g>>8] + 0.0722*srgb8[bl>>8]
			i := gy*cols + (x-b.Min.X)*cols/b.Dx()
			sum[i] += l
			n[i]++
		}
	}
	for i := range sum {
		if n[i] > 0 {
			sum[i] /= float32(n[i])
		}
		// Back out of linear light before the tone is screened. The averaging
		// had to happen in light, but the dithering must not: coverage taken
		// straight off the light would ask 79% ink for the mid grey a reader
		// calls half way, and the photograph would come out a silhouette.
		sum[i] = encode(sum[i])
		if invert {
			sum[i] = 1 - sum[i]
		}
		if gamma > 0 && gamma != 1 {
			sum[i] = float32(math.Pow(float64(sum[i]), 1/gamma))
		}
	}
	return sum
}

// encode is the sRGB transfer function, linear light back to the value that
// stands for it.
func encode(l float32) float32 {
	switch {
	case l <= 0:
		return 0
	case l >= 1:
		return 1
	case l <= 0.0031308:
		return 12.92 * l
	}
	return float32(1.055*math.Pow(float64(l), 1/2.4) - 0.055)
}

// diffuse rounds every cell to ink or paper and pushes what that rounding got
// wrong into the cells ahead of it.
//
// The scan is serpentine -- left to right, then right to left. A filter run the
// same way down every row walks its own error across the image and lays down
// the diagonal worms that give error diffusion its bad name; turning round at
// the end of each row leaves the error nowhere to march.
func diffuse(tone []float32, cols, rows int, d Dither) (on []bool, dots int) {
	taps, div := d.filter()
	on = make([]bool, cols*rows)
	for row := range rows {
		rev := row%2 == 1
		for i := range cols {
			col := i
			if rev {
				col = cols - 1 - i
			}
			at := row*cols + col
			v := tone[at]
			// Ink is the dark half, and the error is what the cell owed the
			// image after being rounded to one or the other.
			var q float32
			if v >= 0.5 {
				q = 1
			} else {
				on[at] = true
				dots++
			}
			err := v - q
			for _, t := range taps {
				dx := int(t.dx)
				if rev {
					dx = -dx
				}
				nc, nr := col+dx, row+int(t.dy)
				if nc < 0 || nc >= cols || nr >= rows {
					// The error that falls off the edge is dropped. Wrapping it
					// round would print the left margin's mistakes down the
					// right one.
					continue
				}
				tone[nr*cols+nc] += err * float32(t.w) / float32(div)
			}
		}
	}
	return on, dots
}
