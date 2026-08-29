package fontface

// IsDefaultIgnorableRune reports whether the pinned pure-Go HarfBuzz shaper
// treats value as default-ignorable. These code points normally affect shaping
// or protocol state without painting a glyph of their own.
//
// Keep this exact range list shared by scene construction and raster shaping.
// It intentionally follows HarfBuzz's spacing-glyph exceptions for Hangul
// fillers and its shorthand-format-control exception, rather than blindly
// copying Unicode's broader Default_Ignorable_Code_Point property. Using the
// still broader General_Category=Cf would also incorrectly hide visible
// prepended concatenation marks such as U+0600 and U+06DD.
func IsDefaultIgnorableRune(value rune) bool {
	switch {
	case value == 0x00ad,
		value == 0x034f,
		value == 0x061c,
		value >= 0x17b4 && value <= 0x17b5,
		value >= 0x180b && value <= 0x180e,
		value >= 0x200b && value <= 0x200f,
		value >= 0x202a && value <= 0x202e,
		value >= 0x2060 && value <= 0x206f,
		value >= 0xfe00 && value <= 0xfe0f,
		value == 0xfeff,
		value >= 0xfff0 && value <= 0xfff8,
		value >= 0x1d173 && value <= 0x1d17a,
		value >= 0xe0000 && value <= 0xe0fff:
		return true
	default:
		return false
	}
}
