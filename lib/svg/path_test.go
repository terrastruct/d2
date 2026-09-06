package svg_test

import (
	"reflect"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/svg"
)

func TestSVGPathContextTypedCommandsPreservePathData(t *testing.T) {
	t.Parallel()

	path := svg.NewSVGPathContext(geo.NewPoint(10, 20), 2, 3)
	path.StartAt(path.Absolute(1, 2))
	path.H(true, 4)
	path.V(false, 5)
	path.L(true, -2, 1)
	path.C(true, 1, 2, 3, 4, 5, 6)
	path.Z()

	const wantPathData = "M 12 26 H 20 V 35 L 16 38 C 18 44 22 50 26 56 Z"
	if got := path.PathData(); got != wantPathData {
		t.Fatalf("PathData() = %q, want %q", got, wantPathData)
	}

	wantCommands := []svg.PathCommand{
		{Kind: svg.PathCommandMove, End: geo.Point{X: 12, Y: 26}},
		{Kind: svg.PathCommandLine, End: geo.Point{X: 20, Y: 26}},
		{Kind: svg.PathCommandLine, End: geo.Point{X: 20, Y: 35}},
		{Kind: svg.PathCommandLine, End: geo.Point{X: 16, Y: 38}},
		{
			Kind:     svg.PathCommandCubic,
			Control1: geo.Point{X: 18, Y: 44},
			Control2: geo.Point{X: 22, Y: 50},
			End:      geo.Point{X: 26, Y: 56},
		},
		{Kind: svg.PathCommandClose},
	}
	if got := path.PathCommands(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("PathCommands() = %#v, want %#v", got, wantCommands)
	}
}

func TestSVGPathContextPathCommandsReturnsCopy(t *testing.T) {
	t.Parallel()

	path := svg.NewSVGPathContext(geo.NewPoint(0, 0), 1, 1)
	path.StartAt(path.Absolute(1, 2))
	path.L(false, 3, 4)

	commands := path.PathCommands()
	commands[0].End.X = 99

	want := []svg.PathCommand{
		{Kind: svg.PathCommandMove, End: geo.Point{X: 1, Y: 2}},
		{Kind: svg.PathCommandLine, End: geo.Point{X: 3, Y: 4}},
	}
	if got := path.PathCommands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PathCommands() after caller mutation = %#v, want %#v", got, want)
	}
	if got, wantData := path.PathData(), "M 1 2 L 3 4"; got != wantData {
		t.Fatalf("PathData() after typed command mutation = %q, want %q", got, wantData)
	}
}
