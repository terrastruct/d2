package d2scenebuild

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

const (
	paperPatternAssetID d2scene.AssetID = "builtin:fill-pattern:paper:" + paperPatternSourceSHA256
	grainPatternAssetID d2scene.AssetID = "builtin:fill-pattern:grain:" + grainPatternPNGSourceSHA256
)

type builtinPattern struct {
	name  string
	paint d2scene.PatternPaint
}

var paperPatternColors = [...]color.NRGBA{
	{A: 0xff},
	{R: 0xef, G: 0xef, B: 0xef, A: 0xff},
	{R: 0xf5, G: 0xf5, B: 0xf5, A: 0xff},
	{R: 0xf7, G: 0xf7, B: 0xf7, A: 0xff},
	{R: 0xf9, G: 0xf9, B: 0xf9, A: 0xff},
	{R: 0xfc, G: 0xfc, B: 0xfc, A: 0xff},
	{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
}

func canonicalFillPattern(value string) (string, error) {
	switch canonical := strings.ToLower(value); canonical {
	case "", "none", "dots", "lines", "grain", "paper":
		return canonical, nil
	default:
		return "", fmt.Errorf("must be one of none, dots, lines, grain, or paper")
	}
}

func validateFillPattern(object, value string) error {
	if _, err := canonicalFillPattern(value); err != nil {
		return invalidField(object, "fillPattern", value, err.Error())
	}
	return nil
}

func (b *builder) builtinPattern(value string) (*builtinPattern, error) {
	name, err := canonicalFillPattern(value)
	if err != nil {
		return nil, err
	}
	switch name {
	case "", "none":
		return nil, nil
	case "dots":
		root := d2scene.NewNode(nil)
		root.ID = "builtin:fill-pattern:dots:tile"
		root.Opacity = .1
		root.Blend = d2scene.BlendMultiply
		ink := d2scene.SolidPaint{Color: color.NRGBA{R: 0x0a, G: 0x0f, B: 0x25, A: 0xff}}
		for _, point := range [][2]float64{
			{2, 2}, {12, 2}, {12, 12}, {2, 12}, {2, 7}, {12, 7}, {7, 2}, {7, 12}, {7, 7},
		} {
			dot := d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{X: point[0], Y: point[1], Width: 1, Height: 1}, Fill: ink,
			})
			dot.ID = fmt.Sprintf("builtin:fill-pattern:dots:%.0f:%.0f", point[0], point[1])
			root.Children = append(root.Children, dot)
		}
		return newBuiltinPattern(name, d2scene.Box{Width: 15, Height: 15}, root), nil
	case "lines":
		root := d2scene.NewNode(nil)
		root.ID = "builtin:fill-pattern:lines:tile"
		root.Opacity = .05
		root.Blend = d2scene.BlendMultiply
		ink := d2scene.SolidPaint{Color: color.NRGBA{R: 0x0a, G: 0x0f, B: 0x25, A: 0xff}}
		for _, y := range []float64{2, 7, 12} {
			line := d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{Y: y, Width: 15, Height: 1}, Fill: ink,
			})
			line.ID = fmt.Sprintf("builtin:fill-pattern:lines:%.0f", y)
			root.Children = append(root.Children, line)
		}
		return newBuiltinPattern(name, d2scene.Box{Width: 15, Height: 15}, root), nil
	case "grain":
		if err := b.ensureGrainPatternAsset(); err != nil {
			return nil, err
		}
		root := d2scene.NewNode(nil)
		root.ID = "builtin:fill-pattern:grain:tile"
		root.Opacity = .8
		imageNode := d2scene.NewNode(d2scene.Image{
			Asset: grainPatternAssetID,
			Box:   d2scene.Box{Width: 300, Height: 300},
			Aspect: d2scene.AspectRatio{
				Align: d2scene.AlignNone,
			},
		})
		imageNode.ID = "builtin:fill-pattern:grain:image"
		imageNode.Opacity = .9
		root.Children = []*d2scene.Node{imageNode}
		return newBuiltinPattern(name, d2scene.Box{Width: 300, Height: 300}, root), nil
	case "paper":
		if err := b.ensurePaperPatternAsset(); err != nil {
			return nil, err
		}
		root := d2scene.NewNode(d2scene.Image{
			Asset: paperPatternAssetID,
			Box:   d2scene.Box{Width: 75, Height: 75},
			Aspect: d2scene.AspectRatio{
				Align: d2scene.AlignNone,
			},
		})
		root.ID = "builtin:fill-pattern:paper:image"
		return newBuiltinPattern(name, d2scene.Box{Width: 75, Height: 75}, root), nil
	default:
		panic("unreachable fill pattern")
	}
}

