package d2parser_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

// TODO: next step for parser is writing as many tests and grouping them nicely
// TODO: add assertions
// to layout *all* expected behavior.
func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		assert func(t testing.TB, ast *d2ast.Map, err error)

		// exp is in testdata/d2parser/TestParse/${name}.json
	}{

		{
			name: "empty",
			text: ``,
		},
		{
			name: "semicolons",
			text: `;;;;;`,
		},
		{
			name: "bad_curly",
			text: `;;;};;;`,
		},
		{
			name: "one_line_comment",
			text: `
# hello
`,
		},
		{
			name: "multiline_comment",
			text: `

  # hello
# world
# earth
#
#globe
 # very good
   # not so bad
#
#yes indeed
#The good (I am convinced, for one)
#Is but the bad one leaves undone.
#Once your reputation's done
#You can live a life of fun.
#    -- Wilhelm Busch


`,
		},
		{
			name: "one_line_block_comment",
			text: `
""" dmaslkmdlksa """
`,
		},
		{
			name: "block_comment",
			text: `
""" dmaslkmdlksa

dasmlkdas
mkdlasdmkas
  dmsakldmklsadsa

	dsmakldmaslk
	damklsdmklas

	echo hi
x   """

""" ok
meow
"""
`,
		},
		{
			name: "key",
			text: `
x
`,
		},
		{
			name: "edge",
			text: `
x -> y
`,
		},
		{
			name: "multiple_edges",
			text: `
x -> y -> z
`,
		},
		{
			name: "key_with_edge",
			text: `
x.(z->q)
`,
		},
		{
			name: "edge_key",
			text: `
x.(z->q)[343].hola: false
`,
		},
		{
			name: "subst",
			text: `
x -> y: ${meow.ok}
`,
		},
		{
			name: "primary",
			text: `
x -> y: ${meow.ok} {
	label: |
"Hi, I'm Preston A. Mantis, president of Consumers Retail Law Outlet. As you
can see by my suit and the fact that I have all these books of equal height
on the shelves behind me, I am a trained legal attorney. Do you have a car
or a job?  Do you ever walk around?  If so, you probably have the makings of
an excellent legal case.  Although of course every case is different, I
would definitely say that based on my experience and training, there's no
reason why you shouldn't come out of this thing with at least a cabin
cruiser.

"Remember, at the Preston A. Mantis Consumers Retail Law Outlet, our motto
is: 'It is very difficult to disprove certain kinds of pain.'"
		-- Dave Barry, "Pain and Suffering"
|
}
`,
		},
		{
			name: "()_keys",
			text: `
my_fn() -> wowa()
meow.(x -> y -> z)[3].shape: "all hail corn"
`,
		},
		{
			name: "errs",
			text: `
--: meow]]] ` + `
meow][: ok ` + `
ok: "dmsadmakls"    dsamkldkmsa   ` + `
 ` + `
s.shape: orochimaru       ` + `
x.shape: dasdasdas       ` + `

wow:

: ` + `
 ` + `
[]

  {}

"""
wsup
"""

'

meow: ${ok}
meow.(x->)[:
x -> x

x: [][]𐀀𐀀𐀀𐀀𐀀𐀀
`,
		},
		{
			name: "block_string",
			text: `
x: ||
meow
meo
# ok
    code
yes
||
x: || meow
meo
# ok
    code
yes ||

# compat
x: |` + "`" + `
meow
meow
meow
` + "`" + `| {
}
`,
		},
		{
			name: "trailing_whitespace",
			text: `
s.shape: orochimaru       ` + `
`,
		},
		{
			name: "table_and_class",
			text: `
sql_example: sql_example {
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
    id: {type: int, constraint: primary_key}
    representation: {type: jsonb}
  }

  views: {
    shape: sql_table
    id: {type: int, constraint: primary_key}
    representation: {type: jsonb}
    diagram: int {constraint: foreign_key}
  }

  # steps: {
  # shape: sql_table
  # id: {type: int, constraint: primary_key}
  # representation: {type: jsonb}
  # diagram: int {constraint: foreign_key}
  # }
  # Uncomment to make autolayout panic:
  meow <- diagrams.id
}

D2 AST Parser: {
  shape: class

  +prevRune: rune
  prevColumn: int

  +eatSpace(eatNewlines bool): (rune, error)
  unreadRune()

  \#scanKey(r rune): (k Key, _ error)
}
`,
		},
		{
			name: "missing_map_value",
			text: `
x:
			`,
		},
		{
			name: "edge_line_continuation",
			text: `
super long shape id here --\
  -> super long shape id even longer here
		   `,
		},
		{
			name: "edge_line_continuation_2",
			text: `
super long shape id here --\
> super long shape id even longer here
	 `,
		},
		{
			name: "field_line_continuation",
			text: `
meow \
	ok \
		super: yes \
		wow so cool
  \
xd \
\
  ok does it work: hopefully
	 `,
		},
		{
			name: "block_with_delims",
			text: `
a: ||
  |pipe|
||

"""
b: ""
"""
`,
		},
		{
			name: "block_one_line",
			text: `
a: |   hello  |
"""   hello  """
`,
		},
		{
			name: "block_trailing_space",
			text: `
x: |
	meow   ` + `
|
"""   hello    ` + `
"""
`,
		},
		{
			name: "block_edge_case",
			text: `
x: | meow   ` + `
  hello
yes
|
`,
		},
		{
			name: "single_quote_block_string",
			text: `
x: |'
	bs
'|
not part of block string
`,
		},
		{
			name: "edge_group_value",
			text: `
q.(x -> y).z: (rawr)
`,
		},
		{
			name: "less_than_edge#955",
			text: `
x <= y
`,
		},
		{
			name: "merged_shapes_#322",
			text: `
a-
b-
c-
`,
		},
		{
			name: "whitespace_range",
			text: `a -> b -> c`,
			assert: func(t testing.TB, ast *d2ast.Map, err error) {
				assert.Equal(t, "1:1", ast.Nodes[0].MapKey.Edges[0].Src.Range.Start.String())
				assert.Equal(t, "1:2", ast.Nodes[0].MapKey.Edges[0].Src.Range.End.String())
				assert.Equal(t, "1:6", ast.Nodes[0].MapKey.Edges[0].Dst.Range.Start.String())
				assert.Equal(t, "1:7", ast.Nodes[0].MapKey.Edges[0].Dst.Range.End.String())
				assert.Equal(t, "1:6", ast.Nodes[0].MapKey.Edges[1].Dst.Range.Start.String())
				assert.Equal(t, "1:6", ast.Nodes[0].MapKey.Edges[1].Dst.Range.End.String())
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2parser/%v.d2", t.Name())
			ast, err := d2parser.Parse(d2Path, strings.NewReader(tc.text), nil)

			if tc.assert != nil {
				tc.assert(t, ast, err)
			}

			got := struct {
				AST *d2ast.Map `json:"ast"`
				Err error      `json:"err"`
			}{
				AST: ast,
				Err: err,
			}

			err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2parser", t.Name()), got)
			assert.Success(t, err)
		})
	}
}
