// Package xgif quantizes, normalizes, and encodes rendered animation frames as
// GIF. Frames are centered on a common canvas within GIF's 256-color limit,
// with one palette entry normalized to the white canvas background.
package xgif

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
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
// frames and while normalizing each paletted frame. The third-party median-cut
// quantizer and its GIF round trip cannot themselves be interrupted. The final
// standard-library EncodeAll call observes cancellation at writer boundaries
// and on return. Callers must also impose per-frame and aggregate pixel ceilings
// appropriate to their environment.
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
// Median-cut quantization cannot be interrupted while the third-party
// quantizer is running. The context is checked before and after that bounded
// operation. Callers must impose an appropriate pixel limit before calling.
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

	palette := append(color.Palette(nil), img.Palette...)
	palette, backgroundIndex := paletteWithBackground(palette)
	frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
	const fillChunkPixels = 1 << 20
	for start := 0; start < len(frame.Pix); start += fillChunkPixels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+fillChunkPixels, len(frame.Pix))
		for index := start; index < end; index++ {
			frame.Pix[index] = uint8(backgroundIndex)
		}
	}

	top := (height - bounds.Dy()) / 2
	left := (width - bounds.Dx()) / 2
	for sourceY := bounds.Min.Y; sourceY < bounds.Max.Y; sourceY++ {
		if (sourceY-bounds.Min.Y)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		sourceOffset := img.PixOffset(bounds.Min.X, sourceY)
		destinationY := top + sourceY - bounds.Min.Y
		destinationOffset := frame.PixOffset(left, destinationY)
		copy(frame.Pix[destinationOffset:destinationOffset+bounds.Dx()], img.Pix[sourceOffset:sourceOffset+bounds.Dx()])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return frame, nil
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

// animatePalettedImages encodes already-normalized paletted frames without
// quantizing or copying them. Every frame must have identical zero-based
// bounds and a valid GIF palette. The animation loops forever, and frame
// delays use the same schedule as animateImagesWithConcurrency.
func animatePalettedImages(ctx context.Context, images []*image.Paletted, animIntervalMs int) ([]byte, error) {
	return AnimatePalettedImagesWithLimit(ctx, images, animIntervalMs, math.MaxInt64)
}

// AnimatePalettedImagesWithLimit is AnimatePalettedImages with a maximum
// encoded output length. maxBytes must be positive. Encoding rejects a write
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

	anim := &gif.GIF{
		Image:     images,
		Delay:     delays,
		LoopCount: INFINITE_LOOP,
		Config: image.Config{
			Width:  bounds.Dx(),
			Height: bounds.Dy(),
		},
	}
	buf := limitedBuffer{maxBytes: maxOutputBytes}
	err := gif.EncodeAll(contextWriter{ctx: ctx, output: &buf}, anim)
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
	maxBytes int64
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	if int64(len(data)) > w.maxBytes-int64(w.Len()) {
		return 0, fmt.Errorf("GIF output exceeds the %d-byte limit", w.maxBytes)
	}
	return w.Buffer.Write(data)
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
	if totalFrames <= 0 {
		return nil, fmt.Errorf("GIF animation requires at least one PNG frame")
	}
	framesPerBoard, err := animationFrameCount(intervalMs)
	if err != nil {
		return nil, err
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
	delays := make([]int, totalFrames)
	for boardStart := 0; boardStart < totalFrames; boardStart += framesPerBoard {
		errorTerm := 0
		for frame := 0; frame < framesPerBoard; frame++ {
			delay := baseDelay
			if extraDelays != 0 && errorTerm >= framesPerBoard-extraDelays {
				delay++
				errorTerm -= framesPerBoard - extraDelays
			} else {
				errorTerm += extraDelays
			}
			delays[boardStart+frame] = delay
		}
	}
	return delays, nil
}

func findWhiteIndex(palette color.Palette) int {
	nearestIndex := 0
	nearestScore := 0.
	for i, c := range palette {
		r, g, b, _ := c.RGBA()
		if r == 255 && g == 255 && b == 255 {
			return i
		}

		avg := float64(r+g+b) / 255.
		if avg > nearestScore {
			nearestScore = avg
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
