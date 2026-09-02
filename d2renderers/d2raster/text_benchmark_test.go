package d2raster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var (
	benchmarkGlyphPaths    []subpath
	benchmarkTextPrimitive *preparedPrimitive
)

func TestFlattenGlyphArenaMatchesSeparateContourReference(t *testing.T) {
	point := func(x, y int32) fixed.Point26_6 {
		return fixed.Point26_6{X: fixed.Int26_6(x), Y: fixed.Int26_6(y)}
	}
	segment := func(op sfnt.SegmentOp, points ...fixed.Point26_6) sfnt.Segment {
		var result sfnt.Segment
		result.Op = op
		copy(result.Args[:], points)
		return result
	}

	curves := sfnt.Segments{
		segment(sfnt.SegmentOpMoveTo, point(0, 0)),
		segment(sfnt.SegmentOpCubeTo, point(0, 4096), point(4096, -4096), point(4096, 0)),
		segment(sfnt.SegmentOpQuadTo, point(2048, 6144), point(0, 0)),
		segment(sfnt.SegmentOpMoveTo, point(128, 128)),
		segment(sfnt.SegmentOpLineTo, point(128, 128)),
		segment(sfnt.SegmentOpLineTo, point(256, 128)),
	}
	cases := []sfnt.Segments{
		nil,
		{segment(sfnt.SegmentOpMoveTo, point(0, 0))},
		curves,
		{
			segment(sfnt.SegmentOpMoveTo, point(0, 0)),
			segment(sfnt.SegmentOpLineTo, point(64, 0)),
			segment(sfnt.SegmentOpMoveTo, point(128, 0)),
			segment(sfnt.SegmentOpLineTo, point(192, 0)),
		},
	}
	seed := uint32(0x8ac51d37)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed
	}
	for range 200 {
		segments := sfnt.Segments{segment(sfnt.SegmentOpMoveTo, point(int32(next()%4097)-2048, int32(next()%4097)-2048))}
		for range 1 + int(next()%16) {
			op := sfnt.SegmentOp(1 + next()%3)
			if next()%11 == 0 {
				op = sfnt.SegmentOpMoveTo
			}
			arguments := 1
			if op == sfnt.SegmentOpQuadTo {
				arguments = 2
			} else if op == sfnt.SegmentOpCubeTo {
				arguments = 3
			}
			points := make([]fixed.Point26_6, arguments)
			for index := range points {
				points[index] = point(int32(next()%4097)-2048, int32(next()%4097)-2048)
			}
			segments = append(segments, segment(op, points...))
		}
		cases = append(cases, segments)
	}

	for caseIndex, segments := range cases {
		referenceCount := 0
		reference, referenceErr := flattenGlyphBenchmarkBaseline(context.Background(), segments, d2scene.Point{X: 1.25, Y: -2.5}, 0.25, func() error {
			referenceCount++
			return nil
		})
		candidateCount := 0
		candidate, candidateErr := flattenGlyph(context.Background(), segments, d2scene.Point{X: 1.25, Y: -2.5}, 0.25, func() error {
			candidateCount++
			return nil
		})
		if fmt.Sprint(candidateErr) != fmt.Sprint(referenceErr) {
			t.Fatalf("case %d error = %v, want %v", caseIndex, candidateErr, referenceErr)
		}
		if candidateCount != referenceCount {
			t.Fatalf("case %d callback count = %d, want %d", caseIndex, candidateCount, referenceCount)
		}
		if !reflect.DeepEqual(candidate, reference) {
			t.Fatalf("case %d paths differ:\n got  %#v\n want %#v", caseIndex, candidate, reference)
		}
		for pathIndex := range candidate {
			if cap(candidate[pathIndex].points) != len(candidate[pathIndex].points) {
				t.Fatalf("case %d path %d capacity = %d, want length %d", caseIndex, pathIndex, cap(candidate[pathIndex].points), len(candidate[pathIndex].points))
			}
		}
	}

	paths, err := flattenGlyph(context.Background(), curves, d2scene.Point{}, 0.25, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("curve path count = %d, want 2", len(paths))
	}
	second := append([]d2scene.Point(nil), paths[1].points...)
	paths[0].points = append(paths[0].points, d2scene.Point{X: 999, Y: 999})
	if !reflect.DeepEqual(paths[1].points, second) {
		t.Fatalf("appending to first contour changed second: got %#v, want %#v", paths[1].points, second)
	}

	sentinel := errors.New("point limit")
	for limit := 1; limit <= 8; limit++ {
		run := func(flatten func(context.Context, sfnt.Segments, d2scene.Point, float64, func() error) ([]subpath, error)) (int, string) {
			calls := 0
			_, err := flatten(context.Background(), curves, d2scene.Point{}, 0.25, func() error {
				calls++
				if calls == limit {
					return sentinel
				}
				return nil
			})
			return calls, fmt.Sprint(err)
		}
		referenceCalls, referenceErr := run(flattenGlyphBenchmarkBaseline)
		candidateCalls, candidateErr := run(flattenGlyph)
		if candidateCalls != referenceCalls || candidateErr != referenceErr {
			t.Fatalf("limit %d = (%d, %q), want (%d, %q)", limit, candidateCalls, candidateErr, referenceCalls, referenceErr)
		}
	}

	for remaining := 0; remaining <= 16; remaining++ {
		referenceContext := &cancelAfterErrChecks{remaining: remaining}
		_, referenceErr := flattenGlyphBenchmarkBaseline(referenceContext, curves, d2scene.Point{}, 0.25, func() error { return nil })
		candidateContext := &cancelAfterErrChecks{remaining: remaining}
		_, candidateErr := flattenGlyph(candidateContext, curves, d2scene.Point{}, 0.25, func() error { return nil })
		if fmt.Sprint(candidateErr) != fmt.Sprint(referenceErr) || candidateContext.remaining != referenceContext.remaining {
			t.Fatalf("cancellation after %d checks = (%v, %d), want (%v, %d)", remaining, candidateErr, candidateContext.remaining, referenceErr, referenceContext.remaining)
		}
	}
}

