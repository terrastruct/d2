//go:build !js || !wasm

package fontface

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"

	gotextfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/font/sfnt"
)

func TestBundledNotoColorEmojiCOLRv1PlansStayWithinTrustedProfile(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	limits := bundledNotoColorEmojiCOLRv1Limits()
	if !face.IsBundledNotoColorEmoji() {
		t.Fatal("parsed Noto face lacks bundled-source provenance")
	}
	var buffer sfnt.Buffer
	wantGlyphs := []rune{'😀', '✈', '✅'}
	for _, value := range wantGlyphs {
		glyph, err := face.Outline.GlyphIndex(&buffer, value)
		if err != nil || glyph == 0 {
			t.Fatalf("glyph index for %U = %d, %v", value, glyph, err)
		}
		plan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyph))
		if err != nil || !found || plan == nil || plan.Clip == nil {
			t.Fatalf("COLRv1 plan for %U = %#v/%v/%v", value, plan, found, err)
		}
		supported, err := face.SupportsRenderableRune(value)
		if err != nil || !supported {
			t.Fatalf("trusted COLRv1 coverage for %U = %v, %v", value, supported, err)
		}
		bounds, hasInk, err := face.GlyphRenderBounds(uint32(glyph), 64*64)
		if err != nil || !hasInk || bounds.Empty() {
			t.Fatalf("trusted COLRv1 clip bounds for %U = %#v/%v/%v", value, bounds, hasInk, err)
		}
	}
	untrusted := *face
	untrusted.Shaping = gotextfont.NewFace(face.Shaping.Font)
	glyph, err := face.Outline.GlyphIndex(&buffer, '😀')
	if err != nil || glyph == 0 {
		t.Fatalf("untrusted test glyph = %d, %v", glyph, err)
	}
	if supported, err := untrusted.SupportsRenderableRune('😀'); err == nil || supported {
		t.Fatalf("untrusted empty-outline COLRv1 coverage = %v, %v", supported, err)
	}
	if _, _, err := untrusted.GlyphRenderBounds(uint32(glyph), 64*64); err == nil {
		t.Fatal("untrusted empty-outline COLRv1 bounds unexpectedly succeeded")
	}

	types := make(map[string]int)
	sourceTypes := make(map[string]int)
	modes := make(map[COLRv1CompositeMode]int)
	var roots, maxNodes, maxDepth, maxLayers, maxStops int
	var nodeGlyph, depthGlyph, layerGlyph, stopGlyph uint32
	for glyph := 1; glyph < face.Outline.NumGlyphs(); glyph++ {
		sourcePaint, sourceFound, err := searchCOLRPaint(face.Shaping.COLR, tables.GlyphID(glyph))
		if err != nil {
			t.Fatalf("source glyph %d: %v", glyph, err)
		}
		if sourceFound {
			walkSourceCOLRv1Profile(t, face, sourcePaint, sourceTypes)
		}
		plan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyph))
		if err != nil {
			t.Fatalf("compile glyph %d: %v", glyph, err)
		}
		if !found {
			continue
		}
		roots++
		if plan.Clip == nil {
			t.Fatalf("glyph %d has no static COLRv1 clip box", glyph)
		}
		assertCOLRv1Stops(t, plan.Root)
		walkCOLRv1Plan(plan.Root, types, modes)
		if plan.Usage.PaintNodes > maxNodes {
			maxNodes, nodeGlyph = plan.Usage.PaintNodes, uint32(glyph)
		}
		if plan.Usage.MaxDepth > maxDepth {
			maxDepth, depthGlyph = plan.Usage.MaxDepth, uint32(glyph)
		}
		if plan.Usage.MaxLayers > maxLayers {
			maxLayers, layerGlyph = plan.Usage.MaxLayers, uint32(glyph)
		}
		if plan.Usage.MaxGradientStops > maxStops {
			maxStops, stopGlyph = plan.Usage.MaxGradientStops, uint32(glyph)
		}
	}
	if roots != 3_993 || maxNodes != 6_665 || maxDepth != 10 || maxLayers != 255 || maxStops != 13 {
		t.Fatalf("COLRv1 profile roots/nodes/depth/layers/stops = %d/%d/%d/%d/%d (glyphs %d/%d/%d/%d)", roots, maxNodes, maxDepth, maxLayers, maxStops, nodeGlyph, depthGlyph, layerGlyph, stopGlyph)
	}
	for _, kind := range []string{"layers", "solid", "linear", "radial", "glyph", "transform", "composite"} {
		if types[kind] == 0 {
			t.Fatalf("COLRv1 did not compile %s paint", kind)
		}
	}
	if modes[COLRv1CompositeSrcIn] == 0 || modes[COLRv1CompositeSoftLight] == 0 || len(modes) != 2 {
		t.Fatalf("COLRv1 composite modes = %#v", modes)
	}
	wantSourceTypes := map[string]int{
		"layers": 14_565, "solid": 111_668, "linear": 8_627,
		"radial": 9_844, "glyph": 129_823, "reference": 0,
		"transform": 33_998, "translate": 116, "scale": 1_845,
		"scale-center": 22, "composite": 578,
	}
	for kind, want := range wantSourceTypes {
		if got := sourceTypes[kind]; got != want {
			t.Fatalf("pinned COLRv1 source %s count = %d, want %d; profile = %#v", kind, got, want, sourceTypes)
		}
	}
	if len(sourceTypes) != len(wantSourceTypes)-1 { // The zero-count reference key is intentionally absent.
		t.Fatalf("pinned COLRv1 source has unexpected paint forms: %#v", sourceTypes)
	}
	if limits.MaxPaintNodes <= maxNodes || limits.MaxDepth <= maxDepth || limits.MaxLayers <= maxLayers || limits.MaxGradientStops <= maxStops {
		t.Fatalf("production limits %#v do not leave source-profile headroom", limits)
	}
}

