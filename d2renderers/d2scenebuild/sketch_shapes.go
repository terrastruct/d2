package d2scenebuild

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	libcolor "github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
	"github.com/d2lang/d2/lib/svg"
	rough "github.com/d2lang/rough-go"
)

const (
	sketchCubicFlatness             = 0.35
	sketchCubicMaxDepth             = 8
	sketchStreakSubpathCount        = 85
	sketchStreakCompoundSubpath     = 16
	sketchStreakCompoundSubpathSpan = 2
)

func newSketchGenerator() *rough.Generator {
	return rough.NewGenerator(&rough.Config{Options: &rough.Options{Seed: rough.Float64(1)}})
}

func sketchShapeOptions(strokeWidth float64, dashes []float64, dashOffset float64) *rough.Options {
	return &rough.Options{
		Fill:                 rough.String("#000"),
		Stroke:               rough.String("#000"),
		StrokeWidth:          rough.Float64(strokeWidth),
		FillWeight:           rough.Float64(2),
		HachureGap:           rough.Float64(16),
		FillStyle:            rough.String("solid"),
		Bowing:               rough.Float64(2),
		Seed:                 rough.Float64(1),
		StrokeLineDash:       append([]float64(nil), dashes...),
		StrokeLineDashOffset: rough.Float64(dashOffset),
	}
}

// compileSketchShapeDrawable applies the D2 fill and stroke to every emitted
// rough path instead of rough-go's per-set paint. Geometry remains structured,
// and the shared paint keeps filled outlines intact.
func (b *builder) compileSketchShapeDrawable(
	object, idPrefix, fillRaw, strokeRaw string,
	strokeWidth float64,
	dashes []float64,
	dashOffset float64,
	drawable rough.Drawable,
	transform d2scene.Matrix,
	targetShape d2target.Shape,
) ([]*d2scene.Node, error) {
	paths, err := b.compileSketchDrawable(object, drawable)
	if err != nil {
		return nil, err
	}
	nodes := make([]*d2scene.Node, 0, len(paths))
	for index, source := range paths {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		source.Fill = noneIfSketchEmpty(fillRaw)
		source.Stroke = noneIfSketchEmpty(strokeRaw)
		source.StrokeWidth = strokeWidth
		source.Dash = append([]float64(nil), dashes...)
		source.DashOffset = dashOffset
		path, err := b.sketchScenePath(object, source)
		if err != nil {
			return nil, err
		}
		if path.Fill == nil && path.Stroke == nil {
			continue
		}
		node := d2scene.NewNode(path)
		node.ID = fmt.Sprintf("%s:set:%d", idPrefix, index)
		node.Transform = transform
		b.maskSketchShapeNode(node, targetShape)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func noneIfSketchEmpty(value string) string {
	if value == "" {
		return libcolor.None
	}
	return value
}

func (b *builder) maskSketchShapeNode(node *d2scene.Node, targetShape d2target.Shape) {
	if node == nil || b.connectionMask == nil || targetShape.Label == "" || !label.FromString(targetShape.LabelPosition).IsBorder() {
		return
	}
	node.Mask = b.connectionMask
}

func (b *builder) buildSketchOrdinaryShape(targetShape d2target.Shape, idPrefix string) ([]*d2scene.Node, error) {
	if targetShape.Type == d2target.ShapeText {
		return nil, nil
	}
	if idPrefix == "" {
		idPrefix = targetShape.ID + ":sketch"
	}
	fillRaw, strokeRaw := d2themes.ShapeTheme(targetShape)
	stroke, err := b.stroke(strokeRaw, targetShape.StrokeWidth, targetShape.StrokeDash, d2scene.CapRound, d2scene.JoinRound, fmt.Sprintf("shape %q sketch stroke", targetShape.ID))
	if err != nil {
		return nil, err
	}
	strokeWidth, dashes, dashOffset := sketchStrokeValues(stroke)
	options := sketchShapeOptions(strokeWidth, dashes, dashOffset)
	object := fmt.Sprintf("shape %q", targetShape.ID)

	var nodes []*d2scene.Node
	switch targetShape.Type {
	case d2target.ShapeOval, d2target.ShapeCircle:
		drawable := newSketchGenerator().Ellipse(
			float64(targetShape.Width)/2, float64(targetShape.Height)/2,
			float64(targetShape.Width), float64(targetShape.Height), options,
		)
		nodes, err = b.compileSketchShapeDrawable(
			object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset,
			drawable, d2scene.Translate(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)), targetShape,
		)
	case d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeSequenceDiagram, d2target.ShapeSequenceDiagramV2, d2target.ShapeSequenceDiagramEdgeGroup, d2target.ShapeSequenceDiagramActorGroup, d2target.ShapeSequenceDiagramActor, d2target.ShapeHierarchy, "":
		drawable := newSketchGenerator().Rectangle(0, 0, float64(targetShape.Width), float64(targetShape.Height), options)
		nodes, err = b.compileSketchShapeDrawable(
			object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset,
			drawable, d2scene.Translate(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)), targetShape,
		)
	default:
		nodes, err = b.buildSketchTypedShapePaths(targetShape, object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset, options)
	}
	if err != nil {
		return nil, err
	}
	roughNodes := nodes
	nodes, err = b.appendSketchBuiltinPattern(targetShape, nodes, idPrefix)
	if err != nil {
		return nil, err
	}

	overlayPrimitives := make([]d2scene.Primitive, 0, len(nodes))
	switch targetShape.Type {
	case d2target.ShapeOval, d2target.ShapeCircle:
		overlayPrimitives = append(overlayPrimitives, d2scene.Ellipse{
			Center: d2scene.Point{
				X: float64(targetShape.Pos.X) + float64(targetShape.Width)/2,
				Y: float64(targetShape.Pos.Y) + float64(targetShape.Height)/2,
			},
			RadiusX: float64(targetShape.Width) / 2,
			RadiusY: float64(targetShape.Height) / 2,
		})
	case d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeSequenceDiagram, d2target.ShapeSequenceDiagramV2, d2target.ShapeSequenceDiagramEdgeGroup, d2target.ShapeSequenceDiagramActorGroup, d2target.ShapeSequenceDiagramActor, d2target.ShapeHierarchy, "":
		overlayPrimitives = append(overlayPrimitives, d2scene.Rect{Box: sketchShapeBox(targetShape)})
	default:
		for _, node := range roughNodes {
			if path, ok := sketchNodePath(node); ok {
				overlayPrimitives = append(overlayPrimitives, path)
			}
		}
	}
	overlays, err := b.buildSketchStreakOverlays(targetShape, fillRaw, idPrefix, overlayPrimitives)
	if err != nil {
		return nil, err
	}
	return append(nodes, overlays...), nil
}