func newBuiltinPattern(name string, tile d2scene.Box, root *d2scene.Node) *builtinPattern {
	return &builtinPattern{
		name: name,
		paint: d2scene.PatternPaint{
			Tile: tile, Root: root, Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		},
	}
}

func (b *builder) ensureGrainPatternAsset() error {
	if _, ok := b.assets[grainPatternAssetID]; ok {
		return nil
	}
	source, err := sharedGrainPatternSource(b.ctx)
	if err != nil {
		return fmt.Errorf("scene: load built-in grain fill pattern: %w", err)
	}
	b.assets[grainPatternAssetID] = d2scene.RasterAsset{
		MIMEType: "image/png", Data: append([]byte(nil), source.png...),
		PixelWidth: source.pixelWidth, PixelHeight: source.pixelHeight,
		DecodedBytes: source.decodedBytes,
	}
	return nil
}

func (b *builder) ensurePaperPatternAsset() error {
	if _, ok := b.assets[paperPatternAssetID]; ok {
		return nil
	}
	source, err := sharedPaperPatternSource(b.ctx)
	if err != nil {
		return fmt.Errorf("scene: load built-in paper fill pattern: %w", err)
	}
	asset, err := newPaperPatternAsset(b.ctx, source)
	if err != nil {
		return fmt.Errorf("scene: build built-in paper fill pattern: %w", err)
	}
	b.assets[paperPatternAssetID] = asset
	return nil
}

func newPaperPatternAsset(ctx context.Context, source *paperPatternSource) (d2scene.VectorAsset, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || len(source.paths) != paperPatternPathCount {
		return d2scene.VectorAsset{}, fmt.Errorf("invalid shared paper pattern source")
	}
	root := d2scene.NewNode(nil)
	root.ID = "builtin:fill-pattern:paper:content"
	root.Opacity = .3
	root.Blend = d2scene.BlendMultiply
	root.Children = make([]*d2scene.Node, len(source.paths))
	commandCount := 0
	for pathIndex, path := range source.paths {
		if pathIndex&255 == 0 {
			if err := ctx.Err(); err != nil {
				return d2scene.VectorAsset{}, err
			}
		}
		if int(path.color) >= len(paperPatternColors) || len(path.commands) == 0 {
			return d2scene.VectorAsset{}, fmt.Errorf("paper path %d has invalid cached data", pathIndex)
		}
		commands := append([]d2scene.PathCommand(nil), path.commands...)
		node := d2scene.NewNode(d2scene.Path{
			Commands: commands,
			Fill:     d2scene.SolidPaint{Color: paperPatternColors[path.color]},
		})
		node.ID = fmt.Sprintf("builtin:fill-pattern:paper:path:%d", pathIndex)
		root.Children[pathIndex] = node
		commandCount += len(commands)
	}
	if commandCount != paperPatternCommandCount {
		return d2scene.VectorAsset{}, fmt.Errorf("paper command count %d, want %d", commandCount, paperPatternCommandCount)
	}
	return d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 75, Height: 75}, Root: root}, nil
}

