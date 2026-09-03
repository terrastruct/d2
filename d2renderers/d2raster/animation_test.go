package d2raster

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestRenderTypedAnimationAtTimeWithoutMutation(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
	node.ID = "animated"
	node.Animations = []d2scene.Track{
		animationTrack(d2scene.AnimateTransform, d2scene.TransformValue(d2scene.Identity()), d2scene.TransformValue(d2scene.Translate(10, 0))),
		animationTrack(d2scene.AnimateFillColor, d2scene.ColorValue(color.NRGBA{R: 255, A: 255}), d2scene.ColorValue(color.NRGBA{B: 255, A: 255})),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 30, Height: 10}, node)
	originalTransform := node.Transform
	originalPrimitive := node.Primitive
	originalTracks := append([]d2scene.Track(nil), node.Animations...)

	tests := []struct {
		name string
		time time.Duration
		x    int
		want color.NRGBA
	}{
		{name: "start", x: 5, want: color.NRGBA{R: 255, A: 255}},
		{name: "middle", time: 500 * time.Millisecond, x: 10, want: color.NRGBA{R: 128, B: 128, A: 255}},
		{name: "end", time: time.Second, x: 15, want: color.NRGBA{B: 255, A: 255}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := testOptions()
			options.Time = test.time
			frame, err := Render(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			assertPixel(t, frame.NRGBAAt(test.x, 5), test.want)
		})
	}

	if node.Transform != originalTransform || node.Primitive != originalPrimitive || len(node.Animations) != len(originalTracks) {
		t.Fatal("render mutated animated scene node")
	}
	for i := range originalTracks {
		if node.Animations[i].Property != originalTracks[i].Property || len(node.Animations[i].Keyframes) != len(originalTracks[i].Keyframes) {
			t.Fatal("render mutated animation tracks")
		}
	}
}

func TestRenderStrokeColorAnimation(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(1, 5), d2scene.LineTo(19, 5)},
		Stroke:   &d2scene.Stroke{Paint: red, Width: 4, Cap: d2scene.CapButt},
	})
	node.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateStrokeColor,
		d2scene.ColorValue(color.NRGBA{R: 255, A: 255}),
		d2scene.ColorValue(color.NRGBA{B: 255, A: 255}),
	)}
	options := testOptions()
	options.Time = 500 * time.Millisecond
	frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 20, Height: 10}, node), options)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(10, 5), color.NRGBA{R: 128, B: 128, A: 255})
}

func TestRenderAnimationOpacityAndDashOffset(t *testing.T) {
	t.Parallel()

	opacityNode := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
	opacityNode.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateOpacity, d2scene.NumberValue(1), d2scene.NumberValue(0),
	)}
	opacityOptions := testOptions()
	opacityOptions.Time = 500 * time.Millisecond
	opacityFrame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, opacityNode), opacityOptions)
	if err != nil {
		t.Fatal(err)
	}
	pixel := opacityFrame.NRGBAAt(5, 5)
	if pixel.R != 255 || pixel.A < 127 || pixel.A > 128 {
		t.Fatalf("half-opacity pixel = %#v, want red with alpha 127/128", pixel)
	}

	dashNode := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(1, 5), d2scene.LineTo(29, 5)},
		Stroke:   &d2scene.Stroke{Paint: black, Width: 2, Dashes: []float64{4, 4}, Cap: d2scene.CapButt},
	})
	dashNode.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateStrokeDashOffset, d2scene.NumberValue(0), d2scene.NumberValue(4),
	)}
	dashDocument := d2scene.NewDocument(d2scene.Box{Width: 30, Height: 10}, dashNode)
	start, err := renderTestPNG(context.Background(), dashDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	endOptions := testOptions()
	endOptions.Time = time.Second
	end, err := renderTestPNG(context.Background(), dashDocument, endOptions)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(start, end) {
		t.Fatal("animated dash offset did not change rendered pixels")
	}
}

func TestRenderAnimationEasingPixels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		easing d2scene.Easing
		min    float64
		max    float64
	}{
		{name: "linear", easing: d2scene.Easing{Kind: d2scene.EaseLinear}, min: .25, max: .25},
		{
			name: "cubic ease-in", easing: d2scene.Easing{
				Kind: d2scene.EaseCubicBezier, X1: .42, Y1: 0, X2: 1, Y2: 1,
			},
			min: .09, max: .10,
		},
		{name: "step start", easing: d2scene.Easing{Kind: d2scene.EaseStepStart}, min: 1, max: 1},
		{name: "step end", easing: d2scene.Easing{Kind: d2scene.EaseStepEnd}, min: 0, max: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			track := d2scene.Track{
				Property: d2scene.AnimateOpacity,
				Duration: time.Second,
				Keyframes: []d2scene.Keyframe{
					{Offset: 0, Value: d2scene.NumberValue(0), Easing: test.easing},
					{Offset: 1, Value: d2scene.NumberValue(1)},
				},
			}
			const timestamp = 250 * time.Millisecond
			options := testOptions()
			options.Time = timestamp
			rendererValue, err := (&preflight{ctx: context.Background(), options: options}).animationValueAt(track)
			if err != nil {
				t.Fatal(err)
			}
			if rendererValue.Number < test.min || rendererValue.Number > test.max {
				t.Fatalf("easing value = %.12g, want in [%.3g, %.3g]", rendererValue.Number, test.min, test.max)
			}

			node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
			node.Animations = []d2scene.Track{track}
			frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node), options)
			if err != nil {
				t.Fatal(err)
			}
			pixel := frame.NRGBAAt(5, 5)
			wantAlpha := uint8(math.Round(255 * rendererValue.Number))
			if difference := math.Abs(float64(pixel.A) - float64(wantAlpha)); difference > 1 {
				t.Fatalf("rendered alpha = %d, want %d (animation value %.12g)", pixel.A, wantAlpha, rendererValue.Number)
			}
			if pixel.A > 0 && (pixel.R != 255 || pixel.G != 0 || pixel.B != 0) {
				t.Fatalf("rendered color = %#v, want unpremultiplied red", pixel)
			}
		})
	}
}

