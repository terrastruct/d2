package d2fonts

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func TestSystemFallbackResolverUsesBoundedOwnedFontBytes(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	path := filepath.Join(root, "SourceSansPro-Regular.ttf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{
		limits: SystemFallbackLimits{
			MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 10,
			MaxCoverageChecks: 10, MaxResolvedBytes: int64(len(data)),
			MaxFileBytes: int64(len(data)), MaxScannedBytes: int64(len(data)),
		},
		roots: []string{root},
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u0416', '\u0416'}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 1 || fonts[0].Name != filepath.Base(path) || fonts[0].FaceIndex != 0 || fonts[0].MIMEType != "font/ttf" || !bytes.Equal(fonts[0].Data, data) {
		t.Fatalf("resolved fonts = %#v", fonts)
	}
	fonts[0].Data[0] ^= 0xff
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, data) {
		t.Fatal("resolved bytes alias the source font file")
	}
}

func TestSystemFallbackResolverErrorsAreBoundedAndActionable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "not-a-font.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	limits := SystemFallbackLimits{
		MaxDirectoryEntries: 1, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 10,
		MaxCoverageChecks: 10, MaxResolvedBytes: 1024,
		MaxFileBytes: 1024, MaxScannedBytes: 1024,
	}
	resolver := &SystemFallbackResolver{limits: limits, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u4e2d'}}); err == nil || !strings.Contains(err.Error(), "directory entries exceed limit") {
		t.Fatalf("directory limit error = %v", err)
	}

	limits.MaxDirectoryEntries = 10
	resolver = &SystemFallbackResolver{limits: limits, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u4e2d'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("missing font result/error = %#v/%v", fonts, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveFallbacks(ctx, FallbackRequest{Runes: []rune{'A'}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestSystemFallbackResolverRejectsOversizeColorEmojiExplicitly(t *testing.T) {
	root := t.TempDir()
	// The candidate deliberately exceeds MaxFileBytes, just as Apple Color
	// Emoji does under the CLI policy. The resolver must stop before an
	// irrelevant generic-font scan and describe the supported remedy.
	if err := os.WriteFile(filepath.Join(root, "Apple Color Emoji.ttf"), []byte("oversize"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{
		limits: SystemFallbackLimits{
			MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 10,
			MaxCoverageChecks: 10, MaxResolvedBytes: 1,
			MaxFileBytes: 1, MaxScannedBytes: 1,
		},
		roots: []string{root},
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("unsupported emoji result/error = %#v/%v", fonts, err)
	}
}

func TestNewSystemFallbackResolverRequiresEveryLimit(t *testing.T) {
	if _, err := NewSystemFallbackResolver(SystemFallbackLimits{}); err == nil {
		t.Fatal("zero limits accepted")
	}
	limits := SystemFallbackLimits{MaxDirectoryEntries: 1, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 1, MaxCoverageChecks: 1, MaxFileBytes: 1, MaxScannedBytes: 1, MaxResolvedBytes: 1}
	if _, err := NewSystemFallbackResolver(limits); err != nil {
		t.Fatal(err)
	}
}

func TestSystemFallbackResolverBoundsInputCoverageAndOutput(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "custom.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	limits := SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 2,
		MaxCoverageChecks: 10, MaxFileBytes: int64(len(data)), MaxScannedBytes: int64(len(data)), MaxResolvedBytes: int64(len(data)),
	}

	resolver := &SystemFallbackResolver{limits: limits, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A', 'B', 'C'}}); err == nil || !strings.Contains(err.Error(), "requested fallback rune count") {
		t.Fatalf("requested-rune limit error = %v", err)
	}

	coverageLimited := limits
	coverageLimited.MaxCoverageChecks = 1
	resolver = &SystemFallbackResolver{limits: coverageLimited, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A', 'B'}}); err == nil || !strings.Contains(err.Error(), "coverage checks exceed limit") {
		t.Fatalf("coverage limit error = %v", err)
	}

	outputLimited := limits
	outputLimited.MaxResolvedBytes = int64(len(data) - 1)
	resolver = &SystemFallbackResolver{limits: outputLimited, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}}); err == nil || !strings.Contains(err.Error(), "resolved fallback font bytes exceed limit") {
		t.Fatalf("resolved-byte limit error = %v", err)
	}
}

