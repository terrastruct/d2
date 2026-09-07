package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestAdmissionBeforeRendering(t *testing.T) {
	for _, o := range []Options{
		{Width: 1, Height: 64}, {Width: 63, Height: 64}, {Width: 64}, {Width: -1, Height: 100},
		{Width: MaxDimension + 1, Height: 100}, {Width: 4096, Height: 4096},
		{Format: GIF, Width: 1600, Height: 1000}, {Format: "unknown"}, {Timeout: -1}, {Timeout: 3 * time.Minute},
	} {
		if _, err := Render(context.Background(), nil, &o); err == nil {
			t.Fatalf("admitted %+v", o)
		}
	}
	if o, err := normalize(&Options{Width: 64, Height: 64}); err != nil || o.Width != 64 {
		t.Fatalf("minimum: %+v %v", o, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, seconds := range [][]float64{nil, {math.NaN()}, {math.Inf(1)}, {-1}, {86401}, make([]float64, 105)} {
		if err := CaptureFrames(context.Background(), nil, nil, seconds, true, func(int, []byte) error { return nil }); err == nil {
			t.Fatalf("admitted times %v", seconds)
		}
	}
}

func TestSharedPaletteAndOpaqueDelta(t *testing.T) {
	ctx := context.Background()
	p := color.Palette{color.Black, color.White, color.RGBA{255, 0, 0, 255}}
	lookup, err := paletteLookup(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.Paletted, 3)
	for i := range frames {
		raw := image.NewRGBA(image.Rect(0, 0, 8, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 8; x++ {
				raw.Set(x, y, color.White)
			}
		}
		raw.Set(i+1, 2, color.RGBA{255, 0, 0, 255})
		frames[i], err = indexFrame(ctx, raw, p, lookup)
		if err != nil {
			t.Fatal(err)
		}
	}
	animation := &gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: p, Width: 8, Height: 4}}
	for i, f := range frames {
		delta := f
		if i > 0 {
			delta = frameDelta(frames[i-1], f)
		}
		animation.Image = append(animation.Image, delta)
		animation.Delay = append(animation.Delay, 8)
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, animation); err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.NewPaletted(image.Rect(0, 0, 8, 4), p)
	for i, f := range decoded.Image {
		for y := f.Rect.Min.Y; y < f.Rect.Max.Y; y++ {
			for x := f.Rect.Min.X; x < f.Rect.Max.X; x++ {
				canvas.Set(x, y, f.At(x, y))
			}
		}
		if !bytes.Equal(canvas.Pix, frames[i].Pix) {
			t.Fatalf("delta reconstruction frame%d", i)
		}
		if !reflect.DeepEqual(decoded.Config.ColorModel, f.Palette) {
			t.Fatal("local palette differs")
		}
	}
	if frameDelta(frames[0], frames[0]).Rect != image.Rect(0, 0, 1, 1) {
		t.Fatal("unchanged delta must preserve one pixel")
	}
	sum := 0
	for i := 0; i < FrameCount; i++ {
		d := frameDelay(i)
		if d != 8 && d != 9 {
			t.Fatalf("delay %d", d)
		}
		sum += d
	}
	if sum != 833 {
		t.Fatalf("loop=%dcs", sum)
	}
}

func TestFrameAndWriterLimits(t *testing.T) {
	var dst bytes.Buffer
	w := boundedWriter{ctx: context.Background(), w: &dst, remaining: 2}
	if _, err := w.Write([]byte("123")); err == nil || dst.Len() != 0 {
		t.Fatal("byte limit wrote partial output")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.ctx = ctx
	if _, err := w.Write([]byte("1")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func captureDiagram() *d2target.Diagram {
	d := &d2target.Diagram{}
	for i, id := range []string{"worker", "db"} {
		s := d2target.BaseShape()
		s.ID = id
		s.Type = d2target.ShapeRectangle
		s.Width = 160
		s.Height = 100
		s.Pos = d2target.Point{X: i * 260, Y: 0}
		s.Label = id
		s.LabelWidth = 55
		s.LabelHeight = 20
		s.FontSize = 16
		s.Fill = "#b4d7ec"
		s.Stroke = "#52647a"
		d.Shapes = append(d.Shapes, *s)
	}
	c := d2target.BaseConnection()
	c.ID = "edge"
	c.Src = "worker"
	c.Dst = "db"
	c.DstArrow = d2target.ArrowArrowhead
	c.Animated = true
	c.Route = []*geo.Point{{X: 160, Y: 50}, {X: 260, Y: 50}}
	d.Connections = []d2target.Connection{*c}
	return d
}

func TestNativeDeterministicCapture(t *testing.T) {
	// A machine with no executable search path still renders every format.
	t.Setenv("PATH", "")
	opts := Options{Width: 320, Height: 200}
	var frames [][]byte
	err := CaptureFrames(context.Background(), captureDiagram(), &opts, []float64{0, CycleSeconds / 4, CycleSeconds}, true, func(i int, p []byte) error { frames = append(frames, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frames[0], frames[2]) {
		t.Fatal("traffic phase0 and1 differ")
	}
	if bytes.Equal(frames[0], frames[1]) {
		t.Fatal("traffic does not move")
	}
	p, err := Render(context.Background(), captureDiagram(), &opts)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(p))
	if err != nil || config.Width != 320 || config.Height != 200 {
		t.Fatalf("PNG contract %+v %v", config, err)
	}
	repeat, err := Render(context.Background(), captureDiagram(), &opts)
	if err != nil || !bytes.Equal(p, repeat) {
		t.Fatalf("repeated static render differs: %v", err)
	}
}

func TestCaptureCallbackCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	err := CaptureFrames(ctx, captureDiagram(), &Options{Width: 160, Height: 100}, []float64{0, 1}, true, func(int, []byte) error {
		count++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatalf("canceled capture emitted %d frames: %v", count, err)
	}
	want := errors.New("encoder stopped")
	err = CaptureFrames(context.Background(), captureDiagram(), &Options{Width: 160, Height: 100}, []float64{0, 1}, true, func(int, []byte) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("callback error lost: %v", err)
	}
}

func TestNativeGIFWithoutTraffic(t *testing.T) {
	t.Setenv("PATH", "")
	diagram := captureDiagram()
	diagram.Connections = nil
	opts := Options{Format: GIF, Width: 160, Height: 100}
	data, err := Render(context.Background(), diagram, &opts)
	if err != nil {
		t.Fatal(err)
	}
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(animation.Image) != FrameCount || animation.LoopCount != 0 || animation.Config.Width != 160 || animation.Config.Height != 100 {
		t.Fatal("native GIF frame/canvas/loop contract")
	}
	total := 0
	for i, frame := range animation.Image {
		total += animation.Delay[i]
		if animation.Disposal[i] != gif.DisposalNone || !reflect.DeepEqual(frame.Palette, animation.Config.ColorModel) {
			t.Fatalf("palette/disposal changed at frame %d", i)
		}
		if i > 0 && frame.Rect != image.Rect(0, 0, 1, 1) {
			t.Fatalf("static scene changed at frame %d: %v", i, frame.Rect)
		}
	}
	if total != 833 {
		t.Fatalf("native GIF loop=%dcs", total)
	}
}
