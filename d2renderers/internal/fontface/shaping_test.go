package fontface

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func TestShapeTextKeepsGraphemeOnFallbackFace(t *testing.T) {
	primary := shapingTestFace(t, "FuzzyBubbles-Regular.ttf")
	fallback := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	const base = '\u0416'
	const mark = '\u0301'
	if supported, err := primary.SupportsRenderableRune(base); err != nil || supported {
		t.Fatalf("primary base coverage = %v/%v, want false/nil", supported, err)
	}
	if supported, err := primary.SupportsRenderableRune(mark); err != nil || !supported {
		t.Fatalf("primary mark coverage = %v/%v, want true/nil", supported, err)
	}
	for _, value := range []rune{base, mark} {
		if supported, err := fallback.SupportsRenderableRune(value); err != nil || !supported {
			t.Fatalf("fallback coverage for %U = %v/%v, want true/nil", value, supported, err)
		}
	}

	// The same acute mark belongs to a primary cluster first and a fallback
	// cluster second. Face selection must therefore be occurrence-specific,
	// rather than a map keyed only by rune value.
	shaped, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), "e\u0301 \u0416\u0301", fixed.I(20), []ShapeFace{
		{ID: "primary", Face: primary},
		{ID: "fallback", Face: fallback},
	}, shapingTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	primaryCluster := false
	fallbackCluster := false
	for _, glyph := range shaped.Glyphs {
		switch {
		case glyph.SourceIndex < 2:
			primaryCluster = true
			if glyph.Face != 0 {
				t.Fatalf("primary grapheme glyph uses face %d: %#v", glyph.Face, glyph)
			}
		case glyph.SourceIndex >= 3:
			fallbackCluster = true
			if glyph.Face != 1 {
				t.Fatalf("fallback grapheme glyph uses face %d: %#v", glyph.Face, glyph)
			}
		}
	}
	if !primaryCluster || !fallbackCluster {
		t.Fatalf("shaped glyphs do not contain both grapheme clusters: %#v", shaped.Glyphs)
	}
}

func TestShapeTextIsBoundedAndCancellable(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	limits := shapingTestLimits()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := new(ShapingWorkspace).ShapeTextTransient(cancelled, "text", fixed.I(12), faces, limits); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ShapeText error = %v", err)
	}

	tooShort := limits
	tooShort.Runes = 1
	if _, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), "ab", fixed.I(12), faces, tooShort); err == nil || !strings.Contains(err.Error(), "rune count") {
		t.Fatalf("rune-limit error = %v", err)
	}

	tooFewGlyphs := limits
	tooFewGlyphs.Glyphs = 1
	if _, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), "ab", fixed.I(12), faces, tooFewGlyphs); err == nil || !strings.Contains(err.Error(), "glyph count") {
		t.Fatalf("glyph-limit error = %v", err)
	}
}

func TestShapeTextUsesDeterministicPlaceholderForMissingScalar(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	var replacement uint32
	var replacementRune rune
	for _, value := range missingGlyphPlaceholderRunes {
		glyph, ok := face.Shaping.NominalGlyph(value)
		if ok && glyph != 0 {
			replacement = uint32(glyph)
			replacementRune = value
			break
		}
	}
	if replacement == 0 {
		t.Fatal("Source Sans Pro has no deterministic placeholder glyph")
	}
	if replacementRune == '?' {
		t.Fatal("Source Sans Pro placeholder unexpectedly fell back to a question mark")
	}
	shaped, err := new(ShapingWorkspace).ShapeTextTransient(
		context.Background(), "\U0010ffff", fixed.I(18),
		[]ShapeFace{{ID: "primary", Face: face}}, shapingTestLimits(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(shaped.Glyphs) != 1 {
		t.Fatalf("missing-scalar glyphs = %#v, want one placeholder", shaped.Glyphs)
	}
	glyph := shaped.Glyphs[0]
	if glyph.ID != replacement || glyph.ID == 0 || !glyph.HasInk || glyph.Advance <= 0 || glyph.Source != '\U0010ffff' || glyph.SourceIndex != 0 {
		t.Fatalf("missing-scalar placeholder = %#v, want drawable %U glyph %d", glyph, replacementRune, replacement)
	}
}

func TestASCIISemanticFastPathMatchesSegmenter(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	texts := []string{"plain ASCII text", "1234!? ()[]{}", "\x00\t\n\r\x7f", "(abc) 123 [xyz]"}
	for value := range utf8.RuneSelf {
		texts = append(texts, string(rune(value)))
	}
	seed := uint32(0x7f4a7c15)
	for range 500 {
		length := 1 + int(seed%96)
		value := make([]byte, length)
		for index := range value {
			seed = seed*1664525 + 1013904223
			value[index] = byte(seed % utf8.RuneSelf)
		}
		texts = append(texts, string(value))
	}

	for _, text := range texts {
		runes := []rune(text)
		input := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: fixed.I(16)}
		var reference shaping.Segmenter
		want := reference.Split(input, oneFaceMap{face: face.Shaping})
		asciiLatin := false
		for _, value := range runes {
			if value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' {
				asciiLatin = true
				break
			}
		}
		var workspace ShapingWorkspace
		got := workspace.splitSemanticInputs(input, face.Shaping, true, asciiLatin)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic split for %q:\n got  %#v\n want %#v", text, got, want)
		}
	}
}

