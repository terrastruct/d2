package d2graph_test

import (
	"strings"
	"testing"

	"github.com/d2lang/util-go/assert"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2parser"
)

func TestKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  string
		exp  string
	}{
		{
			name: "simple",
			key:  "meow.foo.bar",
			exp:  "meow.foo.bar",
		},
		{
			name: "specials_1",
			key:  `'null.$$$.---'''.",,,.{}{}-\\-><"`,
			exp:  `"null.$$$.---'".",,,.{}{}-\\-><"`,
		},
		{
			name: "specials_2",
			key:  `"&&####;;".| ;;::** |`,
			exp:  `"&&####;;".";;::**"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			k, err := d2parser.ParseKey(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			assert.String(t, tc.exp, strings.Join(d2graph.Key(k), "."))
		})
	}
}

func TestReversedEquivalentEdgeIndexes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		firstSrcArrow    bool
		firstDstArrow    bool
		secondSrcArrow   bool
		secondDstArrow   bool
		secondIndex      int
		query            string
		queryFindsSecond bool
	}{
		{
			name:             "undirected",
			secondIndex:      1,
			query:            "(a -- b)[1]",
			queryFindsSecond: true,
		},
		{
			name:             "bidirectional",
			firstSrcArrow:    true,
			firstDstArrow:    true,
			secondSrcArrow:   true,
			secondDstArrow:   true,
			secondIndex:      1,
			query:            "(a <-> b)[1]",
			queryFindsSecond: true,
		},
		{
			name:             "directed reversed notation",
			firstDstArrow:    true,
			secondSrcArrow:   true,
			secondIndex:      1,
			query:            "(a -> b)[1]",
			queryFindsSecond: true,
		},
		{
			name:           "opposite direction",
			firstDstArrow:  true,
			secondDstArrow: true,
			secondIndex:    0,
			query:          "(a -> b)[1]",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := d2graph.NewGraph()
			a := []d2ast.String{d2ast.FlatUnquotedString("a")}
			b := []d2ast.String{d2ast.FlatUnquotedString("b")}
			_, err := g.Root.Connect(a, b, tc.firstSrcArrow, tc.firstDstArrow, "")
			assert.Success(t, err)
			second, err := g.Root.Connect(b, a, tc.secondSrcArrow, tc.secondDstArrow, "")
			assert.Success(t, err)
			assert.Equal(t, tc.secondIndex, second.Index)

			mk, err := d2parser.ParseMapKey(tc.query)
			assert.Success(t, err)
			got, ok := g.Root.HasEdge(mk)
			assert.Equal(t, tc.queryFindsSecond, ok)
			if tc.queryFindsSecond {
				assert.Equal(t, second, got)
			}
		})
	}
}
