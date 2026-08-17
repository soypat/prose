package prose

import piudoc "github.com/soypat/piudf/piupage/piudoc"

// The piudoc vocabulary a document names, re-exported so that writing one needs
// this package alone. They are aliases, not wrappers: a value here is the same
// value piudoc sees, and anything built with piudoc directly still fits.
type (
	Style       = piudoc.Style
	LinkStyle   = piudoc.LinkStyle
	Align       = piudoc.Align
	VAlign      = piudoc.VAlign
	Padding     = piudoc.Padding
	PageSize    = piudoc.PageSize
	Margins     = piudoc.Margins
	BulletStyle = piudoc.BulletStyle
)

const (
	Left    = piudoc.Left
	Center  = piudoc.Center
	Right   = piudoc.Right
	Justify = piudoc.Justify

	Top    = piudoc.Top
	Middle = piudoc.Middle
	Bottom = piudoc.Bottom
)

// Page sizes.
func SizeA4() PageSize     { return piudoc.SizeA4() }
func SizeLetter() PageSize { return piudoc.SizeLetter() }

// PadAll is the same padding on all four sides.
func PadAll(p float64) Padding { return piudoc.PadAll(p) }