func TestSystemFallbackResolverDoesNotTrustFilenameForEmojiCoverage(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	face, err := fontface.ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var symbol rune
	for value := rune(0x2600); value <= 0x27ff; value++ {
		supported, err := face.SupportsRenderableRune(value)
		if err != nil {
			t.Fatal(err)
		}
		if unicode.Is(unicode.So, value) && supported {
			symbol = value
			break
		}
	}
	if symbol == 0 {
		t.Fatal("embedded Source Sans Pro has no likely-emoji test glyph")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "generic-custom-name.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 1,
		MaxCoverageChecks: 1, MaxFileBytes: int64(len(data)), MaxScannedBytes: int64(len(data)), MaxResolvedBytes: int64(len(data)),
	}, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{symbol}})
	if err != nil || len(fonts) != 1 {
		t.Fatalf("generic-name symbol fallback for %U = %#v, %v", symbol, fonts, err)
	}
}

func TestSystemFallbackResolverIgnoresSymlinkCandidates(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	outside := filepath.Join(t.TempDir(), "outside.ttf")
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked.ttf")); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 1,
		MaxCoverageChecks: 1, MaxFileBytes: int64(len(data)), MaxScannedBytes: int64(len(data)), MaxResolvedBytes: int64(len(data)),
	}, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("symlink candidate result/error = %#v/%v", fonts, err)
	}
	if handle, err := openVerifiedDirectory(outside); err == nil || handle != nil {
		t.Fatalf("regular file accepted as directory: handle=%v err=%v", handle, err)
	}
}

func TestSystemFallbackResolverSkipsGuaranteedMissingScalarsAndEmptyRoots(t *testing.T) {
	limits := SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 2,
		MaxCoverageChecks: 1, MaxFileBytes: 1, MaxScannedBytes: 1, MaxResolvedBytes: 1,
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "candidate.ttf"), []byte("not parsed"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: limits, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\U0010ffff'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("noncharacter result/error = %#v/%v", fonts, err)
	}
	if resolver.work.directoryEntries != 0 || resolver.work.files != 0 || resolver.work.scannedBytes != 0 || resolver.work.faces != 0 || resolver.work.coverageChecks != 0 {
		t.Fatalf("noncharacter triggered system-font discovery: %+v", resolver.work)
	}

	resolver = &SystemFallbackResolver{limits: limits, roots: []string{}}
	fonts, err = resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("empty roots result/error = %#v/%v", fonts, err)
	}
	if resolver.work.directoryEntries != 0 || resolver.work.files != 0 || resolver.work.scannedBytes != 0 {
		t.Fatalf("empty roots consumed discovery work: %+v", resolver.work)
	}
}

func TestSystemFallbackResolverUsesCmapProbeForOrdinaryMiss(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SourceSansPro-Regular.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 1,
		MaxCoverageChecks: 1, MaxFileBytes: int64(len(data)),
		MaxScannedBytes: int64(len(data)), MaxResolvedBytes: int64(len(data)),
	}, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u0378'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("ordinary miss result/error = %#v/%v", fonts, err)
	}
	if resolver.work.faces != 1 || resolver.work.coverageChecks != 0 || resolver.work.resolvedBytes != 0 {
		t.Fatalf("ordinary miss work = %+v", resolver.work)
	}
	if resolver.work.scannedBytes <= 0 || resolver.work.scannedBytes >= int64(len(data))/2 {
		t.Fatalf("ordinary miss cmap probe read %d of %d font bytes", resolver.work.scannedBytes, len(data))
	}
}

