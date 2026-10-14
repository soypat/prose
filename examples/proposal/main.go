// Command proposal sets a markdown proposal as a PDF: a cover carrying the
// title and the offer's particulars, then each top-level division on pages of
// its own. Markdown is read with github.com/soypat/ohimark and set with prose.
//
//	go run ./examples/proposal -i local/proposal/proposal.md
//
// The document's shape is read from the markdown's own conventions:
//
//   - Paragraphs before the first "#" heading are the covering letter.
//   - The first "#" heading is the title, a "##" right under it the subtitle,
//     and a headerless two-column table after those the offer's particulars.
//   - Every later "#" heading opens a division on a new page; "## 1. Name" is a
//     numbered section and "### Name [tag]" a heading with the tag set right.
//   - Two strong lines on their own, a label over an amount, are a total; under
//     a table they become its foot.
//   - A strong first line followed by text or a list is a term and what it
//     owns, set as one row of a ruled two-column grid.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soypat/piudf/piupage"
	"github.com/soypat/prose"
)

func main() {
	in := flag.String("i", "local/proposal/proposal.md", "markdown proposal to set")
	out := flag.String("o", "", "PDF to write; defaults to the input's name with a .pdf extension")
	org := flag.String("org", "Indgrade", "organisation the proposal is from")
	logoPath := flag.String("logo", "", "logo for the cover, PNG or JPEG; defaults to logo.* or <org>.* beside the markdown")
	flag.Parse()
	if *out == "" {
		*out = strings.TrimSuffix(*in, filepath.Ext(*in)) + ".pdf"
	}
	start := time.Now()
	n, err := run(*in, *out, *org, *logoPath)
	if err != nil {
		log.Fatalln("proposal:", err)
	}
	log.Printf("wrote %s (%.0f kB) in %s", *out, float64(n)/1000, time.Since(start).Round(time.Millisecond))
}

// Palette is a navy that reads as engineering rather than marketing, with a
// steel blue to point with and an amber kept for the cover alone.
func palette() prose.Palette {
	return prose.Palette{
		Ink:    piupage.HexColor("#0f2133"),
		Body:   piupage.HexColor("#2a3441"),
		Muted:  piupage.HexColor("#67727e"),
		Hair:   piupage.HexColor("#d3dae1"),
		Accent: piupage.HexColor("#1c5d91"),
		Band:   piupage.HexColor("#eef2f6"),
		CodeBg: piupage.HexColor("#f3f5f8"),
		Paper:  piupage.HexColor("#ffffff"),
		Link:   piupage.HexColor("#9fc6e6"),
	}
}

// run sets the proposal twice: the first pass learns which page each section
// lands on, and the second writes those pages into the contents.
func run(in, out, org, logoPath string) (int64, error) {
	src, err := os.ReadFile(in)
	if err != nil {
		return 0, err
	}
	if logoPath == "" {
		logoPath = findLogo(filepath.Dir(in), org)
	}
	var lg *logo
	if logoPath != "" {
		if lg, err = loadLogo(logoPath, palette().Ink); err != nil {
			return 0, err
		}
	}
	var known []int
	for pass := 0; ; pass++ {
		d, toc, err := build(in, src, org, lg, known)
		if err != nil {
			return 0, err
		}
		if pass > 0 {
			return d.WriteFile(out)
		}
		if err = d.Write(io.Discard); err != nil {
			return 0, err
		}
		known = make([]int, len(toc))
		for i, e := range toc {
			known[i] = *e.page
		}
	}
}

func build(in string, src []byte, org string, lg *logo, known []int) (*prose.Doc, []tocEntry, error) {
	th := prose.DefaultTheme()
	// The scale was resolved against the default palette; zeroing it has the
	// kit resolve it again against this one.
	th.Palette, th.Scale, th.Bullet = palette(), prose.Scale{}, prose.BulletStyle{}
	th.Margins = prose.Margins{Left: 64, Right: 64, Top: 64, Bottom: 68}

	d, err := prose.New(th, prose.Fonts{})
	if err != nil {
		return nil, nil, err
	}
	th = d.Theme()
	blocks, err := parseMarkdown(in, src, hexColor(th.Palette.Accent))
	if err != nil {
		return nil, nil, err
	}
	c, rest := splitCover(blocks)
	title := "Propuesta"
	if c.title != "" {
		title = plain(&block{kind: bPara, lines: []line{{markup: c.title}}})
	}
	subtitle := ""
	if c.subtitle != "" {
		subtitle = plain(&block{kind: bPara, lines: []line{{markup: c.subtitle}}})
	}

	d.Meta = prose.Meta{
		Title:   strings.TrimSuffix(title+" — "+subtitle, " — "),
		Author:  org,
		Subject: subtitle,
		Creator: "github.com/soypat/prose",
		Lang:    "es",
		Date:    time.Now(),
	}
	d.Budget = prose.Budget{MaxPages: 64, BytesPerPage: 96 << 10, EncScratch: 16 << 10}
	d.Head = prose.RunningHead{
		Left:      strings.ToUpper(org) + "  ·  " + strings.TrimSuffix(title+" — "+subtitle, " — "),
		Right:     reference(c.meta),
		SkipFirst: true,
		Rules:     true,
		Folio:     func(num, total int) string { return fmt.Sprintf("Página %d de %d", num, total) },
	}

	l := &layout{k: d.Kit(), th: th, dir: filepath.Dir(in), hi: piupage.HexColor("#f2a900"),
		pages: new(pageFinder)}
	l.add(anchor{l.pages})
	toc := -1
	if c.title != "" {
		toc = l.cover(c, org, lg, title, subtitle)
	}
	l.blocks(rest)
	if l.err != nil {
		return nil, nil, l.err
	}
	if toc >= 0 && len(l.toc) > 0 {
		// The contents is on the first page and lists what comes after it, so
		// its place was kept until the rest had been laid out.
		l.pieces[toc].e = l.termRow("Contenido", l.contents(known))
	}
	d.Add(l.story()...)
	return d, l.toc, nil
}

