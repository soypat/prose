package prose

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/soypat/lefevre"
	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Builtin is the standard-14 family f belongs to, at every weight and slant that
// family carries. Symbol and ZapfDingbats stand alone; an invalid f gives the
// zero family, which reads as unset.
func Builtin(f canvas.FontBuiltin) piudoc.Family {
	switch f {
	case canvas.FontHelvetica, canvas.FontHelveticaBold, canvas.FontHelveticaOblique, canvas.FontHelveticaBoldOblique:
		return piudoc.Family{Regular: canvas.FontHelvetica, Bold: canvas.FontHelveticaBold,
			Italic: canvas.FontHelveticaOblique, BoldItalic: canvas.FontHelveticaBoldOblique}
	case canvas.FontTimesRoman, canvas.FontTimesBold, canvas.FontTimesItalic, canvas.FontTimesBoldItalic:
		return piudoc.Family{Regular: canvas.FontTimesRoman, Bold: canvas.FontTimesBold,
			Italic: canvas.FontTimesItalic, BoldItalic: canvas.FontTimesBoldItalic}
	case canvas.FontCourier, canvas.FontCourierBold, canvas.FontCourierOblique, canvas.FontCourierBoldOblique:
		return piudoc.Family{Regular: canvas.FontCourier, Bold: canvas.FontCourierBold,
			Italic: canvas.FontCourierOblique, BoldItalic: canvas.FontCourierBoldOblique}
	case canvas.FontSymbol, canvas.FontZapfDingbats:
		return piudoc.Family{Regular: f}
	}
	return piudoc.Family{}
}

// loadFamily reads the named faces out of fsys and returns them as a family in
// Regular, Bold, Italic order. Only the first is required.
func loadFamily(fsys fs.FS, dir string, names ...string) (piudoc.Family, error) {
	if len(names) == 0 {
		return piudoc.Family{}, fmt.Errorf("prose: no face named")
	}
	faces := make([]*lefevre.Font, len(names))
	for i, name := range names {
		b, err := fs.ReadFile(fsys, path.Join(dir, name+".ttf"))
		if err != nil {
			return piudoc.Family{}, fmt.Errorf("prose: %s: %w", name, err)
		}
		faces[i] = new(lefevre.Font)
		if err = faces[i].LoadBytes(b, 0); err != nil {
			return piudoc.Family{}, fmt.Errorf("prose: %s: %w", name, err)
		}
	}
	fam := piudoc.Family{Regular: faces[0]}
	if len(faces) > 1 {
		fam.Bold = faces[1]
	}
	if len(faces) > 2 {
		fam.Italic = faces[2]
	}
	return fam, nil
}

// baseFamily strips a weight or slant suffix, e.g. "Text-Bold" -> "Text". It is
// piudoc's own rule, repeated here because piudoc keeps it unexported.
func baseFamily(name string) string {
	if i := strings.IndexByte(name, '-'); i >= 0 {
		return name[:i]
	}
	if name == "" {
		return FamilyText
	}
	return name
}

// stripTags removes markup so that only drawable text is glyph-checked.
func stripTags(s string) string {
	if !strings.ContainsRune(s, '<') {
		return s
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			return b.String()
		}
		s = s[i+j+1:]
	}
}
