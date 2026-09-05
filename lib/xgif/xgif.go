// Package xgif quantizes, normalizes, and encodes rendered animation frames as
// GIF. Frames are centered on a common canvas within GIF's 256-color limit,
// with one palette entry normalized to the white canvas background.
package xgif

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"math"
	"math/bits"
	"reflect"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/d2lang/d2/internal/testutil"

	"github.com/ericpauley/go-quantize/quantize"

	"github.com/d2lang/util-go/go2"
)

const INFINITE_LOOP = 0

var BG_COLOR = color.White

const fps = 30
const workers = 16

// animateImagesWithConcurrency encodes already-rendered frames with a bounded
// quantizer-worker ceiling. A lower value trades throughput for lower peak
// memory; frame renderers use one worker because median-cut quantization has
// substantial per-pixel scratch storage. The context is checked between
// frames and while normalizing each paletted frame. Median-cut quantization and
// dithering cannot themselves be interrupted. The final GIF encoder observes
// cancellation at bounded-output writes and on return.
// Callers must also impose per-frame and aggregate pixel ceilings appropriate
// to their environment.
func animateImagesWithConcurrency(ctx context.Context, images []image.Image, animIntervalMs, concurrency int) ([]byte, error) {
	if concurrency <= 0 || concurrency > workers {
		return nil, fmt.Errorf("GIF quantizer concurrency must be in [1,%d]", workers)
	}
	delays, err := gifFrameDelays(len(images), animIntervalMs)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return animateImages(ctx, images, delays, concurrency)
}

// QuantizeImage converts one rendered image to the indexed-color form required
// by GIF. It deliberately does not normalize the frame to an animation canvas;
// callers that render frames serially can release the larger source image as
// soon as this function returns, retain the smaller paletted result, and call
// NormalizePalettedImage after the animation dimensions are known.
//
// Median-cut quantization and Floyd-Steinberg dithering cannot be interrupted
// while they are running. The context is checked between those bounded
// operations. Callers must impose an appropriate pixel limit before calling.
func QuantizeImage(ctx context.Context, img image.Image) (*image.Paletted, error) {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if isNilImage(img) {
		return nil, fmt.Errorf("GIF image is nil")
	}
	if _, _, err := gifImageDimensions(img.Bounds()); err != nil {
		return nil, err
	}
	switch img.(type) {
	case *image.NRGBA, *image.RGBA:
		return quantizeRasterImage(ctx, img)
	default:
		return quantizeImageThroughGIF(ctx, img)
	}
}

// OpaqueQuantizationWorkspace owns reusable indexed pixels and dithering
// storage for a serial sequence of fully opaque raster frames. Its zero value
// is ready for use. Calls must not overlap, including from a Quantize callback.
// The workspace retains storage sized for the largest frame it has processed;
// assigning a zero value releases that storage for garbage collection.
type OpaqueQuantizationWorkspace struct {
	frame           image.Paletted
	pixels          []byte
	palette         color.Palette
	ditherErrors    [][3]int32
	paletteChannels [][3]int32
	paletteNodes    []paletteNode
	paletteSeeds    [8 * 8 * 8]uint8
}

// Quantize converts one opaque RGBA or NRGBA image and calls consume with the
// resulting indexed frame. The frame and its pixel and palette storage are
// borrowed: callers must finish using all three before consume returns.
func (w *OpaqueQuantizationWorkspace) Quantize(ctx context.Context, img image.Image, consume func(*image.Paletted) error) error {
	if w == nil {
		return fmt.Errorf("GIF quantization workspace is nil")
	}
	if consume == nil {
		return fmt.Errorf("GIF quantization workspace consumer is nil")
	}
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if isNilImage(img) {
		return fmt.Errorf("GIF image is nil")
	}
	if _, _, err := gifImageDimensions(img.Bounds()); err != nil {
		return err
	}
	switch img.(type) {
	case *image.NRGBA, *image.RGBA:
	default:
		return fmt.Errorf("GIF opaque quantization workspace requires an RGBA or NRGBA image")
	}
	paletted, err := quantizeRasterImageWithWorkspace(ctx, img, w, true)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := consume(paletted); err != nil {
		return err
	}
	return ctx.Err()
}

func (w *OpaqueQuantizationWorkspace) paletted(bounds image.Rectangle, palette color.Palette) *image.Paletted {
	pixelCount := bounds.Dx() * bounds.Dy()
	if cap(w.pixels) < pixelCount {
		w.pixels = make([]byte, pixelCount)
	} else {
		w.pixels = w.pixels[:pixelCount]
	}
	w.frame.Pix = w.pixels
	w.frame.Stride = bounds.Dx()
	w.frame.Rect = bounds
	w.frame.Palette = palette
	return &w.frame
}

func (w *OpaqueQuantizationWorkspace) emptyPalette() color.Palette {
	if cap(w.palette) < 256 {
		w.palette = make(color.Palette, 0, 256)
	}
	return w.palette[:0]
}

// quantizeRasterImage performs the same palette selection and dithering as
// gif.Encode without temporarily compressing and decoding the indexed pixels.
// The renderer's raster frames use these concrete image types. Other Image
// implementations retain the standard-library conversion path below.
func quantizeRasterImage(ctx context.Context, img image.Image) (*image.Paletted, error) {
	return quantizeRasterImageWithWorkspace(ctx, img, nil, false)
}

