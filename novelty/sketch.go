// Package novelty draws the figures a document makes an exception for.
//
// Nothing here is typography. A prose document sets its text in a scale and a
// palette it does not argue with; these are the drawings that sit inside it,
// and they are allowed a voice of their own.
package novelty

import (
	"image/color"
	"math"
	"math/rand/v2"

	canvas "github.com/soypat/piudf/piupage"
)

// maxSamples bounds the wander of one segment, so a sketch needs no heap of its
// own. At the default wave that is a segment about nine inches long.
const maxSamples = 96

// Sketch draws in the xkcd manner: a straight run of the pen is replaced by one
// that leaves the ideal line and comes back, so the result reads as drawn by a
// hand that meant to be straight and was not.
//
// The wander is a random walk taken along the segment and tapered to nothing at
// each end, which keeps joins exact -- three axes meeting at an origin still
// meet there, and a corner never shows a gap.
//
// The seed is the caller's. A document redrawn from the same seed is the same
// document byte for byte, which is what keeps a PDF diff and a golden test
// worth reading; a wobble reseeded from the clock would make every rendering
// differ from every other rendering of itself.
//
// The zero Sketch is usable and seeded 0. Append strokes with [Sketch.Segment]
// and [Sketch.Arrow], then paint them all with one [Sketch.Stroke].
type Sketch struct {
	// Amp is how far the pen may stray from the line, in points. 0 means 1.6,
	// which is about right against a 2pt pen.
	Amp float64
	// Wave is the distance between two draws of the wandering hand. 0 means 9;
	// smaller is jittery, larger is lazy.
	Wave float64

	rng  *rand.Rand
	path canvas.Path
}

// NewSketch returns a sketch whose wobble is decided entirely by seed.
func NewSketch(seed uint64) *Sketch {
	s := &Sketch{}
	s.Seed(seed)
	return s
}

// Seed restarts the wandering hand, so that a figure drawn twice from the same
// seed is the same figure.
func (s *Sketch) Seed(seed uint64) {
	s.rng = rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
}

// Path is the sketch's accumulated strokes, for a caller that would rather fill
// or clip them than hand them to [Sketch.Stroke].
func (s *Sketch) Path() *canvas.Path { return &s.path }

// Bind gives the sketch caller-owned buffers to record its path into, after
// which it allocates nothing. ops holds one byte per curve and pts six floats;
// a segment costs about one of each per [Sketch.Wave] of its length.
func (s *Sketch) Bind(ops []byte, pts []float64) { s.path.Reset(ops, pts) }

func (s *Sketch) amp() float64 {
	if s.Amp > 0 {
		return s.Amp
	}
	return 1.6
}

func (s *Sketch) wave() float64 {
	if s.Wave > 0 {
		return s.Wave
	}
	return 9
}

// Segment appends a hand-drawn (x0, y0) -> (x1, y1) to the sketch's path. The
// path is not painted until [Sketch.Stroke], so a figure of many strokes stays
// one path and one run of PDF operators.
func (s *Sketch) Segment(x0, y0, x1, y1 float64) {
	s.segment(x0, y0, x1, y1, 1)
}

// segment draws with its wander scaled by k, which the arrowheads turn down: a
// 9pt barb wobbled like a 120pt axis would not read as a barb.
func (s *Sketch) segment(x0, y0, x1, y1, k float64) {
	if s.rng == nil {
		s.Seed(0)
	}
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	// The unit vector along the segment, and the one at right angles to it that
	// the wander is measured on.
	ux, uy := dx/length, dy/length
	nx, ny := -uy, ux

	n := min(max(int(length/s.wave())+1, 3), maxSamples)

	var px, py [maxSamples + 1]float64
	amp := s.amp() * k
	var off float64
	for i := 0; i <= n; i++ {
		// A random walk, held inside the amplitude and then tapered by a half
		// sine so both ends land exactly on the ideal endpoints.
		off = min(max(off+(s.rng.Float64()*2-1)*amp*0.75, -amp), amp)
		t := float64(i) / float64(n)
		w := off * math.Sin(math.Pi*t)
		px[i] = x0 + ux*length*t + nx*w
		py[i] = y0 + uy*length*t + ny*w
	}

	// Through the samples rather than at them: each is the control of a
	// quadratic ending halfway to the next, which rounds the walk's corners off
	// without pulling the curve away from it.
	s.path.MoveTo(px[0], py[0])
	for i := 1; i < n; i++ {
		s.path.QuadTo(px[i], py[i], (px[i]+px[i+1])/2, (py[i]+py[i+1])/2)
	}
	s.path.LineTo(px[n], py[n])
}

// Arrow appends a segment ending in a two-barb arrowhead at (x1, y1). A head of
// 0 means 9 points.
func (s *Sketch) Arrow(x0, y0, x1, y1, head float64) {
	s.Segment(x0, y0, x1, y1)
	if head <= 0 {
		head = 9
	}
	// Back down the segment, opened out either side of it.
	ang := math.Atan2(y0-y1, x0-x1)
	const spread = 26 * math.Pi / 180
	for _, a := range [2]float64{ang - spread, ang + spread} {
		s.segment(x1, y1, x1+head*math.Cos(a), y1+head*math.Sin(a), 0.35)
	}
}

// Stroke paints everything appended since the last one and empties the path. A
// width of 0 means 2 points.
func (s *Sketch) Stroke(c *canvas.Canvas, col color.Color, width float64) {
	if width <= 0 {
		width = 2
	}
	// Round caps and joins are most of the look: a hand-drawn line has no
	// mitred corner and does not end on a square edge.
	c.Stroke(&s.path, canvas.Pen{
		Color: col, Width: width,
		Cap: canvas.RoundCap, Join: canvas.RoundJoin,
	})
	s.path.Clear()
}

// Iso projects a right-handed (x, y, z) onto the page: x runs to the lower
// right, y to the lower left and z straight up, the three 120 degrees apart,
// which is the isometric an axis tripod is read in. The result is in points,
// measured from the origin of the axes.
func Iso(x, y, z float64) (px, py float64) {
	const cos30 = 0.86602540378443865
	return (x - y) * cos30, z - (x+y)/2
}
