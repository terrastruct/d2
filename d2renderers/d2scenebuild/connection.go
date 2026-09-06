package d2scenebuild

import (
	"fmt"
	"math"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/svg"
)

type connectionPathProvenance uint8

const (
	connectionPathLine connectionPathProvenance = iota
	connectionPathCubic
	connectionPathSmoothCubic
)

type connectionPathSegment struct {
	kind    connectionPathProvenance
	start   d2scene.Point
	command d2scene.PathCommand
}

func (b *builder) buildConnection(connection d2target.Connection) (*d2scene.Node, error) {
	strokePaint, err := b.paint(connection.Stroke, fmt.Sprintf("connection %q stroke", connection.ID))
	if err != nil {
		return nil, err
	}
	strokeDash := connection.StrokeDash
	if connection.Animated && strokeDash == 0 {
		// Preserve Connection.CSSStyle's implicit animated dash pattern.
		strokeDash = 5
	}
	stroke, err := b.stroke(connection.Stroke, connection.StrokeWidth, strokeDash, d2scene.CapRound, d2scene.JoinRound, fmt.Sprintf("connection %q stroke", connection.ID))
	if err != nil {
		return nil, err
	}
	srcAdjustment, dstAdjustment := arrowheadAdjustments(connection, b.idToShape)
	path, segments, start, end, err := connectionPath(connection, srcAdjustment, dstAdjustment)
	if err != nil {
		return nil, fmt.Errorf("scene: connection %q: %w", connection.ID, err)
	}
	path.Stroke = stroke
	var forwardTrack, reverseTrack *d2scene.Track
	if connection.Animated && stroke != nil {
		baseOffset, duration, err := animatedConnectionTiming(connection)
		if err != nil {
			return nil, fmt.Errorf("scene: connection %q animation: %w", connection.ID, err)
		}
		stroke.DashOffset = baseOffset
		forward := animatedConnectionTrack(baseOffset, duration, false)
		reverse := animatedConnectionTrack(baseOffset, duration, true)
		forwardTrack = &forward
		reverseTrack = &reverse
	}
	if b.options.Sketch && !connection.Animated {
		path, err = b.sketchConnectionPath(connection, path)
		if err != nil {
			return nil, err
		}
	}

	group := d2scene.NewNode(nil)
	group.ID = connection.ID
	group.Classes = append([]string(nil), connection.Classes...)
	group.Opacity = connection.Opacity
	icon, err := b.buildConnectionIcon(connection)
	if err != nil {
		return nil, err
	}
	if icon != nil {
		group.Children = append(group.Children, icon)
	}
	var srcArrowNode, dstArrowNode *d2scene.Node
	if connection.SrcArrow != d2target.NoArrowhead {
		arrow, err := b.arrowhead(connection, connection.SrcArrow, false, start, connection.Route[0].VectorTo(connection.Route[1]), strokePaint)
		if err != nil {
			return nil, err
		}
		srcArrowNode = arrow
	}
	if connection.DstArrow != d2target.NoArrowhead {
		last := len(connection.Route) - 1
		arrow, err := b.arrowhead(connection, connection.DstArrow, true, end, connection.Route[last-1].VectorTo(connection.Route[last]), strokePaint)
		if err != nil {
			return nil, err
		}
		dstArrowNode = arrow
	}

	var geometryChildren []*d2scene.Node
	splitDirections := connection.Animated && (connection.SrcArrow == d2target.NoArrowhead) == (connection.DstArrow == d2target.NoArrowhead)
	if splitDirections {
		reversePath, forwardPath, err := b.splitAnimatedConnectionPath(path, segments)
		if err != nil {
			return nil, fmt.Errorf("scene: connection %q animation split: %w", connection.ID, err)
		}
		reverseNode := animatedConnectionPathNode(connection.ID+":path:reverse", reversePath, reverseTrack)
		forwardNode := animatedConnectionPathNode(connection.ID+":path:forward", forwardPath, forwardTrack)
		// SVG paints each path's markers after that path's stroke. Interleave
		// the explicit source arrow with the split paths so a two-arrow
		// connection retains path1/marker-start/path2/marker-end z-order.
		geometryChildren = append(geometryChildren, reverseNode)
		if srcArrowNode != nil {
			geometryChildren = append(geometryChildren, srcArrowNode)
		}
		geometryChildren = append(geometryChildren, forwardNode)
		if dstArrowNode != nil {
			geometryChildren = append(geometryChildren, dstArrowNode)
		}
	} else {
		pathNode := animatedConnectionPathNode(connection.ID+":path", path, forwardTrack)
		geometryChildren = append(geometryChildren, pathNode)
		if srcArrowNode != nil {
			geometryChildren = append(geometryChildren, srcArrowNode)
		}
		if dstArrowNode != nil {
			geometryChildren = append(geometryChildren, dstArrowNode)
		}
	}
	if b.connectionMask == nil {
		group.Children = append(group.Children, geometryChildren...)
	} else {
		geometry := d2scene.NewNode(nil)
		geometry.ID = connection.ID + ":geometry"
		geometry.Mask = b.connectionMask
		geometry.Children = geometryChildren
		group.Children = append(group.Children, geometry)
	}
	labels, err := b.buildConnectionLabels(connection)
	if err != nil {
		return nil, err
	}
	group.Children = append(group.Children, labels...)
	return group, nil
}

