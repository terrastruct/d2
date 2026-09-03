package testutil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"slices"

	"github.com/d2lang/d2/internal/testutil/imagediff"
)

const infiniteLoop = 0

// ValidateGIF checks the frame count, loop behavior, dimensions, and interval
// of gifBytes against D2's expected GIF output.
func ValidateGIF(gifBytes []byte, nFrames int, intervalMS int) error {
	anim, err := gif.DecodeAll(bytes.NewBuffer(gifBytes))
	if err != nil {
		return err
	}

	if nFrames > 1 && anim.LoopCount != infiniteLoop {
		return fmt.Errorf("expected infinite loop, got=%d", anim.LoopCount)
	} else if nFrames == 1 && anim.LoopCount != -1 {
		return fmt.Errorf("wrong loop count for single frame gif, got=%d", anim.LoopCount)
	}

	if len(anim.Image) != nFrames {
		return fmt.Errorf("expected %d frames, got=%d", nFrames, len(anim.Image))
	}

	interval := intervalMS / 10
	width, height := anim.Config.Width, anim.Config.Height
	for i, frame := range anim.Image {
		w := frame.Bounds().Dx()
		if w != width {
			return fmt.Errorf("expected all frames to have the same width=%d, got=%d at frame=%d", width, w, i)
		}
		h := frame.Bounds().Dy()
		if h != height {
			return fmt.Errorf("expected all frames to have the same height=%d, got=%d at frame=%d", height, h, i)
		}
		if anim.Delay[i] != interval {
			return fmt.Errorf("expected interval between frames to be %d, got=%d at frame=%d", interval, anim.Delay[i], i)
		}
	}

	return nil
}

// GIFInspection is the decoded, display-equivalent GIF contract used by the
// raster E2E suite. FrameHashes are hashes of composited NRGBA
// pixels in playback order, not hashes of palette indices or encoded bytes.
type GIFInspection struct {
	Width, Height             int
	LoopCount                 int
	Delays                    []int
	TotalDurationCentiseconds int
	FrameHashes               [][sha256.Size]byte
	ChangedFramePairs         int

	frames []*image.NRGBA
}

// GIFCompareOptions controls exact display-pixel comparison. When
// RequireFrameChange is true, a constant animation is rejected even if the
// expected file is also accidentally constant.
type GIFCompareOptions struct {
	RequireFrameChange bool
	Image              imagediff.Options
}

// GIFComparison contains the frame diagnostics available on a pixel mismatch.
// FrameResult can be written as a self-contained HTML report.
type GIFComparison struct {
	Expected, Actual *GIFInspection
	FrameIndex       int
	FrameResult      *imagediff.Result
}