// logoWidth is how wide the logo is set on the cover, in points.
const logoWidth = 150

// logo is the cover's mark.
type logo struct {
	img  prose.Image
	w, h int
}

// height is the logo's height when set w wide.
func (lg *logo) height(w float64) float64 { return w * float64(lg.h) / float64(lg.w) }

// findLogo looks beside the markdown for a logo.* or <org>.* image.
func findLogo(dir, org string) string {
	for _, name := range []string{"logo", strings.ToLower(org)} {
		for _, ext := range []string{".png", ".jpg", ".jpeg"} {
			p := filepath.Join(dir, name+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// loadLogo reads a logo for the cover. A logo drawn on an opaque ground is
// recoloured so that its ground becomes to, the colour of the band it sits on,
// and it reads as printed on the band rather than pasted over it.
func loadLogo(name string, to color.Color) (*logo, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	px, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	r := px.Bounds()
	lg := &logo{w: r.Dx(), h: r.Dy()}
	// The corner is the ground if it is opaque: a logo with a transparent
	// ground has none to match, and goes in as it stands.
	ground := color.NRGBAModel.Convert(px.At(r.Min.X, r.Min.Y)).(color.NRGBA)
	if ground.A != 0xff {
		lg.img, err = prose.ReadImage(b)
		return lg, err
	}
	lg.img = recolor(px, ground, to)
	return lg, nil
}

// recolor shifts every pixel of src in HSV by what takes from to to: the hue
// rotated, the saturation scaled, and the value remapped so that from's lands
// on to's while black and white stay where they are. The ground becomes to
// exactly, white marks stay white, and the antialiased edges between them stay
// between them.
func recolor(src image.Image, from color.NRGBA, to color.Color) *image.NRGBA {
	fh, fs, fv := toHSV(from)
	th, ts, tv := toHSV(color.NRGBAModel.Convert(to).(color.NRGBA))
	r := src.Bounds()
	dst := image.NewNRGBA(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			h, s, v := toHSV(c)
			h = math.Mod(h+th-fh+360, 360)
			if fs > 0 {
				s = min(s*ts/fs, 1)
			}
			switch {
			case v <= fv && fv > 0:
				v *= tv / fv
			case v > fv:
				v = tv + (v-fv)*(1-tv)/(1-fv)
			}
			out := fromHSV(h, s, v)
			out.A = c.A
			dst.SetNRGBA(x, y, out)
		}
	}
	return dst
}

// toHSV is c's hue in degrees, and its saturation and value in [0, 1].
func toHSV(c color.NRGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	v, d := hi, hi-lo
	if hi > 0 {
		s = d / hi
	}
	switch {
	case d == 0:
	case hi == r:
		h = 60 * math.Mod((g-b)/d+6, 6)
	case hi == g:
		h = 60 * ((b-r)/d + 2)
	default:
		h = 60 * ((r-g)/d + 4)
	}
	return h, s, v
}

func fromHSV(h, s, v float64) color.NRGBA {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	to8 := func(f float64) uint8 { return uint8(math.Round(255 * min(max(f+m, 0), 1))) }
	return color.NRGBA{to8(r), to8(g), to8(b), 0xff}
}

// cover is what the markdown says before its first division begins.
type cover struct {
	title, subtitle string
	meta            [][2]string // label, value; markup
	letter          []block
}

// splitCover takes the cover's parts off the front of the blocks.
func splitCover(bs []block) (c cover, rest []block) {
	i := 0
	for ; i < len(bs); i++ {
		if bs[i].kind == bHeading && bs[i].level == 1 {
			break
		}
		if bs[i].kind != bRule {
			c.letter = append(c.letter, bs[i])
		}
	}
	if i == len(bs) {
		return cover{}, bs // No title: the document has no cover to give.
	}
	c.title = bs[i].joined()
	i++
	skip := func() {
		for i < len(bs) && bs[i].kind == bRule {
			i++
		}
	}
	skip()
	if i < len(bs) && bs[i].kind == bHeading && bs[i].level == 2 {
		c.subtitle = bs[i].joined()
		i++
		skip()
	}
	if i < len(bs) && bs[i].kind == bTable && len(bs[i].align) == 2 {
		headless := true
		for _, cell := range bs[i].rows[0] {
			if strings.TrimSpace(stripTags(cell)) != "" {
				headless = false
			}
		}
		if headless {
			for _, r := range bs[i].rows[1:] {
				c.meta = append(c.meta, [2]string{r[0], r[1]})
			}
			i++
		}
	}
	return c, bs[i:]
}

// metaLabel is a particular's label as plain text, without its colon.
func metaLabel(markup string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stripTags(markup)), ":"))
}

