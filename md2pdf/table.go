package md2pdf

import (
	"image/color"
	"io"
	"regexp"
	"strings"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
)

// Table is a table drawn a row at a time, so that one broken across pages
// carries its header onto each, and holds what comes before and after it to its
// first and last rows: a heading never ends a page above its table, nor does a
// total begin one below it.
type Table struct {
	// Lead is drawn above the header and held to the first row. The renderer
	// sets it to whatever was held for the table, a heading most often.
	Lead prose.Elem
	// Head is drawn above the first row and again atop every page the table
	// continues on. A headless table has none.
	Head prose.Elem
	Rows []prose.Elem
	// Foot is drawn under the last row and held to it: a total, say.
	Foot prose.Elem
	// After is the space left under the table.
	After float64
}

func (t *Table) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (adv int, y float64, err error) {
	y = yTop
	height := func(es []prose.Elem) (h float64, err error) {
		for _, e := range es {
			if e == nil {
				continue
			}
			eh, err := piudoc.Measure(dst[adv:], e, f, y-h)
			if err != nil {
				return 0, err
			}
			h += eh
		}
		return h, nil
	}
	draw := func(es []prose.Elem) error {
		for _, e := range es {
			if e == nil {
				continue
			}
			n, yn, err := e.Draw(dst[adv:], f, y)
			adv, y = adv+n, yn
			if err != nil {
				return err
			}
		}
		return nil
	}
	n := len(t.Rows)
	for i := 0; i < max(n, 1); i++ {
		var group []prose.Elem
		if i == 0 {
			group = append(group, t.Lead, t.Head)
		}
		if i < n {
			group = append(group, t.Rows[i])
		}
		if i >= n-1 {
			group = append(group, t.Foot)
		}
		h, err := height(group)
		if err != nil {
			return adv, y, err
		}
		if y-h < f.Bottom && y < f.Top {
			adv++
			if adv >= len(dst) {
				return adv, y, io.ErrShortBuffer
			}
			y = f.Top
			if i > 0 && t.Head != nil {
				group = append([]prose.Elem{t.Head}, group...)
			}
		}
		if err := draw(group); err != nil {
			return adv, y, err
		}
	}
	return adv, y - t.After, nil
}

// table sets a table with a rule under its header and hairlines between its
// rows. A table whose header cells are all empty is headless, which is how
// markdown spells a grid of label and value.
func (r *Renderer) table(b *Block) {
	k, th := r.Kit, r.Kit.Theme
	ncol := len(b.Align)
	if ncol == 0 || len(b.Rows) == 0 {
		return
	}
	rows := b.Rows
	headless := true
	for _, c := range rows[0] {
		if strings.TrimSpace(StripTags(c)) != "" {
			headless = false
		}
	}
	cellSt, headSt := th.Scale.Cell, th.Scale.CellHead
	if ncol >= 5 {
		// A wide table trades a point of size for a column that does not wrap.
		cellSt.Size, cellSt.Leading = 8.4, 12
	}
	aligns := make([]prose.Align, ncol)
	for j := range aligns {
		switch b.Align[j] {
		case AlignCenter:
			aligns[j] = prose.Center
		case AlignRight:
			aligns[j] = prose.Right
		case AlignLeft:
			aligns[j] = prose.Left
		default:
			if numericColumn(rows[1:], j) {
				aligns[j] = prose.Right
			}
		}
	}
	const padX, padY = 6, 6
	widths := colWidths(rows, headless, headSt, cellSt, th.Measure(), 2*padX)

	mk := func(cells []string, st prose.Style, rule float64, ruleCol color.Color) prose.Elem {
		row := make([]piudoc.Cell, ncol)
		for j := range row {
			row[j].Drawer = k.Styled(cells[j], st)
		}
		t := &piudoc.Table{ColWidths: widths, Rows: [][]piudoc.Cell{row},
			Pad: prose.Padding{Left: padX, Right: padX, Top: padY, Bottom: padY}}
		t.Style.All().Valign(prose.Top)
		// The outer columns sit flush with the measure, as the text above does.
		t.Style.Col(0).Pad(prose.Padding{Right: padX, Top: padY, Bottom: padY})
		if ncol > 1 {
			t.Style.Col(-1).Pad(prose.Padding{Left: padX, Top: padY, Bottom: padY})
		}
		for j, a := range aligns {
			t.Style.Col(j).Align(a)
		}
		if rule > 0 {
			t.Style.Row(0).LineBelow(rule, ruleCol)
		}
		return t
	}
	tb := &Table{After: 14}
	if !headless {
		head := mk(rows[0], headSt, 0.9, th.Palette.Ink).(*piudoc.Table)
		// Headers of one line and of three all stand on the rule.
		head.Style.All().Valign(prose.Bottom)
		tb.Head = head
	}
	body := rows[1:]
	for i, row := range body {
		rule, col := th.Hairline, th.Palette.Hair
		if i == len(body)-1 {
			rule, col = 0.9, th.Palette.Ink // The table's close, which a total sits under.
		}
		tb.Rows = append(tb.Rows, mk(row, cellSt, rule, col))
	}
	r.Add(tb)
}

var numericCell = regexp.MustCompile(`^[\d.,\s]+$`)

// numericColumn reports whether most of a column's cells are amounts, which
// are right-aligned so their places line up.
func numericColumn(rows [][]string, j int) bool {
	num, filled := 0, 0
	for _, r := range rows {
		s := strings.TrimSpace(StripTags(r[j]))
		if s == "" {
			continue
		}
		filled++
		s = strings.NewReplacer("U$S", "", "US$", "", "USD", "", "$", "", "%", "", "€", "").Replace(s)
		if numericCell.MatchString(s) && strings.ContainsAny(s, "0123456789") {
			num++
		}
	}
	return num > 0 && 2*num >= filled
}

// colWidths shares the measure among columns the way a browser lays out an
// automatic table: each gets at least its longest word, and what is left goes
// to the columns in proportion to how much more they would take to set their
// longest cell unbroken. Text is measured in Helvetica, which the faces a
// document embeds instead are near enough to for sharing out a measure.
func colWidths(rows [][]string, headless bool, headSt, cellSt prose.Style, measure, pad float64) []float64 {
	n := len(rows[0])
	lo, hi := make([]float64, n), make([]float64, n)
	for i, row := range rows {
		if i == 0 && headless {
			continue
		}
		st := cellSt
		if i == 0 {
			st = headSt
		}
		for j, c := range row {
			face := piupage.FontHelvetica
			if i == 0 || strings.HasPrefix(strings.TrimSpace(c), "<b>") {
				face = piupage.FontHelveticaBold
			}
			txt := StripTags(c)
			hi[j] = max(hi[j], piupage.StringWidth(face, txt, st.Size))
			for _, w := range strings.Fields(txt) {
				lo[j] = max(lo[j], piupage.StringWidth(face, w, st.Size))
			}
		}
	}
	var sumLo, sumHi float64
	for j := range lo {
		lo[j] += pad + 1
		hi[j] += pad + 1
		sumLo += lo[j]
		sumHi += hi[j]
	}
	w := make([]float64, n)
	switch {
	case sumHi <= measure:
		// Everything fits on one line; the slack goes where the text is.
		for j := range w {
			w[j] = hi[j] + (measure-sumHi)*hi[j]/sumHi
		}
	case sumLo >= measure:
		for j := range w {
			w[j] = lo[j] * measure / sumLo
		}
	default:
		share := (measure - sumLo) / (sumHi - sumLo)
		for j := range w {
			w[j] = lo[j] + (hi[j]-lo[j])*share
		}
	}
	return w
}
