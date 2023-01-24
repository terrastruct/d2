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
	}

	runa(t, tcs)
}
