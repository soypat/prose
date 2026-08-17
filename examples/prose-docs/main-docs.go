package main

import (
	"fmt"
	"log"
	"time"

	"github.com/soypat/piudf/piupage"
	"github.com/soypat/prose"
)

const name = "prose-docs"

func main() {
	start := time.Now()
	base := time.Since(start)
	err := run()
	elapsed := time.Since(start) - base // subtract the time it takes to measure time.
	if err != nil {
		log.Fatalln("failed to create", name, elapsed, err)
	}
	log.Println(name, "created in", elapsed.String())
}

func run() error {
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
		Title:   name + " — a themed document layer",
		Author:  "soypat",
		Subject: "Building PDFs with piudf without holding typography in the generator.",
		Creator: "piudf/piupage",
		Lang:    "en",
		Date:    time.Now(),
	}
	d.Head = prose.RunningHead{
		Left:      "prose — a themed document layer",
		Right:     "github.com/soypat/prose",
		SkipFirst: false,
		Rules:     true,
	}

	story(d)
	filename := name + ".pdf"
	n, err := d.WriteFile(filename)
	if err != nil {
		return err
	}
	log.Printf("wrote %s (%.0f kB)", filename, float64(n)/1000)
	return nil
}

func story(d *prose.Doc) {
	k, th := d.Kit(), d.Theme()

	// Masthead: the one place the document raises its voice.
	d.Eyebrow("A THEMED DOCUMENT LAYER OVER PIUDF").
		Title("prose").
		Space(9).
		Add(k.ShortRule(58, 2.4, th.Palette.Accent)).
		Space(16).
		Lead("A generator should hold its content and not its typography. prose keeps the palette, the type scale and the furniture a long document needs, so what you write is the document.").
		Body(`Every element it makes is a <b>piudoc.Drawer</b>, so anything prose does not name can still be built with piudoc directly and passed to <b>Doc.Add</b>. Nothing here is a wall: the builder underneath is reachable, and so is the table style.`)

	// The ruled band of practical facts, three columns of label over text.
	col := (th.Measure() - 2*14) / 3
	d.Add(k.Cards(prose.CardStyle{
		Widths:      []float64{col, col, col + 28},
		TopStyle:    th.Scale.CellHead,
		BottomStyle: th.Scale.Small,
		Gap:         3,
		Pad:         th.Pad,
		RuleAbove:   0.8,
		RuleBelow:   0.8,
	},
		prose.Card{Top: "IMPORT", Bottom: `<a href="https://github.com/soypat/prose">github.com/soypat/prose</a>`},
		prose.Card{Top: "BUILT ON", Bottom: `<a href="https://github.com/soypat/piudf">piudf/piupage/piudoc</a>`},
		prose.Card{Top: "FONTS", Bottom: "Standard-14 by default.<br/>Embedded subsets on request."},
	))

	d.Section("01", "Two receivers, one vocabulary",
		k.Keep(k.H3("Doc appends."), k.Body("Its methods chain and return the document, so a story reads top to bottom in the order it is set.")))
	d.Add(k.Keep(k.H3("Kit constructs."), k.Body("The same names on the kit return an element instead, for the pieces that nest inside a card, a cell or a group held together.")))

	// The figures band: value over label, the same shape as the contact strip
	// with the weight on the other line.
	q := th.Measure() / 4
	d.Add(k.Cards(prose.CardStyle{
		Widths:      []float64{q, q, q, q},
		TopStyle:    th.Scale.Figure,
		BottomStyle: th.Scale.Caption,
		Gap:         2,
		Background:  th.Palette.Band,
		Pad:         th.Pad,
		Inset:       10,
	},
		prose.Card{Top: "471 pt", Bottom: "the measure DefaultTheme leaves"},
		prose.Card{Top: "14", Bottom: "standard fonts, none embedded"},
		prose.Card{Top: "3", Bottom: "ways to bind a family"},
		prose.Card{Top: "0", Bottom: "files this document reads"},
	))

	d.Section("02", "Fonts",
		k.Body("A family is bound by the first of three sources that is given: a family you already hold, the .ttf files named in Fonts, or its standard-14 counterpart. A theme is therefore drawable before any font file exists."))

	d.Code(`d, err := prose.New(prose.DefaultTheme(), prose.Fonts{
    TextFamily: prose.Builtin(canvas.FontHelvetica),
    MonoFamily: prose.Builtin(canvas.FontCourier),
})
d.Title("prose").Body("A generator should hold its content.")`)

	// Held together, so the header row cannot be orphaned from its body: piudoc
	// does not repeat a header across a break.
	d.Add(k.Keep(
		k.Subsection("What each source is for"),
		k.Table(112, 0).
			Head("SOURCE", "WHEN YOU REACH FOR IT").
			Row("<b>TextFamily</b>", "A face loaded elsewhere, or a standard-14 face other than the default. <b>Builtin</b> names one in a line.").
			Row("<b>Text</b>", "Faces read out of an fs.FS — an embed.FS, a directory, or a test's own map.").
			Row("<i>neither</i>", "Helvetica for text and Courier for mono, embedding nothing.").
			Hairlines().
			Valign(prose.Top).
			Done(),
	))

	d.Section("03", "What the theme decides",
		k.Body("Restyling touches one value. The scale resolves whatever a caller leaves zero, so overriding two styles leaves the other thirteen alone."))

	d.Bullets(
		"<b>A palette and a scale.</b> One style per role the document has — body, lead, headings, cells, code, captions — each filled from the palette when it is left zero.",
		"<b>Furniture that knows the page.</b> Running heads, folios and rules are drawn per page from a single RunningHead value.",
		"<b>Groups that will not break.</b> A section holds its heading with the element that follows it, which is the thing a stranded heading at the foot of a page exists to prevent.",
	)

	d.Note("Text arrives as markup — <b>&lt;b&gt;</b>, <b>&lt;i&gt;</b>, <b>&lt;a href&gt;</b>, <b>&lt;br/&gt;</b> — because a document's own words are formatting its author wrote. Data that must not be read as markup goes through <b>Kit.Literal</b>.")

	d.Add(k.Keep(k.Space(18), k.Panel(`A document that needs a glyph its faces cannot draw is refused rather than written, so the fault reaches the author and never a reader. See <a href="https://github.com/soypat/prose">github.com/soypat/prose</a>.`)))
}
