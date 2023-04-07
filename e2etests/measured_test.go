package e2etests

import (
	_ "embed"
	"testing"

	"oss.terrastruct.com/d2/d2target"
)

// testMeasured exercises the code paths that provide pre-measured texts
func testMeasured(t *testing.T) {
	tcs := []testCase{
		{
			name:   "empty-shape",
			mtexts: []*d2target.MText{},
			script: `a: ""
`,
		},
		{
			name:   "empty-class",
			mtexts: []*d2target.MText{},
			script: `a: "" { shape: class }
`,
		},
		{
			name:   "empty-sql_table",
			mtexts: []*d2target.MText{},
			script: `a: "" { shape: sql_table }
`,
		},
		{
			name:   "empty-markdown",
			mtexts: []*d2target.MText{},
			script: `a: |md
` + " " + `
|
`,
		},
		{
			name: "empty-code",
			mtexts: []*d2target.MText{
				{
					Text:     "  ",
					FontSize: 16,
					IsBold:   true,
					Shape:    "code",
					Language: "java",
					Dimensions: d2target.TextDimensions{
						Width:  55,
						Height: 30,
					},
				},
			},
			script: `a: |java
` + "  " + `
|
`,
		},
	}

	runa(t, tcs)
}
