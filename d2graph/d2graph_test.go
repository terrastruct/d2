package d2graph_test

import (
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2parser"
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
