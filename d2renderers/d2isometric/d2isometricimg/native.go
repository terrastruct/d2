package d2isometricimg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// A capture owns the scene and cached static raster for one bounded export.
// Frame production is serial; neither the input diagram nor caller images are
// retained outside this session. No files or external processes are created.
type capture struct {
	ctx   context.Context
	opts  Options
	scene *nativeScene
	close context.CancelFunc
}

func openCapture(ctx context.Context, diagram *d2target.Diagram, o Options) (*capture, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	scene, err := d2isometric.BuildScene(diagram, &o.Render)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, err
	}
	native, err := newNativeSceneWithOptions(ctx, scene, o.Width, o.Height, o.Assets, o.Fonts, nativeSceneOptions{fitContent: o.FitContent, camera: o.camera, outputDensity: sceneOutputDensity(scene, o.Width, o.Height, o.camera), links: o.LinkBudget})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("initialize native isometric renderer: %w", err)
	}
	o.Width, o.Height = native.width, native.height
	return &capture{ctx: ctx, opts: o, scene: native, close: cancel}, nil
}

func (s *capture) frameImage(seconds float64, traffic bool) (*image.RGBA, error) {
	return s.frameImageAt(seconds, seconds, traffic)
}

// Independent packet time lets whole-second authored animations and traffic
// share a seamless GIF loop without changing the public exact-time frame API.
func (s *capture) frameImageAt(seconds, packetSeconds float64, traffic bool) (*image.RGBA, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	frame, err := s.scene.Frame(s.ctx, seconds, traffic, packetSeconds)
	if err != nil {
		return nil, fmt.Errorf("render isometric frame at %.4fs: %w", seconds, err)
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if frame == nil || frame.Bounds() != image.Rect(0, 0, s.opts.Width, s.opts.Height) {
		return nil, fmt.Errorf("native isometric renderer returned unexpected frame bounds")
	}
	return frame, nil
}

func (s *capture) frame(seconds float64, traffic bool) ([]byte, error) {
	frame, err := s.frameImage(seconds, traffic)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	w := &boundedWriter{ctx: s.ctx, w: &output, remaining: maxPNGBytes}
	if err := png.Encode(w, frame); err != nil {
		return nil, fmt.Errorf("encode isometric PNG: %w", err)
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