func sketchStrokeValues(stroke *d2scene.Stroke) (float64, []float64, float64) {
	if stroke == nil {
		return 0, nil, 0
	}
	return stroke.Width, append([]float64(nil), stroke.Dashes...), stroke.DashOffset
}

func sketchShapeBox(targetShape d2target.Shape) d2scene.Box {
	return d2scene.Box{
		X: float64(targetShape.Pos.X), Y: float64(targetShape.Pos.Y),
		Width: float64(targetShape.Width), Height: float64(targetShape.Height),
	}
}

func sketchNodePath(node *d2scene.Node) (d2scene.Path, bool) {
	if node == nil {
		return d2scene.Path{}, false
	}
	switch path := node.Primitive.(type) {
	case d2scene.Path:
		path.Fill, path.Stroke = nil, nil
		return path, true
	case *d2scene.Path:
		if path == nil {
			return d2scene.Path{}, false
		}
		copy := *path
		copy.Fill, copy.Stroke = nil, nil
		return copy, true
	default:
		return d2scene.Path{}, false
	}
}

func (b *builder) appendSketchBuiltinPattern(targetShape d2target.Shape, nodes []*d2scene.Node, idPrefix string) ([]*d2scene.Node, error) {
	pattern, err := b.builtinPattern(targetShape.FillPattern)
	if err != nil {
		return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
	}
	if pattern == nil {
		return nodes, nil
	}
	output := make([]*d2scene.Node, 0, len(nodes)*2)
	for index, node := range nodes {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		output = append(output, node)
		path, ok := sketchNodePath(node)
		if !ok {
			continue
		}
		if err := b.chargeSketchPathCommands(
			fmt.Sprintf("shape %q sketch fill-pattern overlay", targetShape.ID), len(path.Commands),
		); err != nil {
			return nil, err
		}
		overlay, err := overlayPatternNode(node, pattern, fmt.Sprintf("%s:fill-pattern:%d", idPrefix, index))
		if err != nil {
			return nil, err
		}
		output = append(output, overlay)
	}
	return output, nil
}

