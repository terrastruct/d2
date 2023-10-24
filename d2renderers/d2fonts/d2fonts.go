// d2fonts holds fonts for renderings

// TODO write a script to do this as part of CI
// Currently using an online converter: https://dopiaza.org/tools/datauri/index.php
package d2fonts

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"oss.terrastruct.com/d2/lib/font"
	fontlib "oss.terrastruct.com/d2/lib/font"
)

type FontFamily string
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

func (f Font) GetEncodedSubset(corpus string) string {
	var uniqueChars string
	uniqueMap := make(map[rune]bool)
	for _, char := range corpus {
		if _, exists := uniqueMap[char]; !exists {
			uniqueMap[char] = true
			uniqueChars = uniqueChars + string(char)
		}
	}

	FontFamiliesMu.Lock()
	defer FontFamiliesMu.Unlock()
	var face []byte
	{
		ff, _ := FontFaces.Load(f)
		face = ff.([]byte)
	}
	fontBuf := make([]byte, len(face))
	copy(fontBuf, face)
	fontBuf = font.UTF8CutFont(fontBuf, uniqueChars)

	fontBuf, err := fontlib.Sfnt2Woff(fontBuf)
	if err != nil {
		// If subset fails, return full encoding
		fe, _ := FontEncodings.Load(f)
		return fe.(string)
	}

	return fmt.Sprintf("data:application/font-woff;base64,%v", base64.StdEncoding.EncodeToString(fontBuf))
}

const (
	FONT_SIZE_XS   = 13
	FONT_SIZE_S    = 14
	FONT_SIZE_M    = 16
	FONT_SIZE_L    = 20
	FONT_SIZE_XL   = 24
	FONT_SIZE_XXL  = 28
	FONT_SIZE_XXXL = 32

	FONT_STYLE_REGULAR  FontStyle = "regular"
	FONT_STYLE_BOLD     FontStyle = "bold"
	FONT_STYLE_SEMIBOLD FontStyle = "semibold"
	FONT_STYLE_ITALIC   FontStyle = "italic"

	SourceSansPro FontFamily = "SourceSansPro"
	SourceCodePro FontFamily = "SourceCodePro"
	HandDrawn     FontFamily = "HandDrawn"
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
	FONT_STYLE_SEMIBOLD,
	FONT_STYLE_ITALIC,
}

var FontFamilies = []FontFamily{
	SourceSansPro,
	SourceCodePro,
	HandDrawn,
}

var FontFamiliesMu sync.Mutex

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

// FontEncodings map[Font]string
var FontEncodings sync.Map

// FontFaces map[Font][]byte
var FontFaces sync.Map

func init() {
	FontEncodings.Store(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_REGULAR,
		},
		sourceSansProRegularBase64)

	FontEncodings.Store(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_BOLD,
		},
		sourceSansProBoldBase64)

	FontEncodings.Store(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_SEMIBOLD,
		},
		sourceSansProSemiboldBase64)

	FontEncodings.Store(
		Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_ITALIC,
		},
		sourceSansProItalicBase64)

	FontEncodings.Store(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_REGULAR,
		},
		sourceCodeProRegularBase64)

	FontEncodings.Store(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_BOLD,
		},
		sourceCodeProBoldBase64)

	FontEncodings.Store(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_SEMIBOLD,
		},
		sourceCodeProSemiboldBase64)

	FontEncodings.Store(
		Font{
			Family: SourceCodePro,
			Style:  FONT_STYLE_ITALIC,
		},
		sourceCodeProItalicBase64)

	FontEncodings.Store(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_REGULAR,
		},
		fuzzyBubblesRegularBase64)

	FontEncodings.Store(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_ITALIC,
			// This font has no italic, so just reuse regular
		}, fuzzyBubblesRegularBase64)
	FontEncodings.Store(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_BOLD,
		}, fuzzyBubblesBoldBase64)
	FontEncodings.Store(
		Font{
			Family: HandDrawn,
			Style:  FONT_STYLE_SEMIBOLD,
			// This font has no semibold, so just reuse bold
		}, fuzzyBubblesBoldBase64)

	FontEncodings.Range(func(k, v any) bool {
		FontEncodings.Swap(k, strings.TrimSuffix(v.(string), "\n"))
		return true
	})

	b, err := fontFacesFS.ReadFile("ttf/SourceSansPro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_REGULAR,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_REGULAR,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_BOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Semibold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_BOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Semibold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/FuzzyBubbles-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_REGULAR,
	}, b)
	FontFaces.Store(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_ITALIC,
	}, b)

	b, err = fontFacesFS.ReadFile("ttf/FuzzyBubbles-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces.Store(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_BOLD,
	}, b)
	FontFaces.Store(Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_SEMIBOLD,
	}, b)
}