// reference is the offer's number and revision, for the running head.
func reference(meta [][2]string) string {
	var parts []string
	for _, m := range meta {
		label, value := strings.ToLower(metaLabel(m[0])), strings.TrimSpace(stripTags(m[1]))
		switch {
		case value == "":
		case strings.HasPrefix(label, "oferta"):
			parts = append(parts, "Oferta "+value)
		case strings.HasPrefix(label, "revisi"):
			parts = append(parts, "Rev. "+value)
		}
	}
	return strings.Join(parts, "  ·  ")
}

// cover sets the first page and returns the index of the piece the contents
// is to take.
func (l *layout) cover(c cover, org string, lg *logo, title, subtitle string) int {
	k, th, pal := l.k, l.th, l.th.Palette
	eyebrow := prose.Style{Font: fontBold, Size: 9.5, Leading: 12, Color: pal.Link}
	titleSt := prose.Style{Font: fontBold, Size: 33, Leading: 38, Color: pal.Paper}
	subSt := prose.Style{Font: fontRegular, Size: 17, Leading: 22, Color: pal.Link}
	// The masthead is the logo where there is one, and the name where not.
	// Either way the title starts the same distance down the band.
	const titleAt = 120
	mast, mastH := k.Styled(escape(strings.ToUpper(org)), eyebrow), eyebrow.Leading
	if lg != nil {
		mast, mastH = k.Image(lg.img, logoWidth), lg.height(logoWidth)
	}
	band := k.Stack(
		mast,
		k.Space(max(titleAt-mastH, 16)),
		k.Styled(escape(title), titleSt),
		k.Space(6),
		k.Styled(escape(subtitle), subSt),
		k.Space(20),
		k.ShortRule(64, 3, l.hi),
	)
	l.add(k.Bookmark(title, 0))
	l.add(backdrop{page: th.Page, h: 262, ground: pal.Ink, content: band})
	l.add(k.Space(34))

	// The particulars, three to a row: label over value.
	valueSt := prose.Style{Font: fontBold, Size: 10.5, Leading: 14, Color: pal.Ink}
	const perRow = 3
	w := th.Measure() / perRow
	for i := 0; i < len(c.meta); i += perRow {
		var cards []prose.Card
		for _, m := range c.meta[i:min(i+perRow, len(c.meta))] {
			v := strings.TrimSpace(m[1])
			if p := stripTags(v); strings.HasPrefix(p, "[") && strings.HasSuffix(p, "]") {
				// A blank the author has still to fill is shown as one.
				v = `<font color="` + hexColor(pal.Muted) + `"><i>` + escape(p) + "</i></font>"
			}
			cards = append(cards, prose.Card{Top: escape(strings.ToUpper(metaLabel(m[0]))), Bottom: v})
		}
		cs := prose.CardStyle{
			Widths:      []float64{w, w, w},
			TopStyle:    th.Scale.CellHead,
			BottomStyle: valueSt,
			Gap:         3,
			Pad:         prose.Padding{Right: 14, Top: 10, Bottom: 10},
			RuleBelow:   0.5,
		}
		if i == 0 {
			cs.RuleAbove = 0.9
		}
		l.add(k.Cards(cs, cards...))
	}
	l.add(k.Space(30))
	for i := range c.letter {
		if b := &c.letter[i]; b.kind == bPara {
			l.add(k.Lead(b.joined()))
		} else {
			l.blocks(c.letter[i : i+1])
		}
	}
	l.add(k.Space(14))
	l.add(k.Space(0))
	return len(l.pieces) - 1
}

// hexColor renders a colour as markup names one.
func hexColor(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
