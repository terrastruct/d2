package d2scenebuild

import (
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2sketch"
	rough "github.com/d2lang/rough-go"
)

func validateSketchBudget(budget SketchBudget) error {
	if budget.MaxOperationSets <= 0 || budget.MaxOperations <= 0 || budget.MaxPathCommands <= 0 {
		return invalidField("options", "sketchBudget", budget, "must provide positive MaxOperationSets, MaxOperations, and MaxPathCommands")
	}
	return nil
}

// compileSketchDrawable is the single accounting boundary between rough-go's
// structured intermediate operations and retained scene paths. It deliberately
// accepts a Drawable rather than serialized SVG so no rough output can enter
// the raster renderer through a path parser.
func (b *builder) compileSketchDrawable(object string, drawable rough.Drawable) ([]d2sketch.ScenePath, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	budget := b.options.SketchBudget
	remainingSets := budget.MaxOperationSets - b.sketchOperationSets
	remainingOperations := budget.MaxOperations - b.sketchOperations
	remainingCommands := budget.MaxPathCommands - b.sketchPathCommands
	if remainingSets <= 0 {
		return nil, fmt.Errorf("scene: %s sketch operation set count exceeds limit %d", object, budget.MaxOperationSets)
	}
	if remainingOperations <= 0 {
		return nil, fmt.Errorf("scene: %s sketch operation count exceeds limit %d", object, budget.MaxOperations)
	}
	if remainingCommands <= 0 {
		return nil, fmt.Errorf("scene: %s sketch path command count exceeds limit %d", object, budget.MaxPathCommands)
	}

	paths, err := d2sketch.CompileScenePaths(b.ctx, drawable, d2sketch.SceneLimits{
		MaxSets: remainingSets,
		MaxOps:  remainingOperations,
	})
	if err != nil {
		return nil, fmt.Errorf("scene: %s sketch geometry: %w", object, err)
	}
	commands := 0
	for _, path := range paths {
		commands += len(path.Path.Commands)
	}
	if commands > remainingCommands {
		return nil, fmt.Errorf("scene: %s sketch path command count exceeds limit %d", object, budget.MaxPathCommands)
	}
	operations := 0
	for _, set := range drawable.Sets {
		operations += len(set.Ops)
	}
	b.sketchOperationSets += len(drawable.Sets)
	b.sketchOperations += operations
	if err := b.chargeSketchPathCommands(object, commands); err != nil {
		return nil, err
	}
	return paths, nil
}

func (b *builder) chargeSketchPathCommands(object string, count int) error {
	budget := b.options.SketchBudget.MaxPathCommands
	if count < 0 || count > budget-b.sketchPathCommands {
		return fmt.Errorf("scene: %s sketch path command count exceeds limit %d", object, budget)
	}
	b.sketchPathCommands += count
	return nil
}

// remainingSketchOperations lets geometry producers reject input expansion
// before invoking rough-go. It is intentionally capped by the retained-command
// budget because every supported structured operation becomes one scene path
// command.
func (b *builder) remainingSketchOperations() int {
	operations := b.options.SketchBudget.MaxOperations - b.sketchOperations
	commands := b.options.SketchBudget.MaxPathCommands - b.sketchPathCommands
	if commands < operations {
		return commands
	}
	return operations
}

func (b *builder) remainingSketchSceneLimits() d2sketch.SceneLimits {
	return d2sketch.SceneLimits{
		MaxSets: b.options.SketchBudget.MaxOperationSets - b.sketchOperationSets,
		MaxOps:  b.remainingSketchOperations(),
	}
}

func (b *builder) sketchScenePath(object string, source d2sketch.ScenePath) (d2scene.Path, error) {
	fill, err := b.paint(source.Fill, object+" fill")
	if err != nil {
		return d2scene.Path{}, err
	}
	strokePaint, err := b.paint(source.Stroke, object+" stroke")
	if err != nil {
		return d2scene.Path{}, err
	}
	path := source.Path
	path.Fill = fill
	if strokePaint != nil && source.StrokeWidth > 0 {
		path.Stroke = &d2scene.Stroke{
			Paint:      strokePaint,
			Width:      source.StrokeWidth,
			Cap:        d2scene.CapRound,
			Join:       d2scene.JoinRound,
			Dashes:     append([]float64(nil), source.Dash...),
			DashOffset: source.DashOffset,
		}
	}
	return path, nil
}
