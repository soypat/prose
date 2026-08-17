package prose

import (
	"strings"
	"testing"
	"testing/fstest"
)

// shape is never called. It exists so the compiler checks that the API can
// actually express the document it was extracted from: the masthead, a contact
// strip, a numbered section holding its first paragraph, a figures band, a code
// block, a list, a ruled table and the closing panel.
//
// If a signature here stops compiling, the API lost something the prospectus needs.
func shape(d *Doc) {
	k, th := d.Kit(), d.Theme()

	// Masthead: the one place the document raises its voice.
	d.Eyebrow("EMBEDDED SYSTEMS CONTRACTING").
		Title("Patricio Whittingslow").
		Space(9).
		Add(k.ShortRule(58, 2.4, th.Palette.Accent)).
		Space(16).
		Lead("I build the layers most projects assume already exist…").
		Body(`I am the creator of <a href="https://github.com/soypat/lneto">lneto</a>, the only…`)

	// The ruled band of practical facts, three columns of label over text.
	col := (th.Measure() - 2*14) / 3
	d.Add(k.Cards(CardStyle{
		Widths:      []float64{col, col, col + 28},
		TopStyle:    th.Scale.CellHead,
		BottomStyle: th.Scale.Small,
		Gap:         3,
		Pad:         th.Pad,
		RuleAbove:   0.8,
		RuleBelow:   0.8,
	},
		Card{"CONTACT", `<a href="mailto:graded.sp@gmail.com">graded.sp@gmail.com</a>`},
		Card{"BASED IN", "Buenos Aires, Argentina.<br/>Remote…"},
		Card{"AVAILABILITY", "Currently taking contract work."},
	))

	// A section holds its first element on the same page as its heading.
	d.Section("01", "What I do",
		k.Keep(k.H3("Device drivers."), k.Body("SPI, RMII/MDIO, SDIO…")))
	d.Add(k.Keep(k.H3("Storage and boot."), k.Body("Filesystems on block devices…")))

	// The figures band: value over label, the same shape as the contact strip
	// with the weight on the other line.
	q := th.Measure() / 4
	d.Add(k.Cards(CardStyle{
		Widths:      []float64{q, q, q, q},
		TopStyle:    th.Scale.Figure,
		BottomStyle: th.Scale.Caption,
		Gap:         2,
		Background:  th.Palette.Band,
	},
		Card{"2.3 kB", "the full async stack struct excluding buffers"},
		Card{"~177 ns", "ARP exchange, 0 alloc/op"},
	))

	d.Code("type StackNode interface {\n    Demux(carrierData []byte, frameOffset int) error\n}")
	d.Subsection("It took three attempts to get there")

	// Two columns, the year in the margin and the account beside it.
	d.Add(k.Table(62, 0).
		Row("2021", `<b>ether-swtch.</b> An Encode/Decode frame interface…`).
		Row("2023", `<b>seqs.</b> Could finally send packets autonomously…`).
		Hairlines().
		Valign(Top).
		Done())

	// A header that rules off from its body.
	d.Add(k.Table(112, 0).
		Head("PROJECT", "WHAT IT IS").
		Row(`<a href="https://github.com/soypat/lneto">lneto</a>`, "Userspace networking stack.").
		Hairlines().
		Done())

	d.Bullets(
		"<b>Heapless by default, and I can prove it.</b> No allocations in hot paths…",
		"<b>Runtime-free.</b> You supply time, memory and threading…",
	)
	d.Note("I will be straight about maturity, because you will find this out anyway…")
	d.Add(k.Keep(k.Space(18), k.Panel("If you have a device that needs to talk to a network…")))
}

// The theme is a value a caller may amend without restating it: overriding two
// styles must leave the other thirteen resolved.
func TestThemeIsAParameter(t *testing.T) {
	th := DefaultTheme()
	th.Palette.Accent = th.Palette.Ink
	th.Scale.Body.Size = 11
	th.fill()

	if th.Scale.H2.Size == 0 {
		t.Error("overriding one style left another unresolved")
	}
	if th.Measure() != th.Page.W-th.Margins.Left-th.Margins.Right {
		t.Error("the measure does not follow the margins")
	}
}

// A document is built from an fs.FS, so faces may be embedded, on disk, or
// synthesized by a test.
func TestFontsComeFromAnFS(t *testing.T) {
	// A face that will not parse is refused where it is loaded, not where it
	// would have failed to draw.
	fsys := fstest.MapFS{"fonts/Text-Regular.ttf": {Data: []byte("not a font")}}
	_, err := New(DefaultTheme(), Fonts{FS: fsys, Dir: "fonts", Text: []string{"Text-Regular"}})
	if err == nil {
		t.Fatal("a corrupt face loaded without complaint")
	}
	// A face that is not there at all names the file it wanted.
	_, err = New(DefaultTheme(), Fonts{FS: fsys, Dir: "fonts", Text: []string{"Absent"}})
	if err == nil || !strings.Contains(err.Error(), "Absent") {
		t.Fatalf("a missing face gave %v, which does not name it", err)
	}
}

var _ = shape
