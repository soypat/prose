package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
	"github.com/soypat/prose/novelty"
)

const name = "prospectus"

//go:embed gc-25.png
var portraitPNG []byte

// portrait is the screened photograph. Screening is the expensive half of a
// halftone and the drawing is the cheap one, so it is done once here rather
// than inside the benchmark's loop, where it would measure the dither instead
// of the document.
var portrait = sync.OnceValue(func() *novelty.Halftone {
	img, err := png.Decode(bytes.NewReader(portraitPNG))
	if err != nil {
		panic(err)
	}
	return novelty.Screen(img, novelty.Screening{Cols: 200, Gamma: 0.8})
})

func TestGenerateProspectus(t *testing.T) {
	var buf bytes.Buffer
	results := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			err := run(&buf, testing.BenchmarkResult{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	results.Extra["size"] = float64(buf.Len())
	buf.Reset()
	run(&buf, results)
	os.WriteFile(name+".pdf", buf.Bytes(), 0666)
}

func run(w io.Writer, results testing.BenchmarkResult) error {
	// Both families are bound to standard-14 faces, which every reader carries.
	// Passing a zero Fonts binds the same two; naming them says so out loud.
	d, err := prose.New(prose.DefaultTheme(), prose.Fonts{
		TextFamily: prose.Builtin(piupage.FontHelvetica),
		MonoFamily: prose.Builtin(piupage.FontCourier),
	})
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	d.Meta = prose.Meta{
		Title:   name,
		Author:  "Pato Whittingslow",
		Subject: "Prospectus -- Software Engineering like it's 1969",
		Creator: "soypat/piudf",
		Lang:    "en",
		Date:    time.Now(),
	}
	// A page of halftone is thousands of dots of content stream, which the
	// dozen kilobytes a page of text lives in will not hold.
	d.Budget = prose.Budget{MaxPages: 8, BytesPerPage: 512 << 10, EncScratch: 16 << 10}
	d.Head = prose.RunningHead{
		Left:      "lneto, tinygo and beyond",
		Right:     "",
		SkipFirst: true,
		Rules:     true,
	}

	story(d, results)
	err = d.Write(w)
	if err != nil {
		return err
	}
	return nil
}

func story(d *prose.Doc, br testing.BenchmarkResult) {
	const kB = 1 << 10
	k, th := d.Kit(), d.Theme()
	size := int(br.Extra["size"])
	// Masthead: the one place the document raises its voice.
	d.Eyebrow("LNETO TEAM").
		Title("prospectus - Lneto, embedded and beyond").
		Space(9).
		Add(k.ShortRule(58, 2.4, th.Palette.Accent)).
		Space(9).
		Code(fmt.Sprintf(`This %dkB PDF1.7-compliant was generated in %s using a %dkB heap (N=%d)`, size/kB, time.Duration(br.NsPerOp()).Truncate(time.Microsecond), br.AllocedBytesPerOp()/kB, br.N)).
		Space(16).
		Lead("Smaller deployable binaries; and the highest quality libraries in the Go ecosystem. This document will detail why companies prefer my libraries over tried and tested Google infrastructure.")

	// The tripod beside the claim it stands for. A zero width takes what the
	// fixed column leaves, so the text is told nothing about the figure.
	const axesW = 200
	d.Add(k.Columns([]float64{0, axesW},
		k.Stack(
			k.H2("Axes of our work"),
			k.Bullet(`<b>Networking</b>: <a href="https://github.com/soypat/lneto">Lneto</a> is a userspace networking stack`+
				` used in small and large places. Minimal hardware requirements.`),
			k.Bullet(`<b>Hardware</b>: We drive the design of Go's embedded space.`+
				` See <a href="https://github.com/tinygo-org/pio">pio</a>.`),
			k.Bullet(`<b>Infrastructure</b>: Methodology of building the high quality tools early on.`),
		),
		axes(th, axesW)))

	d.Add(
		k.H2("Funding our work"),
		k.Stack(
			k.Bullet(""),
		),
	)

	const pw, gutter = 150, 16
	h := portrait()
	h.Color = th.Palette.Ink
	d.H2("About Me")
	d.Add(k.Columns([]float64{pw + gutter, 0},
		novelty.Figure{H: h.Height(pw), Paint: func(c *piupage.Canvas, f piudoc.Frame, yTop float64) {
			h.Paint(c, f.X, yTop, pw)
		}},
		k.Body("I build the layers most projects assume already exist. Drivers that talk to real silicon, a networking stack that allocates nothing in its hot path, and the filesystems underneath both. The photograph to the left is thirteen thousand circles and no image data; the same is true of everything else here.")))

	// The ruled band of practical facts, three columns of label over text.
	col := (th.Measure() - 2*14) / 3
	d.H3("About the PDF generator").
		Add(k.Cards(prose.CardStyle{
			Widths:      []float64{col, col, col + 28},
			TopStyle:    th.Scale.CellHead,
			BottomStyle: th.Scale.Small,
			Gap:         3,
			Pad:         th.Pad,
			RuleAbove:   0.8,
			RuleBelow:   0.8,
		},
			prose.Card{Top: "THIS PROGRAM", Bottom: `<a href="https://github.com/soypat/prose/tree/main/examples/prospectus">soypat/prose/examples/prospectus</a>`},
			prose.Card{Top: "BUILT ON", Bottom: `<a href="https://github.com/soypat/piudf">soypat/piudf/piupage/piudoc</a>`},
			// prose.Card{Top: "FONTS", Bottom: "Standard-14 by default.<br/>Embedded subsets on request."},
		))

}

// axes is three arrows off one origin, drawn by a hand that meant them to be
// straight. The document is set in a scale it does not argue with; a figure
// inside it is allowed a voice of its own.
//
// w is the width it is to be drawn in, which is a column's rather than the
// page's now that it shares a row with text. The arms are cut to leave room for
// the labels that hang off their tips, since those are what the figure is
// actually about.
func axes(th prose.Theme, w float64) prose.Elem {
	const (
		size  = 10 // the label
		lead  = 18 // the air a label needs past a tip
		label = 64 // and the room the longest of them takes
	)
	// The tripod spans 2cos30 arms across and 1.5 down.
	arm := min((w-2*label)/1.7320508, 96)
	height := 1.5*arm + 2*lead
	return novelty.Figure{H: height, Paint: func(c *piupage.Canvas, f piudoc.Frame, yTop float64) {
		// A wider stray and a shorter wave than the default: at this length the
		// hand has room to wander and the arms should say so.
		s := &novelty.Sketch{Amp: 2.6, Wave: 7}
		s.Seed(7)
		// The origin sits a label clear of the bottom of the box, so the two
		// arms that fall have somewhere to fall to.
		ox, oy := f.X+f.Width/2, yTop-lead-arm

		var tip [3][2]float64
		tip[0][0], tip[0][1] = novelty.Iso(arm, 0, 0) // x, to the lower right
		tip[1][0], tip[1][1] = novelty.Iso(0, arm, 0) // y, to the lower left
		tip[2][0], tip[2][1] = novelty.Iso(0, 0, arm) // z, straight up
		for _, t := range tip {
			s.Arrow(ox, oy, ox+t[0], oy+t[1], 0)
		}
		s.Stroke(c, th.Palette.Ink, 1.8)

		// Each name hangs off its own tip, so a longer arm carries it along.
		const size = 10
		face := piupage.FontHelveticaOblique
		c.SetFont(face, size)
		const (
			titleX = "Hardware"
			titleY = "Networking"
			titleZ = "Efficiency"
		)
		c.Text(ox+tip[0][0]+7, oy+tip[0][1]-size*0.35, titleX, th.Palette.Ink)
		c.TextRight(ox+tip[1][0]-7, oy+tip[1][1]-size*0.35, titleY, th.Palette.Ink)
		c.Text(ox+tip[2][0]-piupage.StringWidth(face, titleZ, size)/2, oy+tip[2][1]+9, titleZ, th.Palette.Ink)
	}}
}
