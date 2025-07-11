package d2ascii

import (
	"context"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
)

func TestIntegration(t *testing.T) {
	// Start with a simple hardcoded case
	tcs := []testCase{
		{
			name:   "simple",
			script: `a -> b`,
		},
	}

	// Automatically discover all .d2 files in testdata
	testdataDir := "testdata"
	files, err := ioutil.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("Failed to read testdata directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".d2") {
			// Remove .d2 extension for test name
			testName := strings.TrimSuffix(file.Name(), ".d2")
			tcs = append(tcs, testCase{
				name:   testName,
				script: readTestFile(t, file.Name()),
			})
		}
	}

	runIntegrationTests(t, tcs)
}

type testCase struct {
	name   string
	script string
	skip   bool
}

func runIntegrationTests(t *testing.T, tcs []testCase) {
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			t.Parallel()

			runIntegrationTest(t, tc)
		})
	}
}

func runIntegrationTest(t *testing.T, tc testCase) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}

	// Compile the D2 diagram
	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Render as ASCII
	opts := &RenderOpts{
		Pad: nil, // Use default padding
	}
	asciiBytes, err := Render(diagram, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Write to testdata for comparison
	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestIntegration/"))
	pathGotTXT := filepath.Join(dataPath, "ascii.got.txt")

	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = os.WriteFile(pathGotTXT, asciiBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotTXT)

	// Compare against golden file
	err = diff.Testdata(filepath.Join(dataPath, "ascii"), ".txt", asciiBytes)
	assert.Success(t, err)
}

func readTestFile(t *testing.T, filename string) string {
	content, err := ioutil.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filename, err)
	}
	return string(content)
} 