func flattenGlyphBenchmarkBaseline(ctx context.Context, segments sfnt.Segments, origin d2scene.Point, tolerance float64, count func() error) ([]subpath, error) {
	var paths []subpath
	var current subpath
	var cursor d2scene.Point
	haveCursor := false
	flush := func() {
		if len(current.points) != 0 {
			current.closed = true
			paths = append(paths, current)
		}
		current = subpath{}
	}
	appendPoint := func(point d2scene.Point) error {
		if !finitePoint(point) {
			return fmt.Errorf("non-finite point")
		}
		if err := count(); err != nil {
			return err
		}
		if len(current.points) == 0 || !samePoint(current.points[len(current.points)-1], point) {
			current.points = append(current.points, point)
		}
		return nil
	}
	point := func(value fixed.Point26_6) d2scene.Point {
		return d2scene.Point{X: origin.X + fixedToFloat(value.X), Y: origin.Y + fixedToFloat(value.Y)}
	}

	for index, segment := range segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			flush()
			cursor = point(segment.Args[0])
			if err := appendPoint(cursor); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			haveCursor = true
		case sfnt.SegmentOpLineTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: line before move", index)
			}
			cursor = point(segment.Args[0])
			if err := appendPoint(cursor); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
		case sfnt.SegmentOpQuadTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: quadratic before move", index)
			}
			control, end := point(segment.Args[0]), point(segment.Args[1])
			if err := flattenQuadratic(ctx, cursor, control, end, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			cursor = end
		case sfnt.SegmentOpCubeTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: cubic before move", index)
			}
			control1, control2, end := point(segment.Args[0]), point(segment.Args[1]), point(segment.Args[2])
			if err := flattenCubic(ctx, cursor, control1, control2, end, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			cursor = end
		default:
			return nil, fmt.Errorf("segment %d: unknown operation %d", index, segment.Op)
		}
	}
	flush()
	return paths, nil
}

