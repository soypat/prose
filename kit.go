package prose

import (
	"fmt"
	"image/color"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Elem is one piece of a document.
type Elem = piudoc.Drawer

// Kit makes elements in a theme. It holds the one builder a document parses
// through and the faces its text is checked against; every constructor returns
// a finished element and appends nothing.
//
// Text arrives as markup — <b>, <i>, <a href>, <br/> — because a document's own
// words are formatting its author wrote. Data that must not be read as markup
// goes through [Kit.Literal].
type Kit struct {
	Theme Theme

	bld piudoc.Builder
	// faces is the Regular of every registered family, by family name, which is
	// what a rune is checked against.
	faces map[string]canvas.Font
	// missing collects runes no face could draw. See Kit.check.
	missing []rune
}

// NewKit loads f into a kit set in th, resolving any style th left zero.
func NewKit(th Theme, f Fonts) (*Kit, error) {
	th.fill()
	k := &Kit{Theme: th, faces: make(map[string]canvas.Font, 2)}
	for _, fam := range []struct {
		name  string
		files []string
		given piudoc.Family
		std   canvas.FontBuiltin
	}{
		{FamilyText, f.Text, f.TextFamily, canvas.FontHelvetica},
		{FamilyMono, f.Mono, f.MonoFamily, canvas.FontCourier},
	} {
		bound := fam.given
		if bound.Regular == nil && len(fam.files) > 0 {
			loaded, err := loadFamily(f.FS, f.Dir, fam.files...)
			if err != nil {
				return nil, err
			}
			bound = loaded
		}
		// Every family is bound, so no style falls through to a face the theme
		// never named and the glyph guard is never quietly off.
		if bound.Regular == nil {
			bound = Builtin(fam.std)
		}
		k.bld.SetFamily(fam.name, bound)
		k.faces[fam.name] = bound.Regular
	}
	return k, k.bld.Err()
}

// Err reports the first fault the kit met: a tag the dialect does not carry, or
// a rune no registered face can draw. A .notdef box in a document sent to a
// client is invisible while writing and unmissable on arrival, so a document
// that needs one is refused rather than written.
func (k *Kit) Err() error {
	if err := k.bld.Err(); err != nil {
		return err
	}
	if len(k.missing) != 0 {
		return fmt.Errorf("prose: text needs glyphs the faces lack: %q", k.missing)
	}
	return nil
}

// Builder is the piudoc builder underneath, for elements this package does not
// name. Text built through it is not glyph-checked.
func (k *Kit) Builder() *piudoc.Builder { return &k.bld }

// check records any rune the style's face cannot draw. It runs as text goes in
// rather than over the finished story, which no longer says what it holds.
func (k *Kit) check(text string, st Style) {
	f := k.faces[baseFamily(st.Font)]
	if f == nil {
		return // An unregistered family draws in a standard-14 face.
	}
	for _, r := range text {
		switch r {
		case '\n', '\t', '\r':
			continue // Layout, never a glyph.
		}
		if f.GlyphID(r) != 0 {
			continue
		}
		if !containsRune(k.missing, r) {
			k.missing = append(k.missing, r)
		}
	}
}

func containsRune(rs []rune, r rune) bool {
	for _, v := range rs {
		if v == r {
			return true
		}
	}
	return false
}

// styled is the door every markup constructor goes through.
func (k *Kit) styled(markup string, st Style) Elem {
	k.check(stripTags(markup), st)
	return k.bld.P(markup, st)
}

// Text elements. Each takes markup and is set in the style its name asks for.

func (k *Kit) Eyebrow(markup string) Elem { return k.styled(markup, k.Theme.Scale.Eyebrow) }
func (k *Kit) Title(markup string) Elem   { return k.styled(markup, k.Theme.Scale.Title) }
func (k *Kit) Lead(markup string) Elem    { return k.styled(markup, k.Theme.Scale.Lead) }
func (k *Kit) Body(markup string) Elem    { return k.styled(markup, k.Theme.Scale.Body) }
func (k *Kit) Small(markup string) Elem   { return k.styled(markup, k.Theme.Scale.Small) }
func (k *Kit) H2(markup string) Elem      { return k.styled(markup, k.Theme.Scale.H2) }
func (k *Kit) H3(markup string) Elem      { return k.styled(markup, k.Theme.Scale.H3) }
func (k *Kit) Sub(markup string) Elem     { return k.styled(markup, k.Theme.Scale.Sub) }

// Styled sets markup in a style of the caller's own.
func (k *Kit) Styled(markup string, st Style) Elem { return k.styled(markup, st) }

// Literal sets text with no tag read and no entity resolved, which is the door
// data takes.
func (k *Kit) Literal(text string, st Style) Elem {
	k.check(text, st)
	return k.bld.Text(text, st)
}

// Rule is a hairline across the measure, in the palette's rule colour.
func (k *Kit) Rule(thickness float64) Elem {
	return piudoc.HRule{Thickness: thickness, Color: k.Theme.Palette.Hair}
}

// ShortRule is a rule of a given width and colour, the kind that sits under a
// masthead rather than between two blocks.
func (k *Kit) ShortRule(width, thickness float64, col color.Color) Elem {
	return piudoc.HRule{Width: width, Thickness: thickness, Color: col}
}

// Space is vertical blank.
func (k *Kit) Space(h float64) Elem { return piudoc.Spacer{H: h} }

// Bullet is one list item, marked and indented by the theme.
func (k *Kit) Bullet(markup string) Elem {
	return k.bld.Bullet(k.Theme.Bullet, k.Body(markup))
}

// Note is a set-off caveat: the same words as body text on a tinted ground, so
// a reader skimming for the catch finds it.
func (k *Kit) Note(markup string) Elem {
	st := k.Theme.Scale.Body
	st.SpaceAfter = 0
	return k.ground(k.styled(markup, st), k.Theme.Palette.Band, k.Theme.Rhythm.PanelPad, nil, 0)
}

// Panel is the ask, reversed out of the ink so the eye lands on it whether or
// not the reader read what came before in order.
func (k *Kit) Panel(markup string) Elem {
	return k.ground(k.styled(markup, k.Theme.Scale.Panel), k.Theme.Palette.Ink,
		Padding{Left: 22, Right: 22, Top: 20, Bottom: 20}, nil, 0)
}

// Code is source on a tinted ground with an accent rule down its left edge. Its
// lines are drawn as written and never re-wrapped.
func (k *Kit) Code(src string) Elem {
	st := k.Theme.Scale.Code
	k.check(src, st)
	r := k.Theme.Rhythm
	return k.ground(k.bld.Preformatted(src, st), k.Theme.Palette.CodeBg, r.CodePad,
		k.Theme.Palette.Accent, r.CodeRule)
}

// ground sets one element on a coloured field, optionally ruled down its left
// edge. It is a one-cell table, which is what piudoc gives a block a ground with.
func (k *Kit) ground(e Elem, bg color.Color, pad Padding, rule color.Color, ruleW float64) Elem {
	t := &piudoc.Table{Pad: pad, Rows: [][]piudoc.Cell{{k.bld.Cell(e)}}}
	sel := t.Style.All().Background(bg)
	if rule != nil && ruleW > 0 {
		sel.LineLeft(ruleW, rule)
	}
	return t
}

// Section opens a numbered division: a rule, the number set in the accent, the
// title, and first held on the same page as its heading — a heading stranded at
// the foot of a page being the thing that holding together prevents.
func (k *Kit) Section(num, title string, first Elem) Elem {
	r := k.Theme.Rhythm
	return k.bld.KeepTogether(
		k.Space(r.SectionBefore),
		piudoc.HRule{Thickness: 0.9, Color: k.Theme.Palette.Hair, SpaceAfter: r.SectionRule},
		k.bld.Bookmark(num+" "+title, 1),
		k.styled(`<font color="`+hex(k.Theme.Palette.Accent)+`">`+num+"</font>   "+title, k.Theme.Scale.H2),
		k.Space(r.SectionAfter),
		first,
	)
}

// Subsection is a minor heading inside a division.
func (k *Kit) Subsection(title string) Elem {
	return k.bld.KeepTogether(k.Space(k.Theme.Rhythm.SubBefore), k.Sub(title))
}

// Keep draws its elements one above another and will not break between them.
func (k *Kit) Keep(elems ...Elem) Elem { return k.bld.KeepTogether(elems...) }

// Stack draws its elements one above another, breaking where it must.
func (k *Kit) Stack(elems ...Elem) Elem { return k.bld.Stack(elems...) }

// Bookmark names a place in the reader's navigation pane. It draws nothing.
func (k *Kit) Bookmark(title string, level int) Elem { return k.bld.Bookmark(title, level) }
