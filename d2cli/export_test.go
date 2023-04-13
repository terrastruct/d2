package d2cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutputFormat(t *testing.T) {
	type testCase struct {
		outputPath        string
		extension         exportExtension
		supportsDarkTheme bool
		supportsAnimation bool
		requiresPngRender bool
	}
	testCases := []testCase{
		{
			outputPath:        "/out.svg",
			extension:         ".svg",
			supportsDarkTheme: true,
			supportsAnimation: true,
			requiresPngRender: false,
		},
		{
			// assumes SVG by default
			outputPath:        "/out",
			extension:         ".svg",
			supportsDarkTheme: true,
			supportsAnimation: true,
			requiresPngRender: false,
		},
		{
			outputPath:        "-",
			extension:         ".svg",
			supportsDarkTheme: true,
			supportsAnimation: true,
			requiresPngRender: false,
		},
		{
			outputPath:        "/out.png",
			extension:         ".png",
			supportsDarkTheme: false,
			supportsAnimation: false,
			requiresPngRender: true,
		},
		{
			outputPath:        "/out.pptx",
			extension:         ".pptx",
			supportsDarkTheme: false,
			supportsAnimation: false,
			requiresPngRender: true,
		},
		{
			outputPath:        "/out.pdf",
			extension:         ".pdf",
			supportsDarkTheme: false,
			supportsAnimation: false,
			requiresPngRender: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.outputPath, func(t *testing.T) {
			extension := getExportExtension(tc.outputPath)
			assert.Equal(t, tc.extension, extension)
			assert.Equal(t, tc.supportsAnimation, extension.supportsAnimation())
			assert.Equal(t, tc.supportsDarkTheme, extension.supportsDarkTheme())
			assert.Equal(t, tc.requiresPngRender, extension.requiresPngRenderer())
		})
	}
}