func walkSourceCOLRv1Profile(t *testing.T, face *ParsedFace, paint tables.PaintTable, counts map[string]int) {
	t.Helper()
	switch paint := paint.(type) {
	case tables.PaintColrLayers:
		counts["layers"]++
		children, err := face.Shaping.COLR.LayerList.Resolve(paint)
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range children {
			walkSourceCOLRv1Profile(t, face, child, counts)
		}
	case tables.PaintSolid:
		counts["solid"]++
	case tables.PaintLinearGradient:
		counts["linear"]++
	case tables.PaintRadialGradient:
		counts["radial"]++
	case tables.PaintGlyph:
		counts["glyph"]++
		walkSourceCOLRv1Profile(t, face, paint.Paint, counts)
	case tables.PaintColrGlyph:
		counts["reference"]++
	case tables.PaintTransform:
		counts["transform"]++
		walkSourceCOLRv1Profile(t, face, paint.Paint, counts)
	case tables.PaintTranslate:
		counts["translate"]++
		walkSourceCOLRv1Profile(t, face, paint.Paint, counts)
	case tables.PaintScale:
		counts["scale"]++
		walkSourceCOLRv1Profile(t, face, paint.Paint, counts)
	case tables.PaintScaleAroundCenter:
		counts["scale-center"]++
		walkSourceCOLRv1Profile(t, face, paint.Paint, counts)
	case tables.PaintComposite:
		counts["composite"]++
		walkSourceCOLRv1Profile(t, face, paint.BackdropPaint, counts)
		walkSourceCOLRv1Profile(t, face, paint.SourcePaint, counts)
	default:
		counts[fmt.Sprintf("unsupported:%T", paint)]++
	}
}

func TestBundledNotoColorEmojiCOLRv1FixedLimitsAndTypedRejections(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	limits := bundledNotoColorEmojiCOLRv1Limits()
	forged := &ParsedFace{Outline: face.Outline, Shaping: face.Shaping}
	if _, _, err := forged.CompileBundledNotoColorEmojiCOLRv1Plan(3811); err == nil {
		t.Fatal("synthesized COLRv1 face unexpectedly succeeded")
	} else {
		var target COLRv1UntrustedFontError
		if !errors.As(err, &target) {
			t.Fatalf("untrusted error = %T %v", err, err)
		}
	}

	for _, limit := range []struct {
		name   string
		update func(*trustedCOLRv1Limits)
	}{
		{"nodes", func(p *trustedCOLRv1Limits) { p.MaxPaintNodes = 6_664 }},
		{"depth", func(p *trustedCOLRv1Limits) { p.MaxDepth = 9 }},
		{"layers", func(p *trustedCOLRv1Limits) { p.MaxLayers = 254 }},
		{"stops", func(p *trustedCOLRv1Limits) { p.MaxGradientStops = 12 }},
	} {
		t.Run(limit.name, func(t *testing.T) {
			limited := limits
			limit.update(&limited)
			foundLimit := false
			for glyph := 1; glyph < face.Outline.NumGlyphs(); glyph++ {
				_, _, err := face.compileBundledNotoColorEmojiCOLRv1Plan(uint32(glyph), limited)
				var target *COLRv1LimitError
				if errors.As(err, &target) {
					foundLimit = true
					break
				}
				if err != nil {
					t.Fatalf("unexpected error for glyph %d: %v", glyph, err)
				}
			}
			if !foundLimit {
				t.Fatal("no COLRv1 plan reached the reduced limit")
			}
		})
	}

	b := colrv1Builder{face: face, limits: limits}
	if _, err := b.paint(tables.PaintVarSolid{}, 1); err == nil {
		t.Fatal("variable paint unexpectedly compiled")
	} else {
		var target *COLRv1UnsupportedPaintError
		if !errors.As(err, &target) {
			t.Fatalf("unsupported paint error = %T %v", err, err)
		}
	}
	if _, err := b.paint(tables.PaintColrGlyph{GlyphID: 1}, 1); err == nil {
		t.Fatal("COLR glyph reference unexpectedly compiled")
	} else {
		var target *COLRv1UnsupportedPaintError
		if !errors.As(err, &target) {
			t.Fatalf("COLR glyph reference error = %T %v", err, err)
		}
	}
	if _, err := colrv1CompositeMode(tables.CompositeColorBurn); err == nil {
		t.Fatal("unsupported composite unexpectedly compiled")
	} else {
		var target *COLRv1UnsupportedCompositeError
		if !errors.As(err, &target) {
			t.Fatalf("unsupported composite error = %T %v", err, err)
		}
	}
	if _, err := b.colorLine(tables.ColorLine{
		Extend: tables.ExtendRepeat,
		ColorStops: []tables.ColorStop{{
			StopOffset: 0, PaletteIndex: 0, Alpha: tables.Fixed214(1 << 14),
		}},
	}); err == nil {
		t.Fatal("repeat gradient unexpectedly compiled")
	} else {
		var target *COLRv1UnsupportedExtendError
		if !errors.As(err, &target) {
			t.Fatalf("unsupported extend error = %T %v", err, err)
		}
	}
}

