package d2isometricimg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"time"

	"github.com/ericpauley/go-quantize/quantize"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

const MaxTimelineBoards = 128
const maxTimelineFrames = 1000

// RenderTimeline renders ordered boards into one looping GIF. Each board is
// shown for interval, rounded to GIF centiseconds. Authored animated edges move
// during their board; static boards need only one held frame. A single palette
// covers all boards. Only one board's raster is retained at a time.
func RenderTimeline(ctx context.Context, boards []*d2target.Diagram, opts *Options, interval time.Duration) ([]byte, error) {
	o, err := normalize(opts)
	if err != nil {
		return nil, err
	}
	if o.Format != GIF || len(boards) == 0 || len(boards) > MaxTimelineBoards || interval <= 0 || interval > 10*time.Minute {
		return nil, fmt.Errorf("isometric timeline requires GIF, 1..%d boards, and an interval in (0,10m]", MaxTimelineBoards)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	counts := make([]int, len(boards))
	total := 0
	for i, board := range boards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if board == nil || board.IsFolderOnly {
			return nil, fmt.Errorf("isometric timeline board %d has no drawable diagram", i)
		}
		if len(board.Shapes) > d2isometric.MaxNodes || len(board.Connections) > d2isometric.MaxEdges || board.Legend != nil && (len(board.Legend.Shapes) > d2isometric.MaxNodes || len(board.Legend.Connections) > d2isometric.MaxEdges) {
			return nil, fmt.Errorf("isometric timeline board %d exceeds the source shape/connection limit", i)
		}
		counts[i] = 1
		if timelineBoardAnimated(board) {
			counts[i] = max(1, int(math.Ceil(interval.Seconds()*12)))
		}
		total += counts[i]
	}
	if total > maxTimelineFrames || int64(total+4*len(boards))*int64(o.Width)*int64(o.Height) > MaxAnimationPixels {
		return nil, fmt.Errorf("isometric timeline exceeds frame/pixel budget; reduce --scale, interval, or board count")
	}
	camera, err := timelineCamera(ctx, boards, o)
	if err != nil {
		return nil, fmt.Errorf("isometric timeline framing: %w", err)
	}
	o.Width, o.Height, o.camera = camera.width, camera.height, &camera
	samples := image.NewRGBA(image.Rect(0, 0, 128, 128*len(boards)))
	for i, board := range boards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s, err := openCapture(ctx, board, o)
		if err != nil {
			return nil, fmt.Errorf("isometric timeline board %d: %w", i, err)
		}
		for phase := 0; phase < 4; phase++ {
			frame, err := s.frameImage(float64(phase)*interval.Seconds()/4, true)
			if err != nil {
				s.close()
				return nil, err
			}
			for y := 0; y < 64; y++ {
				for x := 0; x < 64; x++ {
					samples.Set((phase%2)*64+x, i*128+(phase/2)*64+y, frame.At(x*o.Width/64, y*o.Height/64))
				}
			}
		}
		s.close()
	}
	palette := quantize.MedianCutQuantizer{}.Quantize(make(color.Palette, 0, 256), samples)
	lookup, err := paletteLookup(ctx, palette)
	if err != nil {
		return nil, err
	}
	animation := &gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: palette, Width: o.Width, Height: o.Height}}
	var previous *image.Paletted
	delay := max(1, int(math.Round(float64(interval)/float64(10*time.Millisecond))))
	for boardIndex, board := range boards {
		s, err := openCapture(ctx, board, o)
		if err != nil {
			return nil, err
		}
		for i := 0; i < counts[boardIndex]; i++ {
			frame, err := s.frameImage(float64(i)*interval.Seconds()/float64(counts[boardIndex]), true)
			if err != nil {
				s.close()
				return nil, err
			}
			indexed, err := indexFrame(ctx, frame, palette, lookup)
			if err != nil {
				s.close()
				return nil, err
			}
			cropped := indexed
			if previous != nil {
				cropped = frameDelta(previous, indexed)
			}
			animation.Image = append(animation.Image, cropped)
			a := int(math.Round(float64(i) * float64(delay) / float64(counts[boardIndex])))
			b := int(math.Round(float64(i+1) * float64(delay) / float64(counts[boardIndex])))
			animation.Delay = append(animation.Delay, b-a)
			animation.Disposal = append(animation.Disposal, gif.DisposalNone)
			previous = indexed
		}
		s.close()
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&boundedWriter{ctx: ctx, w: &output, remaining: MaxOutputBytes}, animation); err != nil {
		return nil, err
	}
	return output.Bytes(), ctx.Err()
}

func timelineBoardAnimated(board *d2target.Diagram) bool {
	edges := [][]d2target.Connection{board.Connections}
	shapes := [][]d2target.Shape{board.Shapes, {board.Root}}
	if board.Legend != nil {
		edges = append(edges, board.Legend.Connections)
		shapes = append(shapes, board.Legend.Shapes)
	}
	for _, group := range edges {
		for _, edge := range group {
			if edge.Animated && edge.Opacity > 0 && edge.StrokeWidth > 0 {
				return true
			}
		}
	}
	for _, group := range shapes {
		for _, shape := range group {
			if shape.Animated && shape.Opacity > 0 {
				return true
			}
		}
	}
	return false
}