func quantizeRasterImageWithWorkspace(ctx context.Context, img image.Image, workspace *OpaqueQuantizationWorkspace, requireOpaque bool) (*image.Paletted, error) {
	paletted, opaque := quantizeFewOpaqueColorsWithWorkspace(img, workspace)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if paletted != nil {
		return finishQuantizedRaster(paletted), nil
	}
	if requireOpaque && !opaque {
		return nil, fmt.Errorf("GIF quantization workspace requires fully opaque pixels")
	}

	quantizerInput := img
	if source, ok := img.(*image.NRGBA); ok {
		// RGBA and NRGBA have the same byte representation when every pixel is
		// opaque. Raster-export frames have an opaque canvas, so present their
		// existing storage through the quantizer's concrete RGBA fast path instead
		// of allocating and filling a second full-size image.
		if opaque {
			quantizerInput = &image.RGBA{Pix: source.Pix, Stride: source.Stride, Rect: source.Rect}
		} else {
			// Keep the general NRGBA path exact as well: palette selection consumes
			// premultiplied colors, while dithering below must still consume the
			// original straight-alpha values.
			premultiplied := image.NewRGBA(source.Bounds())
			draw.Draw(premultiplied, premultiplied.Bounds(), source, source.Bounds().Min, draw.Src)
			quantizerInput = premultiplied
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	paletteStorage := make(color.Palette, 0, 256)
	if workspace != nil {
		paletteStorage = workspace.emptyPalette()
	}
	palette := quantize.MedianCutQuantizer{}.Quantize(paletteStorage, quantizerInput)
	if workspace != nil {
		workspace.palette = palette
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	if workspace == nil {
		paletted = image.NewPaletted(bounds, palette)
	} else {
		paletted = workspace.paletted(bounds, palette)
	}
	// Dither from the original color model. Although quantizerInput exposes
	// identical RGBA64 values, image/draw's optimized RGBA path accumulates
	// quantization error with slightly different intermediate rounding than its
	// NRGBA path.
	if opaque {
		ditherOpaqueRasterWithWorkspace(paletted, img, workspace)
	} else {
		ditherTranslucentRaster(paletted, img)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return finishQuantizedRaster(paletted), nil
}

type fewOpaqueColor struct {
	color color.RGBA
	count uint32
}

type fewOpaqueBucket []fewOpaqueColor

type fewOpaqueBucketRange struct {
	start, end uint16
}

func (bucket fewOpaqueBucket) split() (fewOpaqueBucket, fewOpaqueBucket) {
	minimum := [3]uint8{255, 255, 255}
	var maximum [3]uint8
	var counts [3][256]uint64
	var total uint64
	for _, entry := range bucket {
		channels := [...]uint8{entry.color.R, entry.color.G, entry.color.B}
		for axis, channel := range channels {
			minimum[axis] = min(minimum[axis], channel)
			maximum[axis] = max(maximum[axis], channel)
			counts[axis][channel] += uint64(entry.count)
		}
		total += uint64(entry.count)
	}
	axis := 2
	if maximum[0]-minimum[0] > maximum[1]-minimum[1] && maximum[0]-minimum[0] > maximum[2]-minimum[2] {
		axis = 0
	} else if maximum[1]-minimum[1] > maximum[2]-minimum[2] {
		axis = 1
	}
	var accumulated uint64
	mean := 0
	for index, count := range counts[axis] {
		mean = index
		if accumulated > total/2 || accumulated+count == total {
			break
		}
		accumulated += count
	}
	channel := func(entry fewOpaqueColor) uint8 {
		switch axis {
		case 0:
			return entry.color.R
		case 1:
			return entry.color.G
		default:
			return entry.color.B
		}
	}
	left, right := 0, len(bucket)-1
	for left < right {
		bucket[left], bucket[right] = bucket[right], bucket[left]
		for int(channel(bucket[left])) < mean && left < right {
			left++
		}
		for int(channel(bucket[right])) >= mean && left < right {
			right--
		}
	}
	if left == 0 {
		return bucket[:1], bucket[1:]
	}
	if left == len(bucket)-1 {
		return bucket[:len(bucket)-1], bucket[len(bucket)-1:]
	}
	return bucket[:left], bucket[left:]
}

// quantizeFewOpaqueColorsWithWorkspace combines the opacity check required by the general
// raster path with a fixed-size color census. When an opaque image has at most
// 256 colors, it reproduces MedianCutQuantizer's palette ordering and fills the
// exact palette indexes directly: Floyd-Steinberg error is zero at every pixel.
// The returned bool reports opacity even when there are too many colors.
func quantizeFewOpaqueColorsWithWorkspace(source image.Image, workspace *OpaqueQuantizationWorkspace) (*image.Paletted, bool) {
	bounds := source.Bounds()
	var colorsStorage [256]fewOpaqueColor
	colors := colorsStorage[:0]
	var colorSlots [512]uint16
	pixels, start, stride, _ := rasterPixels(source)
	fewColors := true
	for y := 0; y < bounds.Dy(); y++ {
		row := pixels[start+y*stride : start+y*stride+4*bounds.Dx()]
		if !fewColors {
			if !opaqueRasterPixels(row) {
				return nil, false
			}
			continue
		}
		for x := 0; x < bounds.Dx(); x++ {
			offset := x * 4
			if row[offset+3] != 255 {
				return nil, false
			}
			value := color.RGBA{R: row[offset], G: row[offset+1], B: row[offset+2], A: 255}
			key := uint32(value.R)<<16 | uint32(value.G)<<8 | uint32(value.B)
			probe := int(key*2654435761) & (len(colorSlots) - 1)
			for step := 1; ; step++ {
				entryIndex := colorSlots[probe]
				if entryIndex == 0 {
					if len(colors) == 256 {
						fewColors = false
						if !opaqueRasterPixels(row[offset+4:]) {
							return nil, false
						}
						break
					}
					colorSlots[probe] = uint16(len(colors) + 1)
					colors = append(colors, fewOpaqueColor{color: value, count: 1})
					break
				}
				entry := &colors[entryIndex-1]
				if entry.color == value {
					entry.count++
					break
				}
				probe = (probe + step) & (len(colorSlots) - 1)
			}
			if !fewColors {
				break
			}
		}
	}
	if !fewColors {
		return nil, true
	}
	if bits.UintSize == 32 {
		maximumInt := int(^uint(0) >> 1)
		if bounds.Dx() > maximumInt/bounds.Dy()/2 {
			// The standard conversion remains the compatibility path when the
			// quantizer's two-slots-per-pixel table cannot fit in an int.
			return nil, true
		}
	}
	return buildFewOpaquePaletted(source, colors, &colorSlots, workspace), true
}

func opaqueRasterPixels(pixels []byte) bool {
	const alphaPair = uint64(0xff000000ff000000)
	for len(pixels) >= 32 {
		missingAlpha := ^binary.LittleEndian.Uint64(pixels[0:8]) |
			^binary.LittleEndian.Uint64(pixels[8:16]) |
			^binary.LittleEndian.Uint64(pixels[16:24]) |
			^binary.LittleEndian.Uint64(pixels[24:32])
		if missingAlpha&alphaPair != 0 {
			return false
		}
		pixels = pixels[32:]
	}
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 255 {
			return false
		}
	}
	return true
}

func buildFewOpaquePaletted(source image.Image, colors []fewOpaqueColor, colorSlots *[512]uint16, workspace *OpaqueQuantizationWorkspace) *image.Paletted {
	bounds := source.Bounds()
	pixelCount := bounds.Dx() * bounds.Dy()
	tableSize := pixelCount * 2
	var quantizerSlots [256]int
	for index := range colors {
		value := colors[index].color
		probe := int(value.R)<<16 | int(value.G)<<8 | int(value.B)
		for step := 1; ; step++ {
			slot := probe % tableSize
			occupied := false
			for previous := 0; previous < index; previous++ {
				if quantizerSlots[previous] == slot {
					occupied = true
					break
				}
			}
			if !occupied {
				quantizerSlots[index] = slot
				break
			}
			probe += 1 + step
		}
	}
	for index := 1; index < len(colors); index++ {
		entry := colors[index]
		slot := quantizerSlots[index]
		position := index
		for position > 0 && slot < quantizerSlots[position-1] {
			colors[position] = colors[position-1]
			quantizerSlots[position] = quantizerSlots[position-1]
			position--
		}
		colors[position] = entry
		quantizerSlots[position] = slot
	}
	var bucketStorage [256]fewOpaqueBucketRange
	bucketStorage[0] = fewOpaqueBucketRange{end: uint16(len(colors))}
	head, tail, bucketCount := 0, 1, 1
	appendBucket := func(start, end uint16) {
		bucketStorage[tail] = fewOpaqueBucketRange{start: start, end: end}
		tail = (tail + 1) & (len(bucketStorage) - 1)
		bucketCount++
	}
	for bucketCount < len(colors) {
		bucketRange := bucketStorage[head]
		head = (head + 1) & (len(bucketStorage) - 1)
		bucketCount--
		bucket := fewOpaqueBucket(colors[bucketRange.start:bucketRange.end])
		if len(bucket) < 2 {
			appendBucket(bucketRange.start, bucketRange.end)
		} else if len(bucket) == 2 {
			appendBucket(bucketRange.start, bucketRange.start+1)
			appendBucket(bucketRange.start+1, bucketRange.end)
		} else {
			left, _ := bucket.split()
			split := bucketRange.start + uint16(len(left))
			appendBucket(bucketRange.start, split)
			appendBucket(split, bucketRange.end)
		}
	}
	var palette color.Palette
	if workspace == nil {
		palette = make(color.Palette, bucketCount, 256)
	} else {
		palette = workspace.emptyPalette()[:bucketCount]
		workspace.palette = palette
	}
	var paletteKeys [256]uint32
	clear(colorSlots[:])
	for index := range bucketCount {
		bucket := bucketStorage[(head+index)&(len(bucketStorage)-1)]
		palette[index] = colors[bucket.start].color
		c := colors[bucket.start].color
		key := uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
		paletteKeys[index] = key
		probe := int(key*2654435761) & (len(colorSlots) - 1)
		for step := 1; ; step++ {
			if colorSlots[probe] == 0 {
				colorSlots[probe] = uint16(index + 1)
				break
			}
			probe = (probe + step) & (len(colorSlots) - 1)
		}
	}
	var paletted *image.Paletted
	if workspace == nil {
		paletted = image.NewPaletted(bounds, palette)
	} else {
		paletted = workspace.paletted(bounds, palette)
	}
	pixels, start, stride, _ := rasterPixels(source)
	for y := 0; y < bounds.Dy(); y++ {
		row := pixels[start+y*stride : start+y*stride+4*bounds.Dx()]
		destination := paletted.Pix[y*paletted.Stride : y*paletted.Stride+bounds.Dx()]
		for x := range destination {
			offset := x * 4
			key := uint32(row[offset])<<16 | uint32(row[offset+1])<<8 | uint32(row[offset+2])
			probe := int(key*2654435761) & (len(colorSlots) - 1)
			for step := 1; ; step++ {
				paletteIndex := colorSlots[probe]
				if paletteIndex != 0 {
					if paletteKeys[paletteIndex-1] == key {
						destination[x] = uint8(paletteIndex - 1)
						break
					}
				}
				probe = (probe + step) & (len(colorSlots) - 1)
			}
		}
	}
	return paletted
}

func finishQuantizedRaster(paletted *image.Paletted) *image.Paletted {
	if paletted.Rect.Min != (image.Point{}) {
		paletted.Rect = paletted.Rect.Sub(paletted.Rect.Min)
	}

	// GIF color tables have a power-of-two length with at least two entries.
	// Encoding discards partial alpha, decoding restores opaque RGB, and the
	// first fully transparent entry is restored as transparent black.
	palette := paletted.Palette
	paletteLength := len(palette)
	tableSize := 2
	if paletteLength > 2 {
		tableSize = 1 << bits.Len(uint(paletteLength-1))
	}
	if cap(palette) < tableSize {
		expanded := make(color.Palette, tableSize)
		copy(expanded, palette)
		palette = expanded
	} else {
		palette = palette[:tableSize]
	}
	transparentIndex := -1
	for index := 0; index < paletteLength; index++ {
		r, g, b, a := palette[index].RGBA()
		palette[index] = color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
		if transparentIndex < 0 && a == 0 {
			transparentIndex = index
		}
	}
	for index := paletteLength; index < tableSize; index++ {
		palette[index] = color.RGBA{A: 255}
	}
	if transparentIndex >= 0 {
		palette[transparentIndex] = color.RGBA{}
	}
	paletted.Palette = palette
	return paletted
}

type palettePoint struct {
	channels [4]uint16
	index    uint8
}

type paletteNode struct {
	point       palettePoint
	left, right int16
	axis        uint8
}

type paletteTree struct {
	nodes        []paletteNode
	seedNodes    *[8 * 8 * 8]uint8
	root         int16
	uniqueColors bool
}

const paletteTraversalStackCapacity = 16

// newPaletteTree indexes a palette for exact nearest-color queries.
// A balanced spatial index avoids testing every palette entry for every GIF
// pixel while retaining Palette.Index's first-entry tie breaking.
func newPaletteTree(palette color.Palette, dimensions int) paletteTree {
	return newPaletteTreeWithStorage(palette, dimensions, nil)
}

func newPaletteTreeWithStorage(palette color.Palette, dimensions int, storage []paletteNode) paletteTree {
	var nodes []paletteNode
	if cap(storage) < len(palette) {
		nodes = make([]paletteNode, len(palette))
	} else {
		nodes = storage[:len(palette)]
	}
	for index, c := range palette {
		r, g, b, a := c.RGBA()
		nodes[index].point = palettePoint{
			channels: [4]uint16{uint16(r), uint16(g), uint16(b), uint16(a)},
			index:    uint8(index),
		}
	}
	uniqueColors := true
	for index, node := range nodes {
		for _, previous := range nodes[:index] {
			if node.point.channels == previous.point.channels {
				uniqueColors = false
				break
			}
		}
		if !uniqueColors {
			break
		}
	}
	// The node slice doubles as the partitioning workspace, avoiding a second
	// allocation for an intermediate point slice.
	tree := paletteTree{nodes: nodes, root: -1, uniqueColors: uniqueColors}
	var build func([]paletteNode, int) int16
	build = func(subset []paletteNode, offset int) int16 {
		if len(subset) == 0 {
			return -1
		}
		axis := widestPaletteAxis(subset, dimensions)
		sortPaletteNodes(subset, axis)
		middle := len(subset) / 2
		nodeIndex := offset + middle
		left := build(subset[:middle], offset)
		right := build(subset[middle+1:], nodeIndex+1)
		tree.nodes[nodeIndex].left = left
		tree.nodes[nodeIndex].right = right
		tree.nodes[nodeIndex].axis = uint8(axis)
		return int16(nodeIndex)
	}
	tree.root = build(nodes, 0)
	return tree
}

func (tree *paletteTree) prepareSeeds() {
	tree.prepareSeedsWithStorage(nil)
}

func (tree *paletteTree) prepareSeedsWithStorage(storage *[8 * 8 * 8]uint8) {
	if len(tree.nodes) <= 16 {
		return
	}
	seeds := storage
	if seeds == nil {
		seeds = new([8 * 8 * 8]uint8)
	}
	var nodeForIndex [256]uint8
	for nodeIndex, node := range tree.nodes {
		nodeForIndex[node.point.index] = uint8(nodeIndex)
	}
	for r := range 8 {
		for g := range 8 {
			for b := range 8 {
				index := r<<6 | g<<3 | b
				paletteIndex := tree.nearest(int32(r<<13|1<<12), int32(g<<13|1<<12), int32(b<<13|1<<12))
				seeds[index] = nodeForIndex[paletteIndex]
			}
		}
	}
	tree.seedNodes = seeds
}

func widestPaletteAxis(nodes []paletteNode, dimensions int) int {
	minimum := nodes[0].point.channels
	maximum := nodes[0].point.channels
	for _, node := range nodes[1:] {
		for axis := range dimensions {
			minimum[axis] = min(minimum[axis], node.point.channels[axis])
			maximum[axis] = max(maximum[axis], node.point.channels[axis])
		}
	}
	widestAxis := 0
	if maximum[1]-minimum[1] > maximum[widestAxis]-minimum[widestAxis] {
		widestAxis = 1
	}
	if maximum[2]-minimum[2] > maximum[widestAxis]-minimum[widestAxis] {
		widestAxis = 2
	}
	if dimensions == 4 && maximum[3]-minimum[3] > maximum[widestAxis]-minimum[widestAxis] {
		widestAxis = 3
	}
	return widestAxis
}

func paletteNodeLess(left, right paletteNode, axis int) bool {
	if left.point.channels[axis] != right.point.channels[axis] {
		return left.point.channels[axis] < right.point.channels[axis]
	}
	return left.point.index < right.point.index
}

// sortPaletteNodes uses an allocation-free quicksort. Palette slices contain
// at most 256 entries, and sorting each balanced subtree keeps tree construction
// negligible compared with quantizing and dithering a frame.
func sortPaletteNodes(nodes []paletteNode, axis int) {
	for len(nodes) > 12 {
		pivot := nodes[len(nodes)/2]
		left, right := 0, len(nodes)-1
		for left <= right {
			for paletteNodeLess(nodes[left], pivot, axis) {
				left++
			}
			for paletteNodeLess(pivot, nodes[right], axis) {
				right--
			}
			if left <= right {
				nodes[left], nodes[right] = nodes[right], nodes[left]
				left++
				right--
			}
		}
		if right+1 < len(nodes)-left {
			sortPaletteNodes(nodes[:right+1], axis)
			nodes = nodes[left:]
		} else {
			sortPaletteNodes(nodes[left:], axis)
			nodes = nodes[:right+1]
		}
	}
	for index := 1; index < len(nodes); index++ {
		node := nodes[index]
		position := index
		for position > 0 && paletteNodeLess(node, nodes[position-1], axis) {
			nodes[position] = nodes[position-1]
			position--
		}
		nodes[position] = node
	}
}

func squaredColorDifference(left, right int32) uint32 {
	difference := uint32(left - right)
	return difference * difference >> 2
}

func (tree *paletteTree) nearest(r, g, b int32) uint8 {
	if len(tree.nodes) <= 16 {
		bestIndex := uint8(0)
		bestDistance := ^uint32(0)
		for _, node := range tree.nodes {
			distance := squaredColorDifference(r, int32(node.point.channels[0])) +
				squaredColorDifference(g, int32(node.point.channels[1])) +
				squaredColorDifference(b, int32(node.point.channels[2]))
			if distance < bestDistance || distance == bestDistance && node.point.index < bestIndex {
				bestDistance = distance
				bestIndex = node.point.index
			}
		}
		return bestIndex
	}
	type pendingNode struct {
		index           int16
		minimumDistance uint32
	}
	target := [3]int32{r, g, b}
	bestIndex := uint8(0)
	bestDistance := ^uint32(0)
	if tree.seedNodes != nil {
		seed := tree.nodes[tree.seedNodes[int(r>>13)<<6|int(g>>13)<<3|int(b>>13)]]
		bestIndex = seed.point.index
		bestDistance = squaredColorDifference(r, int32(seed.point.channels[0])) +
			squaredColorDifference(g, int32(seed.point.channels[1])) +
			squaredColorDifference(b, int32(seed.point.channels[2]))
	}
	// The middle-split tree has at most 256 nodes, so its maximum depth and
	// depth-first pending set are both at most 9. Keep extra fixed capacity so
	// nearest-color queries remain allocation-free; a structural test locks the
	// bound to this capacity.
	stack := [paletteTraversalStackCapacity]pendingNode{{index: tree.root}}
	stackLength := 1
	for stackLength > 0 {
		stackLength--
		pending := stack[stackLength]
		if pending.minimumDistance > bestDistance {
			continue
		}
		node := tree.nodes[pending.index]
		distance := squaredColorDifference(r, int32(node.point.channels[0])) +
			squaredColorDifference(g, int32(node.point.channels[1])) +
			squaredColorDifference(b, int32(node.point.channels[2]))
		if distance == 0 && tree.uniqueColors {
			return node.point.index
		}
		if distance < bestDistance || distance == bestDistance && node.point.index < bestIndex {
			bestDistance = distance
			bestIndex = node.point.index
		}

		axis := int(node.axis)
		near, far := node.left, node.right
		axisValue := int32(node.point.channels[axis])
		if target[axis] >= axisValue {
			near, far = far, near
		}
		if far >= 0 {
			stack[stackLength] = pendingNode{
				index:           far,
				minimumDistance: squaredColorDifference(target[axis], axisValue),
			}
			stackLength++
		}
		if near >= 0 {
			stack[stackLength] = pendingNode{index: near}
			stackLength++
		}
	}
	return bestIndex
}

func (tree *paletteTree) nearestRGBA(r, g, b, a int32) uint8 {
	if len(tree.nodes) <= 16 {
		bestIndex := uint8(0)
		bestDistance := ^uint32(0)
		for _, node := range tree.nodes {
			distance := squaredColorDifference(r, int32(node.point.channels[0])) +
				squaredColorDifference(g, int32(node.point.channels[1])) +
				squaredColorDifference(b, int32(node.point.channels[2])) +
				squaredColorDifference(a, int32(node.point.channels[3]))
			if distance < bestDistance || distance == bestDistance && node.point.index < bestIndex {
				bestDistance = distance
				bestIndex = node.point.index
			}
		}
		return bestIndex
	}
	type pendingNode struct {
		index           int16
		minimumDistance uint32
	}
	target := [4]int32{r, g, b, a}
	bestIndex := uint8(0)
	bestDistance := ^uint32(0)
	stack := [paletteTraversalStackCapacity]pendingNode{{index: tree.root}}
	stackLength := 1
	for stackLength > 0 {
		stackLength--
		pending := stack[stackLength]
		if pending.minimumDistance > bestDistance {
			continue
		}
		node := tree.nodes[pending.index]
		distance := squaredColorDifference(r, int32(node.point.channels[0])) +
			squaredColorDifference(g, int32(node.point.channels[1])) +
			squaredColorDifference(b, int32(node.point.channels[2])) +
			squaredColorDifference(a, int32(node.point.channels[3]))
		if distance == 0 && tree.uniqueColors {
			return node.point.index
		}
		if distance < bestDistance || distance == bestDistance && node.point.index < bestIndex {
			bestDistance = distance
			bestIndex = node.point.index
		}

		axis := int(node.axis)
		near, far := node.left, node.right
		axisValue := int32(node.point.channels[axis])
		if target[axis] >= axisValue {
			near, far = far, near
		}
		if far >= 0 {
			stack[stackLength] = pendingNode{
				index:           far,
				minimumDistance: squaredColorDifference(target[axis], axisValue),
			}
			stackLength++
		}
		if near >= 0 {
			stack[stackLength] = pendingNode{index: near}
			stackLength++
		}
	}
	return bestIndex
}

func clampColorChannel(channel int32) int32 {
	if uint32(channel) <= 0xffff {
		return channel
	}
	if channel < 0 {
		return 0
	}
	return 0xffff
}

func rasterPixels(source image.Image) (pixels []uint8, start, stride int, straightAlpha bool) {
	bounds := source.Bounds()
	switch typed := source.(type) {
	case *image.NRGBA:
		return typed.Pix, typed.PixOffset(bounds.Min.X, bounds.Min.Y), typed.Stride, true
	case *image.RGBA:
		return typed.Pix, typed.PixOffset(bounds.Min.X, bounds.Min.Y), typed.Stride, false
	default:
		panic("xgif: internal raster pixel access received an unsupported image type")
	}
}

// ditherOpaqueRasterWithWorkspace is an exact Floyd-Steinberg implementation specialized
// for the opaque RGBA/NRGBA frames produced by raster export. Alpha error is
// identically zero, and an exact spatial palette index replaces the linear
// scan of up to 256 colors performed for every pixel by image/draw.
func ditherOpaqueRasterWithWorkspace(destination *image.Paletted, source image.Image, workspace *OpaqueQuantizationWorkspace) {
	bounds := source.Bounds()
	width := bounds.Dx()
	var errors [][3]int32
	if workspace == nil {
		errors = make([][3]int32, 2*(width+2))
	} else if required := 2 * (width + 2); cap(workspace.ditherErrors) < required {
		workspace.ditherErrors = make([][3]int32, required)
		errors = workspace.ditherErrors
	} else {
		workspace.ditherErrors = workspace.ditherErrors[:required]
		clear(workspace.ditherErrors)
		errors = workspace.ditherErrors
	}
	current := errors[:width+2]
	next := errors[width+2:]
	var paletteChannels [][3]int32
	if workspace == nil {
		paletteChannels = make([][3]int32, len(destination.Palette))
	} else if cap(workspace.paletteChannels) < len(destination.Palette) {
		workspace.paletteChannels = make([][3]int32, len(destination.Palette))
		paletteChannels = workspace.paletteChannels
	} else {
		workspace.paletteChannels = workspace.paletteChannels[:len(destination.Palette)]
		paletteChannels = workspace.paletteChannels
	}
	for index, c := range destination.Palette {
		r, g, b, _ := c.RGBA()
		paletteChannels[index] = [3]int32{int32(r), int32(g), int32(b)}
	}
	var nodeStorage []paletteNode
	if workspace != nil {
		nodeStorage = workspace.paletteNodes
	}
	palette := newPaletteTreeWithStorage(destination.Palette, 3, nodeStorage)
	if workspace != nil {
		workspace.paletteNodes = palette.nodes
	}
	if bounds.Dx() >= 1024 || bounds.Dy() >= 1024 || bounds.Dx()*bounds.Dy() >= 1024 {
		if workspace == nil {
			palette.prepareSeeds()
		} else {
			palette.prepareSeedsWithStorage(&workspace.paletteSeeds)
		}
	}
	sourcePixels, sourceStart, sourceStride, _ := rasterPixels(source)

	for y := 0; y < bounds.Dy(); y++ {
		sourceRow := sourcePixels[sourceStart+y*sourceStride : sourceStart+y*sourceStride+4*width]
		destinationOffset := destination.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		for x := 0; x < width; x++ {
			sourceOffset := x * 4
			r := clampColorChannel(int32(sourceRow[sourceOffset])*0x101 + current[x+1][0]/16)
			g := clampColorChannel(int32(sourceRow[sourceOffset+1])*0x101 + current[x+1][1]/16)
			b := clampColorChannel(int32(sourceRow[sourceOffset+2])*0x101 + current[x+1][2]/16)
			paletteIndex := palette.nearest(r, g, b)
			destination.Pix[destinationOffset+x] = paletteIndex

			selected := paletteChannels[paletteIndex]
			r -= selected[0]
			g -= selected[1]
			b -= selected[2]
			next[x][0] += r * 3
			next[x][1] += g * 3
			next[x][2] += b * 3
			next[x+1][0] += r * 5
			next[x+1][1] += g * 5
			next[x+1][2] += b * 5
			next[x+2][0] += r
			next[x+2][1] += g
			next[x+2][2] += b
			current[x+2][0] += r * 7
			current[x+2][1] += g * 7
			current[x+2][2] += b * 7
		}
		current, next = next, current
		clear(next)
	}
}

// ditherTranslucentRaster retains all four RGBA error channels for exact
// Floyd-Steinberg output while using the same exact spatial palette search as
// the opaque fast path.
func ditherTranslucentRaster(destination *image.Paletted, source image.Image) {
	bounds := source.Bounds()
	width := bounds.Dx()
	errors := make([][4]int32, 2*(width+2))
	current := errors[:width+2]
	next := errors[width+2:]
	paletteChannels := make([][4]int32, len(destination.Palette))
	for index, c := range destination.Palette {
		r, g, b, a := c.RGBA()
		paletteChannels[index] = [4]int32{int32(r), int32(g), int32(b), int32(a)}
	}
	palette := newPaletteTree(destination.Palette, 4)
	sourcePixels, sourceStart, sourceStride, straightAlpha := rasterPixels(source)

	for y := 0; y < bounds.Dy(); y++ {
		sourceRow := sourcePixels[sourceStart+y*sourceStride : sourceStart+y*sourceStride+4*width]
		destinationOffset := destination.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		for x := 0; x < width; x++ {
			sourceOffset := x * 4
			alpha8 := uint32(sourceRow[sourceOffset+3])
			a := int32(alpha8 * 0x101)
			var r, g, b int32
			if straightAlpha {
				r = int32(uint32(sourceRow[sourceOffset]) * 0x101 * alpha8 / 0xff)
				g = int32(uint32(sourceRow[sourceOffset+1]) * 0x101 * alpha8 / 0xff)
				b = int32(uint32(sourceRow[sourceOffset+2]) * 0x101 * alpha8 / 0xff)
			} else {
				r = int32(sourceRow[sourceOffset]) * 0x101
				g = int32(sourceRow[sourceOffset+1]) * 0x101
				b = int32(sourceRow[sourceOffset+2]) * 0x101
			}
			r = clampColorChannel(r + current[x+1][0]/16)
			g = clampColorChannel(g + current[x+1][1]/16)
			b = clampColorChannel(b + current[x+1][2]/16)
			a = clampColorChannel(a + current[x+1][3]/16)
			paletteIndex := palette.nearestRGBA(r, g, b, a)
			destination.Pix[destinationOffset+x] = paletteIndex

			selected := paletteChannels[paletteIndex]
			r -= selected[0]
			g -= selected[1]
			b -= selected[2]
			a -= selected[3]
			next[x][0] += r * 3
			next[x][1] += g * 3
			next[x][2] += b * 3
			next[x][3] += a * 3
			next[x+1][0] += r * 5
			next[x+1][1] += g * 5
			next[x+1][2] += b * 5
			next[x+1][3] += a * 5
			next[x+2][0] += r
			next[x+2][1] += g
			next[x+2][2] += b
			next[x+2][3] += a
			current[x+2][0] += r * 7
			current[x+2][1] += g * 7
			current[x+2][2] += b * 7
			current[x+2][3] += a * 7
		}
		current, next = next, current
		clear(next)
	}
}

func quantizeImageThroughGIF(ctx context.Context, img image.Image) (*image.Paletted, error) {
	var buf bytes.Buffer
	if err := gif.Encode(contextWriter{ctx: ctx, output: &buf}, img, &gif.Options{
		NumColors: 256,
		Quantizer: quantize.MedianCutQuantizer{},
	}); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gifImage, err := gif.Decode(&buf)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paletted, ok := gifImage.(*image.Paletted)
	if !ok {
		return nil, fmt.Errorf("decoded GIF image could not be cast as *image.Paletted")
	}
	return paletted, nil
}

// NormalizePalettedImage centers img on a width-by-height white canvas while
// preserving its indexed pixels. No color quantization is performed. When img
// already has zero-based width-by-height bounds, its palette is updated and img
// itself is returned so the common path does not allocate a second full pixel
// buffer. Otherwise the returned frame owns its pixel buffer and palette.
// Callers must impose appropriate canvas and aggregate pixel limits.
func NormalizePalettedImage(ctx context.Context, img *image.Paletted, width, height int) (*image.Paletted, error) {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validatePalettedImage(ctx, img); err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("GIF canvas dimensions must be positive")
	}
	bounds := img.Bounds()
	if bounds.Dx() > width || bounds.Dy() > height {
		return nil, fmt.Errorf("GIF image dimensions %dx%d exceed %dx%d canvas", bounds.Dx(), bounds.Dy(), width, height)
	}
	if width > int(^uint(0)>>1)/height {
		return nil, fmt.Errorf("GIF canvas pixel count exceeds the platform integer domain")
	}
	if width >= 1<<16 || height >= 1<<16 {
		return nil, fmt.Errorf("GIF canvas dimensions must be smaller than 65536")
	}
	if bounds.Min == (image.Point{}) && bounds.Dx() == width && bounds.Dy() == height {
		img.Palette, _ = paletteWithBackground(img.Palette)
		return img, nil
	}

	palette, backgroundIndex := ownedPaletteWithBackground(img.Palette)
	frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
	top := (height - bounds.Dy()) / 2
	left := (width - bounds.Dx()) / 2
	sourcePixels := bounds.Dx() * bounds.Dy()
	sourceOnCanvas := image.Rect(left, top, left+bounds.Dx(), top+bounds.Dy())
	// Margin-only filling saves writes for large frames. Restrict it to wide
	// sources so per-row side spans do not cost more than one contiguous fill.
	fillAroundSource := sourcePixels >= len(frame.Pix)/2+len(frame.Pix)%2 && bounds.Dx()*4 >= width*3
	background := uint8(backgroundIndex)
	const fillChunkPixels = 1 << 20
	for start := 0; start < len(frame.Pix); start += fillChunkPixels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+fillChunkPixels, len(frame.Pix))
		if fillAroundSource {
			fillFreshCanvasOutsideSource(frame.Pix, start, end, background, width, sourceOnCanvas)
		} else if background != 0 && start != 0 {
			copy(frame.Pix[start:end], frame.Pix[:end-start])
		} else {
			fillFreshBytes(frame.Pix[start:end], background)
		}
	}

	const copyChunkRows = 256
	if bounds.Dx() == width && img.Stride == width {
		for sourceRow := 0; sourceRow < bounds.Dy(); sourceRow += copyChunkRows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			endRow := min(sourceRow+copyChunkRows, bounds.Dy())
			sourceOffset := img.PixOffset(bounds.Min.X, bounds.Min.Y+sourceRow)
			destinationOffset := frame.PixOffset(0, top+sourceRow)
			copy(frame.Pix[destinationOffset:destinationOffset+(endRow-sourceRow)*width], img.Pix[sourceOffset:sourceOffset+(endRow-sourceRow)*width])
		}
	} else {
		sourceOffset := img.PixOffset(bounds.Min.X, bounds.Min.Y)
		destinationOffset := frame.PixOffset(left, top)
		for sourceRow := 0; sourceRow < bounds.Dy(); sourceRow += copyChunkRows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			endRow := min(sourceRow+copyChunkRows, bounds.Dy())
			for row := sourceRow; row < endRow; row++ {
				copy(frame.Pix[destinationOffset:destinationOffset+bounds.Dx()], img.Pix[sourceOffset:sourceOffset+bounds.Dx()])
				sourceOffset += img.Stride
				destinationOffset += width
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return frame, nil
}

func ownedPaletteWithBackground(source color.Palette) (color.Palette, int) {
	capacity := len(source)
	if capacity < 256 {
		capacity++
	}
	palette := make(color.Palette, len(source), capacity)
	copy(palette, source)
	return paletteWithBackground(palette)
}

// fillFreshBytes fills storage known to be zero-initialized. Copying the
// initialized prefix uses the runtime's optimized bulk-copy implementation.
func fillFreshBytes(destination []byte, value byte) {
	if len(destination) == 0 || value == 0 {
		return
	}
	if len(destination) > 1 && len(destination) <= 32 {
		for index := range destination {
			destination[index] = value
		}
		return
	}
	destination[0] = value
	for initialized := 1; initialized < len(destination); {
		initialized += copy(destination[initialized:], destination[:initialized])
	}
}

func fillFreshCanvasOutsideSource(pixels []byte, start, end int, value byte, width int, source image.Rectangle) {
	if value == 0 {
		return
	}
	sourceStart := source.Min.Y * width
	sourceEnd := source.Max.Y * width
	if start < sourceStart {
		prefixEnd := min(end, sourceStart)
		fillFreshBytes(pixels[start:prefixEnd], value)
		start = prefixEnd
	}
	if start >= end {
		return
	}
	if start >= sourceEnd {
		fillFreshBytes(pixels[start:end], value)
		return
	}
	middleEnd := min(end, sourceEnd)
	if source.Dx() != width {
		for position := start; position < middleEnd; {
			rowStart := position / width * width
			rowEnd := min(middleEnd, rowStart+width)
			leftEnd := min(rowEnd, rowStart+source.Min.X)
			if position < leftEnd {
				fillFreshBytes(pixels[position:leftEnd], value)
			}
			rightStart := max(position, rowStart+source.Max.X)
			if rightStart < rowEnd {
				fillFreshBytes(pixels[rightStart:rowEnd], value)
			}
			position = rowEnd
		}
	}
	if middleEnd < end {
		fillFreshBytes(pixels[middleEnd:end], value)
	}
}

func paletteWithBackground(palette color.Palette) (color.Palette, int) {
	if len(palette) == 256 {
		backgroundIndex := findWhiteIndex(palette)
		palette[backgroundIndex] = BG_COLOR
		return palette, backgroundIndex
	}
	backgroundIndex := len(palette)
	return append(palette, BG_COLOR), backgroundIndex
}

// AnimatePalettedImagesWithLimit encodes already-normalized paletted frames without
// quantizing or copying them. Every frame must have identical zero-based
// bounds and a valid GIF palette. The animation loops forever, and frame
// delays use the same schedule as animateImagesWithConcurrency. maxBytes must
// be positive. Encoding rejects a write
// before the accumulated output length can exceed the limit.
func AnimatePalettedImagesWithLimit(ctx context.Context, images []*image.Paletted, animIntervalMs int, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("GIF output byte limit must be positive")
	}
	delays, err := gifFrameDelays(len(images), animIntervalMs)
	if err != nil {
		return nil, err
	}
	return encodePalettedImages(nonNilContext(ctx), images, delays, maxBytes)
}

