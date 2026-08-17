package prose

import (
	"image/color"

	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Columns places elements side by side. A zero width takes an equal share of
// whatever the fixed ones leave.
func (k *Kit) Columns(widths []float64, cells ...Elem) Elem {
	return k.bld.Columns(piudoc.ColStyle{Widths: widths, Valign: Top}, cells...)
}

// Card is two lines that belong together: a label over an address, or a figure
// over what it measures.
type Card struct{ Top, Bottom string }

// CardStyle is how a row of cards is set. It covers both a contact strip and a
// figures band, which differ only in which of the two lines carries the weight.
type CardStyle struct {
	// Widths are the column widths; a zero entry takes an equal share of the rest.
	Widths []float64
	// TopStyle and BottomStyle are the two lines' own. A zero style means
	// Scale.CellHead above and Scale.Small below.
	TopStyle, BottomStyle Style
	Gap                   float64 // between the two lines
	Pad                   Padding
	Background            color.Color
	RuleAbove, RuleBelow  float64
	// Inset is extra left padding on the first card, so that a banded row's
	// content stands off the band's edge rather than sitting flush against it.
	Inset float64
}

// Cards sets a row of two-line columns.
func (k *Kit) Cards(cs CardStyle, cards ...Card) Elem {
	top, bottom := cs.TopStyle, cs.BottomStyle
	if top.Size == 0 {
		top = k.Theme.Scale.CellHead
	}
	if bottom.Size == 0 {
		bottom = k.Theme.Scale.Small
	}
	row := make([]piudoc.Cell, len(cards))
	for i, c := range cards {
		row[i] = k.bld.Cell(k.bld.KeepTogether(
			k.styled(c.Top, top),
			k.Space(cs.Gap),
			k.styled(c.Bottom, bottom),
		))
	}
	t := &piudoc.Table{ColWidths: cs.Widths, Pad: cs.Pad, Rows: [][]piudoc.Cell{row}}
	t.Style.All().Valign(Top)
	if cs.Background != nil {
		t.Style.All().Background(cs.Background)
	}
	if cs.RuleAbove > 0 {
		t.Style.Row(0).LineAbove(cs.RuleAbove, k.Theme.Palette.Hair)
	}
	if cs.RuleBelow > 0 {
		t.Style.Row(-1).LineBelow(cs.RuleBelow, k.Theme.Palette.Hair)
	}
	if cs.Inset > 0 {
		pad := cs.Pad
		pad.Left += cs.Inset
		t.Style.Col(0).Pad(pad)
	}
	return t
}

// Table accumulates rows and is finished with [Table.Done]. Its methods chain.
type Table struct {
	kit    *Kit
	widths []float64
	rows   [][]piudoc.Cell
	style  piudoc.TableStyle
	pad    Padding
	// head records whether the first row is a header, which is where the
	// hairlines between body rows begin.
	head bool
}

// Table begins a table on a fixed column grid. A zero width takes whatever the
// fixed columns leave, and no widths at all splits the frame evenly.
func (k *Kit) Table(widths ...float64) *Table {
	t := &Table{kit: k, widths: widths, pad: k.Theme.Pad}
	t.style.All().Valign(Top)
	return t
}

// Head sets the header row, in the theme's header style with a rule beneath it.
func (t *Table) Head(cells ...string) *Table {
	st := t.kit.Theme.Scale.CellHead
	row := make([]piudoc.Cell, len(cells))
	for i, c := range cells {
		row[i] = t.kit.bld.Cell(t.kit.styled(c, st))
	}
	t.rows = append([][]piudoc.Cell{row}, t.rows...)
	t.head = true
	pad := t.pad
	pad.Top = 0
	t.style.Row(0).Pad(pad).LineBelow(0.9, t.kit.Theme.Palette.Ink)
	return t
}

// Row appends a row of markup cells in the theme's cell style.
func (t *Table) Row(cells ...string) *Table {
	st := t.kit.Theme.Scale.Cell
	row := make([]Elem, len(cells))
	for i, c := range cells {
		row[i] = t.kit.styled(c, st)
	}
	return t.RowOf(row...)
}

// RowOf appends a row of elements the caller built, for cells that nest.
func (t *Table) RowOf(cells ...Elem) *Table {
	t.rows = append(t.rows, t.kit.bld.Row(cells...))
	return t
}

// Pad overrides the theme's cell padding for this table.
func (t *Table) Pad(p Padding) *Table { t.pad = p; return t }

// Band fills every cell with col.
func (t *Table) Band(col color.Color) *Table { t.style.All().Background(col); return t }

// Hairlines rules below every body row but the last.
func (t *Table) Hairlines() *Table {
	first := 0
	if t.head {
		first = 1
	}
	t.style.Range(0, first, -1, -2).LineBelow(t.kit.Theme.Hairline, t.kit.Theme.Palette.Hair)
	return t
}

// Align sets one column's horizontal alignment; a negative col counts from the
// last.
func (t *Table) Align(col int, a Align) *Table { t.style.Col(col).Align(a); return t }

// Valign sets every cell's vertical alignment.
func (t *Table) Valign(v VAlign) *Table { t.style.All().Valign(v); return t }

// Style is the piudoc style underneath, for rules and grounds this package does
// not name.
func (t *Table) Style() *piudoc.TableStyle { return &t.style }

// Done finishes the table into an element.
func (t *Table) Done() Elem {
	return &piudoc.Table{
		ColWidths: t.widths,
		Rows:      t.rows,
		Style:     t.style,
		Pad:       t.pad,
	}
}
