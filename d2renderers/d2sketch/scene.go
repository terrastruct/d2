package d2sketch

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
	rough "github.com/d2lang/rough-go"
)

// ScenePath is the renderer-neutral form of one ordered rough-go operation
// set. Paint remains a string so callers can resolve D2 theme colors before
// constructing scene paint.
type ScenePath struct {
	Path        d2scene.Path
	Fill        string
	Stroke      string
	StrokeWidth float64
	Dash        []float64
	DashOffset  float64
}

// SceneLimits bounds conversion work after rough-go has generated a drawable.
// D2 callers must additionally charge the generated commands against their
// document-wide scene/path budget.
type SceneLimits struct {
	MaxSets int
	MaxOps  int
}

// CompileScenePaths translates the rough-go operation types emitted by D2 into
// typed scene paths. It rejects path2D operation sets.
func CompileScenePaths(ctx context.Context, drawable rough.Drawable, limits SceneLimits) ([]ScenePath, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits.MaxSets <= 0 || limits.MaxOps <= 0 {
		return nil, fmt.Errorf("d2sketch: scene conversion limits must be positive")
	}
	if len(drawable.Sets) > limits.MaxSets {
		return nil, fmt.Errorf("d2sketch: rough operation set count %d exceeds limit %d", len(drawable.Sets), limits.MaxSets)
	}
	totalOps := 0
	for index, set := range drawable.Sets {
		if len(set.Ops) > limits.MaxOps-totalOps {
			return nil, fmt.Errorf("d2sketch: rough operation count exceeds limit %d at set %d", limits.MaxOps, index)
		}
		totalOps += len(set.Ops)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options := drawable.Options
	if options == nil {
		return nil, fmt.Errorf("d2sketch: rough drawable has no resolved options")
	}
	if options.FixedDecimalPlaceDigits != nil {
		return nil, fmt.Errorf("d2sketch: fixed-decimal rough output is unsupported in typed scene conversion")
	}
	paths := make([]ScenePath, 0, len(drawable.Sets))
	processed := 0
	for setIndex, set := range drawable.Sets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch set.Type {
		case rough.OpSetPath, rough.OpSetFillPath, rough.OpSetFillSketch:
		case rough.OpSetPath2DFill, rough.OpSetPath2DPattern:
			return nil, fmt.Errorf("d2sketch: SVG path2D operation type %q is unsupported in typed scene conversion", set.Type)
		default:
			return nil, fmt.Errorf("d2sketch: unknown rough operation type %q", set.Type)
		}
		// Empty fill sets have no drawable segment, such as a small filled-diamond
		// arrowhead at a wide stroke. Skip them without creating an invalid scene
		// path; the limits above still account for the complete drawable.
		if len(set.Ops) == 0 {
			continue
		}
		path, err := compileRoughOperationSet(ctx, set, &processed)
		if err != nil {
			return nil, fmt.Errorf("d2sketch: rough operation set %d: %w", setIndex, err)
		}
		scenePath := ScenePath{Path: path}
		switch set.Type {
		case rough.OpSetPath:
			scenePath.Fill = "none"
			scenePath.Stroke = noneIfEmpty(options.Stroke)
			scenePath.StrokeWidth = options.StrokeWidth
			scenePath.Dash = append([]float64(nil), options.StrokeLineDash...)
			scenePath.DashOffset = options.StrokeLineDashOffset
		case rough.OpSetFillPath:
			scenePath.Fill = noneIfEmpty(options.Fill)
			scenePath.Stroke = "none"
		case rough.OpSetFillSketch:
			scenePath.Fill = "none"
			scenePath.Stroke = noneIfEmpty(options.Fill)
			scenePath.StrokeWidth = options.FillWeight
			if scenePath.StrokeWidth < 0 {
				scenePath.StrokeWidth = options.StrokeWidth / 2
			}
			scenePath.Dash = append([]float64(nil), options.FillLineDash...)
			scenePath.DashOffset = options.FillLineDashOffset
		}
		if err := validateScenePathStyle(scenePath); err != nil {
			return nil, fmt.Errorf("d2sketch: rough operation set %d: %w", setIndex, err)
		}
		paths = append(paths, scenePath)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

func compileRoughOperationSet(ctx context.Context, set rough.OpSet, processed *int) (d2scene.Path, error) {
	commands := make([]d2scene.PathCommand, 0, len(set.Ops))
	for operationIndex, operation := range set.Ops {
		if *processed&255 == 0 {
			if err := ctx.Err(); err != nil {
				return d2scene.Path{}, err
			}
		}
		(*processed)++
		data, err := finiteRoughData(operation.Data)
		if err != nil {
			return d2scene.Path{}, fmt.Errorf("operation %d %q: %w", operationIndex, operation.Op, err)
		}
		switch operation.Op {
		case rough.OpMove:
			if len(data) != 2 {
				return d2scene.Path{}, roughArityError(operationIndex, operation.Op, len(data), 2)
			}
			commands = append(commands, d2scene.MoveTo(data[0], data[1]))
		case rough.OpLineTo:
			if len(data) != 2 {
				return d2scene.Path{}, roughArityError(operationIndex, operation.Op, len(data), 2)
			}
			commands = append(commands, d2scene.LineTo(data[0], data[1]))
		case rough.OpBCurveTo:
			if len(data) != 6 {
				return d2scene.Path{}, roughArityError(operationIndex, operation.Op, len(data), 6)
			}
			commands = append(commands, d2scene.CubicTo(data[0], data[1], data[2], data[3], data[4], data[5]))
		case rough.OpQCurveTo:
			if len(data) != 4 {
				return d2scene.Path{}, roughArityError(operationIndex, operation.Op, len(data), 4)
			}
			commands = append(commands, d2scene.QuadraticTo(data[0], data[1], data[2], data[3]))
		default:
			return d2scene.Path{}, fmt.Errorf("operation %d has unsupported verb %q", operationIndex, operation.Op)
		}
	}
	if len(commands) == 0 {
		return d2scene.Path{}, fmt.Errorf("operation set is empty")
	}
	return d2scene.Path{Commands: commands}, nil
}

func finiteRoughData(values []float64) ([]float64, error) {
	result := make([]float64, len(values))
	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("coordinate %d is not finite", index)
		}
		// Normalize generated coordinates to six decimal places without
		// serializing and reparsing a path string.
		scaled := value * 1_000_000
		if math.IsInf(scaled, 0) {
			return nil, fmt.Errorf("coordinate %d is too large for six-place normalization", index)
		}
		result[index] = math.Trunc(scaled) / 1_000_000
	}
	return result, nil
}

func roughArityError(index int, operation rough.OpType, got, want int) error {
	return fmt.Errorf("operation %d %q has %d coordinates, want %d", index, operation, got, want)
}

func noneIfEmpty(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func validateScenePathStyle(path ScenePath) error {
	if math.IsNaN(path.StrokeWidth) || math.IsInf(path.StrokeWidth, 0) || path.StrokeWidth < 0 {
		return fmt.Errorf("stroke width must be finite and non-negative")
	}
	if math.IsNaN(path.DashOffset) || math.IsInf(path.DashOffset, 0) {
		return fmt.Errorf("dash offset must be finite")
	}
	for index, dash := range path.Dash {
		if math.IsNaN(dash) || math.IsInf(dash, 0) || dash < 0 {
			return fmt.Errorf("dash %d must be finite and non-negative", index)
		}
	}
	return nil
}