func (b *builder) buildSketchTypedShapePaths(
	targetShape d2target.Shape,
	object, idPrefix, fillRaw, strokeRaw string,
	strokeWidth float64,
	dashes []float64,
	dashOffset float64,
	options *rough.Options,
) ([]*d2scene.Node, error) {
	geometry := targetGeometry(targetShape)
	paths := shape.GetPathCommands(geometry)
	if len(paths) == 0 {
		return nil, unsupported(object, "typed sketch geometry for "+targetShape.Type)
	}
	remaining := b.remainingSketchOperations()
	if remaining <= 0 {
		return nil, fmt.Errorf("scene: %s sketch typed-path expansion exceeds remaining operation budget", object)
	}
	// A rough polygon can retain a solid-fill operation per vertex plus two
	// multi-stroke operations per edge. Eight operations per flattened point,
	// with a small fixed reserve, is deliberately conservative and rejects
	// before rough-go allocates an over-budget drawable.
	pointLimit := (remaining - 32) / 8
	if pointLimit <= 1 {
		return nil, fmt.Errorf("scene: %s sketch typed-path expansion exceeds remaining operation budget", object)
	}

	flattened := make([]flattenedSketchPath, 0, len(paths))
	totalPoints := 0
	for pathIndex, commands := range paths {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		parts, err := flattenSketchPath(b.ctx, commands, pointLimit-totalPoints)
		if err != nil {
			return nil, fmt.Errorf("scene: %s typed sketch path %d: %w", object, pathIndex, err)
		}
		for _, part := range parts {
			totalPoints += len(part.points)
			if totalPoints > pointLimit {
				return nil, fmt.Errorf("scene: %s sketch typed-path expansion exceeds remaining operation budget", object)
			}
			flattened = append(flattened, part)
		}
	}

	var nodes []*d2scene.Node
	for pathIndex, path := range flattened {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if len(path.points) < 2 {
			return nil, fmt.Errorf("scene: %s typed sketch path %d has fewer than two points", object, pathIndex)
		}
		generator := newSketchGenerator()
		var drawable rough.Drawable
		if path.closed {
			drawable = generator.Polygon(path.points, options)
		} else {
			drawable = generator.LinearPath(path.points, options)
		}
		compiled, err := b.compileSketchShapeDrawable(
			fmt.Sprintf("%s typed path %d", object, pathIndex), fmt.Sprintf("%s:path:%d", idPrefix, pathIndex),
			fillRaw, strokeRaw, strokeWidth, dashes, dashOffset, drawable, d2scene.Identity(), targetShape,
		)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, compiled...)
	}
	return nodes, nil
}

type flattenedSketchPath struct {
	points []rough.Point
	closed bool
}

func flattenSketchPath(ctx context.Context, commands []svg.PathCommand, maxPoints int) ([]flattenedSketchPath, error) {
	if maxPoints <= 1 {
		return nil, fmt.Errorf("flattened point limit must exceed one")
	}
	var output []flattenedSketchPath
	var current flattenedSketchPath
	var currentPoint, startPoint rough.Point
	haveCurrent := false
	pointCount := 0

	appendPoint := func(point rough.Point) error {
		if !finite(point[0]) || !finite(point[1]) {
			return fmt.Errorf("non-finite typed point")
		}
		if len(current.points) != 0 && current.points[len(current.points)-1] == point {
			currentPoint = point
			return nil
		}
		if pointCount >= maxPoints {
			return fmt.Errorf("flattened point count exceeds limit %d", maxPoints)
		}
		current.points = append(current.points, point)
		pointCount++
		currentPoint = point
		return nil
	}
	flush := func() {
		if len(current.points) != 0 {
			output = append(output, current)
		}
		current = flattenedSketchPath{}
		haveCurrent = false
	}

	for commandIndex, command := range commands {
		if commandIndex&63 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		switch command.Kind {
		case svg.PathCommandMove:
			flush()
			startPoint = rough.Point{command.End.X, command.End.Y}
			if err := appendPoint(startPoint); err != nil {
				return nil, fmt.Errorf("command %d: %w", commandIndex, err)
			}
			haveCurrent = true
		case svg.PathCommandLine:
			if !haveCurrent {
				return nil, fmt.Errorf("command %d line has no current point", commandIndex)
			}
			if err := appendPoint(rough.Point{command.End.X, command.End.Y}); err != nil {
				return nil, fmt.Errorf("command %d: %w", commandIndex, err)
			}
		case svg.PathCommandCubic:
			if !haveCurrent {
				return nil, fmt.Errorf("command %d cubic has no current point", commandIndex)
			}
			points, err := flattenSketchCubic(
				ctx, currentPoint,
				rough.Point{command.Control1.X, command.Control1.Y},
				rough.Point{command.Control2.X, command.Control2.Y},
				rough.Point{command.End.X, command.End.Y},
				maxPoints-pointCount,
			)
			if err != nil {
				return nil, fmt.Errorf("command %d: %w", commandIndex, err)
			}
			for _, point := range points {
				if err := appendPoint(point); err != nil {
					return nil, fmt.Errorf("command %d: %w", commandIndex, err)
				}
			}
		case svg.PathCommandClose:
			if !haveCurrent {
				return nil, fmt.Errorf("command %d close has no current point", commandIndex)
			}
			current.closed = true
			currentPoint = startPoint
		default:
			return nil, fmt.Errorf("command %d has unsupported kind %d", commandIndex, command.Kind)
		}
	}
	flush()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(output) == 0 {
		return nil, fmt.Errorf("typed path is empty")
	}
	return output, nil
}

