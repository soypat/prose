package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
	"github.com/soypat/prose/md2pdf"
)

const (
	fontRegular = prose.FamilyText
	fontBold    = prose.FamilyText + "-Bold"
)

// coverPage sets what the markdown says before its first division begins as
// the proposal's first page. It is a handler that acts once, on the blocks the
// document opens with.
type coverPage struct {
	org  string
	logo *logo
}

// cover is what the markdown says before its first division begins.
type cover struct {
	title, subtitle *md2pdf.Block
	meta            [][2]string // label, value; markup
	letter          []md2pdf.Block
}

// splitCover reads the cover's parts off the front of the blocks and reports
// how many it took. A document with no title has no cover to give.
func splitCover(bs []md2pdf.Block) (c cover, n int) {
	i := 0
	for ; i < len(bs); i++ {
		if bs[i].Kind == md2pdf.KindHeading && bs[i].Level == 1 {
			break
		}
		if bs[i].Kind != md2pdf.KindRule {
			c.letter = append(c.letter, bs[i])
		}
	}
	if i == len(bs) {
		return cover{}, 0
	}
	c.title = &bs[i]
	i++
	skip := func() {
		for i < len(bs) && bs[i].Kind == md2pdf.KindRule {
			i++
		}
	}
	skip()
	if i < len(bs) && bs[i].Kind == md2pdf.KindHeading && bs[i].Level == 2 {
		c.subtitle = &bs[i]
		i++
		skip()
	}
	if i < len(bs) && bs[i].Kind == md2pdf.KindTable && len(bs[i].Align) == 2 {
		headless := true
		for _, cell := range bs[i].Rows[0] {
			if strings.TrimSpace(md2pdf.StripTags(cell)) != "" {
				headless = false
			}
		}
		if headless {
			for _, r := range bs[i].Rows[1:] {
				c.meta = append(c.meta, [2]string{r[0], r[1]})
			}
			i++
		}
	}
	return c, i
}

// metaLabel is a particular's label as plain text, without its colon.
func metaLabel(markup string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(md2pdf.StripTags(markup)), ":"))
}

// reference is the offer's number and revision, for the running head.
func reference(meta [][2]string) string {
	var parts []string
	for _, m := range meta {
		label, value := strings.ToLower(metaLabel(m[0])), strings.TrimSpace(md2pdf.StripTags(m[1]))
		switch {
		case value == "":
		case strings.HasPrefix(label, "oferta"), strings.HasPrefix(label, "offer"):
			parts = append(parts, metaLabel(m[0])+" "+value)
		case strings.HasPrefix(label, "revisi"):
			parts = append(parts, "Rev. "+value)
		}
	}
	return strings.Join(parts, "  ·  ")
}

func (cp *coverPage) handle(r *md2pdf.Renderer, bs []md2pdf.Block, i int) int {
	if i != 0 || r.Nested() {
		return 0
	}
	c, n := splitCover(bs)
	if n == 0 {
		return 0
	}
	title, subtitle := r.Plain(c.title), ""
	if c.subtitle != nil {
		subtitle = r.Plain(c.subtitle)
	}
	full := strings.TrimSuffix(title+" — "+subtitle, " — ")
	r.Doc.Meta.Title, r.Doc.Meta.Subject = full, subtitle
	r.Doc.Head = runningHead(cp.org, full, reference(c.meta))
	cp.set(r, c, title, subtitle)
	return n
}

// set lays the cover out: a band of ink carrying the masthead and the title,
// the particulars beneath it, the covering letter, and the contents.
func (cp *coverPage) set(r *md2pdf.Renderer, c cover, title, subtitle string) {
	k, th := r.Kit, r.Kit.Theme
	pal := th.Palette
	eyebrow := prose.Style{Font: fontBold, Size: 9.5, Leading: 12, Color: pal.Link}
	titleSt := prose.Style{Font: fontBold, Size: 33, Leading: 38, Color: pal.Paper}
	subSt := prose.Style{Font: fontRegular, Size: 17, Leading: 22, Color: pal.Link}
	// The masthead is the logo where there is one, and the name where not.
	// Either way the title starts the same distance down the band.
	const titleAt = 120
	mast, mastH := k.Styled(md2pdf.Escape(strings.ToUpper(cp.org)), eyebrow), eyebrow.Leading
	if cp.logo != nil {
		mast, mastH = k.Image(cp.logo.img, logoWidth), cp.logo.height(logoWidth)
	}
	band := k.Stack(
		mast,
		k.Space(max(titleAt-mastH, 16)),
		k.Styled(md2pdf.Escape(title), titleSt),
		k.Space(6),
		k.Styled(md2pdf.Escape(subtitle), subSt),
		k.Space(20),
		// The amber is the one colour the cover is allowed beyond the palette.
		k.ShortRule(64, 3, piupage.HexColor("#f2a900")),
	)
	r.Add(k.Bookmark(title, 0))
	r.Add(backdrop{page: th.Page, h: 262, ground: pal.Ink, content: band})
	r.Add(k.Space(34))

	// The particulars, three to a row: label over value.
	valueSt := prose.Style{Font: fontBold, Size: 10.5, Leading: 14, Color: pal.Ink}
	const perRow = 3
	w := th.Measure() / perRow
	for i := 0; i < len(c.meta); i += perRow {
		var cards []prose.Card
		for _, m := range c.meta[i:min(i+perRow, len(c.meta))] {
			v := strings.TrimSpace(m[1])
			if p := md2pdf.StripTags(v); strings.HasPrefix(p, "[") && strings.HasSuffix(p, "]") {
				// A blank the author has still to fill is shown as one.
				v = `<font color="` + hexColor(pal.Muted) + `"><i>` + md2pdf.Escape(p) + "</i></font>"
			}
			cards = append(cards, prose.Card{Top: md2pdf.Escape(strings.ToUpper(metaLabel(m[0]))), Bottom: v})
		}
		cs := prose.CardStyle{
			Widths:      []float64{w, w, w},
			TopStyle:    th.Scale.CellHead,
			BottomStyle: valueSt,
			Gap:         3,
			Pad:         prose.Padding{Right: 14, Top: 10, Bottom: 10},
			RuleAbove:   0.5,
		}
		if i == 0 {
			cs.RuleAbove = 0.9
		}
		r.Add(k.Cards(cs, cards...))
	}
	r.Add(k.Space(30))
	for i := range c.letter {
		if b := &c.letter[i]; b.Kind == md2pdf.KindParagraph {
			r.Add(k.Lead(b.Joined()))
		} else {
			r.Blocks(c.letter[i : i+1])
		}
	}
	r.Add(k.Space(14))
	// The contents is on the first page and lists what comes after it, so it
	// is made once the rest has been laid out.
	r.Add(r.Later(func() prose.Elem {
		if t := r.Contents(); t != nil {
			return md2pdf.TermRow(r, "Contenido", t)
		}
		return nil
	}))
}

// backdrop fills the page from its top edge to h below the cursor, bleeding off
// both sides, and sets content over it. The cursor ends at the backdrop's foot
// whatever the content's height, so what follows is placed against the
// ground and not against the text on it.
type backdrop struct {
	page    prose.PageSize
	h       float64
	ground  color.Color
	content prose.Elem
}

func (b backdrop) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	foot := yTop - b.h
	dst[0].FillRect(0, foot, b.page.W, b.page.H-foot, b.ground)
	_, _, err := b.content.Draw(dst[:1], f, yTop)
	return 0, foot, err
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

// hexColor renders a colour as markup names one.
func hexColor(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
