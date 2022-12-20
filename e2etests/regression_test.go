package e2etests

import (
	"testing"
)

func testRegression(t *testing.T) {
	tcs := []testCase{
		{
			name: "dagre_special_ids",
			script: `
ninety\nnine
eighty\reight
seventy\r\nseven
a\\yode -> there
a\\"ode -> there
a\\node -> there
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

A->B`,
		}, {
			name: "sequence_diagram_span_cover",
			script: `shape: sequence_diagram
b.1 -> b.1
b.1 -> b.1`,
		}, {
			name: "sequence_diagram_no_message",
			script: `shape: sequence_diagram
a: A
b: B`,
		},
		{
			name: "sequence_diagram_name_crash",
			script: `foo: {
	shape: sequence_diagram
	a -> b
}
foobar: {
	shape: sequence_diagram
	c -> d
}
foo -> foobar`,
		},
		{
			name: "sql_table_overflow",
			script: `
table: sql_table_overflow {
	shape: sql_table
	short: loooooooooooooooooooong
	loooooooooooooooooooong: short
}
table_constrained: sql_table_constrained_overflow {
	shape: sql_table
	short: loooooooooooooooooooong {
		constraint: unique
	}
	loooooooooooooooooooong: short {
		constraint: foreign_key
	}
}
`,
		},
	}

	runa(t, tcs)
}
