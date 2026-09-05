package d2scenebuild

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/patternassets"
)

var (
	benchmarkPaperSource  *paperPatternSource
	benchmarkGrainSource  *grainPatternSource
	benchmarkStreakSource []d2scene.PathCommand
	benchmarkPaperAsset   d2scene.VectorAsset
	benchmarkBytes        []byte
)

func BenchmarkPatternSources(b *testing.B) {
	ctx := context.Background()

	b.Run("Paper/FirstUseParse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			source, err := parsePaperPatternSource(ctx, patternassets.PaperSVG())
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPaperSource = source
		}
	})
	paper, err := parsePaperPatternSource(ctx, patternassets.PaperSVG())
	if err != nil {
		b.Fatal(err)
	}
	if _, err := sharedPaperPatternSource(ctx); err != nil {
		b.Fatal(err)
	}
	b.Run("Paper/CachedLookup", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			source, err := sharedPaperPatternSource(ctx)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPaperSource = source
		}
	})
	b.Run("Paper/CachedDocumentCopy", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			asset, err := newPaperPatternAsset(ctx, paper)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPaperAsset = asset
		}
	})

	b.Run("Grain/FirstUseParse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			source, err := parseGrainPatternSource(ctx, patternassets.GrainPNG())
			if err != nil {
				b.Fatal(err)
			}
			benchmarkGrainSource = source
		}
	})
	grain, err := parseGrainPatternSource(ctx, patternassets.GrainPNG())
	if err != nil {
		b.Fatal(err)
	}
	if _, err := sharedGrainPatternSource(ctx); err != nil {
		b.Fatal(err)
	}
	b.Run("Grain/CachedLookup", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			source, err := sharedGrainPatternSource(ctx)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkGrainSource = source
		}
	})
	b.Run("Grain/CachedDocumentCopy", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkBytes = append([]byte(nil), grain.png...)
		}
	})

	b.Run("Streak/FirstUseParse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			commands, err := parseSketchStreakSource(ctx, patternassets.StreakPathData())
			if err != nil {
				b.Fatal(err)
			}
			benchmarkStreakSource = commands
		}
	})
	commands, err := parseSketchStreakSource(ctx, patternassets.StreakPathData())
	if err != nil {
		b.Fatal(err)
	}
	if _, err := sharedSketchStreakCommands(ctx); err != nil {
		b.Fatal(err)
	}
	b.Run("Streak/CachedLookup", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			commands, err := sharedSketchStreakCommands(ctx)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkStreakSource = commands
		}
	})
	b.Run("Streak/CachedBuilderCopy", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkStreakSource = append([]d2scene.PathCommand(nil), commands...)
		}
	})
}