func animatedConnectionPathNode(id string, path d2scene.Path, track *d2scene.Track) *d2scene.Node {
	node := d2scene.NewNode(path)
	node.ID = id
	if track != nil {
		node.Animations = []d2scene.Track{*track}
	}
	return node
}

func validateAnimatedConnection(object string, connection d2target.Connection) error {
	strokeDash := connection.StrokeDash
	if strokeDash == 0 {
		strokeDash = 5
	}
	if err := validateDash(object, strokeDash, connection.StrokeWidth); err != nil {
		return err
	}
	if connection.StrokeWidth == 0 {
		return nil
	}
	if _, _, err := animatedConnectionTiming(connection); err != nil {
		return invalidField(object, "strokeDash", connection.StrokeDash, err.Error())
	}
	return nil
}

func animatedConnectionTiming(connection d2target.Connection) (float64, time.Duration, error) {
	strokeDash := connection.StrokeDash
	if strokeDash == 0 {
		strokeDash = 5
	}
	dashSize, gapSize := svg.GetStrokeDashAttributes(float64(connection.StrokeWidth), strokeDash)
	period := dashSize + gapSize
	if !finite(period) || period < 0 {
		return 0, 0, fmt.Errorf("must produce a finite non-negative dash period")
	}
	direction := -10.0
	if connection.SrcArrow != d2target.NoArrowhead && connection.DstArrow == d2target.NoArrowhead {
		direction = 10
	}
	baseOffset := direction * period
	if !finite(baseOffset) {
		return 0, 0, fmt.Errorf("must produce a finite dash offset")
	}
	durationNanoseconds := gapSize * .5 * float64(time.Second)
	if !finite(durationNanoseconds) || durationNanoseconds < 1 || durationNanoseconds > float64(math.MaxInt64) {
		return 0, 0, fmt.Errorf("must produce an animation duration within [1ns,%s]", time.Duration(math.MaxInt64))
	}
	duration := time.Duration(durationNanoseconds)
	if duration <= 0 {
		return 0, 0, fmt.Errorf("must produce a positive animation duration")
	}
	return baseOffset, duration, nil
}

func animatedConnectionTrack(baseOffset float64, duration time.Duration, reverse bool) d2scene.Track {
	start, end := 0.0, baseOffset
	if reverse {
		start, end = end, start
	}
	return d2scene.Track{
		Property: d2scene.AnimateStrokeDashOffset,
		Duration: duration,
		Repeat:   true,
		Keyframes: []d2scene.Keyframe{
			{Offset: 0, Value: d2scene.NumberValue(start), Easing: d2scene.Easing{Kind: d2scene.EaseLinear}},
			{Offset: 1, Value: d2scene.NumberValue(end)},
		},
	}
}

