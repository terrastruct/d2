package d2scenebuild

import (
	"fmt"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2sketch"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func (b *builder) sketchConnectionPath(connection d2target.Connection, source d2scene.Path) (d2scene.Path, error) {
	object := fmt.Sprintf("connection %q route", connection.ID)
	drawable, err := d2sketch.ConnectionDrawable(b.ctx, source, b.remainingSketchSceneLimits())
	if err != nil {
		return d2scene.Path{}, fmt.Errorf("scene: %s sketch geometry: %w", object, err)
	}
	paths, err := b.compileSketchDrawable(object, drawable)
	if err != nil {
		return d2scene.Path{}, err
	}
	// A connection uses one paint role even when rough-go emits multiple
	// operation sets. Retaining them as subpaths in the existing route primitive
	// preserves its stable ID, mask ownership, and z-order relative to markers.
	roughPath := d2scene.Path{FillRule: source.FillRule, Fill: source.Fill, Stroke: source.Stroke}
	for _, path := range paths {
		roughPath.Commands = append(roughPath.Commands, path.Path.Commands...)
	}
	if len(roughPath.Commands) == 0 {
		return d2scene.Path{}, fmt.Errorf("scene: %s sketch geometry is empty", object)
	}
	return roughPath, nil
}

func (b *builder) sketchArrowhead(connection d2target.Connection, arrowhead d2target.Arrowhead, target bool, endpoint d2scene.Point, tangent geo.Vector) (*d2scene.Node, error) {
	endpointName := "src"
	if target {
		endpointName = "dst"
	}
	arrowID := fmt.Sprintf("%s:%s-arrowhead", connection.ID, endpointName)
	drawables, ok := d2sketch.ArrowheadDrawables(arrowhead, connection.Stroke, connection.StrokeWidth)
	if !ok {
		return nil, unsupported(fmt.Sprintf("connection %q", connection.ID), "sketch arrowhead "+string(arrowhead))
	}

	node := d2scene.NewNode(nil)
	node.ID = arrowID
	pathIndex := 0
	for drawableIndex, drawable := range drawables {
		object := fmt.Sprintf("connection %q %s arrowhead drawable %d", connection.ID, endpointName, drawableIndex)
		paths, err := b.compileSketchDrawable(object, drawable)
		if err != nil {
			return nil, err
		}
		for _, source := range paths {
			path, err := b.sketchScenePath(object, source)
			if err != nil {
				return nil, err
			}
			if pathIndex == 0 {
				node.Primitive = path
			} else {
				child := d2scene.NewNode(path)
				child.ID = fmt.Sprintf("%s:rough:%d", arrowID, pathIndex)
				node.Children = append(node.Children, child)
			}
			pathIndex++
		}
	}
	if pathIndex == 0 {
		return nil, fmt.Errorf("scene: connection %q %s sketch arrowhead is empty", connection.ID, endpointName)
	}

	angle := math.Atan2(tangent[1], tangent[0])
	if !target {
		angle += math.Pi
	}
	node.Transform = d2scene.Translate(endpoint.X, endpoint.Y).Mul(d2scene.Rotate(angle))
	return node, nil
}
