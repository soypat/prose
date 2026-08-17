package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/soypat/piudf/piupage"
	"github.com/soypat/prose"
)

const name = "prospectus"

func BenchmarkGenerateProspectus(b *testing.B) {
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
	buf.Reset()
	run(&buf, results)
	os.WriteFile(name+".pdf", buf.Bytes(), 0777)
}

func run(w io.Writer, br testing.BenchmarkResult) error {
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
	d.Head = prose.RunningHead{
		Left:      "lneto, tinygo and beyond",
		Right:     "",
		SkipFirst: true,
		Rules:     true,
	}

	story(d)
	err = d.Write(w)
	if err != nil {
		return err
	}
	return nil
}

func story(d *prose.Doc) {
	k, th := d.Kit(), d.Theme()

	// Masthead: the one place the document raises its voice.
	d.Eyebrow("PATRICIO WHITTINGSLOW AND CO.").
		Title("prospectus").
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
}