func TestSystemFallbackResolverChargesFailedCmapProbeBytes(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SourceSansPro-Regular.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	probe, err := probeFontFileCoverage(
		context.Background(), filepath.Join(root, "SourceSansPro-Regular.ttf"), int64(len(data)), 32<<10, 1,
		map[rune]struct{}{'A': {}},
	)
	if !errors.Is(err, errFontProbeBytes) || probe.bytes <= 0 {
		t.Fatalf("setup probe = %+v, %v, want a partial byte-limit error", probe, err)
	}
	scanLimit := probe.bytes
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 2,
		MaxCoverageChecks: 2, MaxFileBytes: int64(len(data)),
		MaxScannedBytes: scanLimit, MaxResolvedBytes: int64(len(data)),
	}, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}}); !errors.Is(err, errFontProbeBytes) {
		t.Fatalf("first partial-probe error = %v, want %v", err, errFontProbeBytes)
	}
	charged := resolver.work.scannedBytes
	if charged != scanLimit {
		t.Fatalf("failed probe charged %d bytes, want %d", charged, scanLimit)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'B'}}); err == nil || !strings.Contains(err.Error(), "scan bytes exceed limit") {
		t.Fatalf("retry after failed probe error = %v, want exhausted scan limit", err)
	}
	if resolver.work.scannedBytes != charged {
		t.Fatalf("retry changed scanned bytes from %d to %d", charged, resolver.work.scannedBytes)
	}
}

func TestFallbackPathPriorityPreservesRequestedStyle(t *testing.T) {
	values := map[rune]struct{}{'A': {}}
	italic := newFallbackRequestProfile(values, FallbackRequest{Family: "SourceSansPro", Style: "italic", Weight: 400})
	if got, wantBetterThan := fallbackPathPriority("Custom-Italic.ttf", italic), fallbackPathPriority("Custom-Regular.ttf", italic); got >= wantBetterThan {
		t.Fatalf("italic priority %d is not better than regular %d", got, wantBetterThan)
	}
	boldMono := newFallbackRequestProfile(values, FallbackRequest{Family: "SourceCodePro", Style: "bold", Weight: 700})
	if got, wantBetterThan := fallbackPathPriority("CustomMono-Bold.ttf", boldMono), fallbackPathPriority("Custom-Regular.ttf", boldMono); got >= wantBetterThan {
		t.Fatalf("bold mono priority %d is not better than regular proportional %d", got, wantBetterThan)
	}
	semibold := newFallbackRequestProfile(values, FallbackRequest{Style: "semibold", Weight: 600})
	if !semibold.semibold || semibold.bold {
		t.Fatalf("semibold profile = %+v", semibold)
	}
	bold := newFallbackRequestProfile(values, FallbackRequest{Family: "Source Sans Pro", Style: "bold", Weight: 700})
	if got, broadRegular := fallbackPathPriority("NotoSansArabic-Bold.ttf", bold), fallbackPathPriority("Arial Unicode.ttf", bold); got >= broadRegular {
		t.Fatalf("style-matched script priority %d is not better than broad regular %d", got, broadRegular)
	}
}

func TestSystemFallbackResolverReusesOneDirectorySnapshot(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 3, MaxFiles: 1, MaxFaces: 2, MaxRequestedRunes: 2,
		MaxCoverageChecks: 2, MaxFileBytes: int64(len(data)), MaxScannedBytes: 2 * int64(len(data)), MaxResolvedBytes: 2 * int64(len(data)),
	}, roots: []string{root}}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}}); err != nil {
		t.Fatal(err)
	}
	// A second candidate would exceed MaxFiles if the directory were walked
	// again. The export-scoped resolver intentionally keeps its first immutable
	// snapshot and only re-sorts it for the second request's style.
	if err := os.WriteFile(filepath.Join(root, "second.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'B'}, Style: "italic"}); err != nil {
		t.Fatalf("second request rescanned directory snapshot: %v", err)
	}
}