// splitAnimatedConnectionPath is the typed equivalent of svg.SplitPath at
// 50%. That svg helper intentionally uses endpoint chord lengths, always
// bisects a C command at t=.5, and does not bisect an S command. Retaining the
// C/S provenance here is therefore required even though both serialize into a
// d2scene cubic command.
func (b *builder) splitAnimatedConnectionPath(path d2scene.Path, segments []connectionPathSegment) (d2scene.Path, d2scene.Path, error) {
	if len(path.Commands) == 0 || path.Commands[0].Kind != d2scene.MoveCommand || len(path.Commands) != len(segments)+1 {
		return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("invalid typed connection path")
	}
	totalLength := 0.0
	lengths := make([]float64, len(segments))
	for i, segment := range segments {
		if err := b.ctx.Err(); err != nil {
			return d2scene.Path{}, d2scene.Path{}, err
		}
		end, err := connectionSegmentEndpoint(segment.command)
		if err != nil {
			return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("segment %d: %w", i, err)
		}
		length := math.Hypot(end.X-segment.start.X, end.Y-segment.start.Y)
		if !finite(length) {
			return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("segment %d has non-finite chord length", i)
		}
		lengths[i] = length
		totalLength += length
		if !finite(totalLength) {
			return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("cumulative chord length is non-finite")
		}
	}
	if totalLength <= 0 {
		return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("path has zero painted length")
	}

	first := d2scene.Path{FillRule: path.FillRule, Fill: path.Fill, Stroke: path.Stroke}
	second := d2scene.Path{FillRule: path.FillRule, Fill: path.Fill, Stroke: path.Stroke}
	first.Commands = append(first.Commands, path.Commands[0])
	halfLength := totalLength * .5
	cumulative := 0.0
	pastHalf := false
	for i, segment := range segments {
		if err := b.ctx.Err(); err != nil {
			return d2scene.Path{}, d2scene.Path{}, err
		}
		length := lengths[i]
		cumulative += length
		if pastHalf {
			second.Commands = append(second.Commands, segment.command)
			continue
		}
		if cumulative < halfLength {
			first.Commands = append(first.Commands, segment.command)
			continue
		}

		end, err := connectionSegmentEndpoint(segment.command)
		if err != nil {
			return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("segment %d: %w", i, err)
		}
		switch segment.kind {
		case connectionPathLine:
			if length == 0 {
				return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("line split has zero chord length")
			}
			t := (halfLength - cumulative + length) / length
			if !finite(t) || t < 0 || t > 1 {
				return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("line split parameter %v is outside [0,1]", t)
			}
			split := d2scene.Point{
				X: segment.start.X + (end.X-segment.start.X)*t,
				Y: segment.start.Y + (end.Y-segment.start.Y)*t,
			}
			first.Commands = append(first.Commands, d2scene.LineTo(split.X, split.Y))
			second.Commands = append(second.Commands, d2scene.MoveTo(split.X, split.Y), d2scene.LineTo(end.X, end.Y))
		case connectionPathCubic:
			if segment.command.Kind != d2scene.CubicCommand {
				return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("C provenance has non-cubic command")
			}
			p1 := geo.Point{X: segment.start.X, Y: segment.start.Y}
			p2 := geo.Point{X: segment.command.P1.X, Y: segment.command.P1.Y}
			p3 := geo.Point{X: segment.command.P2.X, Y: segment.command.P2.Y}
			p4 := geo.Point{X: end.X, Y: end.Y}
			_, q2, q3, q4 := svg.BezierCurveSegment(&p1, &p2, &p3, &p4, 0, .5)
			first.Commands = append(first.Commands, d2scene.CubicTo(q2.X, q2.Y, q3.X, q3.Y, q4.X, q4.Y))
			q1, q2, q3, q4 := svg.BezierCurveSegment(&p1, &p2, &p3, &p4, .5, 1)
			second.Commands = append(second.Commands,
				d2scene.MoveTo(q1.X, q1.Y),
				d2scene.CubicTo(q2.X, q2.Y, q3.X, q3.Y, q4.X, q4.Y),
			)
		case connectionPathSmoothCubic:
			// svg.SplitPath keeps the complete S command in the first path
			// and starts the second path at its endpoint.
			first.Commands = append(first.Commands, segment.command)
			second.Commands = append(second.Commands, d2scene.MoveTo(end.X, end.Y))
		default:
			return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("segment %d has unknown provenance %d", i, segment.kind)
		}
		pastHalf = true
	}
	if !pastHalf || len(second.Commands) == 0 {
		return d2scene.Path{}, d2scene.Path{}, fmt.Errorf("path did not cross its halfway point")
	}
	return first, second, nil
}

