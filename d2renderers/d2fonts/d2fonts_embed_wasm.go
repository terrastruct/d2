//go:build js && wasm

package d2fonts

import (
	"bytes"
	"compress/gzip"
	"embed"
	_ "embed"
	"fmt"
	"io"
	"strings"

	"oss.terrastruct.com/d2/lib/syncmap"
)

// Compressed font data for WASM builds

//go:embed encoded/SourceSansPro-Regular.txt.gz
var sourceSansProRegularGz []byte

//go:embed encoded/SourceSansPro-Bold.txt.gz
var sourceSansProBoldGz []byte

//go:embed encoded/SourceSansPro-Semibold.txt.gz
var sourceSansProSemiboldGz []byte

//go:embed encoded/SourceSansPro-Italic.txt.gz
var sourceSansProItalicGz []byte

//go:embed encoded/SourceCodePro-Regular.txt.gz
var sourceCodeProRegularGz []byte

//go:embed encoded/SourceCodePro-Bold.txt.gz
var sourceCodeProBoldGz []byte

//go:embed encoded/SourceCodePro-Semibold.txt.gz
var sourceCodeProSemiboldGz []byte

//go:embed encoded/SourceCodePro-Italic.txt.gz
var sourceCodeProItalicGz []byte

//go:embed encoded/FuzzyBubbles-Regular.txt.gz
var fuzzyBubblesRegularGz []byte

//go:embed encoded/FuzzyBubbles-Bold.txt.gz
var fuzzyBubblesBoldGz []byte

//go:embed ttf/*
var fontFacesFS embed.FS

// decompressGzip decompresses gzipped data
func decompressGzip(compressed []byte) (string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to decompress: %w", err)
	}

	return string(decompressed), nil
}

func init() {
	FontEncodings = syncmap.New[Font, string]()

	// Decompress and register SourceSansPro fonts
	if str, err := decompressGzip(sourceSansProRegularGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceSansPro-Regular: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR}, str)
	}

	if str, err := decompressGzip(sourceSansProBoldGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceSansPro-Bold: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceSansPro, Style: FONT_STYLE_BOLD}, str)
	}

	if str, err := decompressGzip(sourceSansProSemiboldGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceSansPro-Semibold: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceSansPro, Style: FONT_STYLE_SEMIBOLD}, str)
	}

	if str, err := decompressGzip(sourceSansProItalicGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceSansPro-Italic: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceSansPro, Style: FONT_STYLE_ITALIC}, str)
	}

	// Decompress and register SourceCodePro fonts
	if str, err := decompressGzip(sourceCodeProRegularGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceCodePro-Regular: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceCodePro, Style: FONT_STYLE_REGULAR}, str)
	}

	if str, err := decompressGzip(sourceCodeProBoldGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceCodePro-Bold: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceCodePro, Style: FONT_STYLE_BOLD}, str)
	}

	if str, err := decompressGzip(sourceCodeProSemiboldGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceCodePro-Semibold: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceCodePro, Style: FONT_STYLE_SEMIBOLD}, str)
	}

	if str, err := decompressGzip(sourceCodeProItalicGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress SourceCodePro-Italic: %v", err))
	} else {
		FontEncodings.Set(Font{Family: SourceCodePro, Style: FONT_STYLE_ITALIC}, str)
	}

	// Decompress and register FuzzyBubbles fonts
	if str, err := decompressGzip(fuzzyBubblesRegularGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress FuzzyBubbles-Regular: %v", err))
	} else {
		FontEncodings.Set(Font{Family: HandDrawn, Style: FONT_STYLE_REGULAR}, str)
		// HandDrawn has no italic, so reuse regular
		FontEncodings.Set(Font{Family: HandDrawn, Style: FONT_STYLE_ITALIC}, str)
	}

	if str, err := decompressGzip(fuzzyBubblesBoldGz); err != nil {
		panic(fmt.Sprintf("Failed to decompress FuzzyBubbles-Bold: %v", err))
	} else {
		FontEncodings.Set(Font{Family: HandDrawn, Style: FONT_STYLE_BOLD}, str)
		// HandDrawn has no semibold, so reuse bold
		FontEncodings.Set(Font{Family: HandDrawn, Style: FONT_STYLE_SEMIBOLD}, str)
	}

	// Trim trailing newlines
	trimEncodings()

	// Initialize FontFaces with TTF files
	if err := initializeFontFaces(fontFacesFS); err != nil {
		panic(fmt.Sprintf("Failed to initialize font faces: %v", err))
	}
}

// trimEncodings removes trailing newlines from all font encodings
func trimEncodings() {
	FontEncodings.Range(func(k Font, v string) bool {
		FontEncodings.Set(k, strings.TrimSuffix(v, "\n"))
		return true
	})
}

// initializeFontFaces loads TTF font files into FontFaces
func initializeFontFaces(fontFacesFS embed.FS) error {
	FontFaces = syncmap.New[Font, []byte]()

	// SourceSansPro fonts
	fontFiles := []struct {
		file   string
		family FontFamily
		style  FontStyle
	}{
		{"ttf/SourceSansPro-Regular.ttf", SourceSansPro, FONT_STYLE_REGULAR},
		{"ttf/SourceSansPro-Bold.ttf", SourceSansPro, FONT_STYLE_BOLD},
		{"ttf/SourceSansPro-Semibold.ttf", SourceSansPro, FONT_STYLE_SEMIBOLD},
		{"ttf/SourceSansPro-Italic.ttf", SourceSansPro, FONT_STYLE_ITALIC},
		{"ttf/SourceCodePro-Regular.ttf", SourceCodePro, FONT_STYLE_REGULAR},
		{"ttf/SourceCodePro-Bold.ttf", SourceCodePro, FONT_STYLE_BOLD},
		{"ttf/SourceCodePro-Semibold.ttf", SourceCodePro, FONT_STYLE_SEMIBOLD},
		{"ttf/SourceCodePro-Italic.ttf", SourceCodePro, FONT_STYLE_ITALIC},
		{"ttf/FuzzyBubbles-Regular.ttf", HandDrawn, FONT_STYLE_REGULAR},
		{"ttf/FuzzyBubbles-Bold.ttf", HandDrawn, FONT_STYLE_BOLD},
	}

	for _, font := range fontFiles {
		b, err := fontFacesFS.ReadFile(font.file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", font.file, err)
		}
		FontFaces.Set(Font{Family: font.family, Style: font.style}, b)
	}

	// HandDrawn font duplicates for missing styles
	fuzzyRegular := FontFaces.Get(Font{Family: HandDrawn, Style: FONT_STYLE_REGULAR})
	FontFaces.Set(Font{Family: HandDrawn, Style: FONT_STYLE_ITALIC}, fuzzyRegular)

	fuzzyBold := FontFaces.Get(Font{Family: HandDrawn, Style: FONT_STYLE_BOLD})
	FontFaces.Set(Font{Family: HandDrawn, Style: FONT_STYLE_SEMIBOLD}, fuzzyBold)

	return nil
}
