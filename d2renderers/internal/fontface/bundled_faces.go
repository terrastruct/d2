package fontface

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"

	gotextfont "github.com/go-text/typesetting/font"
)

// D2 ships a fixed set of ordinary fonts. Keeping this ceiling independent of
// callers prevents this package-level registry from becoming an unbounded font
// cache if more registration sites are added later.
const maxBundledFaceSources = 16

type bundledFaceEntry struct {
	size   int
	digest [sha256.Size]byte

	mu      sync.RWMutex
	data    []byte
	sources []BundledFaceSource
	err     error
}

func (e *bundledFaceEntry) load(candidate []byte) error {
	e.mu.RLock()
	loaded := e.sources != nil || e.err != nil
	err := e.err
	e.mu.RUnlock()
	if loaded {
		return err
	}

	// Authenticate a private snapshot before acquiring the parse lock. A bad
	// candidate cannot poison the fixed registry, and the snapshot closes the
	// ownership boundary around the parsers' retained table slices.
	owned := bytes.Clone(candidate)
	if len(owned) != e.size || sha256.Sum256(owned) != e.digest {
		return fmt.Errorf("bundled font resource is not authenticated")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sources != nil || e.err != nil {
		return e.err
	}
	collection, err := parseFaceCollectionWithLimitUsingParserAndDigest(owned, maxParsedFontFaces, gotextfont.ParseTTC, e.digest)
	if err != nil {
		e.err = err
		return err
	}
	sources := make([]BundledFaceSource, collection.NumFaces())
	for index := range sources {
		face, err := collection.Face(index)
		if err != nil {
			e.err = err
			return err
		}
		face.source.bundledOutline = true
		sources[index].face = face
	}
	e.data = owned
	e.sources = sources
	return nil
}

var bundledFaceRegistry struct {
	sync.RWMutex
	entries  []*bundledFaceEntry
	byDigest map[[sha256.Size]byte]*bundledFaceEntry
	bySize   map[int][]*bundledFaceEntry
}

// RegisterBundledFace records the authenticated size and digest of a D2 font.
// It does not retain data. Parsing and a private backing copy are created lazily
// from the first matching candidate, so a public font registry cannot expose
// or mutate parser-owned tables. Callers must not register dynamic fonts.
func RegisterBundledFace(data []byte, digest [sha256.Size]byte) error {
	if len(data) == 0 || digest == ([sha256.Size]byte{}) {
		return fmt.Errorf("invalid bundled font registration")
	}
	bundledFaceRegistry.Lock()
	defer bundledFaceRegistry.Unlock()
	if bundledFaceRegistry.byDigest == nil {
		bundledFaceRegistry.byDigest = make(map[[sha256.Size]byte]*bundledFaceEntry)
		bundledFaceRegistry.bySize = make(map[int][]*bundledFaceEntry)
	}
	if existing := bundledFaceRegistry.byDigest[digest]; existing != nil {
		if len(data) == existing.size {
			return nil
		}
		return fmt.Errorf("duplicate bundled font digest has size %d, want %d", len(data), existing.size)
	}
	if len(bundledFaceRegistry.entries) >= maxBundledFaceSources {
		return fmt.Errorf("bundled font registry exceeds limit %d", maxBundledFaceSources)
	}
	entry := &bundledFaceEntry{size: len(data), digest: digest}
	bundledFaceRegistry.entries = append(bundledFaceRegistry.entries, entry)
	bundledFaceRegistry.byDigest[digest] = entry
	bundledFaceRegistry.bySize[len(data)] = append(bundledFaceRegistry.bySize[len(data)], entry)
	return nil
}

// RegisteredBundledFace authenticates data against D2's fixed bundled-font
// registry. A non-match is not an error so arbitrary scene fonts continue down
// the ordinary bounded parser path.
func RegisteredBundledFace(data []byte, faceIndex uint16) (*BundledFaceSource, bool, error) {
	entry := registeredBundledFaceEntry(data, nil)
	return registeredBundledFaceSource(entry, data, faceIndex)
}

// RegisteredBundledFaceDigest is RegisteredBundledFace for callers that have
// already computed the source digest as part of their own bounded cache key.
func RegisteredBundledFaceDigest(data []byte, faceIndex uint16, digest [sha256.Size]byte) (*BundledFaceSource, bool, error) {
	entry := registeredBundledFaceEntry(data, &digest)
	return registeredBundledFaceSource(entry, data, faceIndex)
}

// RegisteredBundledFaceBackingDigest identifies the exact process-owned
// backing of a registered font without scanning it. The source is still
// authenticated and dual-parsed by RegisteredBundledFaceDigest before use.
// Byte-identical copies deliberately do not match this identity-only path.
func RegisteredBundledFaceBackingDigest(data []byte) ([sha256.Size]byte, bool) {
	if len(data) == 0 {
		return [sha256.Size]byte{}, false
	}
	bundledFaceRegistry.RLock()
	defer bundledFaceRegistry.RUnlock()
	for _, candidate := range bundledFaceRegistry.bySize[len(data)] {
		candidate.mu.RLock()
		matches := sameSlice(data, candidate.data)
		candidate.mu.RUnlock()
		if matches {
			return candidate.digest, true
		}
	}
	return [sha256.Size]byte{}, false
}

func registeredBundledFaceEntry(data []byte, knownDigest *[sha256.Size]byte) *bundledFaceEntry {
	if len(data) == 0 {
		return nil
	}
	var digest [sha256.Size]byte
	if knownDigest == nil {
		bundledFaceRegistry.RLock()
		for _, candidate := range bundledFaceRegistry.bySize[len(data)] {
			candidate.mu.RLock()
			loadedData := candidate.data
			candidate.mu.RUnlock()
			if loadedData != nil && (sameSlice(data, loadedData) || bytes.Equal(data, loadedData)) {
				bundledFaceRegistry.RUnlock()
				return candidate
			}
		}
		bundledFaceRegistry.RUnlock()
		digest = sha256.Sum256(data)
	} else {
		digest = *knownDigest
	}
	bundledFaceRegistry.RLock()
	entry := bundledFaceRegistry.byDigest[digest]
	if entry == nil || len(data) != entry.size {
		entry = nil
	}
	bundledFaceRegistry.RUnlock()
	if entry != nil && knownDigest == nil {
		entry.mu.RLock()
		loadedData := entry.data
		entry.mu.RUnlock()
		if loadedData != nil && !sameSlice(data, loadedData) && !bytes.Equal(data, loadedData) {
			entry = nil
		}
	}
	return entry
}

func registeredBundledFaceSource(entry *bundledFaceEntry, data []byte, faceIndex uint16) (*BundledFaceSource, bool, error) {
	if entry == nil {
		return nil, false, nil
	}
	if err := entry.load(data); err != nil {
		return nil, true, err
	}
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	if int(faceIndex) >= len(entry.sources) {
		return nil, true, fmt.Errorf("load font face %d: collection has %d faces", faceIndex, len(entry.sources))
	}
	return &entry.sources[faceIndex], true, nil
}

func sameSlice(left, right []byte) bool {
	return len(left) == len(right) && (len(left) == 0 || &left[0] == &right[0])
}
