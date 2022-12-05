package e2etests

import (
	"testing"
)

func testRegression(t *testing.T) {
	tcs := []testCase{
		{
			name: "dagre_id_with_newline",
			script: `
ninety\nnine
eighty\reight
seventy\r\nseven
`,
		},
		{
			name: "empty_sequence",
			script: `
A: hello {
  shape: sequence_diagram
}

B: goodbye {
  shape: sequence_diagram
}

A->B
`,
		},
	}

	runa(t, tcs)
}