func flattenSketchCubic(ctx context.Context, p0, p1, p2, p3 rough.Point, maxPoints int) ([]rough.Point, error) {
	if maxPoints <= 0 {
		return nil, fmt.Errorf("flattened point count exceeds limit")
	}
	points := make([]rough.Point, 0, 16)
	var split func(rough.Point, rough.Point, rough.Point, rough.Point, int) error
	split = func(a, b, c, d rough.Point, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth >= sketchCubicMaxDepth || cubicSketchFlatEnough(a, b, c, d) {
			if len(points) >= maxPoints {
				return fmt.Errorf("flattened point count exceeds limit")
			}
			points = append(points, d)
			return nil
		}
		ab, bc, cd := roughMidpoint(a, b), roughMidpoint(b, c), roughMidpoint(c, d)
		abc, bcd := roughMidpoint(ab, bc), roughMidpoint(bc, cd)
		mid := roughMidpoint(abc, bcd)
		if err := split(a, ab, abc, mid, depth+1); err != nil {
			return err
		}
		return split(mid, bcd, cd, d, depth+1)
	}
	if err := split(p0, p1, p2, p3, 0); err != nil {
		return nil, err
	}
	return points, nil
}

func roughMidpoint(a, b rough.Point) rough.Point {
	return rough.Point{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
}

func cubicSketchFlatEnough(p0, p1, p2, p3 rough.Point) bool {
	toleranceSquared := sketchCubicFlatness * sketchCubicFlatness
	return sketchPointLineDistanceSquared(p1, p0, p3) <= toleranceSquared &&
		sketchPointLineDistanceSquared(p2, p0, p3) <= toleranceSquared
}

func sketchPointLineDistanceSquared(point, start, end rough.Point) float64 {
	dx, dy := end[0]-start[0], end[1]-start[1]
	denominator := dx*dx + dy*dy
	if denominator == 0 {
		x, y := point[0]-start[0], point[1]-start[1]
		return x*x + y*y
	}
	numerator := dy*point[0] - dx*point[1] + end[0]*start[1] - end[1]*start[0]
	return numerator * numerator / denominator
}

func (b *builder) buildSketchShapeEffects(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	if targetShape.ThreeDee {
		// SVG sketch leaves 3D faces precise; only ordinary outlines are
		// roughened. Keep its main-face fill-pattern behavior and omit streaks.
		nodes, err := b.buildShapeEffects(targetShape, fill, stroke)
		if err != nil {
			return nil, err
		}
		pattern, err := b.builtinPattern(targetShape.FillPattern)
		if err != nil {
			return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
		}
		return b.interleavePattern(nodes, pattern, targetShape.ID, effectPatternNode(targetShape))
	}
	if targetShape.DoubleBorder {
		return b.buildSketchDoubleBorderShape(targetShape, fill, stroke)
	}
	if targetShape.Multiple {
		multiple := targetShape
		multiple.Pos.X += d2target.MULTIPLE_OFFSET
		multiple.Pos.Y -= d2target.MULTIPLE_OFFSET
		duplicate, err := b.buildOrdinaryShapeOutline(multiple, fill, stroke, targetShape.ID+":multiple")
		if err != nil {
			return nil, err
		}
		main, err := b.buildSketchOrdinaryShape(targetShape, targetShape.ID+":main:sketch")
		if err != nil {
			return nil, err
		}
		return append(duplicate, main...), nil
	}
	return nil, fmt.Errorf("scene: shape %q has no sketch shape effect", targetShape.ID)
}

func (b *builder) buildSketchDoubleBorderShape(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	var nodes []*d2scene.Node
	if targetShape.Multiple {
		multiple := targetShape
		multiple.Pos.X += d2target.MULTIPLE_OFFSET
		multiple.Pos.Y -= d2target.MULTIPLE_OFFSET
		duplicate := doubleBorderPair(multiple, fill, fill, stroke, targetShape.ID+":multiple")
		if targetShape.Type == "" || targetShape.Type == d2target.ShapeRectangle || targetShape.Type == d2target.ShapeSquare {
			pattern, err := b.builtinPattern(targetShape.FillPattern)
			if err != nil {
				return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
			}
			duplicate, err = b.interleavePattern(duplicate, pattern, targetShape.ID, func(node *d2scene.Node) bool {
				return node != nil && node.ID == targetShape.ID+":multiple:outer"
			})
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, duplicate...)
	}

	outerTarget := targetShape
	outerTarget.Multiple = false
	outer, err := b.buildSketchOutlineOnly(outerTarget, targetShape.ID+":double-border:outer:sketch")
	if err != nil {
		return nil, err
	}
	outer, err = b.appendSketchBuiltinPattern(targetShape, outer, targetShape.ID+":double-border:outer")
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, outer...)

	inner := outerTarget
	inner.Pos.X += d2target.INNER_BORDER_OFFSET
	inner.Pos.Y += d2target.INNER_BORDER_OFFSET
	inner.Width -= 2 * d2target.INNER_BORDER_OFFSET
	inner.Height -= 2 * d2target.INNER_BORDER_OFFSET
	inner.Fill = "transparent"
	innerNodes, err := b.buildSketchOutlineOnly(inner, targetShape.ID+":double-border:inner:sketch")
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, innerNodes...)

	fillRaw, _ := d2themes.ShapeTheme(targetShape)
	var primitive d2scene.Primitive
	if targetShape.Type == d2target.ShapeOval || targetShape.Type == d2target.ShapeCircle {
		box := sketchShapeBox(targetShape)
		primitive = d2scene.Ellipse{
			Center:  d2scene.Point{X: box.X + box.Width/2, Y: box.Y + box.Height/2},
			RadiusX: box.Width / 2, RadiusY: box.Height / 2,
		}
	} else {
		primitive = d2scene.Rect{Box: sketchShapeBox(targetShape)}
	}
	overlays, err := b.buildSketchStreakOverlays(targetShape, fillRaw, targetShape.ID+":double-border", []d2scene.Primitive{primitive})
	if err != nil {
		return nil, err
	}
	return append(nodes, overlays...), nil
}

