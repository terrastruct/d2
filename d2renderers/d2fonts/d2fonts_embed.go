//go:build !js || !wasm

package d2fonts

import (
	"embed"
	_ "embed"
	"strings"

	"oss.terrastruct.com/d2/lib/syncmap"
)

//go:embed encoded/SourceSansPro-Regular.txt
var sourceSansProRegularBase64 string

//go:embed encoded/SourceSansPro-Bold.txt
var sourceSansProBoldBase64 string

//go:embed encoded/SourceSansPro-Semibold.txt
var sourceSansProSemiboldBase64 string

//go:embed encoded/SourceSansPro-Italic.txt
var sourceSansProItalicBase64 string

//go:embed encoded/SourceCodePro-Regular.txt
var sourceCodeProRegularBase64 string

//go:embed encoded/SourceCodePro-Bold.txt
var sourceCodeProBoldBase64 string

//go:embed encoded/SourceCodePro-Semibold.txt
var sourceCodeProSemiboldBase64 string

//go:embed encoded/SourceCodePro-Italic.txt
var sourceCodeProItalicBase64 string

//go:embed encoded/FuzzyBubbles-Regular.txt
var fuzzyBubblesRegularBase64 string

//go:embed encoded/FuzzyBubbles-Bold.txt
var fuzzyBubblesBoldBase64 string

//go:embed ttf/*
var fontFacesFS embed.FS

func init() {
	FontEncodings = syncmap.New[Font, string]()

	FontEncodings.Set(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_REGULAR,
		},
		sourceSansProRegularBase64)

	FontEncodings.Set(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_BOLD,
		},
		sourceSansProBoldBase64)

	FontEncodings.Set(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_SEMIBOLD,
		},
		sourceSansProSemiboldBase64)

	FontEncodings.Set(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_ITALIC,
		},
		sourceSansProItalicBase64)

	FontEncodings.Set(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_REGULAR,
		},
		sourceCodeProRegularBase64)

	FontEncodings.Set(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_BOLD,
		},
		sourceCodeProBoldBase64)

	FontEncodings.Set(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_SEMIBOLD,
		},
		sourceCodeProSemiboldBase64)

	FontEncodings.Set(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_ITALIC,
		},
		sourceCodeProItalicBase64)

	FontEncodings.Set(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_REGULAR,
		},
		fuzzyBubblesRegularBase64)

	FontEncodings.Set(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_ITALIC,
			// This font has no italic, so just reuse regular
		}, fuzzyBubblesRegularBase64)
	FontEncodings.Set(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_BOLD,
		}, fuzzyBubblesBoldBase64)
	FontEncodings.Set(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_SEMIBOLD,
			// This font has no semibold, so just reuse bold
		}, fuzzyBubblesBoldBase64)

	FontEncodings.Range(func(k Font, v string) bool {
		FontEncodings.Set(k, strings.TrimSuffix(v, "\n"))
		return true
	})

	FontFaces = syncmap.New[Font, []byte]()

	b, err := fontFacesFS.ReadFile("ttf/SourceSansPro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_REGULAR,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_REGULAR,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_BOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Semibold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_BOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Semibold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/FuzzyBubbles-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_REGULAR,
	}, b)
	FontFaces.Set(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/FuzzyBubbles-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Set(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_BOLD,
	}, b)
	FontFaces.Set(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)
}