func connectionSegmentEndpoint(command d2scene.PathCommand) (d2scene.Point, error) {
	switch command.Kind {
	case d2scene.LineCommand:
		return command.P1, nil
	case d2scene.CubicCommand:
		return command.P3, nil
	default:
		return d2scene.Point{}, fmt.Errorf("unsupported path command %d", command.Kind)
	}
}

func arrowheadAdjustment(start, end *geo.Point, arrowhead d2target.Arrowhead, edgeStrokeWidth, shapeStrokeWidth int) *geo.Point {
	distance := (float64(edgeStrokeWidth) + float64(shapeStrokeWidth)) / 2
	if arrowhead != d2target.NoArrowhead {
		distance += float64(edgeStrokeWidth)
	}
	vector := geo.NewVector(end.X-start.X, end.Y-start.Y)
	if vector.Length() == 0 {
		return geo.NewPoint(0, 0)
	}
	return vector.Unit().Multiply(-distance).ToPoint()
}

func arrowheadAdjustments(connection d2target.Connection, shapes map[string]d2target.Shape) (src, dst *geo.Point) {
	srcShape := shapes[connection.Src]
	dstShape := shapes[connection.Dst]
	src = arrowheadAdjustment(connection.Route[1], connection.Route[0], connection.SrcArrow, connection.StrokeWidth, srcShape.StrokeWidth)
	last := len(connection.Route) - 1
	dst = arrowheadAdjustment(connection.Route[last-1], connection.Route[last], connection.DstArrow, connection.StrokeWidth, dstShape.StrokeWidth)
	return src, dst
}

func connectionPath(connection d2target.Connection, srcAdjustment, dstAdjustment *geo.Point) (d2scene.Path, []connectionPathSegment, d2scene.Point, d2scene.Point, error) {
	route := connection.Route
	start := d2scene.Point{X: route[0].X + srcAdjustment.X, Y: route[0].Y + srcAdjustment.Y}
	end := d2scene.Point{X: route[len(route)-1].X + dstAdjustment.X, Y: route[len(route)-1].Y + dstAdjustment.Y}
	path := d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(start.X, start.Y)}}
	segments := make([]connectionPathSegment, 0, len(route))
	appendSegment := func(kind connectionPathProvenance, segmentStart d2scene.Point, command d2scene.PathCommand) {
		path.Commands = append(path.Commands, command)
		segments = append(segments, connectionPathSegment{kind: kind, start: segmentStart, command: command})
	}
	if connection.IsCurve {
		current := start
		for i := 1; i < len(route); i += 3 {
			endpoint := route[i+2]
			if i+2 == len(route)-1 {
				endpoint = geo.NewPoint(endpoint.X+dstAdjustment.X, endpoint.Y+dstAdjustment.Y)
			}
			command := d2scene.CubicTo(
				route[i].X, route[i].Y,
				route[i+1].X, route[i+1].Y,
				endpoint.X, endpoint.Y,
			)
			appendSegment(connectionPathCubic, current, command)
			current = command.P3
		}
		return path, segments, start, end, nil
	}

	current := start
	for i := 1; i < len(route)-1; i++ {
		previousSource := route[i-1]
		corner := route[i]
		next := route[i+1]
		previousVector := previousSource.VectorTo(corner)
		currentVector := corner.VectorTo(next)
		if previousVector.Length() == 0 || currentVector.Length() == 0 {
			appendSegment(connectionPathLine, current, d2scene.LineTo(corner.X, corner.Y))
			current = d2scene.Point{X: corner.X, Y: corner.Y}
			continue
		}
		distance := geo.EuclideanDistance(corner.X, corner.Y, next.X, next.Y)
		units := math.Min(connection.BorderRadius, distance/2)
		previousTranslation := previousVector.Unit().Multiply(units).ToPoint()
		currentTranslation := currentVector.Unit().Multiply(units).ToPoint()
		lineEnd := d2scene.Point{X: corner.X - previousTranslation.X, Y: corner.Y - previousTranslation.Y}
		appendSegment(connectionPathLine, current, d2scene.LineTo(lineEnd.X, lineEnd.Y))
		current = lineEnd

		if units < connection.BorderRadius && i < len(route)-2 {
			nextTarget := route[i+2]
			nextVector := next.VectorTo(nextTarget)
			if nextVector.Length() == 0 {
				continue
			}
			i++
			nextTranslation := nextVector.Unit().Multiply(units).ToPoint()
			curveEnd := d2scene.Point{X: next.X + nextTranslation.X, Y: next.Y + nextTranslation.Y}
			command := d2scene.CubicTo(
				corner.X+previousTranslation.X, corner.Y+previousTranslation.Y,
				next.X-nextTranslation.X, next.Y-nextTranslation.Y,
				curveEnd.X, curveEnd.Y,
			)
			appendSegment(connectionPathCubic, current, command)
			current = curveEnd
		} else {
			curveEnd := d2scene.Point{X: corner.X + currentTranslation.X, Y: corner.Y + currentTranslation.Y}
			// SVG's first smooth-cubic control is the current point because the
			// preceding command is a line.
			command := d2scene.CubicTo(
				current.X, current.Y,
				corner.X, corner.Y,
				curveEnd.X, curveEnd.Y,
			)
			appendSegment(connectionPathSmoothCubic, current, command)
			current = curveEnd
		}
	}
	appendSegment(connectionPathLine, current, d2scene.LineTo(end.X, end.Y))
	return path, segments, start, end, nil
}