func TestRenderAnimatedSceneConcurrentTimesDeterministic(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
	node.Animations = []d2scene.Track{
		animationTrack(d2scene.AnimateTransform, d2scene.TransformValue(d2scene.Identity()), d2scene.TransformValue(d2scene.Translate(10, 0))),
		animationTrack(d2scene.AnimateFillColor, d2scene.ColorValue(color.NRGBA{R: 255, A: 255}), d2scene.ColorValue(color.NRGBA{B: 255, A: 255})),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 10}, node)
	times := []time.Duration{0, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, time.Second}
	want := make([][]byte, len(times))
	for i, timestamp := range times {
		options := testOptions()
		options.Time = timestamp
		var err error
		want[i], err = renderTestPNG(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
	}

	var wait sync.WaitGroup
	errors := make(chan error, len(times)*8)
	for range 8 {
		for i, timestamp := range times {
			wait.Add(1)
			go func(index int, timestamp time.Duration) {
				defer wait.Done()
				options := testOptions()
				options.Time = timestamp
				got, err := renderTestPNG(context.Background(), document, options)
				if err != nil {
					errors <- err
					return
				}
				if !bytes.Equal(got, want[index]) {
					errors <- fmt.Errorf("concurrent animation frame mismatch at timestamp index %d", index)
				}
			}(i, timestamp)
		}
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func TestRenderRejectsInvalidAnimationTargets(t *testing.T) {
	t.Parallel()

	validOpacity := animationTrack(d2scene.AnimateOpacity, d2scene.NumberValue(1), d2scene.NumberValue(0))
	tests := []struct {
		name string
		node *d2scene.Node
		want string
	}{
		{
			name: "duplicate", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}, Fill: red})
				n.Animations = []d2scene.Track{validOpacity, validOpacity}
				return n
			}(), want: "duplicate animation target",
		},
		{
			name: "scalar index", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}, Fill: red})
				track := validOpacity
				track.TargetIndex = 1
				n.Animations = []d2scene.Track{track}
				return n
			}(), want: "non-zero target index",
		},
		{
			name: "opacity range", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}, Fill: red})
				n.Animations = []d2scene.Track{animationTrack(d2scene.AnimateOpacity, d2scene.NumberValue(2), d2scene.NumberValue(2))}
				return n
			}(), want: "opacity outside",
		},
		{
			name: "group paint", node: func() *d2scene.Node {
				n := d2scene.NewNode(nil)
				n.Animations = []d2scene.Track{animationTrack(d2scene.AnimateFillColor, d2scene.ColorValue(color.NRGBA{}), d2scene.ColorValue(color.NRGBA{}))}
				return n
			}(), want: "no primitive",
		},
		{
			name: "missing fill", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}})
				n.Animations = []d2scene.Track{animationTrack(d2scene.AnimateFillColor, d2scene.ColorValue(color.NRGBA{}), d2scene.ColorValue(color.NRGBA{}))}
				return n
			}(), want: "missing paint",
		},
		{
			name: "missing stroke", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}, Fill: red})
				n.Animations = []d2scene.Track{animationTrack(d2scene.AnimateStrokeDashOffset, d2scene.NumberValue(0), d2scene.NumberValue(1))}
				return n
			}(), want: "missing stroke",
		},
		{
			name: "drop shadow target", node: func() *d2scene.Node {
				n := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 5}, Fill: red})
				n.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}}
				n.Animations = []d2scene.Track{animationTrack(
					d2scene.AnimateDropShadow,
					d2scene.ShadowValue(d2scene.DropShadow{}),
					d2scene.ShadowValue(d2scene.DropShadow{}),
				)}
				return n
			}(), want: "does not identify a drop shadow",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, test.node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func animationTrack(property d2scene.AnimationProperty, first, last d2scene.AnimationValue) d2scene.Track {
	return d2scene.Track{
		Property: property, Duration: time.Second,
		Keyframes: []d2scene.Keyframe{{Offset: 0, Value: first}, {Offset: 1, Value: last}},
	}
}
