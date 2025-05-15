package d2chaos_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2chaos"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
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
	err := os.WriteFile(textPath, []byte(text), 0644)
	if err != nil {
		t.Fatal(err)
	}

	goencText := fmt.Sprintf("%#v", text)
	t.Logf("writing d2.goenc to %v (%d bytes)", textPath+".goenc", len(goencText))
	err = os.WriteFile(textPath+".goenc", []byte(goencText), 0644)
	if err != nil {
		t.Fatal(err)
	}

	g, _, err := d2compiler.Compile("", strings.NewReader(text), nil)
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

		ctx := log.WithTB(context.Background(), t)

		ruler, err := textmeasure.NewRuler()
		assert.Nil(t, err)

		err = g.SetDimensions(nil, ruler, nil)
		assert.Nil(t, err)

		err = d2dagrelayout.DefaultLayout(ctx, g)
		if err != nil {
			t.Fatal(err)
		}

		_, err = d2exporter.Export(ctx, g, nil)
		if err != nil {
			t.Fatal(err)
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
