package prose

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // A JPEG a PDF cannot carry as it stands is decoded instead.
	_ "image/png"
	"io/fs"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// ReadImage makes a picture of an image file's bytes. A JPEG is kept as the
// file it is and handed to the reader undecoded, which is both the smallest and
// the most faithful way to carry a photograph; data is kept, not copied, and
// must not change until the document is written. Anything else — a PNG, or a
// JPEG a PDF cannot carry as it stands — is decoded, and its pixels are
// compressed when the document is written. A decoded image lives on the heap,
// outside the document's [Budget].
//
// A picture is written once however many times it is drawn, so one drawn twice
// should be read once and the value reused.
func ReadImage(data []byte) (Image, error) {
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xd8 {
		j := new(canvas.JPEG)
		if j.Reset(data) == nil {
			return j, nil
		}
		// A 12-bit or DNL-sized JPEG: DCTDecode cannot carry it, image/jpeg can.
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("prose: %w", err)
	}
	return img, nil
}

// OpenImage reads the named image file out of fsys. See [ReadImage].
func OpenImage(fsys fs.FS, name string) (Image, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("prose: %s: %w", name, err)
	}
	img, err := ReadImage(b)
	if err != nil {
		return nil, fmt.Errorf("prose: %s: %w", name, err)
	}
	return img, nil
}

// Image draws a picture w points wide, its height following from its aspect
// ratio. A zero w takes the width of whatever frame it lands in, and a w wider
// than that frame is shrunk to it, so a picture never spills out of a column.
// It is set flush left, as text is, and moves whole to the next page rather
// than split.
func (k *Kit) Image(img Image, w float64) Elem { return &picture{img: img, w: w} }

// Figure is a picture over its caption, held on one page. w is the column both
// are set in, as in [Kit.Captioned]: the picture fills it, and a zero w is the
// whole frame.
func (k *Kit) Figure(img Image, w float64, caption string) Elem {
	return k.Captioned(w, k.Image(img, 0), caption)
}

// picture is a piudoc.Image sized against the frame it is drawn in, which is
// the one thing a piudoc.Image cannot know when it is made. Only the width is
// fitted: Measure draws in a frame with no bottom, so a height fitted to the
// page would be measured one size and drawn another.
type picture struct {
	img Image
	w   float64
}

func (p *picture) Draw(dst []canvas.Canvas, f piudoc.Frame, yTop float64) (adv int, yEnd float64, err error) {
	w := p.w
	if w <= 0 || w > f.Width {
		w = f.Width
	}
	im := piudoc.Image{Img: p.img, Width: w}
	return im.Draw(dst, f, yTop)
}
