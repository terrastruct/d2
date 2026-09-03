package d2fonts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	gotextfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

// FallbackFont is one fully resolved host font face. Data is owned by the
// result and may be retained in a network-free renderer scene. FaceIndex
// selects a face when Data contains an OpenType collection.
type FallbackFont struct {
	Name      string
	MIMEType  string
	Data      []byte
	FaceIndex uint16
}

// FallbackRequest carries both missing code points and the source text style.
// Resolvers may ignore style when a face is intrinsically universal, but must
// preserve it when delegating so bold, italic, semibold, and monospace text do
// not silently fall back to an unrelated regular proportional face.
type FallbackRequest struct {
	Runes  []rune
	Family string
	Style  string
	Weight int
}

// FallbackResolver resolves an ordered set of font faces covering runes not
// present in a D2 label's configured primary font. An empty or partial result
// means the remaining runes have no available face; it is not an error.
// Implementations must return owned immutable bytes and must honor context
// cancellation between bounded I/O operations. Malformed resources and work
// or byte limit violations remain errors.
type FallbackResolver interface {
	ResolveFallbacks(context.Context, FallbackRequest) ([]FallbackFont, error)
}

// SystemFallbackLimits bounds deterministic host-font discovery for the
// lifetime of one resolver. FileBytes bounds one candidate; every other
// work/byte limit is cumulative across indexing attempts and ResolveFallbacks
// calls. A CLI export owns one resolver, so these are operation-wide ceilings
// rather than per-board or per-style allowances.
type SystemFallbackLimits struct {
	MaxDirectoryEntries int
	MaxFiles            int
	MaxFaces            int
	MaxRequestedRunes   int
	MaxCoverageChecks   int64
	MaxFileBytes        int64
	MaxScannedBytes     int64
	MaxResolvedBytes    int64
}

func (l SystemFallbackLimits) validate() error {
	if l.MaxDirectoryEntries <= 0 || l.MaxFiles <= 0 || l.MaxFaces <= 0 || l.MaxRequestedRunes <= 0 || l.MaxCoverageChecks <= 0 || l.MaxFileBytes <= 0 || l.MaxScannedBytes <= 0 || l.MaxResolvedBytes <= 0 {
		return fmt.Errorf("d2fonts: every system fallback limit must be positive")
	}
	return nil
}

// SystemFallbackResolver discovers fonts only under the operating system's
// conventional system font roots. It does not follow symlinks or consult a
// platform API, keeping discovery CGO-free and auditable. User-installed D2
// primary fonts continue to flow through AddFontFamily and do not need this
// resolver.
type SystemFallbackResolver struct {
	limits SystemFallbackLimits
	roots  []string

	resolveOnce sync.Once
	resolveGate chan struct{}

	pathsMu      sync.Mutex
	pathsIndexed bool
	indexedPaths []string
	pathFlight   chan struct{}

	work systemFallbackWork
}

type systemFallbackWork struct {
	directoryEntries int
	files            int
	requestedRunes   int64
	scannedBytes     int64
	faces            int
	coverageChecks   int64
	resolvedBytes    int64
}

type fallbackFaceProfile struct {
	family string
	italic bool
	weight int
	mono   bool
}

func NewSystemFallbackResolver(limits SystemFallbackLimits) (*SystemFallbackResolver, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &SystemFallbackResolver{limits: limits}, nil
}

