package prose

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// build writes a small document in fonts and returns the PDF.
func build(t *testing.T, f Fonts, story func(*Doc)) []byte {
	t.Helper()
	d, err := New(DefaultTheme(), f)
	if err != nil {
		t.Fatal(err)
	}
	d.Meta = Meta{Title: "font test"}
	d.Head = RunningHead{Left: "left", Right: "right", Rules: true}
	story(d)
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func hello(d *Doc) {
	d.Title("Standard fonts").Body("The quick brown fox.").Code("func main() {}")
}

// hasFont reports whether the PDF names face as a /BaseFont. The trailing '/'
// matters: the encoder writes names unspaced, so "Helvetica" would otherwise
// match "Helvetica-Bold".
func hasFont(pdf []byte, face string) bool {
	return bytes.Contains(pdf, []byte("/BaseFont/"+face+"/"))
}

// TestNoFontsIsStandard14 is the case the package used to refuse outright: a
// document that names no file at all.
func TestNoFontsIsStandard14(t *testing.T) {
	pdf := build(t, Fonts{}, hello)
	for _, want := range []string{"Helvetica", "Helvetica-Bold", "Courier"} {
		if !hasFont(pdf, want) {
			t.Errorf("missing /BaseFont %s", want)
		}
	}
	if bytes.Contains(pdf, []byte("/FontFile2")) {
		t.Error("embedded a font program in a standard-14 document")
	}
}

// TestMonoIsFixedPitch is what binding FamilyMono to Courier is for: the same
// count of wide and narrow glyphs must measure the same.
func TestMonoIsFixedPitch(t *testing.T) {
	k, err := NewKit(DefaultTheme(), Fonts{})
	if err != nil {
		t.Fatal(err)
	}
	f := k.faces[FamilyMono]
	upem := float64(f.UnitsPerEm())
	wide := float64(f.GlyphAdvance(f.GlyphID('M'))) / upem
	narrow := float64(f.GlyphAdvance(f.GlyphID('i'))) / upem
	if wide != narrow {
		t.Errorf("mono is proportional: M=%v i=%v", wide, narrow)
	}
	if wide != 0.6 {
		t.Errorf("Courier advance = %v, want 0.6", wide)
	}
}

// TestBuiltinFamily reaches a standard-14 face the default binding does not.
func TestBuiltinFamily(t *testing.T) {
	pdf := build(t, Fonts{TextFamily: Builtin(canvas.FontTimesRoman)}, hello)
	if !hasFont(pdf, "Times-Roman") || !hasFont(pdf, "Times-Bold") {
		t.Error("text family is not Times")
	}
	if !hasFont(pdf, "Courier") {
		t.Error("mono lost its default binding")
	}
	if hasFont(pdf, "Helvetica") {
		t.Error("Helvetica survived a rebound text family")
	}
}

func TestBuiltinGroups(t *testing.T) {
	for _, tc := range []struct {
		in   canvas.FontBuiltin
		want piudoc.Family
	}{
		{canvas.FontHelveticaBoldOblique, Builtin(canvas.FontHelvetica)},
		{canvas.FontTimesBoldItalic, Builtin(canvas.FontTimesRoman)},
		{canvas.FontCourierOblique, Builtin(canvas.FontCourier)},
		{canvas.FontSymbol, piudoc.Family{Regular: canvas.FontSymbol}},
		{0, piudoc.Family{}},
	} {
		if got := Builtin(tc.in); got != tc.want {
			t.Errorf("Builtin(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestRebindOneFamily embeds text and leaves mono to the standard-14 default.
func TestRebindOneFamily(t *testing.T) {
	fsys, name := textFace(t)
	pdf := build(t, Fonts{FS: fsys, Text: []string{name}}, hello)
	if !bytes.Contains(pdf, []byte("/FontFile2")) {
		t.Error("text face was not embedded")
	}
	if !hasFont(pdf, "Courier") {
		t.Error("mono did not fall back to Courier")
	}
	if hasFont(pdf, "Helvetica") {
		t.Error("text still resolves to Helvetica")
	}
}

// TestGuardBitesWithoutEmbedding: standard-14 is WinAnsi, so a rune outside it
// is refused rather than drawn as .notdef.
func TestGuardBitesWithoutEmbedding(t *testing.T) {
	d, err := New(DefaultTheme(), Fonts{})
	if err != nil {
		t.Fatal(err)
	}
	d.Body("a CJK rune: 漢")
	name := filepath.Join(t.TempDir(), "out.pdf")
	if _, err := d.WriteFile(name); err == nil {
		t.Fatal("wrote a document the faces cannot draw")
	} else if !strings.Contains(err.Error(), "glyphs the faces lack") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Error("a refused document left a file behind")
	}
}

// TestDefaultTypographyIsWinAnsi guards the theme's own marks: the en dash, em
// dash, curly quotes and bullet must survive a standard-14 document.
func TestDefaultTypographyIsWinAnsi(t *testing.T) {
	th := DefaultTheme()
	k, err := NewKit(th, Fonts{})
	if err != nil {
		t.Fatal(err)
	}
	f := k.faces[FamilyText]
	for _, r := range "–—''\"\"•…" + th.Bullet.Marker {
		if f.GlyphID(r) == 0 {
			t.Errorf("Helvetica cannot draw %q", r)
		}
	}
}

// textFace finds a .ttf to embed, skipping if the prospectus fonts are absent.
func textFace(t *testing.T) (fsys fs.FS, name string) {
	t.Helper()
	const dir = "../go-cfp/contracting/fonts"
	const face = "Lato-Regular"
	if _, err := os.Stat(filepath.Join(dir, face+".ttf")); err != nil {
		t.Skip("no face to embed:", err)
	}
	return os.DirFS(dir), face
}