func (b *builder) arrowhead(connection d2target.Connection, arrowhead d2target.Arrowhead, target bool, endpoint d2scene.Point, tangent geo.Vector, strokePaint d2scene.Paint) (*d2scene.Node, error) {
	if tangent.Length() == 0 {
		return nil, fmt.Errorf("scene: connection %q has zero-length arrowhead tangent", connection.ID)
	}
	if b.options.Sketch {
		return b.sketchArrowhead(connection, arrowhead, target, endpoint, tangent)
	}
	strokeWidth := float64(connection.StrokeWidth)
	width, height := arrowhead.Dimensions(strokeWidth)
	refX := 1.5 * strokeWidth
	if target {
		refX = width - 1.5*strokeWidth
	}
	background, err := b.paint(d2target.BG_COLOR, fmt.Sprintf("connection %q arrowhead background", connection.ID))
	if err != nil {
		return nil, err
	}
	endpointName := "src"
	if target {
		endpointName = "dst"
	}
	arrowID := fmt.Sprintf("%s:%s-arrowhead", connection.ID, endpointName)
	var primitive d2scene.Primitive
	var children []*d2scene.Node
	polygon := func(points ...d2scene.Point) d2scene.Path {
		commands := []d2scene.PathCommand{d2scene.MoveTo(points[0].X, points[0].Y)}
		for _, point := range points[1:] {
			commands = append(commands, d2scene.LineTo(point.X, point.Y))
		}
		commands = append(commands, d2scene.ClosePath())
		return d2scene.Path{Commands: commands, Fill: strokePaint}
	}
	polyline := func(points ...d2scene.Point) d2scene.Path {
		commands := []d2scene.PathCommand{d2scene.MoveTo(points[0].X, points[0].Y)}
		for _, point := range points[1:] {
			commands = append(commands, d2scene.LineTo(point.X, point.Y))
		}
		return d2scene.Path{Commands: commands}
	}
	outlineStroke := func() *d2scene.Stroke {
		return &d2scene.Stroke{Paint: strokePaint, Width: strokeWidth, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4}
	}
	child := func(suffix string, childPrimitive d2scene.Primitive, transform d2scene.Matrix) *d2scene.Node {
		node := d2scene.NewNode(childPrimitive)
		node.ID = arrowID + ":" + suffix
		node.Transform = transform
		return node
	}
	switch arrowhead {
	case d2target.ArrowArrowhead:
		if target {
			primitive = polygon(d2scene.Point{}, d2scene.Point{X: width, Y: height / 2}, d2scene.Point{Y: height}, d2scene.Point{X: width / 4, Y: height / 2})
		} else {
			primitive = polygon(d2scene.Point{Y: height / 2}, d2scene.Point{X: width}, d2scene.Point{X: width * .75, Y: height / 2}, d2scene.Point{X: width, Y: height})
		}
	case d2target.TriangleArrowhead:
		if target {
			primitive = polygon(d2scene.Point{}, d2scene.Point{X: width, Y: height / 2}, d2scene.Point{Y: height})
		} else {
			primitive = polygon(d2scene.Point{X: width}, d2scene.Point{Y: height / 2}, d2scene.Point{X: width, Y: height})
		}
	case d2target.UnfilledTriangleArrowhead:
		inset := strokeWidth / 2
		path := polygon(d2scene.Point{X: inset, Y: inset}, d2scene.Point{X: width - inset, Y: height / 2}, d2scene.Point{X: inset, Y: height - inset})
		if !target {
			path = polygon(d2scene.Point{X: width - inset, Y: inset}, d2scene.Point{X: inset, Y: height / 2}, d2scene.Point{X: width - inset, Y: height - inset})
		}
		path.Fill = background
		path.Stroke = &d2scene.Stroke{Paint: strokePaint, Width: strokeWidth, Join: d2scene.JoinRound}
		primitive = path
	case d2target.LineArrowhead:
		inset := strokeWidth / 2
		points := []d2scene.Point{
			{X: inset, Y: inset},
			{X: width - inset, Y: height / 2},
			{X: inset, Y: height - inset},
		}
		if !target {
			points = []d2scene.Point{
				{X: width - inset, Y: inset},
				{X: inset, Y: height / 2},
				{X: width - inset, Y: height - inset},
			}
		}
		path := polyline(points...)
		path.Stroke = outlineStroke()
		primitive = path
	case d2target.FilledDiamondArrowhead:
		// Filled diamonds use the full height; inset outline geometry belongs only
		// to the unfilled diamond below.
		primitive = polygon(
			d2scene.Point{Y: height / 2},
			d2scene.Point{X: width / 2},
			d2scene.Point{X: width, Y: height / 2},
			d2scene.Point{X: width / 2, Y: height},
		)
	case d2target.DiamondArrowhead:
		path := polygon(d2scene.Point{Y: height / 2}, d2scene.Point{X: width / 2, Y: height / 8}, d2scene.Point{X: width, Y: height / 2}, d2scene.Point{X: width / 2, Y: height * .9})
		if !target {
			path = polygon(
				d2scene.Point{X: width / 8, Y: height / 2},
				d2scene.Point{X: width * .6, Y: height / 8},
				d2scene.Point{X: width * 1.1, Y: height / 2},
				d2scene.Point{X: width * .6, Y: height * 7 / 8},
			)
		}
		path.Fill = background
		path.Stroke = &d2scene.Stroke{Paint: strokePaint, Width: strokeWidth, Join: d2scene.JoinRound}
		if target {
			refX = width - .6*strokeWidth
		} else {
			refX = width/8 + .6*strokeWidth
		}
		primitive = path
	case d2target.FilledCircleArrowhead, d2target.CircleArrowhead:
		radius := width / 2
		centerX := radius - strokeWidth/2
		if target {
			centerX = radius + strokeWidth/2
		}
		ellipse := d2scene.Ellipse{Center: d2scene.Point{X: centerX, Y: radius}, RadiusX: radius - strokeWidth/2, RadiusY: radius - strokeWidth/2, Fill: strokePaint}
		if arrowhead == d2target.CircleArrowhead {
			ellipse.RadiusX = radius - strokeWidth
			ellipse.RadiusY = radius - strokeWidth
			ellipse.Fill = background
			ellipse.Stroke = &d2scene.Stroke{Paint: strokePaint, Width: strokeWidth, Join: d2scene.JoinRound}
		}
		primitive = ellipse
	case d2target.CrossArrowhead:
		inset := strokeWidth / 8
		cross := polygon(
			d2scene.Point{Y: height/2 + inset},
			d2scene.Point{X: width/2 - inset, Y: height/2 + inset},
			d2scene.Point{X: width/2 - inset, Y: height},
			d2scene.Point{X: width/2 + inset, Y: height},
			d2scene.Point{X: width/2 + inset, Y: height/2 + inset},
			d2scene.Point{X: width, Y: height/2 + inset},
			d2scene.Point{X: width, Y: height/2 - inset},
			d2scene.Point{X: width/2 + inset, Y: height/2 - inset},
			d2scene.Point{X: width/2 + inset},
			d2scene.Point{X: width/2 - inset},
			d2scene.Point{X: width/2 - inset, Y: height/2 - inset},
			d2scene.Point{Y: height/2 - inset},
		)
		cross.Fill = background
		cross.Stroke = outlineStroke()
		origin := d2scene.Point{X: width / 2, Y: height / 2}
		rotatedOrigin := d2scene.Rotate(math.Pi / 4).Point(origin)
		crossTransform := d2scene.Translate(-rotatedOrigin.X+width/2, -rotatedOrigin.Y+height/2).Mul(d2scene.Rotate(math.Pi / 4))
		children = append(children, child("cross", cross, crossTransform))

		stemEndX := width
		if !target {
			stemEndX = 0
		}
		stem := polyline(d2scene.Point{X: width / 2, Y: height / 2}, d2scene.Point{X: stemEndX, Y: height / 2})
		stem.Fill = background
		stem.Stroke = outlineStroke()
		children = append(children, child("stem", stem, d2scene.Identity()))
	case d2target.FilledBoxArrowhead, d2target.BoxArrowhead:
		inset := 0.0
		if arrowhead == d2target.BoxArrowhead {
			inset = strokeWidth / 2
		}
		path := polygon(d2scene.Point{X: inset, Y: inset}, d2scene.Point{X: inset, Y: height - inset}, d2scene.Point{X: width - inset, Y: height - inset}, d2scene.Point{X: width - inset, Y: inset})
		if arrowhead == d2target.BoxArrowhead {
			path.Fill = background
			path.Stroke = &d2scene.Stroke{Paint: strokePaint, Width: strokeWidth, Join: d2scene.JoinMiter, MiterLimit: 4}
		}
		primitive = path
	case d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired:
		offset := 3 + strokeWidth*1.8
		internalTransform := d2scene.Identity()
		if !target {
			internalTransform = d2scene.Scale(-1, -1).Mul(d2scene.Translate(-width, -height))
		}

		if arrowhead == d2target.CfOneRequired || arrowhead == d2target.CfManyRequired {
			modifier := polyline(d2scene.Point{X: offset}, d2scene.Point{X: offset, Y: height})
			modifier.Fill = background
			modifier.Stroke = outlineStroke()
			children = append(children, child("modifier", modifier, internalTransform))
		} else {
			modifier := d2scene.Ellipse{
				Center:  d2scene.Point{X: offset/2 + 2, Y: height / 2},
				RadiusX: offset / 2,
				RadiusY: offset / 2,
				Fill:    background,
				Stroke:  outlineStroke(),
			}
			children = append(children, child("modifier", modifier, internalTransform))
		}

		var marks d2scene.Path
		if arrowhead == d2target.CfMany || arrowhead == d2target.CfManyRequired {
			marks = d2scene.Path{Commands: []d2scene.PathCommand{
				d2scene.MoveTo(width-3, height/2), d2scene.LineTo(width+offset, height/2),
				d2scene.MoveTo(offset+3, height/2), d2scene.LineTo(width+offset, 0),
				d2scene.MoveTo(offset+3, height/2), d2scene.LineTo(width+offset, height),
			}}
		} else {
			marks = d2scene.Path{Commands: []d2scene.PathCommand{
				d2scene.MoveTo(width-3, height/2), d2scene.LineTo(width+offset, height/2),
				d2scene.MoveTo(offset*2, 0), d2scene.LineTo(offset*2, height),
			}}
		}
		marks.Fill = background
		marks.Stroke = outlineStroke()
		children = append(children, child("marks", marks, internalTransform))
	default:
		return nil, unsupported(fmt.Sprintf("connection %q", connection.ID), "arrowhead "+string(arrowhead))
	}
	angle := math.Atan2(tangent[1], tangent[0])
	node := d2scene.NewNode(primitive)
	node.ID = arrowID
	node.Children = children
	node.Transform = d2scene.Translate(endpoint.X, endpoint.Y).Mul(d2scene.Rotate(angle)).Mul(d2scene.Translate(-refX, -height/2))
	return node, nil
}