func BenchmarkFlattenGlyph(b *testing.B) {
	point := func(x, y int) fixed.Point26_6 {
		return fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	}
	segment := func(op sfnt.SegmentOp, points ...fixed.Point26_6) sfnt.Segment {
		var result sfnt.Segment
		result.Op = op
		copy(result.Args[:], points)
		return result
	}
	square := func(x, y, size int) sfnt.Segments {
		return sfnt.Segments{
			segment(sfnt.SegmentOpMoveTo, point(x, y)),
			segment(sfnt.SegmentOpLineTo, point(x+size, y)),
			segment(sfnt.SegmentOpLineTo, point(x+size, y+size)),
			segment(sfnt.SegmentOpLineTo, point(x, y+size)),
		}
	}

	simple := square(0, 0, 16)
	var multi sfnt.Segments
	for i := range 8 {
		multi = append(multi, square((i%4)*20, (i/4)*20, 16)...)
	}
	curves := sfnt.Segments{
		segment(sfnt.SegmentOpMoveTo, point(0, 0)),
		segment(sfnt.SegmentOpCubeTo, point(0, 64), point(64, -64), point(64, 0)),
		segment(sfnt.SegmentOpCubeTo, point(64, 64), point(128, -64), point(128, 0)),
		segment(sfnt.SegmentOpQuadTo, point(96, 96), point(64, 0)),
		segment(sfnt.SegmentOpCubeTo, point(32, -96), point(0, 64), point(0, 0)),
	}
	flatQuadratic := sfnt.Segments{
		segment(sfnt.SegmentOpMoveTo, point(0, 0)),
		segment(sfnt.SegmentOpQuadTo, point(8, 0), point(16, 0)),
	}
	curvedQuadratic := sfnt.Segments{
		segment(sfnt.SegmentOpMoveTo, point(0, 0)),
		segment(sfnt.SegmentOpQuadTo, point(8, 16), point(16, 0)),
	}

	for _, benchmark := range []struct {
		name     string
		segments sfnt.Segments
	}{
		{name: "Simple", segments: simple},
		{name: "MultiContour", segments: multi},
		{name: "FlatQuadratic", segments: flatQuadratic},
		{name: "CurvedQuadratic", segments: curvedQuadratic},
		{name: "CurveHeavy", segments: curves},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			for _, implementation := range []struct {
				name    string
				flatten func(context.Context, sfnt.Segments, d2scene.Point, float64, func() error) ([]subpath, error)
			}{
				{name: "Baseline", flatten: flattenGlyphBenchmarkBaseline},
				{name: "Optimized", flatten: flattenGlyph},
			} {
				b.Run(implementation.name, func(b *testing.B) {
					count := func() error { return nil }
					b.ReportAllocs()
					for range b.N {
						paths, err := implementation.flatten(context.Background(), benchmark.segments, d2scene.Point{}, 0.25, count)
						if err != nil {
							b.Fatal(err)
						}
						benchmarkGlyphPaths = paths
					}
				})
			}
		})
	}
}