func (b *builder) buildSketchOutlineOnly(targetShape d2target.Shape, idPrefix string) ([]*d2scene.Node, error) {
	fillRaw, strokeRaw := d2themes.ShapeTheme(targetShape)
	stroke, err := b.stroke(strokeRaw, targetShape.StrokeWidth, targetShape.StrokeDash, d2scene.CapRound, d2scene.JoinRound, fmt.Sprintf("shape %q sketch stroke", targetShape.ID))
	if err != nil {
		return nil, err
	}
	strokeWidth, dashes, dashOffset := sketchStrokeValues(stroke)
	options := sketchShapeOptions(strokeWidth, dashes, dashOffset)
	object := fmt.Sprintf("shape %q", targetShape.ID)
	switch targetShape.Type {
	case d2target.ShapeOval, d2target.ShapeCircle:
		return b.compileSketchShapeDrawable(
			object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset,
			newSketchGenerator().Ellipse(
				float64(targetShape.Width)/2, float64(targetShape.Height)/2,
				float64(targetShape.Width), float64(targetShape.Height), options,
			),
			d2scene.Translate(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)), targetShape,
		)
	case d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeSequenceDiagram, d2target.ShapeSequenceDiagramV2, d2target.ShapeSequenceDiagramEdgeGroup, d2target.ShapeSequenceDiagramActorGroup, d2target.ShapeSequenceDiagramActor, d2target.ShapeHierarchy, "":
		return b.compileSketchShapeDrawable(
			object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset,
			newSketchGenerator().Rectangle(0, 0, float64(targetShape.Width), float64(targetShape.Height), options),
			d2scene.Translate(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)), targetShape,
		)
	default:
		return b.buildSketchTypedShapePaths(targetShape, object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset, options)
	}
}

