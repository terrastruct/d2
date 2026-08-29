package d2raster

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"sync"
	"unsafe"
	"weak"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

const (
	assetDigestChunkBytes   = 32 * 1024
	cacheEntryOverheadBytes = int64(256)
)

// RenderSessionOptions bounds reusable parsed and decoded asset state. Every
// field must be positive. MaxCacheEntries independently caps asset entries and
// immutable-source key memos, so a session may retain up to twice that many
// entries. MaxCacheBytes jointly bounds both groups. Entries charge source and
// MIME bytes for fonts, validated decoded footprint and MIME bytes for raster
// images, or the encoded source length for key memos, plus
// cacheEntryOverheadBytes each.
type RenderSessionOptions struct {
	MaxCacheEntries    int
	MaxCacheBytes      int64
	MaxConcurrentLoads int
}

// RenderSessionStats is an atomic snapshot of cache activity. Hits, Misses,
// and Waits describe parsed-font and decoded-raster lookups. MemoHits,
// MemoMisses, and MemoWaits describe content-key lookups; Hashes counts full
// source scans. SkippedOversize and MemoSkipped count successful results that
// could not be retained under MaxCacheBytes.
type RenderSessionStats struct {
	Hits            uint64
	Misses          uint64
	Waits           uint64
	Evictions       uint64
	SkippedOversize uint64
	Entries         int
	Bytes           int64
	MemoHits        uint64
	MemoMisses      uint64
	MemoWaits       uint64
	Hashes          uint64
	MemoEvictions   uint64
	MemoSkipped     uint64
	MemoEntries     int
	MemoBytes       int64
	RetainedBytes   int64
	ActiveLoads     int
}

// RenderSession reuses immutable parsed fonts and decoded raster pixels
// across renders. Scene validation and every per-document resource limit are
// still applied independently on every Render call. Callers may replace an
// asset value, but must not mutate an existing Data backing allocation, as
// required by d2scene.Asset.
type RenderSession struct {
	options   RenderSessionOptions
	loadSlots chan struct{}

	mu          sync.Mutex
	entries     map[assetCacheKey]*list.Element
	lru         list.List
	flights     map[assetCacheKey]*assetCacheFlight
	bytes       int64
	memos       map[assetMemoIdentity]*list.Element
	memoLRU     list.List
	memoFlights map[assetMemoIdentity]*assetMemoFlight
	memoBytes   int64
	stats       RenderSessionStats
}

type cachedAssetKind uint8

const (
	cachedFontAsset cachedAssetKind = iota + 1
	cachedRasterAsset
)

type assetCacheKey struct {
	kind          cachedAssetKind
	digest        [sha256.Size]byte
	mimeType      string
	fontFaceIndex uint16
	pixelWidth    int
	pixelHeight   int
	decodedBytes  int64
}

type cachedAssetValue struct {
	fontFace     *fontface.ParsedFace
	raster       *preparedRasterAsset
	decodedBytes int64
}

type assetCacheEntry struct {
	key    assetCacheKey
	value  cachedAssetValue
	charge int64
}

type assetCacheFlight struct {
	done  chan struct{}
	value cachedAssetValue
	err   error
}

type assetMemoIdentity struct {
	document      weak.Pointer[d2scene.Document]
	assetID       d2scene.AssetID
	kind          cachedAssetKind
	mimeType      string
	fontFaceIndex uint16
	pixelWidth    int
	pixelHeight   int
	decodedBytes  int64
	dataPointer   weak.Pointer[byte]
	dataLength    int
}

type assetMemoEntry struct {
	identity assetMemoIdentity
	key      assetCacheKey
	charge   int64
}

type assetMemoFlight struct {
	done chan struct{}
	key  assetCacheKey
	err  error
}

// NewRenderSession creates a bounded, concurrent-safe render session.
func NewRenderSession(options RenderSessionOptions) (*RenderSession, error) {
	if options.MaxCacheEntries <= 0 || options.MaxCacheBytes <= 0 || options.MaxConcurrentLoads <= 0 {
		return nil, fmt.Errorf("d2raster: every render session limit must be positive")
	}
	return &RenderSession{
		options:     options,
		loadSlots:   make(chan struct{}, options.MaxConcurrentLoads),
		entries:     make(map[assetCacheKey]*list.Element),
		flights:     make(map[assetCacheKey]*assetCacheFlight),
		memos:       make(map[assetMemoIdentity]*list.Element),
		memoFlights: make(map[assetMemoIdentity]*assetMemoFlight),
	}, nil
}

