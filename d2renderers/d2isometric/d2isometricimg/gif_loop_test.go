package d2isometricimg

import (
	"bytes"
	"context"
	"errors"
	"image/gif"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestNativeGIFAuthoredNodeLoopAndPacketSeam(t *testing.T) {
	d := captureDiagram()
	d.Shapes[0].Animated = true
	o, err := normalize(&Options{Format: GIF, Width: 160, Height: 100})
	if err != nil {
		t.Fatal(err)
	}
	s, err := openCapture(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	frames, seconds := nativeGIFCycle(s.scene)
	if frames != 96 || seconds != 8 {
		t.Fatalf("authored cycle: %d %g", frames, seconds)
	}
	a, err := s.frameImageAt(0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.frameImageAt(seconds, CycleSeconds, true)
	if err != nil || !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("node/packet loop seam: %v", err)
	}
	encoded, err := renderGIF(s)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, delay := range g.Delay {
		total += delay
	}
	if len(g.Image) != 96 || total != 800 || g.LoopCount != 0 {
		t.Fatalf("authored GIF: frames %d duration %d loop %d", len(g.Image), total, g.LoopCount)
	}
}

func TestNativeGIFPanelLoopDoesNotMutateSourceTiming(t *testing.T) {
	d := sourcePanelFixture(t, "txtar/legend-mono/dagre/board.exp.json")
	connection := sourcePanelFixture(t, "stable/sequence_diagram_groups/dagre/board.exp.json").Connections[0]
	connection.Animated = true
	connection.Label = "Message"
	d.Legend.Connections = []d2target.Connection{connection}
	n := nativeFixtureScene(t, d)
	original := n.panels[0].document
	looped, err := nativeGIFLoopScene(context.Background(), n, 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if n.panels[0].document != original || looped.panels[0].document == original || looped.panels[0].document.Root == original.Root {
		t.Fatal("GIF timing modified the caller's surface document")
	}
	var durations func(*d2scene.Node) []time.Duration
	durations = func(n *d2scene.Node) []time.Duration {
		var out []time.Duration
		for _, track := range n.Animations {
			out = append(out, track.Duration)
		}
		for _, child := range n.Children {
			out = append(out, durations(child)...)
		}
		return out
	}
	before, after := durations(original.Root), durations(looped.panels[0].document.Root)
	if len(before) == 0 || len(before) != len(after) || before[0] == after[0] {
		t.Fatalf("nonintegral dash timing not adapted: %v -> %v", before, after)
	}
	a, err := looped.Frame(context.Background(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := looped.Frame(context.Background(), 8, true)
	if err != nil || !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("source-panel GIF loop seam: %v", err)
	}
	if got := durations(original.Root); len(got) != len(before) || got[0] != before[0] {
		t.Fatal("exact-time source track changed")
	}
}

func TestNativeGIFLoopCloneAdmission(t *testing.T) {
	n := d2scene.NewNode(nil)
	n.Children = []*d2scene.Node{n}
	s := &nativeScene{panels: []nativePanel{{animated: true, document: d2scene.NewDocument(d2scene.Box{}, n)}}}
	if _, err := nativeGIFLoopScene(context.Background(), s, 8*time.Second); err == nil {
		t.Fatal("cyclic animation document admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeGIFLoopScene(ctx, s, 8*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNativeGIFLoopCopiesAnimatedMasks(t *testing.T) {
	root, mask := d2scene.NewNode(nil), d2scene.NewNode(nil)
	mask.Animations = []d2scene.Track{{Duration: 2300 * time.Millisecond, Repeat: true}}
	root.Mask = &d2scene.Mask{Root: mask}
	s := &nativeScene{panels: []nativePanel{{animated: true, document: d2scene.NewDocument(d2scene.Box{}, root)}}}
	out, err := nativeGIFLoopScene(context.Background(), s, 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	copy := out.panels[0].document.Root.Mask.Root
	if copy == mask || copy.Animations[0].Duration != 8*time.Second/3 || mask.Animations[0].Duration != 2300*time.Millisecond {
		t.Fatal("GIF-only mask timing was not copied and adapted")
	}
}