func overlayPatternNode(base *d2scene.Node, pattern *builtinPattern, fallbackID string) (*d2scene.Node, error) {
	if base == nil || pattern == nil {
		return nil, fmt.Errorf("scene: cannot pattern a nil node or pattern")
	}
	if len(base.Children) != 0 {
		return nil, fmt.Errorf("scene: cannot apply fill pattern to node %q with children", base.ID)
	}
	if len(base.Animations) != 0 {
		return nil, fmt.Errorf("scene: cannot apply fill pattern to animated node %q", base.ID)
	}
	var primitive d2scene.Primitive
	switch value := base.Primitive.(type) {
	case d2scene.Rect:
		value.Fill = pattern.paint
		value.Stroke = nil
		primitive = value
	case *d2scene.Rect:
		if value == nil {
			return nil, fmt.Errorf("scene: node %q has a nil rectangle", base.ID)
		}
		copy := *value
		copy.Fill = pattern.paint
		copy.Stroke = nil
		primitive = copy
	case d2scene.Ellipse:
		value.Fill = pattern.paint
		value.Stroke = nil
		primitive = value
	case *d2scene.Ellipse:
		if value == nil {
			return nil, fmt.Errorf("scene: node %q has a nil ellipse", base.ID)
		}
		copy := *value
		copy.Fill = pattern.paint
		copy.Stroke = nil
		primitive = copy
	case d2scene.Path:
		value.Fill = pattern.paint
		value.Stroke = nil
		primitive = value
	case *d2scene.Path:
		if value == nil {
			return nil, fmt.Errorf("scene: node %q has a nil path", base.ID)
		}
		copy := *value
		copy.Fill = pattern.paint
		copy.Stroke = nil
		primitive = copy
	default:
		return nil, fmt.Errorf("scene: node %q primitive %T cannot carry a fill pattern", base.ID, base.Primitive)
	}
	overlay := d2scene.NewNode(primitive)
	overlay.ID = fallbackID
	if base.ID != "" {
		overlay.ID = base.ID + ":fill-pattern"
	}
	overlay.Classes = []string{pattern.name + "-overlay"}
	overlay.Transform = base.Transform
	overlay.Opacity = base.Opacity
	overlay.Blend = d2scene.BlendMultiply
	overlay.Clip = base.Clip
	overlay.Mask = base.Mask
	overlay.Filters = append([]d2scene.Filter(nil), base.Filters...)
	return overlay, nil
}

func (b *builder) interleavePattern(nodes []*d2scene.Node, pattern *builtinPattern, idPrefix string, eligible func(*d2scene.Node) bool) ([]*d2scene.Node, error) {
	if pattern == nil {
		return nodes, nil
	}
	output := make([]*d2scene.Node, 0, len(nodes)*2)
	for index, node := range nodes {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		output = append(output, node)
		if eligible != nil && !eligible(node) {
			continue
		}
		overlay, err := overlayPatternNode(node, pattern, fmt.Sprintf("%s:fill-pattern:%d", idPrefix, index))
		if err != nil {
			return nil, err
		}
		output = append(output, overlay)
	}
	return output, nil
}

func ordinaryPatternNode(*d2scene.Node) bool { return true }

func structuredPatternNode(targetShapeID string) func(*d2scene.Node) bool {
	return func(node *d2scene.Node) bool {
		return node != nil && (node.ID == targetShapeID+":outline" ||
			node.ID == targetShapeID+":class-header" || node.ID == targetShapeID+":table-header")
	}
}

func effectPatternNode(targetShape d2target.Shape) func(*d2scene.Node) bool {
	return func(node *d2scene.Node) bool {
		if node == nil {
			return false
		}
		if targetShape.ThreeDee {
			return node.ID == targetShape.ID+":3d-main"
		}
		if targetShape.DoubleBorder {
			if node.ID == targetShape.ID+":double-border:outer" {
				return true
			}
			if targetShape.Multiple && node.ID == targetShape.ID+":multiple:outer" {
				switch targetShape.Type {
				case "", d2target.ShapeRectangle, d2target.ShapeSquare:
					return true
				}
			}
			return false
		}
		if targetShape.Multiple {
			return node.ID == targetShape.ID+":main" || strings.HasPrefix(node.ID, targetShape.ID+":main:path:")
		}
		return false
	}
}