func (r *SystemFallbackResolver) ResolveFallbacks(ctx context.Context, request FallbackRequest) ([]FallbackFont, error) {
	if r == nil {
		return nil, fmt.Errorf("d2fonts: nil system fallback resolver")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := r.acquireResolve(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Family)+len(request.Style) > 1_024 {
		return nil, fmt.Errorf("d2fonts: fallback request font metadata exceeds 1024 bytes")
	}
	if len(request.Runes) > r.limits.MaxRequestedRunes {
		return nil, fmt.Errorf("d2fonts: requested fallback rune count %d exceeds limit %d", len(request.Runes), r.limits.MaxRequestedRunes)
	}
	if int64(len(request.Runes)) > int64(r.limits.MaxRequestedRunes)-r.work.requestedRunes {
		return nil, fmt.Errorf("d2fonts: requested fallback rune count exceeds cumulative resolver limit %d", r.limits.MaxRequestedRunes)
	}
	r.work.requestedRunes += int64(len(request.Runes))
	remaining, err := uniqueFallbackRunes(ctx, request.Runes)
	if err != nil {
		return nil, err
	}
	if len(remaining) == 0 {
		return nil, nil
	}
	requestProfile := newFallbackRequestProfile(remaining, request)
	roots := r.roots
	if roots == nil {
		roots = systemFontRoots()
	}
	if len(roots) == 0 {
		return nil, nil
	}
	paths, err := r.cachedFontPaths(ctx, roots, requestProfile)
	if err != nil {
		return nil, err
	}

	var resolved []FallbackFont
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		probe, err := probeFontFileCoverage(
			ctx, path, r.limits.MaxFileBytes, r.limits.MaxScannedBytes-r.work.scannedBytes,
			r.limits.MaxFaces-r.work.faces, remaining,
		)
		if err != nil {
			r.work.scannedBytes += probe.bytes
			return nil, fmt.Errorf("d2fonts: system font %q: %w", filepath.Base(path), err)
		}
		if probe.faces == 0 {
			r.work.scannedBytes += probe.bytes
			continue
		}
		if probe.faces > r.limits.MaxFaces-r.work.faces {
			r.work.scannedBytes += probe.bytes
			return nil, fmt.Errorf("d2fonts: system font face count exceeds cumulative resolver limit %d while resolving %U", r.limits.MaxFaces, firstRune(remaining))
		}
		if !probe.covers {
			r.work.faces += probe.faces
			r.work.scannedBytes += probe.bytes
			continue
		}
		data, err := readBoundedFontFile(path, r.limits.MaxFileBytes, r.limits.MaxScannedBytes-r.work.scannedBytes)
		if err != nil {
			r.work.scannedBytes += probe.bytes
			return nil, fmt.Errorf("d2fonts: system font %q: %w", filepath.Base(path), err)
		}
		if len(data) == 0 {
			r.work.scannedBytes += probe.bytes
			continue
		}
		r.work.scannedBytes += int64(len(data))
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remainingFaces := r.limits.MaxFaces - r.work.faces
		if remainingFaces <= 0 {
			return nil, fmt.Errorf("d2fonts: system font face count exceeds cumulative resolver limit %d while resolving %U", r.limits.MaxFaces, firstRune(remaining))
		}
		collection, err := fontface.ParseFaceCollectionWithLimit(data, remainingFaces)
		if err != nil {
			var limitError *fontface.FaceCountLimitError
			if errors.As(err, &limitError) {
				return nil, fmt.Errorf("d2fonts: system font %q: %w", filepath.Base(path), err)
			}
			continue
		}
		if collection.NumFaces() > r.limits.MaxFaces-r.work.faces {
			return nil, fmt.Errorf("d2fonts: system font face count exceeds cumulative resolver limit %d while resolving %U", r.limits.MaxFaces, firstRune(remaining))
		}
		r.work.faces += collection.NumFaces()
		selectedFaces := make(map[int]bool)
		faceProfiles := make([]fallbackFaceProfile, collection.NumFaces())
		profiledFaces := make([]bool, collection.NumFaces())
		for len(remaining) != 0 {
			bestIndex, bestPriority, bestCoverage := -1, 0, []rune(nil)
			for index := 0; index < collection.NumFaces(); index++ {
				if index > int(^uint16(0)) {
					break
				}
				if selectedFaces[index] {
					continue
				}
				face, err := collection.Face(index)
				if err != nil {
					continue
				}
				covered, err := r.coveredRunes(ctx, face, remaining)
				if err != nil {
					return nil, err
				}
				if len(covered) == 0 {
					continue
				}
				if !profiledFaces[index] {
					faceProfiles[index] = newFallbackFaceProfile(face)
					profiledFaces[index] = true
				}
				priority := fallbackFacePriority(faceProfiles[index], requestProfile)
				if bestIndex < 0 || priority < bestPriority || priority == bestPriority && len(covered) > len(bestCoverage) {
					bestIndex, bestPriority, bestCoverage = index, priority, covered
				}
			}
			if bestIndex < 0 {
				break
			}
			if int64(len(data)) > r.limits.MaxResolvedBytes-r.work.resolvedBytes {
				return nil, fmt.Errorf("d2fonts: resolved fallback font bytes exceed limit %d while retaining face %d from %q", r.limits.MaxResolvedBytes, bestIndex, filepath.Base(path))
			}
			selectedFaces[bestIndex] = true
			resolved = append(resolved, FallbackFont{
				Name: filepath.Base(path), MIMEType: fontMIMEType(path),
				Data: append([]byte(nil), data...), FaceIndex: uint16(bestIndex),
			})
			r.work.resolvedBytes += int64(len(data))
			for _, value := range bestCoverage {
				delete(remaining, value)
			}
			if len(remaining) == 0 {
				return resolved, nil
			}
		}
	}
	// Absence of coverage is not a malformed-font or resource-budget failure.
	// Callers can deterministically render their missing-glyph placeholder while
	// retaining any faces that did cover part of the request.
	return resolved, nil
}

