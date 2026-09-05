// Package fontfallback contains the renderer-internal handoff used to retain
// trusted fallback assets across multiple scene builds without widening D2's
// public font resolver interface.
package fontfallback

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"sync"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

// Font is one resolved font face. Data is owned by the result and may be
// retained by a scene builder. FaceIndex selects a face in a collection.
type Font struct {
	Name      string
	MIMEType  string
	Data      []byte
	FaceIndex uint16
}

// Request carries missing code points and the source text style.
type Request struct {
	Runes  []rune
	Family string
	Style  string
	Weight int
}

// SceneFont is the internal scene-builder result. Shared is set only for data
// copied into the resolver-owned immutable cache. Face is a build-local clone
// whose mutable shaping caches are never shared between scene builds.
type SceneFont struct {
	Font
	ID     d2scene.AssetID
	Face   *fontface.ParsedFace
	Shared bool
}

// CacheStats reports retained trusted-asset preparation work.
type CacheStats struct {
	Assets      int
	Hashes      uint64
	Copies      uint64
	CopiedBytes int64
}

// SceneResolveFunc resolves fonts for the internal scene-builder handoff.
type SceneResolveFunc func(context.Context, Request) ([]SceneFont, error)

// SceneCache is embedded privately by a resolver that supports the internal
// scene handoff. Its unexported marker keeps the capability out of public
// resolver interfaces while allowing this package to recover the cache.
type SceneCache struct {
	resolve SceneResolveFunc

	mu      sync.Mutex
	trusted *SceneFont
	stats   CacheStats
}

// NewSceneCache creates a private scene handoff backed by resolve.
func NewSceneCache(resolve SceneResolveFunc) *SceneCache {
	return &SceneCache{resolve: resolve}
}

func (c *SceneCache) sceneFallbackCache() *SceneCache {
	return c
}

// HasTrusted reports whether PrepareTrusted has retained its trusted resource.
func (c *SceneCache) HasTrusted() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.trusted != nil
}

type sceneCacheProvider interface {
	sceneFallbackCache() *SceneCache
}

// ResolveForScene uses a resolver's private scene handoff when available.
func ResolveForScene(ctx context.Context, resolver any, request Request) ([]SceneFont, bool, error) {
	provider, ok := resolver.(sceneCacheProvider)
	if !ok {
		return nil, false, nil
	}
	cache := provider.sceneFallbackCache()
	if cache == nil || cache.resolve == nil {
		return nil, false, nil
	}
	fonts, err := cache.resolve(ctx, request)
	return fonts, true, err
}

// CacheStatsFor returns private cache accounting for validation and metrics.
func CacheStatsFor(resolver any) (CacheStats, bool) {
	provider, ok := resolver.(sceneCacheProvider)
	if !ok {
		return CacheStats{}, false
	}
	cache := provider.sceneFallbackCache()
	if cache == nil {
		return CacheStats{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.stats, true
}

// PrepareTrusted copies and hashes the resolver's trusted source exactly once
// for this cache. A canceled attempt publishes neither data nor accounting.
func (c *SceneCache) PrepareTrusted(ctx context.Context, source Font) (SceneFont, bool, error) {
	if c == nil {
		return SceneFont{}, false, fmt.Errorf("font fallback: nil scene cache")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SceneFont{}, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.trusted != nil {
		return *c.trusted, false, nil
	}
	owned := make([]byte, len(source.Data))
	id, err := hashFont(ctx, source, owned)
	if err != nil {
		return SceneFont{}, false, err
	}
	trusted := SceneFont{
		Font: Font{
			Name: source.Name, MIMEType: source.MIMEType, Data: owned, FaceIndex: source.FaceIndex,
		},
		ID:     id,
		Shared: true,
	}
	c.trusted = &trusted
	c.stats.Assets = 1
	c.stats.Hashes++
	c.stats.Copies++
	c.stats.CopiedBytes += int64(len(owned))
	return trusted, true, nil
}

// AssetID computes the scene font identity without retaining caller data.
func AssetID(ctx context.Context, font Font) (d2scene.AssetID, error) {
	return hashFont(ctx, font, nil)
}

func hashFont(ctx context.Context, font Font, destination []byte) (d2scene.AssetID, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if destination != nil && len(destination) != len(font.Data) {
		return "", fmt.Errorf("font fallback: copy destination length %d differs from source %d", len(destination), len(font.Data))
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(font.MIMEType))
	_, _ = hasher.Write([]byte{0})
	var face [2]byte
	binary.BigEndian.PutUint16(face[:], font.FaceIndex)
	_, _ = hasher.Write(face[:])
	if err := copyAndHash(ctx, destination, font.Data, hasher); err != nil {
		return "", err
	}
	return d2scene.AssetID(fmt.Sprintf("font:fallback:%x", hasher.Sum(nil))), nil
}

func copyAndHash(ctx context.Context, destination, source []byte, hasher hash.Hash) error {
	const chunkBytes = 32 * 1024
	for offset := 0; offset < len(source); offset += chunkBytes {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(offset+chunkBytes, len(source))
		chunk := source[offset:end]
		if destination != nil {
			copy(destination[offset:end], chunk)
			chunk = destination[offset:end]
		}
		_, _ = hasher.Write(chunk)
	}
	return ctx.Err()
}