func TestSystemFallbackResolverCachesEmptyDirectorySnapshot(t *testing.T) {
	root := t.TempDir()
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 1, MaxRequestedRunes: 1,
		MaxCoverageChecks: 1, MaxFileBytes: 1, MaxScannedBytes: 1, MaxResolvedBytes: 1,
	}, roots: []string{root}}
	profile := newFallbackRequestProfile(map[rune]struct{}{'A': {}}, FallbackRequest{})
	paths, err := resolver.cachedFontPaths(context.Background(), resolver.roots, profile)
	if err != nil || len(paths) != 0 || !resolver.pathsIndexed {
		t.Fatalf("first empty snapshot = %#v, indexed=%v, err=%v", paths, resolver.pathsIndexed, err)
	}
	if err := os.WriteFile(filepath.Join(root, "later.ttf"), []byte("later"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err = resolver.cachedFontPaths(context.Background(), resolver.roots, profile)
	if err != nil || len(paths) != 0 {
		t.Fatalf("empty snapshot was not reused: %#v, %v", paths, err)
	}
}

func TestSystemFallbackResolverBudgetsAreCumulativeAcrossCalls(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "font.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	base := SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 2, MaxRequestedRunes: 2,
		MaxCoverageChecks: 2, MaxFileBytes: int64(len(data)), MaxScannedBytes: 2 * int64(len(data)), MaxResolvedBytes: 2 * int64(len(data)),
	}
	tests := map[string]struct {
		mutate func(*SystemFallbackLimits)
		want   string
	}{
		"requested runes": {func(l *SystemFallbackLimits) { l.MaxRequestedRunes = 1 }, "requested fallback rune count"},
		"scanned bytes":   {func(l *SystemFallbackLimits) { l.MaxScannedBytes = int64(len(data)) }, "scan bytes"},
		"faces":           {func(l *SystemFallbackLimits) { l.MaxFaces = 1 }, "face count exceeds cumulative"},
		"coverage":        {func(l *SystemFallbackLimits) { l.MaxCoverageChecks = 1 }, "coverage checks exceed limit"},
		"resolved bytes":  {func(l *SystemFallbackLimits) { l.MaxResolvedBytes = int64(len(data)) }, "resolved fallback font bytes"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			limits := base
			test.mutate(&limits)
			resolver := &SystemFallbackResolver{limits: limits, roots: []string{root}}
			if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}}); err != nil {
				t.Fatalf("first call: %v", err)
			}
			if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'B'}}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("second-call cumulative error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSystemFallbackResolverDoesNotRetainScannedFontData(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "font.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 2, MaxRequestedRunes: 2,
		MaxCoverageChecks: 2, MaxFileBytes: int64(len(data)), MaxScannedBytes: 2 * int64(len(data)), MaxResolvedBytes: 2 * int64(len(data)),
	}, roots: []string{root}}
	for _, value := range []rune{'A', 'B'} {
		fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{value}})
		if err != nil || len(fonts) != 1 {
			t.Fatalf("fallback for %q = %#v, %v", value, fonts, err)
		}
	}
	if resolver.work.scannedBytes != 2*int64(len(data)) || resolver.work.faces != 2 ||
		resolver.work.coverageChecks != 2 || resolver.work.resolvedBytes != 2*int64(len(data)) {
		t.Fatalf("actual repeated work was not charged: %+v", resolver.work)
	}
	if !resolver.pathsIndexed || len(resolver.indexedPaths) != 1 {
		t.Fatalf("resolver retained state is not the bounded path index: indexed=%v paths=%#v", resolver.pathsIndexed, resolver.indexedPaths)
	}
}

func TestSystemFallbackResolverWaitIsContextAware(t *testing.T) {
	resolver := &SystemFallbackResolver{}
	release, err := resolver.acquireResolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.acquireResolve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resolver waiter error = %v", err)
	}
	release()
}

func TestSystemFallbackResolverSerializesConcurrentWorkAccounting(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "font.ttf"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	const workers = 8
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: workers, MaxRequestedRunes: workers,
		MaxCoverageChecks: workers, MaxFileBytes: int64(len(data)), MaxScannedBytes: workers * int64(len(data)), MaxResolvedBytes: workers * int64(len(data)),
	}, roots: []string{root}}
	start := make(chan struct{})
	errorsByWorker := make([]error, workers)
	var group sync.WaitGroup
	for worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}})
			if err == nil && len(fonts) != 1 {
				err = fmt.Errorf("resolved %d fonts, want 1", len(fonts))
			}
			errorsByWorker[worker] = err
		}()
	}
	close(start)
	group.Wait()
	for worker, err := range errorsByWorker {
		if err != nil {
			t.Fatalf("worker %d: %v", worker, err)
		}
	}
	if resolver.work.requestedRunes != workers || resolver.work.faces != workers || resolver.work.coverageChecks != workers ||
		resolver.work.scannedBytes != workers*int64(len(data)) || resolver.work.resolvedBytes != workers*int64(len(data)) {
		t.Fatalf("concurrent cumulative work = %+v", resolver.work)
	}
}