func (b *builder) buildSketchStructuredShape(targetShape d2target.Shape, outerFill d2scene.Paint, outerStroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	children, err := b.buildStructuredShape(targetShape, outerFill, outerStroke)
	if err != nil {
		return nil, err
	}
	box := sketchShapeBox(targetShape)
	headerHeight := structuredSketchHeaderHeight(targetShape)
	headerBox := d2scene.Box{X: box.X, Y: box.Y, Width: box.Width, Height: headerHeight}
	outerFillRaw, outerStrokeRaw := d2themes.ShapeTheme(targetShape)
	outerStrokeWidth, outerDashes, outerDashOffset := sketchStrokeValues(outerStroke)
	patternTarget := targetShape

	output := make([]*d2scene.Node, 0, len(children)+8)
	for _, child := range children {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		switch {
		case child != nil && child.ID == targetShape.ID+":outline":
			roughNodes, err := b.compileSketchRectangle(
				fmt.Sprintf("shape %q structured outline", targetShape.ID), targetShape.ID+":outline:sketch",
				box, outerFillRaw, outerStrokeRaw, outerStrokeWidth, outerDashes, outerDashOffset, targetShape,
			)
			if err != nil {
				return nil, err
			}
			roughNodes, err = b.appendSketchBuiltinPattern(patternTarget, roughNodes, targetShape.ID+":outline")
			if err != nil {
				return nil, err
			}
			output = append(output, roughNodes...)
		case child != nil && (child.ID == targetShape.ID+":class-header" || child.ID == targetShape.ID+":table-header"):
			headerPaint, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q structured sketch header", targetShape.ID))
			if err != nil {
				return nil, err
			}
			_ = headerPaint // validates the raw paint before rough generation.
			roughNodes, err := b.compileSketchRectangle(
				fmt.Sprintf("shape %q structured header", targetShape.ID), child.ID+":sketch",
				headerBox, targetShape.Fill, "", 1, nil, 0, targetShape,
			)
			if err != nil {
				return nil, err
			}
			roughNodes, err = b.appendSketchBuiltinPattern(patternTarget, roughNodes, child.ID)
			if err != nil {
				return nil, err
			}
			output = append(output, roughNodes...)
			if targetShape.Type == d2target.ShapeClass {
				overlays, err := b.buildSketchStreakOverlays(targetShape, targetShape.Fill, child.ID, []d2scene.Primitive{d2scene.Rect{Box: headerBox}})
				if err != nil {
					return nil, err
				}
				output = append(output, overlays...)
			}
		case child != nil && (strings.HasSuffix(child.ID, ":separator") || child.ID == targetShape.ID+":class-separator"):
			start, end, ok := sketchLineEndpoints(child)
			if !ok {
				return nil, fmt.Errorf("scene: shape %q structured separator %q is not a typed line", targetShape.ID, child.ID)
			}
			width := 1.0
			if targetShape.Type == d2target.ShapeSQLTable {
				width = 2
			}
			lineNodes, err := b.compileSketchLine(
				fmt.Sprintf("shape %q structured separator", targetShape.ID), child.ID+":sketch",
				start, end, targetShape.Fill, "", width, targetShape,
			)
			if err != nil {
				return nil, err
			}
			lineNodes, err = b.appendSketchBuiltinPattern(patternTarget, lineNodes, child.ID)
			if err != nil {
				return nil, err
			}
			output = append(output, lineNodes...)
		default:
			output = append(output, child)
		}
	}
	if targetShape.Type == d2target.ShapeSQLTable {
		overlays, err := b.buildSketchStreakOverlays(targetShape, targetShape.Fill, targetShape.ID+":table", []d2scene.Primitive{d2scene.Rect{Box: box}})
		if err != nil {
			return nil, err
		}
		output = append(output, overlays...)
	}
	return output, nil
}

func structuredSketchHeaderHeight(targetShape d2target.Shape) float64 {
	switch targetShape.Type {
	case d2target.ShapeClass:
		rows := 2 + len(targetShape.Fields) + len(targetShape.Methods)
		rowHeight := float64(targetShape.Height) / float64(rows)
		return math.Max(2*rowHeight, float64(targetShape.LabelHeight)+2*label.PADDING)
	case d2target.ShapeSQLTable:
		return float64(targetShape.Height) / float64(1+len(targetShape.Columns))
	default:
		return 0
	}
}

func (b *builder) compileSketchRectangle(
	object, idPrefix string,
	box d2scene.Box,
	fillRaw, strokeRaw string,
	strokeWidth float64,
	dashes []float64,
	dashOffset float64,
	targetShape d2target.Shape,
) ([]*d2scene.Node, error) {
	options := sketchShapeOptions(strokeWidth, dashes, dashOffset)
	drawable := newSketchGenerator().Rectangle(0, 0, box.Width, box.Height, options)
	return b.compileSketchShapeDrawable(
		object, idPrefix, fillRaw, strokeRaw, strokeWidth, dashes, dashOffset,
		drawable, d2scene.Translate(box.X, box.Y), targetShape,
	)
}