func TestShapingWorkspaceMatchesFreshResultsAcrossReuse(t *testing.T) {
	primary := shapingTestFace(t, "FuzzyBubbles-Regular.ttf")
	fallback := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: primary}, {ID: "fallback", Face: fallback}}
	cases := []struct {
		text string
		size fixed.Int26_6
	}{
		{text: "repeated ASCII text", size: fixed.I(16)},
		{text: "repeated ASCII text", size: fixed.I(16)},
		{text: "e\u0301 \u0416\u0301", size: fixed.I(20)},
		{text: "\u05e9\u05dc\u05d5\u05dd world", size: fixed.I(18)},
		{text: "\U0010ffff", size: fixed.I(18)},
		{text: "repeated ASCII text", size: fixed.I(24)},
	}
	var workspace ShapingWorkspace
	for index, test := range cases {
		want, wantErr := new(ShapingWorkspace).ShapeTextTransient(context.Background(), test.text, test.size, faces, shapingTestLimits())
		got, gotErr := workspace.ShapeTextTransient(context.Background(), test.text, test.size, faces, shapingTestLimits())
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("case %d error = %v, want %v", index, gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d transient result differs:\n got  %#v\n want %#v", index, got, want)
		}
	}
}

func TestOrderShapedRunsLTRMatchesVisualIndexOrdering(t *testing.T) {
	for count := 0; count <= 8; count++ {
		for directions := 0; directions < 1<<count; directions++ {
			runs := make([]shapedFontRun, count)
			visual := make([]int, count)
			for index := range runs {
				runs[index].face = index
				runs[index].output.Direction = di.DirectionLTR
				if directions&(1<<index) != 0 {
					runs[index].output.Direction = di.DirectionRTL
				}
				visual[index] = index
			}
			bidiStart := -1
			reverseVisual := func(from, to int) {
				for left, right := from, to-1; left < right; left, right = left+1, right-1 {
					visual[left], visual[right] = visual[right], visual[left]
				}
			}
			for index := range runs {
				if runs[index].output.Direction == di.DirectionLTR {
					if bidiStart != -1 {
						reverseVisual(bidiStart, index)
						bidiStart = -1
					}
				} else if bidiStart == -1 {
					bidiStart = index
				}
			}
			if bidiStart != -1 {
				reverseVisual(bidiStart, len(runs))
			}
			want := make([]int, count)
			for index := range want {
				want[index] = index
			}
			sort.SliceStable(want, func(left, right int) bool { return visual[want[left]] < visual[want[right]] })

			orderShapedRunsLTR(runs)
			for index, run := range runs {
				if run.face != want[index] {
					t.Fatalf("count=%d directions=%08b run %d face=%d, want %d", count, directions, index, run.face, want[index])
				}
			}
		}
	}
}

func TestShapingWorkspacesDoNotShareResults(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	var workspace ShapingWorkspace
	result, err := workspace.ShapeTextTransient(context.Background(), "independent result", fixed.I(16), faces, shapingTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := result
	want.Glyphs = append([]ShapedGlyph(nil), result.Glyphs...)
	if _, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), "a longer result in another workspace", fixed.I(16), faces, shapingTestLimits()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result changed after another workspace shaped text:\n got  %#v\n want %#v", result, want)
	}
}

