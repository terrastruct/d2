// d2fonts holds fonts for renderings

// TODO write a script to do this as part of CI
package d2fonts

import (
	"embed"
	"strings"
)

type FontFamily int
type FontStyle string

type Font struct {
	Family FontFamily
	Style  FontStyle
	Size   int
}

func (f FontFamily) Font(size int, style FontStyle) Font {
	return Font{
		Family: f,
		Style:  style,
		Size:   size,
	}
}

const (
	FONT_SIZE_XS   = 13
	FONT_SIZE_S    = 14
	FONT_SIZE_M    = 16
	FONT_SIZE_L    = 20
	FONT_SIZE_XL   = 24
	FONT_SIZE_XXL  = 28
	FONT_SIZE_XXXL = 32

	FONT_STYLE_REGULAR FontStyle = "regular"
	FONT_STYLE_BOLD    FontStyle = "bold"
	FONT_STYLE_ITALIC  FontStyle = "italic"

	SourceSansPro FontFamily = iota
	SourceCodePro FontFamily = iota
)

var FontSizes = []int{
	FONT_SIZE_XS,
	FONT_SIZE_S,
	FONT_SIZE_M,
	FONT_SIZE_L,
	FONT_SIZE_XL,
	FONT_SIZE_XXL,
	FONT_SIZE_XXXL,
}

var FontStyles = []FontStyle{
	FONT_STYLE_REGULAR,
	FONT_STYLE_BOLD,
	FONT_STYLE_ITALIC,
}

var FontFamilies = []FontFamily{
	SourceSansPro,
	SourceCodePro,
}

//go:embed encoded/SourceSansPro-Regular.txt
var sourceSansProRegularBase64 string

//go:embed encoded/SourceSansPro-Bold.txt
var sourceSansProBoldBase64 string

//go:embed encoded/SourceSansPro-Italic.txt
var sourceSansProItalicBase64 string

//go:embed encoded/SourceCodePro-Regular.txt
var sourceCodeProRegularBase64 string

//go:embed ttf/*
var fontFacesFS embed.FS

var FontEncodings map[Font]string
var FontFaces map[Font][]byte

func init() {
	FontEncodings = map[Font]string{
		{
			Family: SourceSansPro,
			Style:  FONT_STYLE_REGULAR,
		}: sourceSansProRegularBase64,
		{
			Family: SourceSansPro,
			Style:  FONT_STYLE_BOLD,
		}: sourceSansProBoldBase64,
		{
			Family: SourceSansPro,
			Style:  FONT_STYLE_ITALIC,
		}: sourceSansProItalicBase64,
		{
			Family: SourceCodePro,
			Style:  FONT_STYLE_REGULAR,
		}: sourceCodeProRegularBase64,
	}

	for k, v := range FontEncodings {
		FontEncodings[k] = strings.TrimSuffix(v, "\n")
	}

	FontFaces = map[Font][]byte{}
	b, err := fontFacesFS.ReadFile("ttf/SourceSansPro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_REGULAR,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_REGULAR,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_BOLD,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_ITALIC,
	}] = b
}
