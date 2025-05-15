package d2format_test

import (
	"fmt"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"

	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

func TestPrint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		exp  string
	}{
		{
			name: "basic",
			in: `
x  ->  y
`,
			exp: `x -> y
`,
		},
		{
			name: "complex",
			in: `
sql_example   :   sql_example   {
board  : {
shape:   sql_table
id: int {constraint: primary_key}
frame: int {constraint: foreign_key}
diagram: int {constraint: foreign_key}
board_objects:   jsonb
last_updated:  		timestamp with time zone
last_thumbgen: timestamp with time zone
dsl				: text
  }

  # Normal.
  board.diagram -> diagrams.id

  # Self referential.
  diagrams.id   -> diagrams.representation

  # SrcArrow test.
  diagrams.id <-   views .  diagram
  diagrams.id <-> steps . diagram

  diagrams: {
    shape: sql_table
    id: {type: int  ; constraint: primary_key}
    representation: {type: jsonb}
  }

  views: {
    shape: sql_table
    id: {type: int; constraint: primary_key}
    representation: {type: jsonb}
    diagram: int {constraint: foreign_key}
}

  steps: 						{
		shape: sql_table
id: {  type: int; constraint: primary_key  }
representation: {  type: jsonb  }
diagram: int {constraint: foreign_key}
  }
  meow <- diagrams.id
}

D2 AST Parser {
  shape: class

     +prevRune  : rune
  prevColumn  : int

  +eatSpace(eatNewlines bool): (rune, error)
    unreadRune()

  		\#scanKey(r rune): (k Key, _ error)
}

"""dmaskkldsamkld """


"""

dmaskdmasl
mdlkasdaskml
daklsmdakms

"""

bs: |
dmasmdkals
dkmsamdklsa
|
bs2: | mdsalldkams|

y-->q: meow
x->y->z

meow: {
x: |` + "`" + `
meow
meow
` + "`" + `| {
}
}


"meow\t": ok
`,
			exp: `sql_example: sql_example {
  board: {
    shape: sql_table
    id: int {constraint: primary_key}
    frame: int {constraint: foreign_key}
    diagram: int {constraint: foreign_key}
    board_objects: jsonb
    last_updated: timestamp with time zone
    last_thumbgen: timestamp with time zone
    dsl: text
  }

  # Normal.
  board.diagram -> diagrams.id

  # Self referential.
  diagrams.id -> diagrams.representation

  # SrcArrow test.
  diagrams.id <- views.diagram
  diagrams.id <-> steps.diagram

  diagrams: {
    shape: sql_table
    id: {type: int; constraint: primary_key}
    representation: {type: jsonb}
  }

  views: {
    shape: sql_table
    id: {type: int; constraint: primary_key}
    representation: {type: jsonb}
    diagram: int {constraint: foreign_key}
  }
  meow <- diagrams.id

  steps: {
    shape: sql_table
    id: {type: int; constraint: primary_key}
    representation: {type: jsonb}
    diagram: int {constraint: foreign_key}
  }
}

D2 AST Parser: {
  shape: class

  +prevRune: rune
  prevColumn: int

  +eatSpace(eatNewlines bool): (rune, error)
  unreadRune()

  \#scanKey(r rune): (k Key, _ error)
}

""" dmaskkldsamkld """

"""

dmaskdmasl
mdlkasdaskml
daklsmdakms

"""

bs: |md
  dmasmdkals
  dkmsamdklsa
|
bs2: |md mdsalldkams |

y -> q: meow
x -> y -> z

meow: {
  x: |` + "`" + `md
    meow
    meow
  ` + "`" + `|
}

"meow\t": ok
`,
		},

		{
			name: "block_comment",
			in: `
"""
D2 AST Parser2: {
  shape: class

  reader: io.RuneReader
  readerPos: d2ast.Position

  lookahead: "[]rune"
  lookaheadPos: d2ast.Position

  peek() (r rune, eof bool)
  -rewind(): ()
  +commit()
  \#peekn(n int) (s string, eof bool)
}
"""
`,
			exp: `"""
D2 AST Parser2: {
  shape: class

  reader: io.RuneReader
  readerPos: d2ast.Position

  lookahead: "[]rune"
  lookaheadPos: d2ast.Position

  peek() (r rune, eof bool)
  -rewind(): ()
  +commit()
  \#peekn(n int) (s string, eof bool)
}
"""
`,
		},

		{
			name: "block_string_indent",
			in: `
parent: {
example_code: |` + "`" + `go
package fs

type FS interface {
	Open(name string) (File, error)
}

type File interface {
	Stat() (FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}

var (
	ErrInvalid    = errInvalid()    // "invalid argument"
	ErrPermission = errPermission() // "permission denied"
	ErrExist      = errExist()      // "file already exists"
	ErrNotExist   = errNotExist()   // "file does not exist"
	ErrClosed     = errClosed()     // "file already closed"
)
` + "`" + `|}`,
			exp: `parent: {
  example_code: |` + "`" + `go
    package fs

    type FS interface {
    	Open(name string) (File, error)
    }

    type File interface {
    	Stat() (FileInfo, error)
    	Read([]byte) (int, error)
    	Close() error
    }

    var (
    	ErrInvalid    = errInvalid()    // "invalid argument"
    	ErrPermission = errPermission() // "permission denied"
    	ErrExist      = errExist()      // "file already exists"
    	ErrNotExist   = errNotExist()   // "file does not exist"
    	ErrClosed     = errClosed()     // "file already closed"
    )
  ` + "`" + `|
}
`,
		},

		{
			// This one we test that the common indent is stripped before the correct indent is
			// applied.
			name: "block_string_indent_2",
			in: `
parent: {
example_code: |` + "`" + `go
	package fs

	type FS interface {
		Open(name string) (File, error)
	}

	type File interface {
		Stat() (FileInfo, error)
		Read([]byte) (int, error)
		Close() error
	}

	var (
		ErrInvalid    = errInvalid()    // "invalid argument"
		ErrPermission = errPermission() // "permission denied"
		ErrExist      = errExist()      // "file already exists"
		ErrNotExist   = errNotExist()   // "file does not exist"
		ErrClosed     = errClosed()     // "file already closed"
	)
` + "`" + `|}`,
			exp: `parent: {
  example_code: |` + "`" + `go
    package fs

    type FS interface {
    	Open(name string) (File, error)
    }

    type File interface {
    	Stat() (FileInfo, error)
    	Read([]byte) (int, error)
    	Close() error
    }

    var (
    	ErrInvalid    = errInvalid()    // "invalid argument"
    	ErrPermission = errPermission() // "permission denied"
    	ErrExist      = errExist()      // "file already exists"
    	ErrNotExist   = errNotExist()   // "file does not exist"
    	ErrClosed     = errClosed()     // "file already closed"
    )
  ` + "`" + `|
}
`,
		},

		{
			// This one we test that the common indent is stripped before the correct indent is
			// applied even when there's too much indent.
			name: "block_string_indent_3",
			in: `
																		parent: {
																		example_code: |` + "`" + `go
																			package fs

																			type FS interface {
																				Open(name string) (File, error)
																			}

																			type File interface {
																				Stat() (FileInfo, error)
																				Read([]byte) (int, error)
																				Close() error
																			}

																			var (
																				ErrInvalid    = errInvalid()    // "invalid argument"
																				ErrPermission = errPermission() // "permission denied"
																				ErrExist      = errExist()      // "file already exists"
																				ErrNotExist   = errNotExist()   // "file does not exist"
																				ErrClosed     = errClosed()     // "file already closed"
																			)
` + "`" + `|}`,
			exp: `parent: {
  example_code: |` + "`" + `go
    package fs

    type FS interface {
    	Open(name string) (File, error)
    }

    type File interface {
    	Stat() (FileInfo, error)
    	Read([]byte) (int, error)
    	Close() error
    }

    var (
    	ErrInvalid    = errInvalid()    // "invalid argument"
    	ErrPermission = errPermission() // "permission denied"
    	ErrExist      = errExist()      // "file already exists"
    	ErrNotExist   = errNotExist()   // "file does not exist"
    	ErrClosed     = errClosed()     // "file already closed"
    )
  ` + "`" + `|
}
`,
		},

		{
			// This one has 3 space indent and whitespace only lines.
			name: "block_string_uneven_indent",
			in: `
parent: {
   example_code: |` + "`" + `go
   	package fs

   	type FS interface {
   		Open(name string) (File, error)
   	}

   	type File interface {
   		Stat() (FileInfo, error)
   		Read([]byte) (int, error)
   		Close() error
   	}

   	var (
   		ErrInvalid    = errInvalid()    // "invalid argument"
   		ErrPermission = errPermission() // "permission denied"
   		ErrExist      = errExist()      // "file already exists"
   		ErrNotExist   = errNotExist()   // "file does not exist"
   		ErrClosed     = errClosed()     // "file already closed"
   	)
` + "`" + `|}`,
			exp: `parent: {
  example_code: |` + "`" + `go
    package fs

    type FS interface {
    	Open(name string) (File, error)
    }

    type File interface {
    	Stat() (FileInfo, error)
    	Read([]byte) (int, error)
    	Close() error
    }

    var (
    	ErrInvalid    = errInvalid()    // "invalid argument"
    	ErrPermission = errPermission() // "permission denied"
    	ErrExist      = errExist()      // "file already exists"
    	ErrNotExist   = errNotExist()   // "file does not exist"
    	ErrClosed     = errClosed()     // "file already closed"
    )
  ` + "`" + `|
}
`,
		},

		{
			// This one has 3 space indent and large whitespace only lines.
			name: "block_string_uneven_indent_2",
			in: `
parent: {
   example_code: |` + "`" + `go
   	package fs

   	type FS interface {
   		Open(name string) (File, error)
   	}

` + "`" + `|}`,
			exp: `parent: {
  example_code: |` + "`" + `go
    package fs

    type FS interface {
    	Open(name string) (File, error)
    }

  ` + "`" + `|
}
`,
		},

		{
			name: "block_comment_indent",
			in: `
parent: {
"""
hello
""" }`,
			exp: `parent: {
  """
  hello
  """
}
`,
		},

		{
			name: "scalars",
			in: `x: null
y: true
z: 343`,
			exp: `x: null
y: true
z: 343
`,
		},

		{
			name: "substitution",
			in:   `x: ${ok}; y: [...${yes}]`,
			exp: `x: ${ok}; y: [...${yes}]
`,
		},

		{
			name: "line_comment_block",
			in: `# wsup
# hello
# The Least Successful Collector`,
			exp: `# wsup
# hello
# The Least Successful Collector
`,
		},

		{
			name: "inline_comment",
			in: `hello: x # soldier
more`,
			exp: `hello: x # soldier
more
`,
		},

		{
			name: "array_one_line",
			in:   `a: [1;2;3;4]`,
			exp: `a: [1; 2; 3; 4]
`,
		},
		{
			name: "array",
			in: `a: [
hi # Fraud is the homage that force pays to reason.
1
2

3
4
5; 6; 7
	]`,
			exp: `a: [
  hi # Fraud is the homage that force pays to reason.
  1
  2

  3
  4
  5
  6
  7
]
`,
		},

		{
			name: "ampersand",
			in:   `&scenario: red`,
			exp: `&scenario: red
`,
		},

		{
			name: "complex_edge",
			in:   `pre.(src -> dst -> more)[3].post`,
			exp: `pre.(src -> dst -> more)[3].post
`,
		},
		{
			name: "edge_index_glob",
			in:   `(x -> y)[*]`,
			exp: `(x -> y)[*]
`,
		},
		{
			name: "bidirectional",
			in:   `x<>y`,
			exp: `x <-> y
`,
		},
		{
			name: "empty_map",
			in: `x: {}
`,
			exp: `x
`,
		},
		{
			name: "leading_space_comments",
			in: `# foo
#   foobar
#     baz
x  ->  y
#foo
y
`,
			exp: `# foo
#   foobar
#     baz
x -> y
# foo
y
`,
		},
		{
			name: "less_than_edge#955",
			in: `
x <= y
`,
			exp: `x <- = y
`,
		},
		{
			name: "import/1",
			in: `
x: @file.d2
`,
			exp: `x: @file
`,
		},
		{
			name: "import/2",
			in: `
x: @file."d2"
`,
			exp: `x: @file."d2"
`,
		},
		{
			name: "import/3",
			in: `
x: @./file
`,
			exp: `x: @file
`,
		},
		{
			name: "import/4",
			in: `
x: @../file
`,
			exp: `x: @../file
`,
		},
		{
			name: "import/4",
			in: `
x: @"x/../file"
`,
			exp: `x: @file
`,
		},
		{
			name: "layers_scenarios_steps_bottom_simple",
			in: `layers: {
  b: {
		e
    scenarios: {
      p: {
        x
      }
    }
  }
	steps: {
		a
	}
}
`,
			exp: `layers: {
  b: {
    e

    scenarios: {
      p: {
        x
      }
    }
  }

  steps: {
    a
  }
}
`,
		},
		{
			name: "layers_scenarios_steps_bottom_complex",
			in: `a

scenarios: {



	scenario-1: {
		steps: {
			step-1: {
        Test
			}
			step-2
		}
		non-step
	}
}

layers: {
  Test super nested: {
		base-layer
		layers: {
			layers: {
				grand-child-layer: {
					grand-child-board
				}
			}
			layer-board
		}
		last-layer
	}
}
b
steps: {
	1: {
		step-1-content
	}
}


scenarios: {
	scenario-2: {
		scenario-2-content
	}
}

c
d

only-layers: {
	layers: {
		X
		Y
	}
	layers: {
		Z
	}
}
`,
			exp: `a
b

c
d

only-layers: {
  layers: {
    X
    Y
  }

  layers: {
    Z
  }
}

layers: {
  Test super nested: {
    base-layer
    last-layer

    layers: {
      layer-board

      layers: {
        grand-child-layer: {
          grand-child-board
        }
      }
    }
  }
}

scenarios: {
  scenario-1: {
    non-step

    steps: {
      step-1: {
        Test
      }
      step-2
    }
  }
}

scenarios: {
  scenario-2: {
    scenario-2-content
  }
}

steps: {
  1: {
    step-1-content
  }
}
`,
		},
		{
			name: "substitution_mid_string",
			in: `vars: {
  test: hello
}

mybox: {
  label: prefix${test}suffix
}
`,
			exp: `vars: {
  test: hello
}

mybox: {
  label: prefix${test}suffix
}
`,
		},
		{
			name: "not-filter",
			in: `jacob: {
	shape: circle
}
jeremy: {
	shape: rectangle
}
*: {
	!&shape: rectangle
	label: I'm not a rectangle
}`,
			exp: `jacob: {
  shape: circle
}
jeremy: {
  shape: rectangle
}
*: {
  !&shape: rectangle
  label: I'm not a rectangle
}
`,
		},
		{
			name: "lowercase-reserved",
			in: `jacob: {
	SHAPE: circle
}
jeremy.SHAPE: rectangle
alice.STYLE.fill: red
bob.style.FILL: red
carmen.STYLE.FILL: red
coop: {
  STYLE: {
    FILL: blue
  }
}
`,
			exp: `jacob: {
  shape: circle
}
jeremy.shape: rectangle
alice.style.fill: red
bob.style.fill: red
carmen.style.fill: red
coop: {
  style: {
    fill: blue
  }
}
`,
		},
		{
			name: "remove-empty-boards",
			in: `k

layers
scenarios: {}
steps: asdf
`,
			exp: `k
`,
		},
		{
			name: "vars",
			in: `vars: {
  a: "a"
  b: "X${a})"
}
`,
			exp: `vars: {
  a: "a"
  b: "X${a})"
}
`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ast, err := d2parser.Parse(fmt.Sprintf("%s.d2", t.Name()), strings.NewReader(tc.in), nil)
			if err != nil {
				t.Fatal(err)
			}
			assert.String(t, tc.exp, d2format.Format(ast))
		})
	}
}

func TestEdge(t *testing.T) {
	t.Parallel()

	mk, err := d2parser.ParseMapKey(`(x -> y)[0]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(mk.Edges) != 1 {
		t.Fatalf("expected one edge: %#v", mk.Edges)
	}

	assert.String(t, `x -> y`, d2format.Format(mk.Edges[0]))
	assert.String(t, `[0]`, d2format.Format(mk.EdgeIndex))
}