// acquireResolve serializes font parsing and cumulative work accounting.
// Waiting callers remain cancellable, and a canceled waiter never consumes a
// resolver-lifetime budget.
func (r *SystemFallbackResolver) acquireResolve(ctx context.Context) (func(), error) {
	r.resolveOnce.Do(func() { r.resolveGate = make(chan struct{}, 1) })
	select {
	case r.resolveGate <- struct{}{}:
		return func() { <-r.resolveGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cachedFontPaths snapshots the fixed system roots once for the lifetime of a
// resolver. One export operation shares one resolver across boards, avoiding a
// full directory walk for every scene while preserving request-specific sort
// priority. Cancellation never poisons the cache: a canceled owner publishes
// no partial index and later callers may retry.
func (r *SystemFallbackResolver) cachedFontPaths(ctx context.Context, roots []string, profile fallbackRequestProfile) ([]string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.pathsMu.Lock()
		if r.pathsIndexed {
			paths := append([]string(nil), r.indexedPaths...)
			r.pathsMu.Unlock()
			sortFontPaths(paths, profile)
			return paths, nil
		}
		if flight := r.pathFlight; flight != nil {
			r.pathsMu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-flight:
				continue
			}
		}
		flight := make(chan struct{})
		r.pathFlight = flight
		r.pathsMu.Unlock()

		paths, err := r.fontPaths(ctx, roots, profile)
		r.pathsMu.Lock()
		if err == nil {
			r.indexedPaths = append([]string(nil), paths...)
			r.pathsIndexed = true
		}
		r.pathFlight = nil
		close(flight)
		r.pathsMu.Unlock()
		return paths, err
	}
}

func (r *SystemFallbackResolver) fontPaths(ctx context.Context, roots []string, profile fallbackRequestProfile) ([]string, error) {
	paths := make([]string, 0, min(r.limits.MaxFiles, 256))
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if r.work.directoryEntries >= r.limits.MaxDirectoryEntries {
			return nil, fmt.Errorf("d2fonts: system font directory entries exceed limit %d", r.limits.MaxDirectoryEntries)
		}
		r.work.directoryEntries++
		stack := []string{root}
		for len(stack) != 0 {
			directory := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			handle, err := openVerifiedDirectory(directory)
			if err != nil {
				continue
			}
			for {
				if err := ctx.Err(); err != nil {
					_ = handle.Close()
					return nil, err
				}
				children, readErr := handle.ReadDir(directoryReadBatch)
				for _, child := range children {
					if r.work.directoryEntries >= r.limits.MaxDirectoryEntries {
						_ = handle.Close()
						return nil, fmt.Errorf("d2fonts: system font directory entries exceed limit %d", r.limits.MaxDirectoryEntries)
					}
					r.work.directoryEntries++
					if r.work.directoryEntries&255 == 0 {
						if err := ctx.Err(); err != nil {
							_ = handle.Close()
							return nil, err
						}
					}
					if child.Type()&os.ModeSymlink != 0 {
						continue
					}
					path := filepath.Join(directory, child.Name())
					if child.IsDir() {
						stack = append(stack, path)
						continue
					}
					if !isFontPath(path) {
						continue
					}
					if r.work.files >= r.limits.MaxFiles {
						_ = handle.Close()
						return nil, fmt.Errorf("d2fonts: system font candidate count exceeds limit %d; raise the bounded fallback search limit or configure a custom font", r.limits.MaxFiles)
					}
					r.work.files++
					paths = append(paths, path)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					break
				}
			}
			_ = handle.Close()
		}
	}
	sortFontPaths(paths, profile)
	return paths, nil
}