// Stats returns a race-free snapshot of cache counters and current retained
// charged storage.
func (s *RenderSession) Stats() RenderSessionStats {
	if s == nil {
		return RenderSessionStats{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.stats
	stats.Entries = len(s.entries)
	stats.Bytes = s.bytes
	stats.MemoEntries = len(s.memos)
	stats.MemoBytes = s.memoBytes
	stats.RetainedBytes = s.bytes + s.memoBytes
	return stats
}

func digestAssetBytes(ctx context.Context, data []byte) ([sha256.Size]byte, error) {
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	hasher := sha256.New()
	for offset := 0; offset < len(data); offset += assetDigestChunkBytes {
		if err := ctx.Err(); err != nil {
			return [sha256.Size]byte{}, err
		}
		end := offset + assetDigestChunkBytes
		if end > len(data) {
			end = len(data)
		}
		_, _ = hasher.Write(data[offset:end])
	}
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func (s *RenderSession) memoizedCacheKey(ctx context.Context, document *d2scene.Document, id d2scene.AssetID, kind cachedAssetKind, mimeType string, data []byte, fontFaceIndex uint16, pixelWidth, pixelHeight int, decodedBytes int64) (assetCacheKey, error) {
	identity := assetMemoIdentity{
		document: weak.Make(document), assetID: id, kind: kind, mimeType: mimeType,
		fontFaceIndex: fontFaceIndex, pixelWidth: pixelWidth, pixelHeight: pixelHeight, decodedBytes: decodedBytes,
		dataPointer: weak.Make(unsafe.SliceData(data)), dataLength: len(data),
	}
	for {
		if err := ctx.Err(); err != nil {
			return assetCacheKey{}, err
		}
		s.mu.Lock()
		if element, ok := s.memos[identity]; ok {
			s.memoLRU.MoveToFront(element)
			s.stats.MemoHits++
			key := element.Value.(*assetMemoEntry).key
			s.mu.Unlock()
			return key, nil
		}
		if flight, ok := s.memoFlights[identity]; ok {
			s.stats.MemoWaits++
			s.mu.Unlock()
			key, retry, err := waitForMemoFlight(ctx, flight)
			if err != nil {
				return assetCacheKey{}, err
			}
			if retry {
				continue
			}
			return key, nil
		}
		s.mu.Unlock()

		if err := s.acquireLoad(ctx); err != nil {
			return assetCacheKey{}, err
		}
		s.mu.Lock()
		if element, ok := s.memos[identity]; ok {
			s.memoLRU.MoveToFront(element)
			s.stats.MemoHits++
			key := element.Value.(*assetMemoEntry).key
			s.mu.Unlock()
			s.releaseLoad()
			return key, nil
		}
		if flight, ok := s.memoFlights[identity]; ok {
			s.stats.MemoWaits++
			s.mu.Unlock()
			s.releaseLoad()
			key, retry, err := waitForMemoFlight(ctx, flight)
			if err != nil {
				return assetCacheKey{}, err
			}
			if retry {
				continue
			}
			return key, nil
		}
		flight := &assetMemoFlight{done: make(chan struct{})}
		s.memoFlights[identity] = flight
		s.stats.MemoMisses++
		s.mu.Unlock()

		digest, err := digestAssetBytes(ctx, data)
		key := assetCacheKey{
			kind: kind, digest: digest, mimeType: mimeType, fontFaceIndex: fontFaceIndex,
			pixelWidth: pixelWidth, pixelHeight: pixelHeight, decodedBytes: decodedBytes,
		}
		s.mu.Lock()
		if err == nil {
			err = ctx.Err()
		}
		delete(s.memoFlights, identity)
		flight.key = key
		flight.err = err
		if err == nil {
			s.stats.Hashes++
			s.insertMemoLocked(identity, key, memoEntryCharge(data, id, mimeType))
		}
		close(flight.done)
		s.mu.Unlock()
		s.releaseLoad()
		return key, err
	}
}

func waitForMemoFlight(ctx context.Context, flight *assetMemoFlight) (assetCacheKey, bool, error) {
	select {
	case <-ctx.Done():
		return assetCacheKey{}, false, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return assetCacheKey{}, false, err
		}
		if flight.err == nil {
			return flight.key, false, nil
		}
		return assetCacheKey{}, true, nil
	}
}

func cacheEntryCharge(payloadBytes int64, mimeType string) int64 {
	if payloadBytes < 0 || payloadBytes > math.MaxInt64-cacheEntryOverheadBytes {
		return math.MaxInt64
	}
	charge := payloadBytes + cacheEntryOverheadBytes
	if int64(len(mimeType)) > math.MaxInt64-charge {
		return math.MaxInt64
	}
	return charge + int64(len(mimeType))
}

func memoEntryCharge(data []byte, id d2scene.AssetID, mimeType string) int64 {
	charge := cacheEntryCharge(int64(len(data)), mimeType)
	if int64(len(id)) > math.MaxInt64-charge {
		return math.MaxInt64
	}
	return charge + int64(len(id))
}

func (s *RenderSession) lookup(key assetCacheKey) (cachedAssetValue, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	element, ok := s.entries[key]
	if !ok {
		return cachedAssetValue{}, false
	}
	s.lru.MoveToFront(element)
	s.stats.Hits++
	return element.Value.(*assetCacheEntry).value, true
}

func (s *RenderSession) getOrLoad(ctx context.Context, key assetCacheKey, charge int64, load func(context.Context) (cachedAssetValue, error)) (cachedAssetValue, error) {
	for {
		if err := ctx.Err(); err != nil {
			return cachedAssetValue{}, err
		}
		s.mu.Lock()
		if element, ok := s.entries[key]; ok {
			s.lru.MoveToFront(element)
			s.stats.Hits++
			value := element.Value.(*assetCacheEntry).value
			s.mu.Unlock()
			return value, nil
		}
		if flight, ok := s.flights[key]; ok {
			s.stats.Waits++
			s.mu.Unlock()
			value, retry, err := waitForAssetFlight(ctx, flight)
			if err != nil {
				return cachedAssetValue{}, err
			}
			if retry {
				continue
			}
			return value, nil
		}
		s.mu.Unlock()

		// Admission precedes flight creation so the session retains at most
		// MaxConcurrentLoads unique in-flight keys and result closures.
		if err := s.acquireLoad(ctx); err != nil {
			return cachedAssetValue{}, err
		}
		s.mu.Lock()
		if element, ok := s.entries[key]; ok {
			s.lru.MoveToFront(element)
			s.stats.Hits++
			value := element.Value.(*assetCacheEntry).value
			s.mu.Unlock()
			s.releaseLoad()
			return value, nil
		}
		if flight, ok := s.flights[key]; ok {
			s.stats.Waits++
			s.mu.Unlock()
			s.releaseLoad()
			value, retry, err := waitForAssetFlight(ctx, flight)
			if err != nil {
				return cachedAssetValue{}, err
			}
			if retry {
				continue
			}
			return value, nil
		}
		flight := &assetCacheFlight{done: make(chan struct{})}
		s.flights[key] = flight
		s.stats.Misses++
		s.mu.Unlock()

		value, err := load(ctx)
		s.mu.Lock()
		if err == nil {
			err = ctx.Err()
		}
		delete(s.flights, key)
		flight.value = value
		flight.err = err
		if err == nil {
			s.insertLocked(key, value, charge)
		}
		close(flight.done)
		s.mu.Unlock()
		s.releaseLoad()
		return value, err
	}
}

func waitForAssetFlight(ctx context.Context, flight *assetCacheFlight) (cachedAssetValue, bool, error) {
	select {
	case <-ctx.Done():
		return cachedAssetValue{}, false, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return cachedAssetValue{}, false, err
		}
		if flight.err == nil {
			return flight.value, false, nil
		}
		// Failed and canceled loads are never cached. A waiter retries with
		// its own context so one owner cannot poison other callers.
		return cachedAssetValue{}, true, nil
	}
}

func (s *RenderSession) acquireLoad(ctx context.Context) error {
	select {
	case s.loadSlots <- struct{}{}:
		s.mu.Lock()
		s.stats.ActiveLoads++
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *RenderSession) releaseLoad() {
	<-s.loadSlots
	s.mu.Lock()
	s.stats.ActiveLoads--
	s.mu.Unlock()
}

func (s *RenderSession) insertLocked(key assetCacheKey, value cachedAssetValue, charge int64) {
	if charge > s.options.MaxCacheBytes {
		s.stats.SkippedOversize++
		return
	}
	for len(s.entries) >= s.options.MaxCacheEntries {
		s.evictAssetLocked()
	}
	for charge > s.options.MaxCacheBytes-s.bytes-s.memoBytes {
		if s.memoLRU.Back() != nil {
			s.evictMemoLocked()
		} else {
			s.evictAssetLocked()
		}
	}
	key.mimeType = strings.Clone(key.mimeType)
	entry := &assetCacheEntry{key: key, value: value, charge: charge}
	element := s.lru.PushFront(entry)
	s.entries[key] = element
	s.bytes += charge
}

func (s *RenderSession) insertMemoLocked(identity assetMemoIdentity, key assetCacheKey, charge int64) {
	if charge > s.options.MaxCacheBytes {
		s.stats.MemoSkipped++
		return
	}
	for len(s.memos) >= s.options.MaxCacheEntries {
		s.evictMemoLocked()
	}
	for charge > s.options.MaxCacheBytes-s.bytes-s.memoBytes {
		if s.memoLRU.Back() == nil {
			s.stats.MemoSkipped++
			return
		}
		s.evictMemoLocked()
	}
	identity.assetID = d2scene.AssetID(strings.Clone(string(identity.assetID)))
	identity.mimeType = strings.Clone(identity.mimeType)
	key.mimeType = identity.mimeType
	entry := &assetMemoEntry{identity: identity, key: key, charge: charge}
	element := s.memoLRU.PushFront(entry)
	s.memos[identity] = element
	s.memoBytes += charge
}

func (s *RenderSession) evictAssetLocked() {
	oldest := s.lru.Back()
	if oldest == nil {
		return
	}
	entry := oldest.Value.(*assetCacheEntry)
	delete(s.entries, entry.key)
	s.lru.Remove(oldest)
	s.bytes -= entry.charge
	s.stats.Evictions++
}

func (s *RenderSession) evictMemoLocked() {
	oldest := s.memoLRU.Back()
	if oldest == nil {
		return
	}
	entry := oldest.Value.(*assetMemoEntry)
	delete(s.memos, entry.identity)
	s.memoLRU.Remove(oldest)
	s.memoBytes -= entry.charge
	s.stats.MemoEvictions++
}

func copyAssetBytes(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owned := make([]byte, len(data))
	for offset := 0; offset < len(data); offset += assetDigestChunkBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + assetDigestChunkBytes
		if end > len(data) {
			end = len(data)
		}
		copy(owned[offset:end], data[offset:end])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return owned, nil
}

func (s *RenderSession) font(ctx context.Context, document *d2scene.Document, id d2scene.AssetID, asset d2scene.FontAsset) (*preparedFont, error) {
	key, err := s.memoizedCacheKey(ctx, document, id, cachedFontAsset, asset.MIMEType, asset.Data, asset.FaceIndex, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	value, err := s.getOrLoad(ctx, key, cacheEntryCharge(int64(len(asset.Data)), asset.MIMEType), func(ctx context.Context) (cachedAssetValue, error) {
		owned, err := copyAssetBytes(ctx, asset.Data)
		if err != nil {
			return cachedAssetValue{}, err
		}
		font, err := parsePreparedFont(owned, asset.FaceIndex)
		if err != nil {
			return cachedAssetValue{}, err
		}
		return cachedAssetValue{fontFace: font.face}, nil
	})
	if err != nil {
		return nil, err
	}
	face, err := value.fontFace.Clone()
	if err != nil {
		return nil, err
	}
	return newPreparedFont(face), nil
}

func (s *RenderSession) raster(ctx context.Context, document *d2scene.Document, id d2scene.AssetID, asset d2scene.RasterAsset, availableBytes int64) (*preparedRasterAsset, int64, error) {
	key, err := s.memoizedCacheKey(ctx, document, id, cachedRasterAsset, asset.MIMEType, asset.Data, 0, asset.PixelWidth, asset.PixelHeight, asset.DecodedBytes)
	if err != nil {
		return nil, 0, err
	}
	if value, ok := s.lookup(key); ok {
		if err := validateDecodedAssetBudget(id, value.decodedBytes, availableBytes); err != nil {
			return nil, 0, err
		}
		return value.raster, value.decodedBytes, nil
	}

	validation, err := validateRasterAsset(ctx, id, asset, availableBytes)
	if err != nil {
		return nil, 0, err
	}
	value, err := s.getOrLoad(ctx, key, cacheEntryCharge(validation.decodedBytes, asset.MIMEType), func(ctx context.Context) (cachedAssetValue, error) {
		prepared, err := decodeRasterAsset(ctx, id, asset, validation)
		if err != nil {
			return cachedAssetValue{}, err
		}
		return cachedAssetValue{raster: prepared, decodedBytes: validation.decodedBytes}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	// A concurrent owner may have validated the same content under a different
	// remaining document budget. Charge this caller independently regardless.
	if err := validateDecodedAssetBudget(id, value.decodedBytes, availableBytes); err != nil {
		return nil, 0, err
	}
	return value.raster, value.decodedBytes, nil
}
