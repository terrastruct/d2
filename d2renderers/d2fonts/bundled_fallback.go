package d2fonts

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2renderers/internal/fontfallback"
)

type sceneCache = fontfallback.SceneCache

// BundledFallbackLimits bound all requests and returned bytes over the
// lifetime of one composite resolver. MaxBundledBytes reserves an explicit
// sub-budget for owned copies of D2's trusted font; MaxResolvedBytes bounds
// those copies plus downstream results. The downstream resolver remains
// responsible for its own discovery and coverage-work limits.
type BundledFallbackLimits struct {
	MaxRequestedRunes int
	// MaxBundledBytes defaults to MaxResolvedBytes when zero.
	MaxBundledBytes  int64
	MaxResolvedBytes int64
}

func (l BundledFallbackLimits) normalized() (BundledFallbackLimits, error) {
	if l.MaxBundledBytes == 0 {
		l.MaxBundledBytes = l.MaxResolvedBytes
	}
	if l.MaxRequestedRunes <= 0 || l.MaxBundledBytes <= 0 || l.MaxResolvedBytes <= 0 {
		return BundledFallbackLimits{}, fmt.Errorf("d2fonts: every bundled fallback limit must be positive")
	}
	if l.MaxBundledBytes > l.MaxResolvedBytes {
		return BundledFallbackLimits{}, fmt.Errorf("d2fonts: bundled fallback byte limit %d exceeds aggregate resolved byte limit %d", l.MaxBundledBytes, l.MaxResolvedBytes)
	}
	return l, nil
}

// BundledFallbackResolver resolves supported symbols and emoji from D2's
// pinned Noto Color Emoji face before consulting the host. This makes those
// glyphs deterministic across machines while leaving other scripts to a
// bounded downstream resolver. On js/wasm the bundled face is absent and all
// requests pass through to downstream.
type BundledFallbackResolver struct {
	*sceneCache

	next   FallbackResolver
	limits BundledFallbackLimits

	resolveOnce sync.Once
	resolveGate chan struct{}
	work        bundledFallbackWork
	// go-text faces own mutable lookup caches. resolveGate serializes every use
	// of this resolver-local coverage face while returned font bytes remain
	// independently cloned and cumulatively charged below.
	bundledFace *fontface.ParsedFace
	loadBundled func() ([]byte, error)
}

type bundledFallbackWork struct {
	requestedRunes int64
	bundledBytes   int64
	resolvedBytes  int64
}

func NewBundledFallbackResolver(next FallbackResolver, limits BundledFallbackLimits) (*BundledFallbackResolver, error) {
	limits, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	resolver := &BundledFallbackResolver{
		next: next, limits: limits, loadBundled: bundledNotoColorEmoji,
	}
	resolver.sceneCache = fontfallback.NewSceneCache(resolver.resolveForScene)
	return resolver, nil
}

func (r *BundledFallbackResolver) ResolveFallbacks(ctx context.Context, request FallbackRequest) ([]FallbackFont, error) {
	fonts, err := r.resolve(ctx, request, false)
	if err != nil {
		return nil, err
	}
	result := make([]FallbackFont, len(fonts))
	for index := range fonts {
		result[index] = fallbackFont(fonts[index].Font)
	}
	return result, nil
}

func (r *BundledFallbackResolver) resolveForScene(ctx context.Context, request fontfallback.Request) ([]fontfallback.SceneFont, error) {
	return r.resolve(ctx, FallbackRequest{
		Runes: request.Runes, Family: request.Family, Style: request.Style, Weight: request.Weight,
	}, true)
}

