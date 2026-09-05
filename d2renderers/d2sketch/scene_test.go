package d2sketch

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	rough "github.com/d2lang/rough-go"
)

func TestCompileScenePathsPreservesStructuredRoughOperations(t *testing.T) {
	generator := newGenerator()
	drawable := generator.Rectangle(0, 0, 80, 40, baseOptions(2))
	paths, err := CompileScenePaths(context.Background(), drawable, SceneLimits{MaxSets: 10, MaxOps: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("compiled paths = %d, want fill and outline sets", len(paths))
	}
	var filled, stroked bool
	for _, path := range paths {
		if len(path.Path.Commands) == 0 {
			t.Fatal("compiled an empty rough path")
		}
		for _, command := range path.Path.Commands {
			switch command.Kind {
			case d2scene.MoveCommand, d2scene.LineCommand, d2scene.QuadraticCommand, d2scene.CubicCommand:
			default:
				t.Fatalf("unexpected typed rough verb %v", command.Kind)
			}
		}
		filled = filled || path.Fill != "none"
		stroked = stroked || path.Stroke != "none" && path.StrokeWidth == 2
	}
	if !filled || !stroked {
		t.Fatalf("compiled rough fill/stroke = %v/%v: %#v", filled, stroked, paths)
	}
}

func TestCompileScenePathsMapsEveryStructuredVerb(t *testing.T) {
	drawable := rough.Drawable{
		Options: &rough.ResolvedOptions{Stroke: "#123456", StrokeWidth: 3},
		Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{
			{Op: rough.OpMove, Data: []float64{1.123456789, 2}},
			{Op: rough.OpLineTo, Data: []float64{3, 4}},
			{Op: rough.OpQCurveTo, Data: []float64{5, 6, 7, 8}},
			{Op: rough.OpBCurveTo, Data: []float64{9, 10, 11, 12, 13, 14}},
		}}},
	}
	paths, err := CompileScenePaths(context.Background(), drawable, SceneLimits{MaxSets: 1, MaxOps: 4})
	if err != nil {
		t.Fatal(err)
	}
	commands := paths[0].Path.Commands
	if len(commands) != 4 || commands[0].Kind != d2scene.MoveCommand || commands[1].Kind != d2scene.LineCommand ||
		commands[2].Kind != d2scene.QuadraticCommand || commands[3].Kind != d2scene.CubicCommand {
		t.Fatalf("compiled commands = %#v", commands)
	}
	if commands[0].P1.X != 1.123456 {
		t.Fatalf("rough coordinate = %.9f, want six-place truncation", commands[0].P1.X)
	}
}