func (b *builder) compileSketchLine(
	object, idPrefix string,
	start, end d2scene.Point,
	fillRaw, strokeRaw string,
	strokeWidth float64,
	targetShape d2target.Shape,
) ([]*d2scene.Node, error) {
	drawable := newSketchGenerator().Line(start.X, start.Y, end.X, end.Y, sketchShapeOptions(strokeWidth, nil, 0))
	return b.compileSketchShapeDrawable(
		object, idPrefix, fillRaw, strokeRaw, strokeWidth, nil, 0,
		drawable, d2scene.Identity(), targetShape,
	)
}

func sketchLineEndpoints(node *d2scene.Node) (d2scene.Point, d2scene.Point, bool) {
	path, ok := sketchNodePath(node)
	if !ok || len(path.Commands) != 2 || path.Commands[0].Kind != d2scene.MoveCommand || path.Commands[1].Kind != d2scene.LineCommand {
		return d2scene.Point{}, d2scene.Point{}, false
	}
	return path.Commands[0].P1, path.Commands[1].P1, true
}

func (b *builder) buildSketchStreakOverlays(
	targetShape d2target.Shape,
	fillRaw, idPrefix string,
	primitives []d2scene.Primitive,
) ([]*d2scene.Node, error) {
	if len(primitives) == 0 || strings.EqualFold(fillRaw, libcolor.None) || fillRaw == "" {
		return nil, nil
	}
	paint, blend, category, err := b.sketchStreakPattern(fillRaw, fmt.Sprintf("shape %q", targetShape.ID))
	if err != nil {
		return nil, err
	}
	nodes := make([]*d2scene.Node, 0, len(primitives))
	for index, primitive := range primitives {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if commandCount := sketchPrimitivePathCommandCount(primitive); commandCount != 0 {
			if err := b.chargeSketchPathCommands(
				fmt.Sprintf("shape %q sketch streak overlay", targetShape.ID), commandCount,
			); err != nil {
				return nil, err
			}
		}
		overlayPrimitive, err := sketchOverlayPrimitive(primitive, paint)
		if err != nil {
			return nil, fmt.Errorf("scene: shape %q streak overlay %d: %w", targetShape.ID, index, err)
		}
		node := d2scene.NewNode(overlayPrimitive)
		node.ID = fmt.Sprintf("%s:streak:%d", idPrefix, index)
		node.Classes = []string{"sketch-streak-overlay", "sketch-streak-" + category}
		node.Blend = blend
		b.maskSketchShapeNode(node, targetShape)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func sketchPrimitivePathCommandCount(primitive d2scene.Primitive) int {
	switch value := primitive.(type) {
	case d2scene.Path:
		return len(value.Commands)
	case *d2scene.Path:
		if value != nil {
			return len(value.Commands)
		}
	}
	return 0
}

func sketchOverlayPrimitive(primitive d2scene.Primitive, paint d2scene.PatternPaint) (d2scene.Primitive, error) {
	switch value := primitive.(type) {
	case d2scene.Rect:
		value.Fill, value.Stroke = paint, nil
		return value, nil
	case *d2scene.Rect:
		if value == nil {
			return nil, fmt.Errorf("nil rectangle")
		}
		copy := *value
		copy.Fill, copy.Stroke = paint, nil
		return copy, nil
	case d2scene.Ellipse:
		value.Fill, value.Stroke = paint, nil
		return value, nil
	case *d2scene.Ellipse:
		if value == nil {
			return nil, fmt.Errorf("nil ellipse")
		}
		copy := *value
		copy.Fill, copy.Stroke = paint, nil
		return copy, nil
	case d2scene.Path:
		value.Commands = append([]d2scene.PathCommand(nil), value.Commands...)
		value.Fill, value.Stroke = paint, nil
		return value, nil
	case *d2scene.Path:
		if value == nil {
			return nil, fmt.Errorf("nil path")
		}
		copy := *value
		copy.Commands = append([]d2scene.PathCommand(nil), value.Commands...)
		copy.Fill, copy.Stroke = paint, nil
		return copy, nil
	default:
		return nil, fmt.Errorf("primitive %T cannot carry a sketch streak", primitive)
	}
}

func (b *builder) sketchStreakPattern(fillRaw, object string) (d2scene.PatternPaint, d2scene.BlendMode, string, error) {
	category, err := b.sketchLuminanceCategory(fillRaw)
	if err != nil {
		return d2scene.PatternPaint{}, d2scene.BlendNormal, "", fmt.Errorf("scene: %s sketch streak fill %q: %w", object, fillRaw, err)
	}
	blend, ink, err := sketchStreakStyle(category)
	if err != nil {
		return d2scene.PatternPaint{}, d2scene.BlendNormal, "", err
	}
	if pattern, ok := b.sketchStreakPatterns[category]; ok {
		return pattern, blend, category, nil
	}
	if b.sketchStreakCommands == nil {
		commands, err := sharedSketchStreakCommands(b.ctx)
		if err != nil {
			return d2scene.PatternPaint{}, d2scene.BlendNormal, "", fmt.Errorf("scene: load built-in sketch streak: %w", err)
		}
		b.sketchStreakCommands = append([]d2scene.PathCommand(nil), commands...)
	}
	if err := b.chargeSketchPathCommands(object+" sketch streak pattern", len(b.sketchStreakCommands)); err != nil {
		return d2scene.PatternPaint{}, d2scene.BlendNormal, "", err
	}
	root, err := sketchStreakRoot(b.ctx, b.sketchStreakCommands, ink, category)
	if err != nil {
		return d2scene.PatternPaint{}, d2scene.BlendNormal, "", fmt.Errorf("scene: build built-in sketch streak: %w", err)
	}
	pattern := d2scene.PatternPaint{
		Tile: d2scene.Box{Width: 100, Height: 100}, Root: root,
		Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
	}
	if b.sketchStreakPatterns == nil {
		b.sketchStreakPatterns = make(map[string]d2scene.PatternPaint, 4)
	}
	b.sketchStreakPatterns[category] = pattern
	return pattern, blend, category, nil
}

// sketchStreakRoot preserves the canonical streak path's even-odd result while
// bounding raster point-in-edge work to each local closed shape. Subpaths 16
// and 17 form the source's sole nested compound shape and must remain in one
// even-odd primitive. Every command slice otherwise aliases the builder-owned,
// once-accounted backing store.
func sketchStreakRoot(ctx context.Context, commands []d2scene.PathCommand, ink color.NRGBA, category string) (*d2scene.Node, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	starts := make([]int, 0, sketchStreakSubpathCount+1)
	for commandIndex, command := range commands {
		if commandIndex&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if command.Kind == d2scene.MoveCommand {
			starts = append(starts, commandIndex)
		}
	}
	if len(starts) != sketchStreakSubpathCount || len(starts) == 0 || starts[0] != 0 {
		return nil, fmt.Errorf("unexpected subpath topology: got %d subpaths, want %d", len(starts), sketchStreakSubpathCount)
	}
	starts = append(starts, len(commands))
	for subpath := 0; subpath < sketchStreakSubpathCount; subpath++ {
		if starts[subpath+1] <= starts[subpath]+1 || commands[starts[subpath+1]-1].Kind != d2scene.CloseCommand {
			return nil, fmt.Errorf("unexpected open or empty subpath %d", subpath)
		}
	}

	root := d2scene.NewNode(nil)
	root.ID = "builtin:sketch-streak:" + category
	for subpath := 0; subpath < sketchStreakSubpathCount; subpath++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		endSubpath := subpath + 1
		if subpath == sketchStreakCompoundSubpath {
			endSubpath = subpath + sketchStreakCompoundSubpathSpan
		}
		path := d2scene.Path{
			Commands: commands[starts[subpath]:starts[endSubpath]],
			FillRule: d2scene.EvenOdd,
			Fill:     d2scene.SolidPaint{Color: ink},
		}
		node := d2scene.NewNode(path)
		node.ID = fmt.Sprintf("%s:subpath:%d", root.ID, subpath)
		root.Children = append(root.Children, node)
		if endSubpath > subpath+1 {
			subpath = endSubpath - 1
		}
	}
	return root, ctx.Err()
}

func (b *builder) sketchLuminanceCategory(fillRaw string) (string, error) {
	if libcolor.IsGradient(fillRaw) || libcolor.IsURLGradientID(fillRaw) {
		return "normal", nil
	}
	resolved := d2themes.ResolveThemeColor(b.theme, fillRaw)
	if resolved == "" {
		return "", fmt.Errorf("unknown theme color")
	}
	return libcolor.LuminanceCategory(resolved)
}

func sketchStreakStyle(category string) (d2scene.BlendMode, color.NRGBA, error) {
	switch category {
	case "bright":
		return d2scene.BlendDarken, color.NRGBA{A: 26}, nil
	case "normal":
		return d2scene.BlendColorBurn, color.NRGBA{A: 41}, nil
	case "dark":
		return d2scene.BlendOverlay, color.NRGBA{A: 82}, nil
	case "darker":
		return d2scene.BlendLighten, color.NRGBA{R: 255, G: 255, B: 255, A: 61}, nil
	default:
		return d2scene.BlendNormal, color.NRGBA{}, fmt.Errorf("scene: invalid sketch streak luminance category %q", category)
	}
}