func TestBundledNotoColorEmojiCOLRv1CompilationIgnoresCallerMutation(t *testing.T) {
	const childEnvironment = "D2_TEST_NOTO_CALLER_MUTATION"
	if os.Getenv(childEnvironment) == "" {
		command := exec.Command(os.Args[0], "-test.run=^TestBundledNotoColorEmojiCOLRv1CompilationIgnoresCallerMutation$")
		command.Env = append(os.Environ(), childEnvironment+"=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("fresh-process mutation regression failed: %v\n%s", err, output)
		}
		return
	}

	// Keep this as the first authenticated parse in the child process. ParseFace
	// must register a private copy even when these exact bytes came from an
	// external resolver rather than d2fonts' bundled loader.
	data := append([]byte(nil), bundledNotoColorEmojiForTest(t)...)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buffer sfnt.Buffer
	glyph, err := face.Outline.GlyphIndex(&buffer, '😀')
	if err != nil || glyph == 0 {
		t.Fatalf("emoji glyph = %d, %v", glyph, err)
	}
	officialPlan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyph))
	if err != nil || !found || officialPlan == nil {
		t.Fatalf("compile before caller mutation = %#v/%v/%v", officialPlan, found, err)
	}

	face.Shaping.Font.COLR = nil
	face.Shaping.Font.CPAL = nil
	for index := range data {
		data[index] ^= 0xff
	}
	if !face.IsBundledNotoColorEmoji() {
		t.Fatal("caller mutation discarded parser-issued source provenance")
	}
	mutatedPlan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyph))
	if err != nil || !found || mutatedPlan == nil {
		t.Fatalf("compile with caller-owned source and tables mutated = %#v/%v/%v", mutatedPlan, found, err)
	}
	if !reflect.DeepEqual(mutatedPlan, officialPlan) {
		t.Fatal("caller-owned source or table mutation changed the private bundled plan")
	}
}

func assertCOLRv1Stops(t *testing.T, paint COLRv1Paint) {
	t.Helper()
	check := func(line COLRv1ColorLine) {
		previous := -1.0
		for index, stop := range line.Stops {
			if stop.Offset < 0 || stop.Offset > 1 {
				t.Fatalf("COLRv1 stop %d offset %g is outside [0,1]", index, stop.Offset)
			}
			if stop.Offset < previous {
				t.Fatalf("COLRv1 stop %d offset %g is less than preceding %g", index, stop.Offset, previous)
			}
			previous = stop.Offset
		}
	}
	switch value := paint.(type) {
	case COLRv1Layers:
		for _, child := range value.Paints {
			assertCOLRv1Stops(t, child)
		}
	case COLRv1Glyph:
		assertCOLRv1Stops(t, value.Paint)
	case COLRv1Transform:
		assertCOLRv1Stops(t, value.Paint)
	case COLRv1Composite:
		assertCOLRv1Stops(t, value.Backdrop)
		assertCOLRv1Stops(t, value.Source)
	case COLRv1LinearGradient:
		check(value.ColorLine)
	case COLRv1RadialGradient:
		check(value.ColorLine)
	}
}

func walkCOLRv1Plan(paint COLRv1Paint, types map[string]int, modes map[COLRv1CompositeMode]int) {
	switch value := paint.(type) {
	case COLRv1Layers:
		types["layers"]++
		for _, child := range value.Paints {
			walkCOLRv1Plan(child, types, modes)
		}
	case COLRv1Solid:
		types["solid"]++
	case COLRv1LinearGradient:
		types["linear"]++
	case COLRv1RadialGradient:
		types["radial"]++
	case COLRv1Glyph:
		types["glyph"]++
		walkCOLRv1Plan(value.Paint, types, modes)
	case COLRv1Transform:
		types["transform"]++
		walkCOLRv1Plan(value.Paint, types, modes)
	case COLRv1Composite:
		types["composite"]++
		modes[value.Mode]++
		walkCOLRv1Plan(value.Backdrop, types, modes)
		walkCOLRv1Plan(value.Source, types, modes)
	default:
		panic("unexpected COLRv1 paint")
	}
}