func BenchmarkFlattenBundledGlyph(b *testing.B) {
	data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		b.Fatal("Source Sans Pro is not bundled")
	}
	parsed, err := sfnt.Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	for _, benchmark := range []struct {
		name string
		rune rune
	}{
		{name: "I", rune: 'I'},
		{name: "B", rune: 'B'},
		{name: "S", rune: 'S'},
		{name: "Ampersand", rune: '&'},
		{name: "At", rune: '@'},
	} {
		glyph, err := parsed.GlyphIndex(nil, benchmark.rune)
		if err != nil {
			b.Fatal(err)
		}
		segments, err := parsed.LoadGlyph(nil, glyph, fixed.I(32), nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(benchmark.name, func(b *testing.B) {
			for _, implementation := range []struct {
				name    string
				flatten func(context.Context, sfnt.Segments, d2scene.Point, float64, func() error) ([]subpath, error)
			}{
				{name: "Baseline", flatten: flattenGlyphBenchmarkBaseline},
				{name: "Optimized", flatten: flattenGlyph},
			} {
				b.Run(implementation.name, func(b *testing.B) {
					count := func() error { return nil }
					b.ReportAllocs()
					for range b.N {
						paths, err := implementation.flatten(context.Background(), segments, d2scene.Point{}, 0.25, count)
						if err != nil {
							b.Fatal(err)
						}
						benchmarkGlyphPaths = paths
					}
				})
			}
		})
	}
}

func BenchmarkPrepareText(b *testing.B) {
	fontData := func(font d2fonts.Font) []byte {
		data, ok := d2fonts.FontFaces.Lookup(font)
		if !ok {
			b.Fatalf("font %+v is not bundled", font)
		}
		return data
	}
	regular, err := parsePreparedFont(fontData(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR}), 0)
	if err != nil {
		b.Fatal(err)
	}
	handDrawn, err := parsePreparedFont(fontData(d2fonts.Font{Family: d2fonts.HandDrawn, Style: d2fonts.FONT_STYLE_REGULAR}), 0)
	if err != nil {
		b.Fatal(err)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 500, Height: 100}, d2scene.NewNode(nil))
	options := testOptions()
	options.MaxFontFacesPerText = 8
	options.MaxTextCoverageChecks = 100_000
	options.MaxTextShapingRuns = 10_000
	for _, benchmark := range []struct {
		name  string
		text  d2scene.TextRun
		fonts map[d2scene.AssetID]*preparedFont
	}{
		{
			name: "SingleFontMonochrome",
			text: d2scene.TextRun{
				Text: "Deterministic D2 raster renderer", Origin: d2scene.Point{X: 10, Y: 40},
				Font: d2scene.Font{Asset: "regular", Size: 28}, Fill: black,
			},
			fonts: map[d2scene.AssetID]*preparedFont{"regular": regular},
		},
		{
			name: "SingleFontMeasuredInk",
			text: d2scene.TextRun{
				Text: "Deterministic D2 raster renderer", Origin: d2scene.Point{X: 10, Y: 40},
				Font: d2scene.Font{Asset: "regular", Size: 28}, Fill: black,
				Ink: d2scene.NewBounds(10, 12, 430, 44),
			},
			fonts: map[d2scene.AssetID]*preparedFont{"regular": regular},
		},
		{
			name: "SingleFontDecorated",
			text: d2scene.TextRun{
				Text: "Deterministic D2 raster renderer", Origin: d2scene.Point{X: 10, Y: 40},
				Font: d2scene.Font{Asset: "regular", Size: 28}, Fill: black, Underline: true, Strike: true,
			},
			fonts: map[d2scene.AssetID]*preparedFont{"regular": regular},
		},
		{
			name: "MixedFontMonochrome",
			text: d2scene.TextRun{
				Text: "AЖAЖAЖAЖ", Origin: d2scene.Point{X: 10, Y: 40},
				Font: d2scene.Font{Asset: "hand", Size: 28}, Fallbacks: []d2scene.AssetID{"regular"}, Fill: black,
			},
			fonts: map[d2scene.AssetID]*preparedFont{"hand": handDrawn, "regular": regular},
		},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				preflight := &preflight{
					ctx: context.Background(), document: document, options: options,
					fonts: benchmark.fonts,
				}
				primitive, err := preflight.text("benchmark", benchmark.text, d2scene.Identity(), animationOverrides{}, 0)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkTextPrimitive = primitive
			}
		})
	}
}
