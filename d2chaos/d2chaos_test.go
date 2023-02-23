package d2chaos_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2chaos"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// usage: D2_CHAOS_MAXI=100 D2_CHAOS_N=100 ./ci/test.sh ./d2chaos
//
// D2_CHAOS_MAXI controls the number of iterations that the dsl generator
// should go through to generate each input D2. It's roughly equivalent to
// the complexity level of each input D2.
//
// D2_CHAOS_N controls the number of D2 texts to generate and run the full
// D2 flow on.
//
// All generated texts are stored in ./out/<n>.d2 and also ./out/<n>.d2.goenc
// The goenc version is the text encoded as a Go string. It lets you replay
// a test by adding it to testPinned below as you can just copy paste the go
// string in.
//
// If D2Chaos fails on CI and you need to investigate the input text that caused the
// failure, all generated texts will be available in the d2chaos-test and d2chaos-race
// github actions artifacts.
func TestD2Chaos(t *testing.T) {
	t.Parallel()

	const outDir = "out"
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("writing generated files to %s", outDir)

	t.Run("pinned", func(t *testing.T) {
		testPinned(t, outDir)
	})

	n := 1
	if os.Getenv("D2_CHAOS_N") != "" {
		envn, err := strconv.Atoi(os.Getenv("D2_CHAOS_N"))
		if err != nil {
			t.Errorf("failed to atoi $D2_CHAOS_N: %v", err)
		} else {
			n = envn
		}
	}

	maxi := 10
	if os.Getenv("D2_CHAOS_MAXI") != "" {
		envMaxi, err := strconv.Atoi(os.Getenv("D2_CHAOS_MAXI"))
		if err != nil {
			t.Errorf("failed to atoi $D2_CHAOS_MAXI: %v", err)
		} else {
			maxi = envMaxi
		}
	}

	for i := 0; i < n; i++ {
		i := i
		t.Run("", func(t *testing.T) {
			t.Parallel()

			text, err := d2chaos.GenDSL(maxi)
			if err != nil {
				t.Fatal(err)
			}

			textPath := filepath.Join(outDir, fmt.Sprintf("%d.d2", i))
			test(t, textPath, text)
		})
	}
}

func test(t *testing.T, textPath, text string) {
	t.Logf("writing d2 to %v (%d bytes)", textPath, len(text))
	err := ioutil.WriteFile(textPath, []byte(text), 0644)
	if err != nil {
		t.Fatal(err)
	}

	goencText := fmt.Sprintf("%#v", text)
	t.Logf("writing d2.goenc to %v (%d bytes)", textPath+".goenc", len(goencText))
	err = ioutil.WriteFile(textPath+".goenc", []byte(goencText), 0644)
	if err != nil {
		t.Fatal(err)
	}

	g, err := d2compiler.Compile("", strings.NewReader(text), nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("layout", func(t *testing.T) {
		defer func() {
			r := recover()
			if r != nil {
				t.Errorf("recovered layout engine panic: %#v\n%s", r, debug.Stack())
			}
		}()

		ctx := log.WithTB(context.Background(), t, nil)

		ruler, err := textmeasure.NewRuler()
		assert.Nil(t, err)

		err = g.SetDimensions(nil, ruler, nil)
		assert.Nil(t, err)

		err = d2dagrelayout.DefaultLayout(ctx, g)
		if err != nil {
			t.Fatal(err)
		}

		_, err = d2exporter.Export(ctx, g, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
	})
	// In a random order, delete every object one by one
	t.Run("d2oracle.Delete", func(t *testing.T) {
		key := ""
		var lastAST *d2ast.Map
		defer func() {
			r := recover()
			if r != nil {
				t.Errorf("recovered d2oracle panic deleting %s: %#v\n%s\n%s", key, r, debug.Stack(), d2format.Format(lastAST))
			}
		}()
		rand.Shuffle(len(g.Objects), func(i, j int) {
			g.Objects[i], g.Objects[j] = g.Objects[j], g.Objects[i]
		})
		for _, obj := range g.Objects {
			key = obj.AbsID()
			lastAST = g.AST
			g, err = d2oracle.Delete(g, key)
			if err != nil {
				t.Fatal(fmt.Errorf("Failed to delete %s in\n%s\n: %v", key, d2format.Format(lastAST), err))
			}
		}
	})
	// In a random order, move every nested object one level up
	t.Run("d2oracle.MoveOut", func(t *testing.T) {
		key := ""
		var lastAST *d2ast.Map
		defer func() {
			r := recover()
			if r != nil {
				t.Errorf("recovered d2oracle panic moving out %s: %#v\n%s\n%s", key, r, debug.Stack(), d2format.Format(lastAST))
			}
		}()
		rand.Shuffle(len(g.Objects), func(i, j int) {
			g.Objects[i], g.Objects[j] = g.Objects[j], g.Objects[i]
		})
		for _, obj := range g.Objects {
			if obj.Parent == obj.Graph.Root {
				continue
			}
			key = obj.AbsID()
			lastAST = g.AST
			g, err = d2oracle.Move(g, key, obj.Parent.AbsID()+"."+obj.ID)
			if err != nil {
				t.Fatal(fmt.Errorf("Failed to move %s in\n%s\n: %v", key, d2format.Format(lastAST), err))
			}
		}
	})
	// In a random order, choose one container (if any), and move all objects into that
	t.Run("d2oracle.MoveIn", func(t *testing.T) {
		var container *d2graph.Object
		key := ""
		var lastAST *d2ast.Map
		defer func() {
			r := recover()
			if r != nil {
				t.Errorf("recovered d2oracle panic moving %s into %s: %#v\n%s\n%s", key, container.AbsID(), r, debug.Stack(), d2format.Format(lastAST))
			}
		}()
		// rand.Shuffle(len(g.Objects), func(i, j int) {
		//   g.Objects[i], g.Objects[j] = g.Objects[j], g.Objects[i]
		// })
		for _, obj := range g.Objects {
			if len(obj.ChildrenArray) > 0 {
				container = obj
			}
			if obj.Attributes.Shape.Value == "sequence_diagram" {
				return
			}
		}
		if container == nil {
			return
		}
		for _, obj := range g.Objects {
			if obj == container || obj.Parent == container {
				continue
			}
			key = obj.AbsID()
			lastAST = g.AST
			g, err = d2oracle.Move(g, key, container.AbsID()+"."+obj.ID)
			if err != nil {
				t.Fatal(fmt.Errorf("Failed to move %s into %s in\n%s\n: %v", key, container.AbsID(), d2format.Format(lastAST), err))
			}
		}
	})
}

func testPinned(t *testing.T, outDir string) {
	t.Parallel()

	outDir = filepath.Join(outDir, t.Name())
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("writing generated files to %v", outDir)

	testCases := []struct {
		name string
		text string
	}{
		{
			name: "internal class edge",
			text: "a: {\n shape: class\n  b -> c\n }",
		},
		{
			name: "table edge order",
			text: "B: {shape: sql_table}\n B.C -- D\n B.C -- D\n D -- B.C\n B -- D\n A -- D",
		},
		{
			name: "child to container edge",
			text: "a.b -> a",
		},
		{
			name: "sequence",
			text: "a: {shape: step}\nb: {shape: step}\na -> b\n",
		},
		{
			name: "orientation",
			text: "a: {\n  b\n  c\n }\n  a <- a.c\n  a.b -> a\n",
		},
		{
			name: "cannot create edge between boards",
			text: `"" <-> ""`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			textPath := filepath.Join(outDir, fmt.Sprintf("%s.d2", tc.name))
			test(t, textPath, tc.text)
		})
	}
}