func TestSystemFallbackResolverSelectsStyledCollectionFace(t *testing.T) {
	regular, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro regular is not loaded")
	}
	bold, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_BOLD})
	if !ok {
		t.Fatal("Source Sans Pro bold is not loaded")
	}
	collection := combineFallbackTestTTFCollection(t, regular, bold)
	root := t.TempDir()
	path := filepath.Join(root, "SourceSans.ttc")
	if err := os.WriteFile(path, collection, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 2, MaxFiles: 1, MaxFaces: 2, MaxRequestedRunes: 1,
		MaxCoverageChecks: 2, MaxFileBytes: int64(len(collection)), MaxScannedBytes: int64(len(collection)), MaxResolvedBytes: int64(len(collection)),
	}, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}, Family: "Source Sans Pro", Style: "bold", Weight: 700})
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 1 || fonts[0].FaceIndex != 1 {
		t.Fatalf("styled TTC fallback = %#v, want bold face index 1", fonts)
	}
}

func combineFallbackTestTTFCollection(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	if len(fonts) == 0 {
		t.Fatal("font collection requires at least one source font")
	}
	for _, font := range fonts {
		if len(font) < 12 {
			t.Fatal("invalid source font")
		}
	}
	headerBytes := 12 + 4*len(fonts)
	totalBytes := headerBytes
	for _, font := range fonts {
		totalBytes += (len(font) + 3) &^ 3
	}
	result := make([]byte, totalBytes)
	copy(result[:4], "ttcf")
	binary.BigEndian.PutUint32(result[4:8], 0x00010000)
	binary.BigEndian.PutUint32(result[8:12], uint32(len(fonts)))
	base := headerBytes
	for index, font := range fonts {
		binary.BigEndian.PutUint32(result[12+4*index:16+4*index], uint32(base))
		copy(result[base:base+len(font)], font)
		tableCount := int(binary.BigEndian.Uint16(result[base+4 : base+6]))
		if 12+16*tableCount > len(font) {
			t.Fatal("invalid source sfnt table directory")
		}
		for table := 0; table < tableCount; table++ {
			offsetAt := base + 12 + table*16 + 8
			offset := binary.BigEndian.Uint32(result[offsetAt : offsetAt+4])
			binary.BigEndian.PutUint32(result[offsetAt:offsetAt+4], offset+uint32(base))
		}
		base += (len(font) + 3) &^ 3
	}
	return result
}

func TestSystemFallbackResolverPrefersStyledFileOverBroadRegular(t *testing.T) {
	regular, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro regular is not loaded")
	}
	bold, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_BOLD})
	if !ok {
		t.Fatal("Source Sans Pro bold is not loaded")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Arial Unicode.ttf"), regular, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "NotoSansArabic-Bold.ttf"), bold, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &SystemFallbackResolver{limits: SystemFallbackLimits{
		MaxDirectoryEntries: 3, MaxFiles: 2, MaxFaces: 1, MaxRequestedRunes: 1,
		MaxCoverageChecks: 1, MaxFileBytes: int64(max(len(regular), len(bold))), MaxScannedBytes: int64(len(bold)), MaxResolvedBytes: int64(len(bold)),
	}, roots: []string{root}}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'A'}, Family: "Source Sans Pro", Style: "bold", Weight: 700})
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 1 || fonts[0].Name != "NotoSansArabic-Bold.ttf" || !bytes.Equal(fonts[0].Data, bold) {
		t.Fatalf("styled file fallback = %#v", fonts)
	}
}