func TestCompileScenePathsRejectsUnboundedOrSVGOnlyWork(t *testing.T) {
	options := &rough.ResolvedOptions{Stroke: "black", StrokeWidth: 1}
	tests := []struct {
		name     string
		drawable rough.Drawable
		limits   SceneLimits
		want     string
	}{
		{name: "zero limits", drawable: rough.Drawable{Options: options}, want: "limits must be positive"},
		{name: "sets", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{}, {}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "set count"},
		{name: "ops", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{{Op: rough.OpMove, Data: []float64{0, 0}}, {Op: rough.OpLineTo, Data: []float64{1, 1}}}}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "operation count"},
		{name: "path2d", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{Type: rough.OpSetPath2DFill, Path: "M0 0"}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "path2D"},
		{name: "arity", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{{Op: rough.OpMove, Data: []float64{0}}}}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "want 2"},
		{name: "nonfinite", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{{Op: rough.OpMove, Data: []float64{math.NaN(), 0}}}}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "not finite"},
		{name: "overflowing normalization", drawable: rough.Drawable{Options: options, Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{{Op: rough.OpMove, Data: []float64{math.MaxFloat64, 0}}}}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "too large"},
		{name: "fixed decimals", drawable: rough.Drawable{Options: &rough.ResolvedOptions{FixedDecimalPlaceDigits: rough.Float64(2)}, Sets: []rough.OpSet{{Type: rough.OpSetPath, Ops: []rough.Op{{Op: rough.OpMove, Data: []float64{0, 0}}}}}}, limits: SceneLimits{MaxSets: 1, MaxOps: 1}, want: "fixed-decimal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileScenePaths(context.Background(), test.drawable, test.limits)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileScenePaths() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileScenePathsIgnoresGeneratedEmptyPaintSets(t *testing.T) {
	options := &rough.ResolvedOptions{Stroke: "black", StrokeWidth: 1}
	drawable := rough.Drawable{
		Options: options,
		Sets: []rough.OpSet{
			{Type: rough.OpSetFillSketch},
			{Type: rough.OpSetPath, Ops: []rough.Op{
				{Op: rough.OpMove, Data: []float64{0, 0}},
				{Op: rough.OpLineTo, Data: []float64{1, 1}},
			}},
		},
	}
	paths, err := CompileScenePaths(context.Background(), drawable, SceneLimits{MaxSets: 2, MaxOps: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(paths[0].Path.Commands) != 2 {
		t.Fatalf("compiled paths = %#v, want one non-empty path", paths)
	}

	drawable.Sets = drawable.Sets[:1]
	paths, err = CompileScenePaths(context.Background(), drawable, SceneLimits{MaxSets: 1, MaxOps: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("all-empty drawable paths = %#v, want none", paths)
	}
}

func TestCompileScenePathsHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CompileScenePaths(ctx, rough.Drawable{Options: &rough.ResolvedOptions{}}, SceneLimits{MaxSets: 1, MaxOps: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CompileScenePaths() error = %v, want context.Canceled", err)
	}
}

func TestConnectionDrawableUsesTypedDeterministicBoundedGeometry(t *testing.T) {
	path := d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(0, 0),
		d2scene.LineTo(10, 0),
		d2scene.CubicTo(20, 0, 20, 20, 30, 20),
	}}
	first, err := ConnectionDrawable(context.Background(), path, SceneLimits{MaxSets: 1, MaxOps: 36})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ConnectionDrawable(context.Background(), path, SceneLimits{MaxSets: 1, MaxOps: 36})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical typed connection paths produced different seeded rough operations")
	}
	paths, err := CompileScenePaths(context.Background(), first, SceneLimits{MaxSets: 1, MaxOps: 36})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || len(paths[0].Path.Commands) != 36 {
		t.Fatalf("compiled connection geometry = %d paths/%d commands, want 1/36", len(paths), len(paths[0].Path.Commands))
	}
	if _, err := ConnectionDrawable(context.Background(), path, SceneLimits{MaxSets: 1, MaxOps: 35}); err == nil || !strings.Contains(err.Error(), "operation limit 35") {
		t.Fatalf("ConnectionDrawable() error = %v, want pre-generation limit error", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConnectionDrawable(ctx, path, SceneLimits{MaxSets: 1, MaxOps: 36}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ConnectionDrawable() error = %v, want context.Canceled", err)
	}
}

func TestConnectionDrawableBoundsSetsAndCancelsLongRoutes(t *testing.T) {
	commands := make([]d2scene.PathCommand, 1, 601)
	commands[0] = d2scene.MoveTo(0, 0)
	for index := 1; index <= 600; index++ {
		commands = append(commands, d2scene.LineTo(float64(index), float64(index%2)))
	}
	path := d2scene.Path{Commands: commands}
	if _, err := ConnectionDrawable(context.Background(), path, SceneLimits{MaxSets: 2, MaxOps: 2_400}); err == nil || !strings.Contains(err.Error(), "operation set limit 2") {
		t.Fatalf("ConnectionDrawable() set error = %v", err)
	}
	ctx := newCancelAfterSceneChecksContext(4)
	if _, err := ConnectionDrawable(ctx, path, SceneLimits{MaxSets: 3, MaxOps: 2_400}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConnectionDrawable() mid-route cancellation = %v, want context.Canceled", err)
	}
}

func TestArrowheadDrawablesCoverEveryArrowhead(t *testing.T) {
	arrowheads := []d2target.Arrowhead{
		d2target.ArrowArrowhead,
		d2target.UnfilledTriangleArrowhead,
		d2target.TriangleArrowhead,
		d2target.LineArrowhead,
		d2target.DiamondArrowhead,
		d2target.FilledDiamondArrowhead,
		d2target.CircleArrowhead,
		d2target.FilledCircleArrowhead,
		d2target.CrossArrowhead,
		d2target.BoxArrowhead,
		d2target.FilledBoxArrowhead,
		d2target.CfOne,
		d2target.CfMany,
		d2target.CfOneRequired,
		d2target.CfManyRequired,
	}
	for _, arrowhead := range arrowheads {
		t.Run(string(arrowhead), func(t *testing.T) {
			first, ok := ArrowheadDrawables(arrowhead, "N1", 2)
			if !ok || len(first) == 0 {
				t.Fatalf("ArrowheadDrawables(%q) = %d/%v", arrowhead, len(first), ok)
			}
			second, ok := ArrowheadDrawables(arrowhead, "N1", 2)
			if !ok || !reflect.DeepEqual(first, second) {
				t.Fatalf("ArrowheadDrawables(%q) is not deterministic", arrowhead)
			}
			for index, drawable := range first {
				paths, err := CompileScenePaths(context.Background(), drawable, SceneLimits{MaxSets: 16, MaxOps: 10_000})
				if err != nil {
					t.Fatalf("drawable %d: %v", index, err)
				}
				if len(paths) == 0 {
					t.Fatalf("drawable %d compiled no paths", index)
				}
			}
		})
	}
	if drawables, ok := ArrowheadDrawables(d2target.NoArrowhead, "N1", 2); ok || drawables != nil {
		t.Fatalf("no-arrow drawables = %#v/%v, want nil/false", drawables, ok)
	}
}

type cancelAfterSceneChecksContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func newCancelAfterSceneChecksContext(remaining int) *cancelAfterSceneChecksContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelAfterSceneChecksContext{Context: ctx, cancel: cancel, remaining: remaining}
}

func (c *cancelAfterSceneChecksContext) Err() error {
	if c.remaining == 0 {
		c.cancel()
	} else {
		c.remaining--
	}
	return c.Context.Err()
}
