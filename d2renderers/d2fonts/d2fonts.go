// d2fonts holds fonts for renderings

// TODO write a script to do this as part of CI
// Currently using an online converter: https://dopiaza.org/tools/datauri/index.php
package d2fonts

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"

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

	fontBuf := make([]byte, len(FontFaces[f]))
	copy(fontBuf, FontFaces[f])
	fontBuf = gofpdf.UTF8CutFont(fontBuf, uniqueChars)

	fontBuf, err := fontlib.Sfnt2Woff(fontBuf)
	if err != nil {
		// If subset fails, return full encoding
		return FontEncodings[f]
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

//go:embed encoded/SourceCodePro-Italic.txt
var sourceCodeProItalicBase64 string

//go:embed encoded/ArchitectsDaughter-Regular.txt
var architectsDaughterRegularBase64 string

//go:embed encoded/FuzzyBubbles-Bold.txt
var fuzzyBubblesBoldBase64 string

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
			Style:  FONT_STYLE_SEMIBOLD,
		}: sourceSansProSemiboldBase64,
		{
			Family: SourceSansPro,
			Style:  FONT_STYLE_ITALIC,
		}: sourceSansProItalicBase64,
		{
			Family: SourceCodePro,
			Style:  FONT_STYLE_REGULAR,
		}: sourceCodeProRegularBase64,
		{
			Family: SourceCodePro,
			Style:  FONT_STYLE_BOLD,
		}: sourceCodeProBoldBase64,
		{
			Family: SourceCodePro,
			Style:  FONT_STYLE_ITALIC,
		}: sourceCodeProItalicBase64,
		{
			Family: HandDrawn,
			Style:  FONT_STYLE_REGULAR,
		}: architectsDaughterRegularBase64,
		{
			Family: HandDrawn,
			Style:  FONT_STYLE_ITALIC,
			// This font has no italic, so just reuse regular
		}: architectsDaughterRegularBase64,
		{
			Family: HandDrawn,
			Style:  FONT_STYLE_BOLD,
		}: fuzzyBubblesBoldBase64,
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
	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_BOLD,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceCodePro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_ITALIC,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_BOLD,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Semibold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_SEMIBOLD,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/SourceSansPro-Italic.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: SourceSansPro,
		Style:  FONT_STYLE_ITALIC,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/ArchitectsDaughter-Regular.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_REGULAR,
	}] = b
	FontFaces[Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_ITALIC,
	}] = b
	b, err = fontFacesFS.ReadFile("ttf/FuzzyBubbles-Bold.ttf")
	if err != nil {
		panic(err)
	}
	FontFaces[Font{
		Family: HandDrawn,
		Style:  FONT_STYLE_BOLD,
	}] = b
}

var D2_FONT_TO_FAMILY = map[string]FontFamily{
	"default": SourceSansPro,
	"mono":    SourceCodePro,
}

func AddFontStyle(font Font, style FontStyle, ttf []byte) error {
	FontFaces[font] = ttf

	woff, err := fontlib.Sfnt2Woff(ttf)
	if err != nil {
		return fmt.Errorf("failed to encode ttf to woff: %v", err)
	}
	encodedWoff := fmt.Sprintf("data:application/font-woff;base64,%v", base64.StdEncoding.EncodeToString(woff))
	FontEncodings[font] = encodedWoff

	return nil
}

func AddFontFamily(name string, regularTTF, italicTTF, boldTTF []byte) (*FontFamily, error) {
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
		FontFaces[regularFont] = FontFaces[fallbackFont]
		FontEncodings[regularFont] = FontEncodings[fallbackFont]
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
		FontFaces[italicFont] = FontFaces[fallbackFont]
		FontEncodings[italicFont] = FontEncodings[fallbackFont]
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
		FontFaces[boldFont] = FontFaces[fallbackFont]
		FontEncodings[boldFont] = FontEncodings[fallbackFont]
	}

	FontFamilies = append(FontFamilies, customFontFamily)

	return &customFontFamily, nil
}