func (r *BundledFallbackResolver) resolve(ctx context.Context, request FallbackRequest, forScene bool) ([]fontfallback.SceneFont, error) {
	if r == nil {
		return nil, fmt.Errorf("d2fonts: nil bundled fallback resolver")
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

	var resolved []fontfallback.SceneFont
	if shouldTryBundledEmoji(remaining) {
		loadBundled := r.loadBundled
		if loadBundled == nil {
			loadBundled = bundledNotoColorEmoji
		}
		data, err := loadBundled()
		if err != nil {
			return nil, err
		}
		if len(data) != 0 && r.bundledFace == nil {
			face, err := fontface.ParseFace(data, 0)
			if err != nil {
				return nil, fmt.Errorf("d2fonts: parse bundled Noto Color Emoji face: %w", err)
			}
			r.bundledFace = face
		}
		var covered []rune
		if r.bundledFace != nil {
			covered, err = coveredBundledRunes(ctx, r.bundledFace, remaining)
			if err != nil {
				return nil, err
			}
		}
		if len(data) != 0 && len(covered) != 0 {
			font := fontfallback.Font{Name: "NotoColorEmoji-COLRv1-v2.051.ttf", MIMEType: "font/ttf", Data: data}
			if forScene {
				if r.sceneCache == nil {
					return nil, fmt.Errorf("d2fonts: bundled fallback scene cache is unavailable")
				}
				sceneFace, err := r.bundledFace.Clone()
				if err != nil {
					return nil, fmt.Errorf("d2fonts: clone bundled Noto Color Emoji face: %w", err)
				}
				if !r.sceneCache.HasTrusted() {
					if err := r.admitBundledBytes(int64(len(data))); err != nil {
						return nil, err
					}
				}
				shared, added, err := r.sceneCache.PrepareTrusted(ctx, font)
				if err != nil {
					return nil, err
				}
				if added {
					r.work.bundledBytes += int64(len(shared.Data))
					r.work.resolvedBytes += int64(len(shared.Data))
				}
				shared.Face = sceneFace
				resolved = append(resolved, shared)
			} else {
				if err := r.admitBundledBytes(int64(len(data))); err != nil {
					return nil, err
				}
				owned, err := cloneFallbackBytes(ctx, data)
				if err != nil {
					return nil, err
				}
				font.Data = owned
				resolved = append(resolved, fontfallback.SceneFont{Font: font})
				r.work.bundledBytes += int64(len(owned))
				r.work.resolvedBytes += int64(len(owned))
			}
			for _, value := range covered {
				delete(remaining, value)
			}
		}
	}
	if len(remaining) == 0 {
		return resolved, nil
	}
	if r.next == nil {
		return resolved, nil
	}

	downstreamRequest := request
	downstreamRequest.Runes = sortedFallbackRunes(remaining)
	downstream, err := r.next.ResolveFallbacks(ctx, downstreamRequest)
	if err != nil {
		return nil, err
	}
	for index, font := range downstream {
		if int64(len(font.Data)) > r.limits.MaxResolvedBytes-r.work.resolvedBytes {
			return nil, fmt.Errorf("d2fonts: resolved fallback font bytes exceed limit %d at downstream resource %d", r.limits.MaxResolvedBytes, index)
		}
		font.Data, err = cloneFallbackBytes(ctx, font.Data)
		if err != nil {
			return nil, err
		}
		r.work.resolvedBytes += int64(len(font.Data))
		resolved = append(resolved, fontfallback.SceneFont{Font: sceneFont(font)})
	}
	return resolved, nil
}

func sceneFont(font FallbackFont) fontfallback.Font {
	return fontfallback.Font{
		Name: font.Name, MIMEType: font.MIMEType, Data: font.Data, FaceIndex: font.FaceIndex,
	}
}

func fallbackFont(font fontfallback.Font) FallbackFont {
	return FallbackFont{
		Name: font.Name, MIMEType: font.MIMEType, Data: font.Data, FaceIndex: font.FaceIndex,
	}
}

func (r *BundledFallbackResolver) admitBundledBytes(bytes int64) error {
	if bytes > r.limits.MaxBundledBytes-r.work.bundledBytes {
		return fmt.Errorf("d2fonts: bundled fallback font bytes exceed limit %d while retaining Noto Color Emoji", r.limits.MaxBundledBytes)
	}
	if bytes > r.limits.MaxResolvedBytes-r.work.resolvedBytes {
		return fmt.Errorf("d2fonts: resolved fallback font bytes exceed limit %d while retaining bundled Noto Color Emoji", r.limits.MaxResolvedBytes)
	}
	return nil
}

func shouldTryBundledEmoji(values map[rune]struct{}) bool {
	for value := range values {
		if fontface.BundledNotoColorEmojiCoversRune(value) {
			return true
		}
	}
	return false
}

func coveredBundledRunes(ctx context.Context, face *fontface.ParsedFace, values map[rune]struct{}) ([]rune, error) {
	covered := make([]rune, 0)
	index := 0
	for value := range values {
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

// acquireResolve serializes the resolver-local shaping face and cumulative
// budget accounting while allowing a waiting caller to cancel promptly.
func (r *BundledFallbackResolver) acquireResolve(ctx context.Context) (func(), error) {
	r.resolveOnce.Do(func() { r.resolveGate = make(chan struct{}, 1) })
	select {
	case r.resolveGate <- struct{}{}:
		return func() { <-r.resolveGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func sortedFallbackRunes(values map[rune]struct{}) []rune {
	result := make([]rune, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func cloneFallbackBytes(ctx context.Context, data []byte) ([]byte, error) {
	result := make([]byte, len(data))
	const chunkSize = 64 * 1024
	for offset := 0; offset < len(data); offset += chunkSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(offset+chunkSize, len(data))
		copy(result[offset:end], data[offset:end])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