// InspectGIF decodes and composites GIF frames according to their disposal
// methods. This catches palette-only, sub-rectangle, and frame-order changes
// that metadata-only validation misses.
func InspectGIF(gifBytes []byte) (*GIFInspection, error) {
	animation, err := gif.DecodeAll(bytes.NewReader(gifBytes))
	if err != nil {
		return nil, fmt.Errorf("decode GIF: %w", err)
	}
	if animation.Config.Width <= 0 || animation.Config.Height <= 0 {
		return nil, fmt.Errorf("GIF has invalid logical dimensions %dx%d", animation.Config.Width, animation.Config.Height)
	}
	if len(animation.Image) == 0 {
		return nil, fmt.Errorf("GIF has no frames")
	}
	if len(animation.Delay) != len(animation.Image) {
		return nil, fmt.Errorf("GIF has %d frames but %d delays", len(animation.Image), len(animation.Delay))
	}
	if len(animation.Disposal) != 0 && len(animation.Disposal) != len(animation.Image) {
		return nil, fmt.Errorf("GIF has %d frames but %d disposal methods", len(animation.Image), len(animation.Disposal))
	}

	logicalBounds := image.Rect(0, 0, animation.Config.Width, animation.Config.Height)
	canvas := image.NewNRGBA(logicalBounds)
	background := gifBackground(animation)
	inspection := &GIFInspection{
		Width:       animation.Config.Width,
		Height:      animation.Config.Height,
		LoopCount:   animation.LoopCount,
		Delays:      slices.Clone(animation.Delay),
		FrameHashes: make([][sha256.Size]byte, len(animation.Image)),
		frames:      make([]*image.NRGBA, len(animation.Image)),
	}
	for frameIndex, frame := range animation.Image {
		if frame == nil {
			return nil, fmt.Errorf("GIF frame %d is nil", frameIndex)
		}
		if !frame.Bounds().In(logicalBounds) {
			return nil, fmt.Errorf("GIF frame %d bounds %v exceed logical bounds %v", frameIndex, frame.Bounds(), logicalBounds)
		}
		before := cloneNRGBA(canvas)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		snapshot := cloneNRGBA(canvas)
		inspection.frames[frameIndex] = snapshot
		inspection.FrameHashes[frameIndex] = sha256.Sum256(snapshot.Pix)
		inspection.TotalDurationCentiseconds += animation.Delay[frameIndex]
		if frameIndex != 0 && inspection.FrameHashes[frameIndex] != inspection.FrameHashes[frameIndex-1] {
			inspection.ChangedFramePairs++
		}

		disposal := byte(gif.DisposalNone)
		if len(animation.Disposal) != 0 {
			disposal = animation.Disposal[frameIndex]
		}
		switch disposal {
		case 0, gif.DisposalNone:
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = before
		default:
			return nil, fmt.Errorf("GIF frame %d has unsupported disposal method %d", frameIndex, disposal)
		}
	}
	return inspection, nil
}

// CompareGIF compares playback metadata and exact composited frame pixels.
// Encoded palette layout and compression may differ without failing the test.
func CompareGIF(expectedBytes, actualBytes []byte, options GIFCompareOptions) (*GIFComparison, error) {
	expected, err := InspectGIF(expectedBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect expected GIF: %w", err)
	}
	actual, err := InspectGIF(actualBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect actual GIF: %w", err)
	}
	comparison := &GIFComparison{Expected: expected, Actual: actual, FrameIndex: -1}
	if expected.Width != actual.Width || expected.Height != actual.Height {
		return comparison, fmt.Errorf("GIF logical dimensions differ: expected %dx%d, actual %dx%d", expected.Width, expected.Height, actual.Width, actual.Height)
	}
	if len(expected.FrameHashes) != len(actual.FrameHashes) {
		return comparison, fmt.Errorf("GIF frame count differs: expected %d, actual %d", len(expected.FrameHashes), len(actual.FrameHashes))
	}
	if expected.LoopCount != actual.LoopCount {
		return comparison, fmt.Errorf("GIF loop count differs: expected %d, actual %d", expected.LoopCount, actual.LoopCount)
	}
	if !slices.Equal(expected.Delays, actual.Delays) {
		return comparison, fmt.Errorf("GIF frame delays differ: expected %v (total %dcs), actual %v (total %dcs)", expected.Delays, expected.TotalDurationCentiseconds, actual.Delays, actual.TotalDurationCentiseconds)
	}
	if options.RequireFrameChange && actual.ChangedFramePairs == 0 {
		return comparison, fmt.Errorf("GIF frames never change")
	}
	for frameIndex := range expected.frames {
		result, err := imagediff.CompareImages(expected.frames[frameIndex], actual.frames[frameIndex], options.Image)
		if err != nil {
			comparison.FrameIndex = frameIndex
			comparison.FrameResult = result
			return comparison, fmt.Errorf("GIF frame %d pixels differ: %w", frameIndex, err)
		}
	}
	return comparison, nil
}

func gifBackground(animation *gif.GIF) color.Color {
	if animation == nil || animation.Config.ColorModel == nil {
		return color.Transparent
	}
	palette, ok := animation.Config.ColorModel.(color.Palette)
	if !ok || int(animation.BackgroundIndex) >= len(palette) {
		return color.Transparent
	}
	return palette[animation.BackgroundIndex]
}

func cloneNRGBA(source *image.NRGBA) *image.NRGBA {
	clone := image.NewNRGBA(source.Bounds())
	copy(clone.Pix, source.Pix)
	return clone
}