func sortFontPaths(paths []string, profile fallbackRequestProfile) {
	sort.Slice(paths, func(i, j int) bool {
		left, right := fallbackPathPriority(paths[i], profile), fallbackPathPriority(paths[j], profile)
		if left != right {
			return left < right
		}
		return paths[i] < paths[j]
	})
}

const directoryReadBatch = 256

// openVerifiedDirectory supports batched ReadDir calls without
// filepath.WalkDir's whole-directory sorted allocation. The final font path
// list is sorted separately, so successful discovery remains deterministic.
func openVerifiedDirectory(path string) (*os.File, error) {
	listed, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !listed.IsDir() || listed.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("path is not a non-symlink directory")
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	if !opened.IsDir() || !os.SameFile(listed, opened) {
		_ = directory.Close()
		return nil, fmt.Errorf("directory identity changed while opening")
	}
	return directory, nil
}

type fontCoverageProbe struct {
	faces  int
	bytes  int64
	covers bool
}

var errFontProbeBytes = errors.New("font cmap probe bytes exceed remaining scan limit")

type boundedFontReaderAt struct {
	ctx       context.Context
	reader    io.ReaderAt
	remaining int64
	read      int64
	limitHit  bool
}

func (r *boundedFontReaderAt) ReadAt(buffer []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(buffer)) > r.remaining-r.read {
		r.limitHit = true
		return 0, errFontProbeBytes
	}
	read, err := r.reader.ReadAt(buffer, offset)
	r.read += int64(read)
	return read, err
}

// probeFontFileCoverage reads only SFNT directories and cmap data through
// ReaderAt. A full go-text parse is substantially more expensive and is
// reserved for files whose outline cmap actually maps one requested rune.
// Matching files are subsequently read in full, so their probe bytes are not
// charged twice against the cumulative scan budget.
func probeFontFileCoverage(ctx context.Context, path string, maxFileBytes, remainingScanBytes int64, maxFaces int, values map[rune]struct{}) (fontCoverageProbe, error) {
	if remainingScanBytes <= 0 {
		return fontCoverageProbe{}, fmt.Errorf("system font scan bytes exceed limit")
	}
	listed, err := os.Lstat(path)
	if err != nil || !listed.Mode().IsRegular() || listed.Mode()&os.ModeSymlink != 0 || listed.Size() <= 0 || listed.Size() > maxFileBytes {
		return fontCoverageProbe{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return fontCoverageProbe{}, nil
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(listed, opened) || opened.Size() != listed.Size() {
		return fontCoverageProbe{}, nil
	}
	reader := &boundedFontReaderAt{ctx: ctx, reader: file, remaining: remainingScanBytes}
	collection, err := opentype.ParseCollectionReaderAt(reader)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fontCoverageProbe{bytes: reader.read}, ctxErr
		}
		if reader.limitHit || errors.Is(err, errFontProbeBytes) {
			return fontCoverageProbe{bytes: reader.read}, errFontProbeBytes
		}
		return fontCoverageProbe{bytes: reader.read}, nil
	}
	probe := fontCoverageProbe{faces: collection.NumFonts(), bytes: reader.read}
	if probe.faces > maxFaces {
		return probe, nil
	}
	var buffer sfnt.Buffer
	for index := 0; index < collection.NumFonts(); index++ {
		if err := ctx.Err(); err != nil {
			return probe, err
		}
		face, err := collection.Font(index)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fontCoverageProbe{faces: probe.faces, bytes: reader.read}, ctxErr
			}
			if reader.limitHit || errors.Is(err, errFontProbeBytes) {
				return fontCoverageProbe{faces: probe.faces, bytes: reader.read}, errFontProbeBytes
			}
			continue
		}
		for value := range values {
			glyph, err := face.GlyphIndex(&buffer, value)
			if err != nil {
				if reader.limitHit || errors.Is(err, errFontProbeBytes) {
					return fontCoverageProbe{faces: probe.faces, bytes: reader.read}, errFontProbeBytes
				}
				continue
			}
			if glyph != 0 {
				probe.bytes = reader.read
				probe.covers = true
				return probe, nil
			}
		}
	}
	probe.bytes = reader.read
	return probe, nil
}