func animateImages(ctx context.Context, images []image.Image, delays []int, concurrency int) ([]byte, error) {
	var width, height int
	for index, img := range images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if isNilImage(img) {
			return nil, fmt.Errorf("GIF frame %d is nil", index)
		}
		bounds := img.Bounds()
		if bounds.Empty() {
			return nil, fmt.Errorf("GIF frame %d has empty bounds", index)
		}
		width = go2.Max(width, bounds.Dx())
		height = go2.Max(height, bounds.Dy())
	}

	palettedImages := make([]*image.Paletted, len(images))

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i := 0; i < len(images); i++ {
		i := i
		g.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			paletted, err := QuantizeImage(groupCtx, images[i])
			if err != nil {
				return err
			}
			frame, err := NormalizePalettedImage(groupCtx, paletted, width, height)
			if err != nil {
				return err
			}
			palettedImages[i] = frame
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return encodePalettedImages(ctx, palettedImages, delays, math.MaxInt64)
}

func encodePalettedImages(ctx context.Context, images []*image.Paletted, delays []int, maxOutputBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("GIF animation requires at least one paletted frame")
	}
	if len(delays) != len(images) {
		return nil, fmt.Errorf("GIF frame and delay counts differ")
	}
	var bounds image.Rectangle
	for index, img := range images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validatePalettedImage(ctx, img); err != nil {
			return nil, fmt.Errorf("GIF frame %d: %w", index, err)
		}
		if img.Bounds().Min != (image.Point{}) {
			return nil, fmt.Errorf("GIF frame %d bounds must start at (0,0)", index)
		}
		if index == 0 {
			bounds = img.Bounds()
		} else if img.Bounds() != bounds {
			return nil, fmt.Errorf("GIF frame %d dimensions %dx%d differ from %dx%d canvas", index, img.Bounds().Dx(), img.Bounds().Dy(), bounds.Dx(), bounds.Dy())
		}
		if delays[index] < 0 {
			return nil, fmt.Errorf("GIF frame %d delay must be non-negative", index)
		}
	}

	buf := limitedBuffer{ctx: ctx, maxBytes: maxOutputBytes}
	err := encodeGIF(&buf, images, delays)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func validatePalettedImage(ctx context.Context, img *image.Paletted) error {
	if img == nil {
		return fmt.Errorf("paletted GIF image is nil")
	}
	bounds := img.Bounds()
	width, height, err := gifImageDimensions(bounds)
	if err != nil {
		return err
	}
	if len(img.Palette) == 0 || len(img.Palette) > 256 {
		return fmt.Errorf("paletted GIF image must contain between 1 and 256 colors")
	}
	for index, paletteColor := range img.Palette {
		if paletteColor == nil {
			return fmt.Errorf("paletted GIF image color %d is nil", index)
		}
	}
	if img.Stride < width {
		return fmt.Errorf("paletted GIF image stride is shorter than its width")
	}
	rowsBeforeLast := height - 1
	if rowsBeforeLast > (int(^uint(0)>>1)-width)/img.Stride {
		return fmt.Errorf("paletted GIF image storage exceeds the platform integer domain")
	}
	requiredPixels := rowsBeforeLast*img.Stride + width
	if requiredPixels > len(img.Pix) {
		return fmt.Errorf("paletted GIF image pixel buffer is too short")
	}
	if len(img.Palette) == 256 {
		// Every byte is a valid index into a full GIF palette. Preserve the
		// per-256-row cancellation points without rereading the pixel buffer.
		for row := 0; row < height; row += 256 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	if len(img.Palette) == 255 && width >= 64 {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			if (y-bounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			offset := img.PixOffset(bounds.Min.X, y)
			if bytes.IndexByte(img.Pix[offset:offset+width], 255) >= 0 {
				return fmt.Errorf("paletted GIF image contains an out-of-range color index")
			}
		}
		return ctx.Err()
	}
	if paletteLength := len(img.Palette); width >= 8 {
		const (
			byteLanes = uint64(0x0101010101010101)
			highBits  = uint64(0x8080808080808080)
		)
		var threshold uint64
		if paletteLength <= 128 {
			threshold = uint64(paletteLength) * byteLanes
		} else {
			threshold = uint64(paletteLength-128) * byteLanes
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			if (y-bounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			offset := img.PixOffset(bounds.Min.X, y)
			row := img.Pix[offset : offset+width]
			for len(row) >= 8 {
				indexes := binary.LittleEndian.Uint64(row)
				comparison := ((indexes | highBits) - threshold) & highBits
				if paletteLength <= 128 {
					comparison |= indexes & highBits
				} else {
					comparison &= indexes
				}
				if comparison != 0 {
					return fmt.Errorf("paletted GIF image contains an out-of-range color index")
				}
				row = row[8:]
			}
			for _, paletteIndex := range row {
				if int(paletteIndex) >= paletteLength {
					return fmt.Errorf("paletted GIF image contains an out-of-range color index")
				}
			}
		}
		return ctx.Err()
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		offset := img.PixOffset(bounds.Min.X, y)
		for _, paletteIndex := range img.Pix[offset : offset+width] {
			if int(paletteIndex) >= len(img.Palette) {
				return fmt.Errorf("paletted GIF image contains an out-of-range color index")
			}
		}
	}
	return ctx.Err()
}

func gifImageDimensions(bounds image.Rectangle) (int, int, error) {
	if bounds.Empty() {
		return 0, 0, fmt.Errorf("GIF image has empty bounds")
	}
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("GIF image dimensions exceed the platform integer domain")
	}
	if width >= 1<<16 || height >= 1<<16 {
		return 0, 0, fmt.Errorf("GIF image dimensions must be smaller than 65536")
	}
	return width, height, nil
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

type limitedBuffer struct {
	bytes.Buffer
	ctx      context.Context
	maxBytes int64
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	if w.ctx != nil {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if int64(len(data)) > w.maxBytes-int64(w.Len()) {
		return 0, fmt.Errorf("GIF output exceeds the %d-byte limit", w.maxBytes)
	}
	written, err := w.Buffer.Write(data)
	if err != nil {
		return written, err
	}
	if w.ctx != nil {
		if contextErr := w.ctx.Err(); contextErr != nil {
			return written, contextErr
		}
	}
	return written, nil
}

func (w *limitedBuffer) WriteByte(data byte) error {
	if w.ctx != nil {
		if err := w.ctx.Err(); err != nil {
			return err
		}
	}
	if int64(w.Len()) >= w.maxBytes {
		return fmt.Errorf("GIF output exceeds the %d-byte limit", w.maxBytes)
	}
	if err := w.Buffer.WriteByte(data); err != nil {
		return err
	}
	if w.ctx != nil {
		return w.ctx.Err()
	}
	return nil
}

func (w *limitedBuffer) Flush() error {
	if w.ctx != nil {
		return w.ctx.Err()
	}
	return nil
}

type contextWriter struct {
	ctx    context.Context
	output io.Writer
}

func (w contextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	written, err := w.output.Write(data)
	if err != nil {
		return written, err
	}
	if contextErr := w.ctx.Err(); contextErr != nil {
		return written, contextErr
	}
	return written, nil
}

func isNilImage(img image.Image) bool {
	if img == nil {
		return true
	}
	value := reflect.ValueOf(img)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// animationFrameCount samples at 30 fps and rounds upward so every positive
// requested interval receives at least one frame and fractional seconds are
// not truncated.
func animationFrameCount(durationMs int) (int, error) {
	if durationMs <= 0 {
		return 0, fmt.Errorf("GIF animation interval must be positive")
	}
	wholeSeconds := durationMs / 1000
	remainderMs := durationMs % 1000
	extraFrames := (remainderMs*fps + 999) / 1000
	maxInt := int(^uint(0) >> 1)
	if wholeSeconds > (maxInt-extraFrames)/fps {
		return 0, fmt.Errorf("GIF animation frame count exceeds the platform integer domain")
	}
	return wholeSeconds*fps + extraFrames, nil
}

// AnimationFrameCount returns the number of 30 fps samples used for one
// positive board interval. The renderer and encoder share this schedule
// so fractional intervals use identical rounding rules.
func AnimationFrameCount(durationMs int) (int, error) {
	return animationFrameCount(durationMs)
}

// AnimationFrameTime returns the timestamp for a zero-based sample in the
// shared 30 fps capture schedule.
func AnimationFrameTime(frameIndex int) (time.Duration, error) {
	if frameIndex < 0 {
		return 0, fmt.Errorf("GIF animation frame index must be non-negative")
	}
	wholeSeconds := frameIndex / fps
	if int64(wholeSeconds) > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("GIF animation frame time exceeds the duration domain")
	}
	remainder := frameIndex % fps
	return time.Duration(wholeSeconds)*time.Second + time.Duration(remainder)*time.Second/fps, nil
}

// gifFrameDelays distributes GIF's centisecond delays across each board's
// sampled frames. The quantized duration rounds upward to avoid shortening the
// requested interval. A non-sampled frame count is treated as one board.
func gifFrameDelays(totalFrames, intervalMs int) ([]int, error) {
	schedule, err := newGIFFrameDelaySchedule(totalFrames, intervalMs)
	if err != nil {
		return nil, err
	}
	delays := make([]int, totalFrames)
	for index := range delays {
		delays[index] = schedule.next()
	}
	return delays, nil
}

type gifFrameDelaySchedule struct {
	framesPerBoard int
	baseDelay      int
	extraDelays    int
	frame          int
	errorTerm      int
}

func newGIFFrameDelaySchedule(totalFrames, intervalMs int) (gifFrameDelaySchedule, error) {
	if totalFrames <= 0 {
		return gifFrameDelaySchedule{}, fmt.Errorf("GIF animation requires at least one PNG frame")
	}
	framesPerBoard, err := animationFrameCount(intervalMs)
	if err != nil {
		return gifFrameDelaySchedule{}, err
	}
	if totalFrames%framesPerBoard != 0 {
		framesPerBoard = totalFrames
	}
	centiseconds := intervalMs / 10
	if intervalMs%10 != 0 {
		centiseconds++
	}
	baseDelay := centiseconds / framesPerBoard
	extraDelays := centiseconds % framesPerBoard
	return gifFrameDelaySchedule{
		framesPerBoard: framesPerBoard,
		baseDelay:      baseDelay,
		extraDelays:    extraDelays,
	}, nil
}

func (s *gifFrameDelaySchedule) next() int {
	delay := s.baseDelay
	if s.extraDelays != 0 && s.errorTerm >= s.framesPerBoard-s.extraDelays {
		delay++
		s.errorTerm -= s.framesPerBoard - s.extraDelays
	} else {
		s.errorTerm += s.extraDelays
	}
	s.frame++
	if s.frame == s.framesPerBoard {
		s.frame = 0
		s.errorTerm = 0
	}
	return delay
}

func findWhiteIndex(palette color.Palette) int {
	nearestIndex := 0
	var nearestScore uint32
	for i, c := range palette {
		var r, g, b uint32
		if rgba, ok := c.(color.RGBA); ok {
			r = uint32(rgba.R) * 0x101
			g = uint32(rgba.G) * 0x101
			b = uint32(rgba.B) * 0x101
		} else {
			r, g, b, _ = c.RGBA()
		}
		if r == 255 && g == 255 && b == 255 {
			return i
		}

		score := r + g + b
		if score > nearestScore {
			nearestScore = score
			nearestIndex = i
		}
	}
	return nearestIndex
}

// Validate checks the frame count, loop behavior, dimensions, and interval of
// gifBytes.
//
// Deprecated: Validate is a test-only helper specific to D2's output and will
// be removed after one compatibility release. Downstream tests should decode
// the GIF and validate the properties they rely on directly.
func Validate(gifBytes []byte, nFrames int, intervalMS int) error {
	return testutil.ValidateGIF(gifBytes, nFrames, intervalMS)
}
