package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	"image/gif"
	"reflect"
	"testing"
	"time"

	"github.com/d2lang/d2/d2target"
)

func TestNativeTimelineBoardOrderPaletteAndDelay(t *testing.T) {
	t.Setenv("PATH", "")
	first, second := captureDiagram(), captureDiagram()
	first.Root.Fill, second.Root.Fill = "#ffd5d5", "#d5e3ff"
	first.Connections[0].Animated, second.Connections[0].Animated = false, false
	boards := []*d2target.Diagram{first, second}
	before, _ := json.Marshal(boards)
	encoded, err := RenderTimeline(context.Background(), boards, &Options{Format: GIF, Width: 160, Height: 100}, 370*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(boards)
	if !bytes.Equal(before, after) {
		t.Fatal("timeline mutated source boards")
	}
	g, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 2 || g.LoopCount != 0 || g.Config.Width != 160 || g.Config.Height != 100 || !reflect.DeepEqual(g.Delay, []int{37, 37}) {
		t.Fatalf("board timeline contract: frames=%d delays=%v config=%+v", len(g.Image), g.Delay, g.Config)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 160, 100))
	for i, frame := range g.Image {
		if !reflect.DeepEqual(g.Config.ColorModel, frame.Palette) || g.Disposal[i] != gif.DisposalNone {
			t.Fatal("timeline changed palette or disposal")
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Src)
		r, _, b, a := canvas.At(0, 0).RGBA()
		if a != 65535 || i == 0 && r <= b || i == 1 && b <= r {
			t.Fatalf("wrong board order or delta reconstruction at frame %d: %x %x %x", i, r, b, a)
		}
	}
}

func TestNativeTimelineAuthoredAnimation(t *testing.T) {
	board := captureDiagram()
	encoded, err := RenderTimeline(context.Background(), []*d2target.Diagram{board}, &Options{Format: GIF, Width: 320, Height: 200}, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 6 {
		t.Fatalf("authored animation frame count %d", len(g.Image))
	}
	total, changed := 0, false
	for i, delay := range g.Delay {
		if delay < 1 {
			t.Fatal("zero GIF delay")
		}
		total += delay
		changed = changed || i > 0 && g.Image[i].Bounds().Dx()*g.Image[i].Bounds().Dy() > 1
	}
	if total != 50 || !changed {
		t.Fatalf("animated board duration/motion %d %v", total, changed)
	}
	board.Connections[0].Animated = false
	var frames [][]byte
	err = CaptureFrames(context.Background(), board, &Options{Width: 160, Height: 100}, []float64{0, CycleSeconds / 4}, true, func(_ int, p []byte) error { frames = append(frames, p); return nil })
	if err != nil || !bytes.Equal(frames[0], frames[1]) {
		t.Fatalf("unanimated route received traffic: %v", err)
	}
	board.Shapes[0].Animated = true
	if !timelineBoardAnimated(board) {
		t.Fatal("authored shape animation omitted from timeline sampling")
	}
}

func TestNativeTimelineAdmission(t *testing.T) {
	o := &Options{Format: GIF, Width: 64, Height: 64}
	for _, interval := range []time.Duration{0, -1, 11 * time.Minute} {
		if _, err := RenderTimeline(context.Background(), []*d2target.Diagram{captureDiagram()}, o, interval); err == nil {
			t.Fatalf("admitted interval %s", interval)
		}
	}
	for _, boards := range [][]*d2target.Diagram{nil, {nil}, {{IsFolderOnly: true}}, make([]*d2target.Diagram, MaxTimelineBoards+1)} {
		if _, err := RenderTimeline(context.Background(), boards, o, time.Second); err == nil {
			t.Fatal("admitted invalid board list")
		}
	}
	if _, err := RenderTimeline(context.Background(), []*d2target.Diagram{captureDiagram()}, o, 10*time.Minute); err == nil {
		t.Fatal("admitted unbounded animated frame count")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RenderTimeline(ctx, []*d2target.Diagram{captureDiagram()}, o, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