func readBoundedFontFile(path string, maxFileBytes, remainingScanBytes int64) ([]byte, error) {
	if remainingScanBytes <= 0 {
		return nil, fmt.Errorf("system font scan bytes exceed limit")
	}
	listed, err := os.Lstat(path)
	if err != nil || !listed.Mode().IsRegular() || listed.Mode()&os.ModeSymlink != 0 || listed.Size() <= 0 {
		return nil, nil
	}
	if listed.Size() > maxFileBytes {
		return nil, nil
	}
	if listed.Size() > remainingScanBytes {
		return nil, fmt.Errorf("system font scan bytes exceed remaining limit %d", remainingScanBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(listed, opened) || opened.Size() <= 0 {
		return nil, nil
	}
	readLimit := min(maxFileBytes, remainingScanBytes)
	if opened.Size() > readLimit {
		return nil, fmt.Errorf("system font file exceeds remaining byte limit %d", readLimit)
	}
	data, err := io.ReadAll(io.LimitReader(file, readLimit+1))
	if int64(len(data)) > readLimit {
		return nil, fmt.Errorf("system font file grew beyond byte limit %d while reading", readLimit)
	}
	if err != nil {
		// Preserve partial bytes so the caller charges the I/O that actually
		// happened. Parsing will reject an incomplete font without retaining it.
		return data, nil
	}
	return data, nil
}

func uniqueFallbackRunes(ctx context.Context, values []rune) (map[rune]struct{}, error) {
	result := make(map[rune]struct{}, len(values))
	for index, value := range values {
		if index&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if fallbackRuneMayHaveGlyph(value) {
			result[value] = struct{}{}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// fallbackRuneMayHaveGlyph rejects only values which Unicode permanently
// reserves as noncharacters (plus invalid scalar values). They can never gain
// text semantics, so walking and parsing every system font for them is pure
// resource amplification. Unassigned and private-use scalars remain eligible:
// custom and future fonts may legitimately map them.
func fallbackRuneMayHaveGlyph(value rune) bool {
	if !utf8.ValidRune(value) {
		return false
	}
	if value >= 0xfdd0 && value <= 0xfdef {
		return false
	}
	tail := value & 0xffff
	return tail != 0xfffe && tail != 0xffff
}

func (r *SystemFallbackResolver) coveredRunes(ctx context.Context, face *fontface.ParsedFace, values map[rune]struct{}) ([]rune, error) {
	covered := make([]rune, 0)
	index := 0
	for value := range values {
		if r.work.coverageChecks >= r.limits.MaxCoverageChecks {
			return nil, fmt.Errorf("d2fonts: system font coverage checks exceed limit %d across this resolver while resolving U+%04X", r.limits.MaxCoverageChecks, firstRune(values))
		}
		r.work.coverageChecks++
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		supported, err := face.SupportsRenderableRune(value)
		if err != nil {
			return nil, err
		}
		if supported {
			covered = append(covered, value)
		}
		index++
	}
	sort.Slice(covered, func(i, j int) bool { return covered[i] < covered[j] })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return covered, nil
}

func firstRune(values map[rune]struct{}) rune {
	var first rune
	found := false
	for value := range values {
		if !found || value < first {
			first = value
			found = true
		}
	}
	return first
}

func isFontPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc", ".otc":
		return true
	default:
		return false
	}
}

func fontMIMEType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttc", ".otc":
		return "font/collection"
	case ".otf":
		return "font/otf"
	default:
		return "font/ttf"
	}
}

const genericFallbackPriority = 20

type fallbackRequestProfile struct {
	cjk, arabic, hebrew, symbol  bool
	italic, bold, semibold, mono bool
	family                       string
	weight                       int
}

func newFallbackRequestProfile(values map[rune]struct{}, request FallbackRequest) fallbackRequestProfile {
	family := normalizeFallbackFamily(request.Family)
	style := strings.ToLower(request.Style)
	styleIsSemibold := strings.Contains(style, "semibold") || strings.Contains(style, "semi-bold")
	weight := request.Weight
	if weight <= 0 {
		switch {
		case styleIsSemibold:
			weight = 600
		case strings.Contains(style, "bold"):
			weight = 700
		default:
			weight = 400
		}
	}
	weight = min(900, max(100, weight))
	profile := fallbackRequestProfile{
		italic:   strings.Contains(style, "italic") || strings.Contains(style, "oblique"),
		bold:     weight >= 700,
		semibold: weight >= 600 && weight < 700,
		mono:     strings.Contains(family, "mono") || strings.Contains(family, "code") || strings.Contains(family, "console"),
		family:   family,
		weight:   weight,
	}
	for value := range values {
		profile.cjk = profile.cjk || isCJK(value)
		profile.arabic = profile.arabic || isArabic(value)
		profile.hebrew = profile.hebrew || isHebrew(value)
		profile.symbol = profile.symbol || isEmojiOrSymbol(value)
	}
	return profile
}

func fallbackPathPriority(path string, profile fallbackRequestProfile) int {
	name := strings.ToLower(filepath.Base(path))
	semanticPriority := 80
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(name, part) {
				return true
			}
		}
		return false
	}
	// Prefer genuinely broad Unicode faces before accumulating one
	// system face per script. This keeps mixed-script scenes within the retained
	// font-byte budget when a single host font can cover the whole label set.
	if contains("arial unicode", "unifont") {
		semanticPriority = -10
	}
	switch {
	case profile.cjk && contains("cjk", "hiragino", "pingfang", "heiti", "gothic", "songti", "mingliu", "simsun", "malgun"):
		semanticPriority = min(semanticPriority, 0)
	case profile.arabic && contains("arab", "amiri", "naskh", "kufi"):
		semanticPriority = min(semanticPriority, 0)
	case profile.hebrew && contains("hebrew", "david", "frank"):
		semanticPriority = min(semanticPriority, 0)
	case profile.symbol && contains("symbol", "emoji", "dingbat", "dejavu", "unifont", "unicode"):
		semanticPriority = min(semanticPriority, 0)
	}
	if contains("noto", "dejavu", "arial unicode", "unifont") {
		semanticPriority = min(semanticPriority, genericFallbackPriority)
	}
	if contains("emoji") {
		// Deprioritize emoji-specific faces because their glyphs may use
		// unsupported bitmap, SVG, or COLRv1 data. Coverage validation still
		// admits ordinary outlines and COLRv0 layers.
		semanticPriority += 10
	}
	if profile.mono && contains("mono", "code", "console") {
		semanticPriority -= 6
	}
	return fallbackPathStylePenalty(name, profile) + semanticPriority
}

