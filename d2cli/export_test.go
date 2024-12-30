package d2cli

import (
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
		requiresPngRender         bool
	}
	testCases := []testCase{
		{
			outputPath:                "/out.svg",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
			requiresPngRender:         false,
		},
		{
			// assumes SVG by default
			outputPath:                "/out",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
			requiresPngRender:         false,
		},
		{
			outputPath:                "-",
			extension:                 SVG,
			supportsDarkTheme:         true,
			supportsAnimation:         true,
			requiresAnimationInterval: false,
			requiresPngRender:         false,
		},
		{
			stdoutFormatFlag:          "png",
			outputPath:                "-",
			extension:                 PNG,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
			requiresPngRender:         true,
		},
		{
			outputPath:                "/out.png",
			extension:                 PNG,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
			requiresPngRender:         true,
		},
		{
			outputPath:                "/out.pptx",
			extension:                 PPTX,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
			requiresPngRender:         true,
		},
		{
			outputPath:                "/out.pdf",
			extension:                 PDF,
			supportsDarkTheme:         false,
			supportsAnimation:         false,
			requiresAnimationInterval: false,
			requiresPngRender:         true,
		},
		{
			outputPath:                "/out.gif",
			extension:                 GIF,
			supportsDarkTheme:         false,
			supportsAnimation:         true,
			requiresAnimationInterval: true,
			requiresPngRender:         true,
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
			assert.Equal(t, tc.requiresPngRender, extension.requiresPNGRenderer())
		})
	}
}
