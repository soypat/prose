package md2pdf

import (
	"regexp"
	"strings"

	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
)

// The handlers below are conventions some documents keep and plain markdown
// does not. Each is a [Handler]; a converter keeps the ones its documents use.

// TOC sets the table of contents where a paragraph reads [TOC] and nothing else.
func TOC(r *Renderer, bs []Block, i int) int {
	b := &bs[i]
	if b.Kind != KindParagraph {
		return 0
	}
	switch strings.ToUpper(b.Text()) {
	case "[TOC]", "[[_TOC_]]":
		r.Add(r.Later(r.Contents))
		return 1
	}
	return 0
}

// Totals sets a paragraph of two strong lines and nothing else, a label over an
// amount, as a bar. Under a table it becomes the table's foot, held to its last
// row; anywhere else it stands on its own.
//
//	**TOTAL PRICE (EXCL. VAT)**
//	**USD 6,500.00**
func Totals(r *Renderer, bs []Block, i int) int {
	b := &bs[i]
	if !isTotal(b) {
		return 0
	}
	label := r.Plain(&Block{Kind: KindParagraph, Lines: b.Lines[:1]})
	bar := TotalBar(r, label, StripTags(b.Lines[1].Markup))
	if t, ok := r.Last().(*Table); ok && t.Foot == nil {
		t.Foot = bar
		return 1
	}
	r.Add(r.Kit.Stack(bar, r.Kit.Space(14)))
	return 1
}

// TotalBar is a label over an amount, reversed out of the ink.
func TotalBar(r *Renderer, label, amount string) prose.Elem {
	k, pal := r.Kit, r.Kit.Theme.Palette
	labelSt := prose.Style{Font: fontBold, Size: 8.4, Leading: 11, Color: pal.Paper}
	amountSt := prose.Style{Font: fontBold, Size: 14, Leading: 17, Color: pal.Paper}
	t := &piudoc.Table{
		ColWidths: []float64{0, 170},
		Rows: [][]piudoc.Cell{{
			{Drawer: k.Styled(Escape(strings.ToUpper(label)), labelSt)},
			{Drawer: k.Styled(Escape(strings.TrimSpace(amount)), amountSt)},
		}},
		Pad: prose.Padding{Left: 12, Right: 12, Top: 10, Bottom: 10},
	}
	t.Style.All().Background(pal.Ink).Valign(prose.Middle)
	t.Style.Col(1).Align(prose.Right)
	return t
}

// Terms sets a paragraph whose first line is strong, together with the blocks
// under it up to the next such paragraph, heading, table or rule, as one row of
// a ruled two-column grid: the label at the left and what it owns beside it.
// It is how a contract's conditions are read, by scanning the left column for
// the one in question. A label's own further lines are an address block, set as
// written.
//
//	**Payment terms:**
//
//	- 30% advance payment.
//	- 70% on completion.
func Terms(r *Renderer, bs []Block, i int) int {
	label := &bs[i]
	if !isLabel(label) {
		return 0
	}
	j := i + 1
	for j < len(bs) && owned(&bs[j]) {
		j++
	}
	k := r.Kit
	name := r.Plain(&Block{Kind: KindParagraph, Lines: label.Lines[:1]})
	var content []prose.Elem
	if len(label.Lines) > 1 {
		rest := Block{Kind: KindParagraph, Lines: append([]Line(nil), label.Lines[1:]...)}
		for n := range rest.Lines {
			rest.Lines[n].Hard = true
		}
		st := k.Theme.Scale.Body
		st.Align = prose.Left
		content = append(content, k.Styled(rest.Joined(), st))
	}
	if j > i+1 {
		content = append(content, r.Sub(bs[i+1:j], true))
	}
	if len(content) == 0 {
		// A label that owns nothing heads what follows it instead.
		r.Hold(k.Keep(k.Space(4), k.Sub(Escape(name))))
		return 1
	}
	r.Add(TermRow(r, name, k.Stack(content...)))
	return j - i
}

// TermRow is one row of a ruled grid: a label, and beside it what it owns.
func TermRow(r *Renderer, label string, content prose.Elem) prose.Elem {
	th := r.Kit.Theme
	pal := th.Palette
	// The label keeps the gap a paragraph leaves, so a label longer than what
	// it owns does not sit on the rule beneath it.
	labelSt := prose.Style{Font: fontBold, Size: 9, Leading: 13, Color: pal.Ink, SpaceAfter: th.Scale.Body.SpaceAfter}
	t := &piudoc.Table{
		ColWidths: []float64{150, 0},
		Rows: [][]piudoc.Cell{{
			{Drawer: r.Kit.Styled(Escape(label), labelSt)},
			{Drawer: content},
		}},
		Pad: prose.Padding{Right: 14, Top: 10},
	}
	t.Style.All().Valign(prose.Top)
	t.Style.Col(-1).Pad(prose.Padding{Top: 10})
	t.Style.Row(0).LineAbove(0.5, pal.Hair).LineBelow(0.5, pal.Hair)
	return t
}

var trailTag = regexp.MustCompile(`^(.*?)\s*\[([^\]]+)\]$`)

// Tags sets a third-level heading that ends in a bracket, a stage and its
// duration, with the bracket as a tag at the heading's right over a shared rule.
//
//	### Stage 1 – Survey [2 weeks]
func Tags(r *Renderer, bs []Block, i int) int {
	b := &bs[i]
	if b.Kind != KindHeading || b.Level != 3 {
		return 0
	}
	m := trailTag.FindStringSubmatch(r.Plain(b))
	if m == nil {
		return 0
	}
	k, th := r.Kit, r.Kit.Theme
	sub := th.Scale.Sub
	sub.SpaceAfter = 0
	t := &piudoc.Table{
		ColWidths: []float64{0, 130},
		Rows: [][]piudoc.Cell{{
			{Drawer: k.Styled(Escape(m[1]), sub)},
			{Drawer: k.Styled(Escape(strings.ToUpper(m[2])), th.Scale.Eyebrow)},
		}},
		Pad: prose.Padding{Bottom: 6},
	}
	t.Style.All().Valign(prose.Bottom)
	t.Style.Col(1).Align(prose.Right)
	t.Style.Row(0).LineBelow(0.6, th.Palette.Hair)
	r.Hold(k.Keep(k.Space(th.Rhythm.SubBefore+8), r.Entry(3, "", m[1]), k.Bookmark(m[1], 2), t, k.Space(8)))
	return 1
}

// isTotal is two strong lines and nothing else: a label over an amount.
func isTotal(b *Block) bool { return b.AllBold() && len(b.Lines) == 2 }

// isLabel is a paragraph whose first line is strong and whose rest, if any, is
// not: a term and, optionally, its definition.
func isLabel(b *Block) bool {
	if b.Kind != KindParagraph || len(b.Lines) == 0 || !b.Lines[0].Bold || isTotal(b) {
		return false
	}
	for _, ln := range b.Lines[1:] {
		if ln.Bold {
			return false
		}
	}
	return true
}

// owned reports whether b belongs to the label above it.
func owned(b *Block) bool {
	switch b.Kind {
	case KindParagraph:
		return !isLabel(b) && !isTotal(b)
	case KindList, KindCode, KindQuote:
		return true
	}
	return false
}