func fallbackPathStylePenalty(name string, request fallbackRequestProfile) int {
	candidate := fallbackFaceProfile{
		family: normalizeFallbackFamily(name),
		italic: strings.Contains(name, "italic") || strings.Contains(name, "oblique"),
		weight: fallbackFilenameWeight(name),
		mono:   strings.Contains(name, "mono") || strings.Contains(name, "code") || strings.Contains(name, "console"),
	}
	return fallbackStylePenalty(candidate, request)
}

func fallbackFilenameWeight(name string) int {
	switch {
	case strings.Contains(name, "extrablack"), strings.Contains(name, "ultrablack"), strings.Contains(name, "superblack"):
		return 900
	case strings.Contains(name, "black"), strings.Contains(name, "heavy"):
		return 900
	case strings.Contains(name, "extrabold"), strings.Contains(name, "extra-bold"), strings.Contains(name, "ultrabold"), strings.Contains(name, "ultra-bold"):
		return 800
	case strings.Contains(name, "semibold"), strings.Contains(name, "semi-bold"), strings.Contains(name, "demibold"), strings.Contains(name, "demi-bold"):
		return 600
	case strings.Contains(name, "bold"):
		return 700
	case strings.Contains(name, "medium"):
		return 500
	case strings.Contains(name, "extralight"), strings.Contains(name, "extra-light"), strings.Contains(name, "ultralight"), strings.Contains(name, "ultra-light"):
		return 200
	case strings.Contains(name, "semilight"), strings.Contains(name, "semi-light"), strings.Contains(name, "demilight"), strings.Contains(name, "demi-light"):
		return 350
	case strings.Contains(name, "thin"):
		return 100
	case strings.Contains(name, "light"):
		return 300
	default:
		return 400
	}
}

