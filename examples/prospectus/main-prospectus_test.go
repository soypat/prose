package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
	"github.com/soypat/prose/novelty"
)

const name = "prospectus"
const orgName = "Working Title Collective"

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
	// One run before the benchmark. A document prose refuses — a glyph the faces
	// cannot draw, a tag the dialect does not carry — fails here saying so, where
	// the same fault inside b.Loop is a b.Fatal that leaves a zero
	// BenchmarkResult, whose nil Extra then panics over the top of the message.
	if err := run(&buf, testing.BenchmarkResult{}); err != nil {
		t.Fatal(err)
	}
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
		Left:      "Lneto, embedded and beyond",
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
	const conferenceLink = "https://www.youtube.com/playlist?list=PLx6oFeWGZv8mW4p6dhHFlAWRGH1IGXWDk"
	const kB = 1 << 10
	k, th := d.Kit(), d.Theme()
	size := int(br.Extra["size"])
	// Masthead: the one place the document raises its voice.
	d.Eyebrow(strings.ToUpper(orgName)).
		Title("Prospectus - Lneto, embedded and beyond").
		Space(9).
		Add(k.ShortRule(58, 2.4, th.Palette.Accent)).
		Space(9).
		Lead("Smaller deployable binaries and the highest-quality libraries in the Go ecosystem. This document will detail why companies prefer our libraries over tried-and-tested Google infrastructure.")
	d.Space(-6)
	d.Code(fmt.Sprintf(`This %dkB PDF 1.7-compliant document was generated in %s using a %dkB heap (N=%d)
Content written by a human. Proofread by an LLM.`, size/kB, time.Duration(br.NsPerOp()).Truncate(time.Microsecond), br.AllocedBytesPerOp()/kB, br.N))
	d.Space(9)
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
			k.Bullet(`<b>Infrastructure</b>: Methodology of building high-quality tools early on.`+
				` We like our tools.`),
		),
		axes(th, axesW)))

	d.Add(
		k.H2("Funding our work"),
		k.Body("The way you choose to fund us, or a particular project, will depend on the urgency and scope of the work."),
		k.H3(`Sponsorship`),
		k.Body(`Sponsorships are great to establish an initial professional tie.`+
			` You can expect to get around an hour of attention on your issues per US$50 spent as long as it is on open source projects related to `+orgName+"."),
		k.H3(`Contracting`),
		k.Body(`Contracting allows us to get a little more creative with defining work and how it is compensated.
We've worked with clients on labor-hour project contracts and are happy to chat about what would best fit
your organization's needs. Expect engagements in ~US$8'000 range for 4-8 weeks.`),
		k.Panel(`Write to Pato at <a href="mailto:graded.sp@gmail.com">graded.sp@gmail.com</a>. 
 What you are building and what stands in the way of your success?`),
		k.Space(9),
	)

	d.Add(
		k.H2("Our Work"),
		k.Body(`All libraries are transparent. We expose every single layer of the stack:`+
			` open-source with no vendor lock-in. It is a core principle of all the work we do.`+
			` Also note libraries meant to run on microcontrollers have been designed to be heapless at runtime e.g: Lneto.`),
		k.H3(`<a href="https://github.com/soypat/lneto">Lneto</a> - Userspace Networking Stack`),
		k.Body(`Lneto today lets hundreds of Gophers to deploy networking solutions to <a href="https://github.com/tinygo-org/espradio">constrained</a> and <a href="https://github.com/usbarmory/go-net">not-so-constrained targets</a>.`+
			` Lneto is also becoming the preferred alternative to GVisor due to its transparent stack, smaller binary size and maintenance story.`),
		k.H3(`<a href="https://github.com/tinygo-org/pio/blob/main/rp2-pio/piolib/parallel.go">pio</a> - JIT compiler for PIO assembly and driver library`),
		k.Body(`This is here to show we are no strangers to the most hardcore hardware programming problems, or to delivering on idiomatic APIs that are better than the usual "idiomatic Go" solution.`),
		k.H3(`<a href="https://github.com/soypat/embedpb">embedpb</a> - Enable compiling protobuf on TinyGo`),
		k.Body(`This is a in progress tool we are creating for <a href="https://netbird.io">NetBird</a>. It enables compiling their SSH client with TinyGo, achieving 8MB WASM binary size down from 62MB. Getting there required fixing two bugs in the TinyGo compiler.`),
		k.H3(`<a href="https://github.com/soypat/gsdf">gsdf</a> - CAD design library`),
		k.Body(`An extremely flexible 3D/2D design library that can fully leverage GPU compute.`+
			` Has been used to draw PCB copper layers ~180× faster than ANSYS Sherlock.`),
		k.H3(`<a href="https://github.com/soypat/lneto/tree/main/http/httphi">httphi</a> - Heapless HTTP/1.1 router+mux`),
		k.Body(`Part of Lneto ecosystem. Enabling reliable HTTP servers on 10 dollar hardware.`),
	)
	third := th.Measure() / 3
	d.Add(
		k.H2(`Lneto Roadmap`),
		k.Body(`Lneto is a highly valued project by users and clients. Below is a list of missing or incomplete features we want to add to Lneto.`),
		k.Bullet("<b>TLS 1.3</b>: Adding TLS support and lightweight+constrained crypto for microcontrollers. Go standard library feature-completion is a non-goal."),
		k.Bullet("<b>Gigabit TCP:</b> The stack was designed for low throughput and low memory consumption. We'll add a new type of StackNode abstraction to support high-throughput targets."),
		k.Cards(prose.CardStyle{
			Widths:      []float64{third, third, third},
			TopStyle:    th.Scale.Figure,
			BottomStyle: th.Scale.Caption,
			Gap:         2,
			Background:  th.Palette.Band,
			Pad:         th.Pad,
			Inset:       10,
		},
			prose.Card{Top: "1", Bottom: "Lneto required amount of goroutines to run"},
			prose.Card{Top: "87%", Bottom: "Lneto WASM Binary reduction compared to GVisor"},
			prose.Card{Top: "0B", Bottom: "Lneto runtime heap allocations"},
		),
	)

	// The same photograph twice over: screened to circles on the left, and on
	// the right its own PNG bytes handed to the reader untouched.
	// photo, err := piupage.PNG(portraitPNG)
	// if err != nil {
	// 	panic(err)
	// }
	// const photoW = 150
	// photoH := photoW * float64(photo.H) / float64(photo.W)
	// d.Add(novelty.Figure{H: photoH, Paint: func(c *piupage.Canvas, f piudoc.Frame, yTop float64) {
	// 	c.Image(photo, f.X+(f.Width-photoW)/2, yTop-photoH, photoW, photoH)
	// }})

	const pw, gutter = 150, 16
	h := portrait()
	h.Color = th.Palette.Ink
	d.H2("About Us")
	d.Space(5)
	d.Add(d.Kit().ShortRule(1, 1, k.Theme.Palette.Accent))

	d.Add(k.Columns([]float64{pw + gutter, 0},
		k.Captioned(pw, novelty.Figure{H: h.Height(pw), Paint: func(c *piupage.Canvas, f piudoc.Frame, yTop float64) {
			h.Paint(c, f.X, yTop, f.Width)
		}}, fmt.Sprintf(`%d circles of Pato speaking at <a href="%s">GopherCon 2024</a>`, h.Dots(), conferenceLink)),
		k.Stack(
			k.Body(`
We are `+orgName+`, a small group of programmers out of Buenos Aires, Argentina.
Pato Whittingslow answers for the work.`),
			k.Body(`Pato has 7 years of Go experience in unlikely places:
 rocket-engine test-bench firmware, space-hardened PCB simulation, bioprocessor cell-augmentation pipelines.`),
		),
	),
	)
	d.Space(9)
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
		s := &novelty.Sketch{Amp: 2.6, Wave: 7}
		s.Seed(7)
		ox, oy := f.X+f.Width/2, yTop-lead-arm

		var tip [3][2]float64
		tip[0][0], tip[0][1] = novelty.Iso(arm, 0, 0) // x, to the lower right
		tip[1][0], tip[1][1] = novelty.Iso(0, arm, 0) // y, to the lower left
		tip[2][0], tip[2][1] = novelty.Iso(0, 0, arm) // z, straight up
		for _, t := range tip {
			s.Arrow(ox, oy, ox+t[0], oy+t[1], 0)
		}
		s.Stroke(c, th.Palette.Ink, 1.8)

		const size = 10
		face := piupage.FontHelveticaOblique
		c.SetFont(face, size)
		const (
			titleX = "Hardware"
			titleY = "Networking"
			titleZ = "Infrastructure"
		)
		c.Text(ox+tip[0][0]+7, oy+tip[0][1]-size*0.35, titleX, th.Palette.Ink)
		c.TextRight(ox+tip[1][0]-7, oy+tip[1][1]-size*0.35, titleY, th.Palette.Ink)
		c.Text(ox+tip[2][0]-piupage.StringWidth(face, titleZ, size)/2, oy+tip[2][1]+9, titleZ, th.Palette.Ink)
	}}
}