var D2_FONT_TO_FAMILY = map[string]FontFamily{
	"default": SourceSansPro,
	"mono":    SourceCodePro,
}

func AddFontStyle(font Font, style FontStyle, ttf []byte) error {
	FontFaces.Store(font, ttf)

	woff, err := fontlib.Sfnt2Woff(ttf)
	if err != nil {
		return fmt.Errorf("failed to encode ttf to woff: %v", err)
	}
	encodedWoff := fmt.Sprintf("data:application/font-woff;base64,%v", base64.StdEncoding.EncodeToString(woff))
	FontEncodings.Store(font, encodedWoff)

	return nil
}

func AddFontFamily(name string, regularTTF, italicTTF, boldTTF, semiboldTTF []byte) (*FontFamily, error) {
	FontFamiliesMu.Lock()
	defer FontFamiliesMu.Unlock()
	customFontFamily := FontFamily(name)

	regularFont := Font{
		Family: customFontFamily,
		Style:  FONT_STYLE_REGULAR,
	}
	if regularTTF != nil {
		err := AddFontStyle(regularFont, FONT_STYLE_REGULAR, regularTTF)
		if err != nil {
			return nil, err
		}
	} else {
		fallbackFont := Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_REGULAR,
		}
		f, _ := FontFaces.Load(fallbackFont)
		FontFaces.Store(regularFont, f)
		e, _ := FontEncodings.Load(fallbackFont)
		FontEncodings.Store(regularFont, e)
	}

	italicFont := Font{
		Family: customFontFamily,
		Style:  FONT_STYLE_ITALIC,
	}
	if italicTTF != nil {
		err := AddFontStyle(italicFont, FONT_STYLE_ITALIC, italicTTF)
		if err != nil {
			return nil, err
		}
	} else {
		fallbackFont := Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_ITALIC,
		}
		f, _ := FontFaces.Load(fallbackFont)
		FontFaces.Store(italicFont, f)
		fb, _ := FontEncodings.Load(fallbackFont)
		FontEncodings.Store(italicFont, fb)
	}

	boldFont := Font{
		Family: customFontFamily,
		Style:  FONT_STYLE_BOLD,
	}
	if boldTTF != nil {
		err := AddFontStyle(boldFont, FONT_STYLE_BOLD, boldTTF)
		if err != nil {
			return nil, err
		}
	} else {
		fallbackFont := Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_BOLD,
		}
		f, _ := FontFaces.Load(fallbackFont)
		FontFaces.Store(boldFont, f)
		fb, _ := FontEncodings.Load(fallbackFont)
		FontEncodings.Store(boldFont, fb)
	}

	semiboldFont := Font{
		Family: customFontFamily,
		Style:  FONT_STYLE_SEMIBOLD,
	}
	if semiboldTTF != nil {
		err := AddFontStyle(semiboldFont, FONT_STYLE_SEMIBOLD, semiboldTTF)
		if err != nil {
			return nil, err
		}
	} else {
		fallbackFont := Font{
			Family: SourceSansPro,
			Style:  FONT_STYLE_SEMIBOLD,
		}
		f, _ := FontFaces.Load(fallbackFont)
		FontFaces.Store(semiboldFont, f)
		fb, _ := FontEncodings.Load(fallbackFont)
		FontEncodings.Store(semiboldFont, fb)
	}

	FontFamilies = append(FontFamilies, customFontFamily)

	return &customFontFamily, nil
}
