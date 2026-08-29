package d2svgimport

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var generousPathLimits = PathLimits{MaxBytes: 1 << 20, MaxCommands: 10_000}

func TestParsePathCompleteCommandGrammar(t *testing.T) {
	t.Parallel()

	data := "M 10 20 l 5-5 H 30 v 10 C 31 32 33 34 35 36 s 2 3 4 5 Q 45 46 47 48 t 3 4 A 5 6 90 1 0 60 70 a 2 3 -45 01 4 5 z"
	got, err := ParsePath(context.Background(), "icon.svg", data, generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	want := []d2scene.PathCommand{
		d2scene.MoveTo(10, 20),
		d2scene.LineTo(15, 15),
		d2scene.LineTo(30, 15),
		d2scene.LineTo(30, 25),
		d2scene.CubicTo(31, 32, 33, 34, 35, 36),
		d2scene.CubicTo(37, 38, 37, 39, 39, 41),
		d2scene.QuadraticTo(45, 46, 47, 48),
		d2scene.QuadraticTo(49, 50, 50, 52),
		d2scene.ArcTo(5, 6, math.Pi/2, true, false, 60, 70),
		d2scene.ArcTo(2, 3, -math.Pi/4, false, true, 64, 75),
		d2scene.ClosePath(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePath() =\n%#v\nwant\n%#v", got, want)
	}
	path := d2scene.Path{Commands: got}
	if _, err := path.GeometryBounds(); err != nil {
		t.Fatalf("parsed commands fail scene validation: %v", err)
	}
}

func TestParsePathImplicitGroupsAndRelativeMove(t *testing.T) {
	t.Parallel()

	got, err := ParsePath(context.Background(), "implicit", "m10 10 5 0 0 5 M30 30 40 40z", generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	want := []d2scene.PathCommand{
		d2scene.MoveTo(10, 10), d2scene.LineTo(15, 10), d2scene.LineTo(15, 15),
		d2scene.MoveTo(30, 30), d2scene.LineTo(40, 40), d2scene.ClosePath(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePath() = %#v, want %#v", got, want)
	}
}

func TestParsePathNumberGrammarAndDegenerateArcs(t *testing.T) {
	t.Parallel()

	got, err := ParsePath(context.Background(), "numbers", "M.5-.25 L1e2,+2.5E-1 A0 4 0 0 1 4 5 A2 2 0 0 1 4 5", generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	want := []d2scene.PathCommand{
		d2scene.MoveTo(.5, -.25),
		d2scene.LineTo(100, .25),
		// A zero radius is a straight line. The final arc has identical
		// endpoints and therefore contributes no segment.
		d2scene.LineTo(4, 5),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePath() = %#v, want %#v", got, want)
	}
}

func TestParsePathResetsSmoothControlAfterOtherCommands(t *testing.T) {
	t.Parallel()

	got, err := ParsePath(context.Background(), "smooth", "M0 0 C1 2 3 4 5 6 L7 8 S9 10 11 12 Q13 14 15 16 M20 20 T22 22", generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	wantCubic := d2scene.CubicTo(7, 8, 9, 10, 11, 12)
	if got[3] != wantCubic {
		t.Fatalf("smooth cubic after line = %#v, want %#v", got[3], wantCubic)
	}
	wantQuad := d2scene.QuadraticTo(20, 20, 22, 22)
	if got[6] != wantQuad {
		t.Fatalf("smooth quadratic after move = %#v, want %#v", got[6], wantQuad)
	}
}

func TestParsePathRejectsMalformedOrUnboundedDataWithoutPartialResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		data   string
		limits PathLimits
		want   string
	}{
		{name: "before move", data: "L1 2", limits: generousPathLimits, want: "before a move"},
		{name: "unknown command", data: "X1 2", limits: generousPathLimits, want: "expected a path command"},
		{name: "missing coordinate", data: "M1", limits: generousPathLimits, want: "finite number"},
		{name: "bad exponent", data: "M1e 2", limits: generousPathLimits, want: "exponent"},
		{name: "overflow", data: "M1e999 2", limits: generousPathLimits, want: "finite number"},
		{name: "negative radius", data: "M0 0 A-1 2 0 0 0 3 4", limits: generousPathLimits, want: "non-negative"},
		{name: "bad first flag", data: "M0 0 A1 2 0 2 0 3 4", limits: generousPathLimits, want: "flag"},
		{name: "bad second flag", data: "M0 0 A1 2 0 0 7 3 4", limits: generousPathLimits, want: "flag"},
		{name: "repeated comma", data: "M0,,1", limits: generousPathLimits, want: "repeated comma"},
		{name: "comma after move", data: "M,0 0", limits: generousPathLimits, want: "comma is not allowed"},
		{name: "comma after line", data: "M0 0L,1 1", limits: generousPathLimits, want: "comma is not allowed"},
		{name: "comma after arc", data: "M0 0A,1 1 0 0 0 1 1", limits: generousPathLimits, want: "comma is not allowed"},
		{name: "relative coordinate overflow", data: "M1e308 0l1e308 0", limits: generousPathLimits, want: "non-finite derived"},
		{name: "smooth reflection overflow", data: "M-1e308 0C1e308 0 1e308 0-1e308 0S0 0 0 0", limits: generousPathLimits, want: "non-finite derived"},
		{name: "after close", data: "M0 0z 1 2", limits: generousPathLimits, want: "expected a path command"},
		{name: "command limit", data: "M0 0L1 1", limits: PathLimits{MaxBytes: 100, MaxCommands: 1}, want: "command count"},
		{name: "byte limit", data: "M0 0", limits: PathLimits{MaxBytes: 3, MaxCommands: 10}, want: "exceeding limit"},
		{name: "zero byte limit", data: "", limits: PathLimits{MaxCommands: 1}, want: "limits must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParsePath(context.Background(), "secret-free.svg", test.data, test.limits)
			if err == nil || !strings.Contains(err.Error(), "secret-free.svg") && test.name != "zero byte limit" || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParsePath() = %#v, %v; want no result and error containing %q", got, err, test.want)
			}
			if got != nil {
				t.Fatalf("malformed parse returned partial commands: %#v", got)
			}
		})
	}
}

func TestParsePathCountsDegenerateSourceArcsAndNormalizesLargeAngles(t *testing.T) {
	t.Parallel()

	data := "M0 0A1 1 0 0 0 0 0A1 1 1e308 0 0 0 0"
	commands, err := ParsePath(context.Background(), "degenerate.svg", data, PathLimits{MaxBytes: len(data), MaxCommands: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0] != d2scene.MoveTo(0, 0) {
		t.Fatalf("degenerate arcs emitted commands: %#v", commands)
	}
	if commands, err = ParsePath(context.Background(), "degenerate.svg", data, PathLimits{MaxBytes: len(data), MaxCommands: 2}); err == nil || commands != nil || !strings.Contains(err.Error(), "command count") {
		t.Fatalf("over-limit degenerate arcs = %#v/%v", commands, err)
	}

	rotated, err := ParsePath(context.Background(), "angle.svg", "M0 0A1 1 1e308 0 0 1 1", generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(rotated) != 2 || math.IsInf(rotated[1].Rotation, 0) || math.IsNaN(rotated[1].Rotation) {
		t.Fatalf("large finite angle produced invalid command: %#v", rotated)
	}
}

func TestParsePathCancellationAndInputOwnership(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := ParsePath(ctx, "cancelled.svg", "M0 0", generousPathLimits); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ParsePath() = %#v, %v", got, err)
	}

	data := "M0 0L1 1"
	commands, err := ParsePath(context.Background(), "owned.svg", data, generousPathLimits)
	if err != nil {
		t.Fatal(err)
	}
	copyOfCommands := append([]d2scene.PathCommand(nil), commands...)
	data = strings.Repeat("x", len(data))
	if !reflect.DeepEqual(commands, copyOfCommands) {
		t.Fatal("parsed commands alias input storage")
	}
}

func TestPathNumberScanChecksCancellationBeforeTokenEnd(t *testing.T) {
	t.Parallel()

	ctx := &cancelOnErrCall{call: 2}
	parser := pathParser{
		ctx: ctx, source: "long-number.svg", data: strings.Repeat("1", 16*1024),
		maxCommands: 1,
	}
	if _, err := parser.number(); !errors.Is(err, context.Canceled) {
		t.Fatalf("number() error = %v, want context cancellation", err)
	}
	if parser.offset >= len(parser.data) {
		t.Fatalf("number scan consumed all %d bytes before observing cancellation", len(parser.data))
	}
}

type cancelOnErrCall struct {
	calls int
	call  int
}

func (*cancelOnErrCall) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*cancelOnErrCall) Done() <-chan struct{}       { return nil }
func (c *cancelOnErrCall) Err() error {
	c.calls++
	if c.calls >= c.call {
		return context.Canceled
	}
	return nil
}
func (*cancelOnErrCall) Value(any) any { return nil }

func FuzzParsePath(f *testing.F) {
	for _, seed := range []string{
		"M0 0L1 1z",
		"m.5-.5c1 2 3 4 5 6s7 8 9 10",
		"M0 0A10 20 45 1 0 30 40",
		"M0,,0",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 4096 {
			t.Skip()
		}
		commands, err := ParsePath(context.Background(), "fuzz.svg", data, PathLimits{MaxBytes: 4096, MaxCommands: 512})
		if err != nil {
			if commands != nil {
				t.Fatalf("error returned partial commands: %#v", commands)
			}
			return
		}
		path := d2scene.Path{Commands: commands}
		if _, boundsErr := path.GeometryBounds(); boundsErr != nil {
			t.Fatalf("successful parse produced invalid scene path: %v", boundsErr)
		}
	})
}