func newFallbackFaceProfile(face *fontface.ParsedFace) fallbackFaceProfile {
	description := face.Shaping.Describe()
	return fallbackFaceProfile{
		family: normalizeFallbackFamily(description.Family),
		italic: description.Aspect.Style == gotextfont.StyleItalic,
		weight: min(900, max(100, int(description.Aspect.Weight+0.5))),
		mono:   face.Shaping.IsMonospace(),
	}
}

func fallbackFacePriority(face fallbackFaceProfile, request fallbackRequestProfile) int {
	return fallbackStylePenalty(face, request)
}

func fallbackStylePenalty(candidate fallbackFaceProfile, request fallbackRequestProfile) int {
	penalty := absInt(candidate.weight - request.weight)
	if candidate.italic != request.italic {
		penalty += 1_000
	}
	if candidate.mono != request.mono {
		if request.mono {
			penalty += 1_000
		} else {
			penalty += 100
		}
	}
	if request.family != "" && candidate.family != "" {
		switch {
		case candidate.family == request.family:
			penalty -= 40
		case sameFallbackFamilyClass(candidate.family, request.family):
			penalty -= 10
		}
	}
	return penalty
}

func sameFallbackFamilyClass(left, right string) bool {
	for _, class := range []string{"sans", "serif", "mono", "code", "console"} {
		if strings.Contains(left, class) && strings.Contains(right, class) {
			return true
		}
	}
	return false
}

func normalizeFallbackFamily(value string) string {
	value = strings.ToLower(value)
	return strings.Map(func(value rune) rune {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			return value
		}
		return -1
	}, value)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func isCJK(value rune) bool {
	return value >= 0x2e80 && value <= 0x9fff || value >= 0xf900 && value <= 0xfaff || value >= 0x20000 && value <= 0x323af
}

func isArabic(value rune) bool {
	return value >= 0x0600 && value <= 0x08ff || value >= 0xfb50 && value <= 0xfdff || value >= 0xfe70 && value <= 0xfeff
}

func isHebrew(value rune) bool {
	return value >= 0x0590 && value <= 0x05ff || value >= 0xfb1d && value <= 0xfb4f
}

func isEmojiOrSymbol(value rune) bool {
	return value >= 0x2190 && value <= 0x2bff || value >= 0x1f000 && value <= 0x1faff
}

func isLikelyEmojiPresentation(value rune) bool {
	return value >= 0x1f000 && value <= 0x1faff ||
		value >= 0x2600 && value <= 0x27ff && unicode.Is(unicode.So, value)
}

func systemFontRoots() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/System/Library/Fonts", "/Library/Fonts"}
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	case "windows":
		root := os.Getenv("WINDIR")
		if root == "" {
			root = `C:\Windows`
		}
		return []string{filepath.Join(root, "Fonts")}
	default:
		return nil
	}
}