func TestShapingWorkspaceClearsMultiFaceScratchBeforeSingleFaceCall(t *testing.T) {
	primary := shapingTestFace(t, "FuzzyBubbles-Regular.ttf")
	fallback := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	var workspace ShapingWorkspace
	if _, err := workspace.ShapeTextTransient(context.Background(), "A\u0416", fixed.I(16), []ShapeFace{
		{ID: "primary", Face: primary}, {ID: "fallback", Face: fallback},
	}, shapingTestLimits()); err != nil {
		t.Fatal(err)
	}
	if len(workspace.inputs) == 0 || len(workspace.faceIndexes) != 2 {
		t.Fatalf("multi-face scratch was not populated: inputs=%d faces=%d", len(workspace.inputs), len(workspace.faceIndexes))
	}
	if _, err := workspace.ShapeTextTransient(context.Background(), "single face", fixed.I(16), []ShapeFace{
		{ID: "fallback", Face: fallback},
	}, shapingTestLimits()); err != nil {
		t.Fatal(err)
	}
	if len(workspace.inputs) != 0 || len(workspace.faceIndexes) != 0 {
		t.Fatalf("single-face call retained multi-face scratch: inputs=%d faces=%d", len(workspace.inputs), len(workspace.faceIndexes))
	}
	for index, input := range workspace.inputs[:cap(workspace.inputs)] {
		if input.Text != nil || input.Face != nil || input.FontFeatures != nil {
			t.Fatalf("retained input %d still owns text or face references: %#v", index, input)
		}
	}
}

func TestShapingWorkspacePreservesCancellationCadenceAfterWarmup(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	limits := shapingTestLimits()
	const text = "repeat repeated ASCII glyph bounds and coverage"
	var workspace ShapingWorkspace
	if _, err := workspace.ShapeTextTransient(context.Background(), text, fixed.I(16), faces, limits); err != nil {
		t.Fatal(err)
	}
	for cancelAt := 1; cancelAt <= 80; cancelAt++ {
		wantContext := &shapeCancelContext{Context: context.Background(), cancelAt: cancelAt}
		want, wantErr := new(ShapingWorkspace).ShapeTextTransient(wantContext, text, fixed.I(16), faces, limits)
		gotContext := &shapeCancelContext{Context: context.Background(), cancelAt: cancelAt}
		got, gotErr := workspace.ShapeTextTransient(gotContext, text, fixed.I(16), faces, limits)
		if !errors.Is(gotErr, context.Canceled) != !errors.Is(wantErr, context.Canceled) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("cancelAt=%d error = %v, want %v", cancelAt, gotErr, wantErr)
		}
		if gotContext.calls != wantContext.calls {
			t.Fatalf("cancelAt=%d Err calls = %d, want %d", cancelAt, gotContext.calls, wantContext.calls)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cancelAt=%d result differs:\n got  %#v\n want %#v", cancelAt, got, want)
		}
	}
}

type shapeCancelContext struct {
	context.Context
	cancelAt int
	calls    int
}

func (c *shapeCancelContext) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func BenchmarkShapeTextRepeatedRunes(b *testing.B) {
	face := shapingTestFace(b, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	limits := shapingTestLimits()
	const text = "repeat repeated runes; repeat repeated runes"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), text, fixed.I(16), faces, limits); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShapeTextWorkspace(b *testing.B) {
	face := shapingTestFace(b, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	limits := shapingTestLimits()
	const text = "repeat repeated runes; repeat repeated runes"
	var shaped ShapedText
	for _, nodes := range []int{1, 100} {
		b.Run(fmt.Sprintf("%dNodes/Stateless", nodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range nodes {
					var err error
					shaped, err = new(ShapingWorkspace).ShapeTextTransient(context.Background(), text, fixed.I(16), faces, limits)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("%dNodes/TransientWorkspace", nodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var workspace ShapingWorkspace
				for range nodes {
					var err error
					shaped, err = workspace.ShapeTextTransient(context.Background(), text, fixed.I(16), faces, limits)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
	benchmarkShapedText = shaped
}

var benchmarkShapedText ShapedText

func shapingTestFace(t testing.TB, filename string) *ParsedFace {
	t.Helper()
	data := testFontData(t, filename)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func shapingTestLimits() ShapeLimits {
	return ShapeLimits{Runes: 1_000, Faces: 8, CoverageChecks: 10_000, Runs: 1_000, Glyphs: 10_000}
}
