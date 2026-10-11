package d2cli

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutputFormat(t *testing.T) {
	type testCase struct {
		stdoutFormatFlag          string
		outputPath                string
		extension                 exportExtension
		supportsDarkTheme         bool
		supportsAnimation         bool
		requiresAnimationInterval bool
	}
	testCases := []testCase{
		{
			outputPath:                "/out.svg",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
		},
		{
			// assumes SVG by default
			outputPath:                "/out",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
		},
		{
			outputPath:                "-",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
		},
		{
			stdoutFormatFlag:          "png",
			outputPath:                "-",
			extension:                 PNG,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
		},
		{
			outputPath:                "/out.png",
			extension:                 PNG,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
		},
		{
			outputPath:                "/out.pptx",
			extension:                 PPTX,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
		},
		{
			outputPath:                "/out.pdf",
			extension:                 PDF,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
		},
		{
			outputPath:                "/out.gif",
			extension:                 GIF,
			supportsDarkTheme:         false,
			supportsAnimation:         true,
			requiresAnimationInterval: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.outputPath, func(t *testing.T) {
			extension, err := getOutputFormat(&tc.stdoutFormatFlag, tc.outputPath)
			assert.NoError(t, err)
			assert.Equal(t, tc.extension, extension)
			assert.Equal(t, tc.supportsAnimation, extension.supportsAnimation())
			assert.Equal(t, tc.supportsDarkTheme, extension.supportsDarkTheme())
		})
	}
}

func TestUnsupportedStdoutFormatDoesNotWrite(t *testing.T) {
	for _, format := range []string{"html", "HTML", "unknown"} {
		t.Run(format, func(t *testing.T) {
			data, err := imageCLI(t, t.TempDir(), "a -> b", "--stdout-format="+format, "-")
			if err == nil || !strings.Contains(err.Error(), "is not a supported format") {
				t.Fatalf("unsupported format %q: got %v", format, err)
			}
			if len(data) != 0 {
				t.Fatal("unsupported format wrote output")
			}
		})
	}
}

func TestUnknownOutputExtensionUsesSVG(t *testing.T) {
	for _, name := range []string{"out.html", "out.unknown"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := imageCLI(t, dir, "a -> b", name); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var document struct{ XMLName xml.Name }
			if err := xml.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			if document.XMLName.Local != "svg" || document.XMLName.Space != "http://www.w3.org/2000/svg" {
				t.Fatalf("unknown extension no longer defaults to SVG: %v", document.XMLName)
			}
		})
	}
}
