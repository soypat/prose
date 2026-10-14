package prose

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// opaqueImage is a w×h picture with no transparency, so it is written as one
// image XObject and no soft mask.
func opaqueImage(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 0x80, 0xff})
		}
	}
	return m
}

func encodeJPEG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, m, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestReadImageJPEGPassthrough checks a JPEG reaches the reader as the file it
// was, never decoded.
func TestReadImageJPEGPassthrough(t *testing.T) {
	data := encodeJPEG(t, opaqueImage(16, 8))
	img, err := ReadImage(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*canvas.JPEG); !ok {
		t.Fatalf("ReadImage of a JPEG gave %T, want *piupage.JPEG", img)
	}
	pdf := build(t, Fonts{}, func(d *Doc) { d.Image(img, 0) })
	if !bytes.Contains(pdf, []byte("/DCTDecode")) {
		t.Error("JPEG not written as a DCTDecode stream")
	}
	if !bytes.Contains(pdf, data) {
		t.Error("JPEG file not carried into the PDF verbatim")
	}
}

func TestReadImagePNG(t *testing.T) {
	img, err := ReadImage(encodePNG(t, opaqueImage(16, 8)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*canvas.JPEG); ok {
		t.Fatal("ReadImage of a PNG gave a *piupage.JPEG")
	}
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 8 {
		t.Errorf("bounds %v, want 16x8", b)
	}
	pdf := build(t, Fonts{}, func(d *Doc) { d.Image(img, 0) })
	if bytes.Contains(pdf, []byte("/DCTDecode")) {
		t.Error("PNG written as a DCTDecode stream")
	}
	if n := bytes.Count(pdf, []byte("/Subtype/Image")); n != 1 {
		t.Errorf("%d image XObjects, want 1", n)
	}
}

func TestReadImageGarbage(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not an image"), {0xff, 0xd8, 0xff}} {
		if _, err := ReadImage(data); err == nil {
			t.Errorf("ReadImage(%q) succeeded", data)
		} else if !strings.HasPrefix(err.Error(), "prose: ") {
			t.Errorf("error %q does not name the package", err)
		}
	}
}

func TestOpenImage(t *testing.T) {
	fsys := fstest.MapFS{"pics/a.png": {Data: encodePNG(t, opaqueImage(4, 2))}}
	img, err := OpenImage(fsys, "pics/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 4 || b.Dy() != 2 {
		t.Errorf("bounds %v, want 4x2", b)
	}
	_, err = OpenImage(fsys, "pics/missing.png")
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("missing file: got %v, want a not-exist error naming the file", err)
	}
}

// TestImageFitsFrame checks a picture takes its frame's width when given none
// and never spills past it when given too much.
func TestImageFitsFrame(t *testing.T) {
	k, err := NewKit(DefaultTheme(), Fonts{})
	if err != nil {
		t.Fatal(err)
	}
	img := opaqueImage(40, 20)
	dst := make([]canvas.Canvas, 1)
	if err := dst[0].Reset(make([]byte, 512)); err != nil {
		t.Fatal(err)
	}
	f := piudoc.Frame{Width: 100, Top: 500}
	for _, tc := range []struct{ w, wantH float64 }{
		{0, 50},   // the frame's width
		{300, 50}, // shrunk to it
		{60, 30},  // as asked
	} {
		h, err := piudoc.Measure(dst, k.Image(img, tc.w), f, f.Top)
		if err != nil {
			t.Fatal(err)
		}
		if h != tc.wantH {
			t.Errorf("w %v: height %v, want %v", tc.w, h, tc.wantH)
		}
	}
}

// TestImageWrittenOnce draws one picture three ways and checks the reader is
// sent it once.
func TestImageWrittenOnce(t *testing.T) {
	img := opaqueImage(8, 8)
	pdf := build(t, Fonts{}, func(d *Doc) {
		d.Image(img, 0).Image(img, 120).Figure(img, 200, "The same picture, captioned.")
	})
	if n := bytes.Count(pdf, []byte("/Subtype/Image")); n != 1 {
		t.Errorf("image written %d times, want once", n)
	}
	if n := bytes.Count(pdf, []byte("/Im1 Do")); n != 3 {
		t.Errorf("image drawn %d times, want 3", n)
	}
}
