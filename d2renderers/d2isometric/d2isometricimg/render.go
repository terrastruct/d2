// Package d2isometricimg renders the D2 3D scene to SVG, PNG or GIF in pure Go.
// It needs no browser, GPU, external executable, or system fonts.
package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

type Format string

const (
	SVG                Format = "svg"
	PNG                Format = "png"
	GIF                Format = "gif"
	CycleSeconds              = 25.0 / 3.0
	FrameCount                = 100
	MaxDimension              = 4096
	MaxPixels                 = 12_000_000
	MaxAnimationPixels        = 100_000_000
	MaxOutputBytes            = 64 << 20
	maxPNGBytes               = 32 << 20
)

// Options uses fixed output pixels. Zero dimensions select 1600x1000 for PNG
// or 1000x625 for GIF.
// Timeout defaults to two minutes and cannot exceed two minutes. An earlier
// caller deadline always wins.
type Options struct {
	Render d2isometric.RenderOpts
	// Assets explicitly supplies bounded image loading. Rendering does not
	// otherwise read local files or fetch remote resources.
	Assets *d2scenebuild.AssetOptions
	// Fonts supplies optional bounded fallback discovery. Configured D2 font
	// families are retained even when no fallback resolver is provided.
	Fonts         *d2scenebuild.FontFallbackOptions
	Format        Format
	Width, Height int
	Timeout       time.Duration
	// FitContent treats Width and Height as maximum dimensions and trims unused
	// canvas to the projected diagram's aspect ratio. Geometry is never resized.
	FitContent bool
	// LinkBudget bounds page annotations before they are accumulated. Nil uses
	// 4096 regions and 1 MiB of destination/tooltip text for RenderPage.
	LinkBudget *d2scenebuild.LinkBudget
	// A timeline shares one logical-pixel camera across all boards.
	camera *rasterCamera
}

func normalize(opts *Options) (Options, error) {
	o := Options{}
	if opts != nil {
		o = *opts
	}
	if o.Format == "" {
		o.Format = PNG
	}
	if o.Format != SVG && o.Format != PNG && o.Format != GIF {
		return o, fmt.Errorf("unsupported isometric image format %q (use svg, png or gif)", o.Format)
	}
	if o.Width == 0 && o.Height == 0 {
		o.Width, o.Height = 1600, 1000
		if o.Format == GIF {
			o.Width, o.Height = 1000, 625
		}
	}
	if o.Width < 64 || o.Height < 64 || o.Width > MaxDimension || o.Height > MaxDimension || int64(o.Width)*int64(o.Height) > MaxPixels {
		return o, fmt.Errorf("isometric image dimensions %dx%d exceed limits (64..%d per side and at most %d pixels)", o.Width, o.Height, MaxDimension, MaxPixels)
	}
	if o.Format == GIF && int64(o.Width)*int64(o.Height)*(FrameCount+4) > MaxAnimationPixels {
		return o, fmt.Errorf("isometric GIF pixels exceed %d across the traffic cycle; reduce --scale", MaxAnimationPixels)
	}
	if o.Timeout == 0 {
		o.Timeout = 2 * time.Minute
	}
	if o.Timeout < 0 || o.Timeout > 2*time.Minute {
		return o, fmt.Errorf("isometric image timeout must be positive and at most two minutes")
	}
	if o.LinkBudget != nil && (o.LinkBudget.MaxRegions <= 0 || o.LinkBudget.MaxRegions > 4096 || o.LinkBudget.MaxStringBytes <= 0 || o.LinkBudget.MaxStringBytes > 1<<20) {
		return o, fmt.Errorf("isometric page link budget must be within 4096 regions and 1 MiB of text")
	}
	return o, nil
}

// Render returns a vector SVG or lossless PNG without traffic particles, or an
// infinitely looping GIF with a fixed camera and deterministic traffic. Rendering stays
// in memory and does not start external processes. Image I/O, when needed,
// is performed only by the explicitly supplied asset resolver.
func Render(ctx context.Context, diagram *d2target.Diagram, opts *Options) ([]byte, error) {
	o, err := normalize(opts)
	if err != nil {
		return nil, err
	}
	if o.Format == SVG {
		return renderNativeSVG(ctx, diagram, o)
	}
	s, err := openCapture(ctx, diagram, o)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if o.Format == PNG {
		return s.frame(0, false)
	}
	return renderGIF(s)
}

// CaptureFrames delivers lossless frames at exact scene times, serially. It is
// useful for deterministic comparison and other bounded image encoders. The
// callback owns each PNG and may retain it; no frame cache is kept here.
func CaptureFrames(ctx context.Context, diagram *d2target.Diagram, opts *Options, seconds []float64, traffic bool, emit func(int, []byte) error) error {
	o, err := normalize(opts)
	if err != nil {
		return err
	}
	if len(seconds) == 0 || len(seconds) > FrameCount+4 || emit == nil {
		return fmt.Errorf("capture needs 1..%d times and a callback", FrameCount+4)
	}
	if int64(o.Width)*int64(o.Height)*int64(len(seconds)) > MaxAnimationPixels {
		return fmt.Errorf("capture aggregate pixels exceed limit")
	}
	for _, t := range seconds {
		if math.IsNaN(t) || math.IsInf(t, 0) || t < 0 || t > 86400 {
			return fmt.Errorf("capture time must be finite and in [0,86400]")
		}
	}
	s, err := openCapture(ctx, diagram, o)
	if err != nil {
		return err
	}
	defer s.close()
	for i, t := range seconds {
		p, err := s.frame(t, traffic)
		if err != nil {
			return err
		}
		if err := emit(i, p); err != nil {
			return err
		}
		if err := s.ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}
