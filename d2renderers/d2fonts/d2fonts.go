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
	"oss.terrastruct.com/d2/lib/syncmap"
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
	face := FontFaces.Get(f)
	fontBuf := make([]byte, len(face))
	copy(fontBuf, face)
	fontBuf = font.UTF8CutFont(fontBuf, uniqueChars)

	fontBuf, err := fontlib.Sfnt2Woff(fontBuf)
	if err != nil {
		// If subset fails, return full encoding
		return FontEncodings.Get(f)
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

var FontEncodings syncmap.SyncMap[Font, string]
var FontFaces syncmap.SyncMap[Font, []byte]

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

var D2_FONT_TO_FAMILY = map[string]FontFamily{
	"default": SourceSansPro,
	"mono":    SourceCodePro,
}

func AddFontStyle(font Font, style FontStyle, ttf []byte) error {
	FontFaces.Set(font, ttf)

	woff, err := fontlib.Sfnt2Woff(ttf)
	if err != nil {
		return fmt.Errorf("failed to encode ttf to woff: %v", err)
	}
	encodedWoff := fmt.Sprintf("data:application/font-woff;base64,%v", base64.StdEncoding.EncodeToString(woff))
	FontEncodings.Set(font, encodedWoff)

	return nil
}

func AddFontFamily(name string, regularTTF, italicTTF, boldTTF, semiboldTTF []byte) (*FontFamily, error) {
	FontFamiliesMu.Lock()
	defer FontFamiliesMu.Unlock()
	customFontFamily := FontFamily(name)

	fallbackFamily := SourceSansPro
	if strings.Contains(strings.ToLower(name), "mono") {
		fallbackFamily = SourceCodePro
	}

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
			Family: fallbackFamily,
			Style:  FONT_STYLE_REGULAR,
		}
		FontFaces.Set(regularFont, FontFaces.Get(fallbackFont))
		FontEncodings.Set(regularFont, FontEncodings.Get(fallbackFont))
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
			Family: fallbackFamily,
			Style:  FONT_STYLE_ITALIC,
		}
		FontFaces.Set(italicFont, FontFaces.Get(fallbackFont))
		FontEncodings.Set(italicFont, FontEncodings.Get(fallbackFont))
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
			Family: fallbackFamily,
			Style:  FONT_STYLE_BOLD,
		}
		FontFaces.Set(boldFont, FontFaces.Get(fallbackFont))
		FontEncodings.Set(boldFont, FontEncodings.Get(fallbackFont))
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
			Family: fallbackFamily,
			Style:  FONT_STYLE_SEMIBOLD,
		}
		FontFaces.Set(semiboldFont, FontFaces.Get(fallbackFont))
		FontEncodings.Set(semiboldFont, FontEncodings.Get(fallbackFont))
	}

	FontFamilies = append(FontFamilies, customFontFamily)

	return &customFontFamily, nil
}
