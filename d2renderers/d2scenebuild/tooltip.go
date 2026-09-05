package d2scenebuild

import (
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	libcolor "github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	positionedTooltipPadding  = 10
	positionedTooltipRadius   = 4
	positionedTooltipTailSize = 8
)

// buildPositionedTooltip appends visible tooltips after diagram objects so they
// remain above all z-indexed shapes and connections.
func (b *builder) buildPositionedTooltip(targetShape d2target.Shape) (*d2scene.Node, error) {
	if targetShape.Tooltip == "" || targetShape.TooltipPosition == "" {
		return nil, nil
	}
	object := fmt.Sprintf("shape %q positioned tooltip", targetShape.ID)
	primary, mono := b.markdownFontFamilies(targetShape.FontFamily)
	ruler, err := b.markdownLayoutRuler()
	if err != nil {
		return nil, fmt.Errorf("scene: %s Markdown ruler: %w", object, err)
	}
	layout, err := textmeasure.LayoutMarkdown(targetShape.Tooltip, ruler, &primary, &mono, d2fonts.FONT_SIZE_M)
	if err != nil {
		return nil, fmt.Errorf("scene: %s Markdown layout: %w", object, err)
	}
	if len(layout.Primitives) > maxMarkdownPrimitives {
		return nil, fmt.Errorf("scene: %s Markdown primitive count %d exceeds limit %d", object, len(layout.Primitives), maxMarkdownPrimitives)
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	tooltipWidth := layout.Width + 2*positionedTooltipPadding
	tooltipHeight := layout.Height + 2*positionedTooltipPadding
	x, y := d2target.CalculateTooltipPosition(
		float64(targetShape.Pos.X), float64(targetShape.Pos.Y),
		float64(targetShape.Width), float64(targetShape.Height),
		tooltipWidth, tooltipHeight, targetShape.TooltipPosition,
	)
	if !finite(x) || !finite(y) || tooltipWidth <= 0 || tooltipHeight <= 0 {
		return nil, invalidField(object, "bounds", nil, "must resolve to finite positive dimensions")
	}

	fill, err := b.paint(libcolor.N7, object+" background")
	if err != nil {
		return nil, err
	}
	strokePaint, err := b.paint(libcolor.N5, object+" border")
	if err != nil {
		return nil, err
	}
	stroke := &d2scene.Stroke{
		Paint: strokePaint, Width: 1,
		Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4,
	}
	root := d2scene.NewNode(nil)
	root.ID = targetShape.ID + ":positioned-tooltip"
	box := d2scene.Box{X: x, Y: y, Width: float64(tooltipWidth), Height: float64(tooltipHeight)}
	background := d2scene.NewNode(d2scene.Rect{
		Box: box, RadiusX: positionedTooltipRadius, RadiusY: positionedTooltipRadius,
		Fill: fill, Stroke: stroke,
	})
	background.ID = root.ID + ":background"
	root.Children = append(root.Children, background)

	tail, err := positionedTooltipTail(targetShape.TooltipPosition, box, fill, stroke)
	if err != nil {
		return nil, fmt.Errorf("scene: %s tail: %w", object, err)
	}
	tail.ID = root.ID + ":tail"
	root.Children = append(root.Children, tail)

	content, err := b.buildMarkdownLabel(
		object, root.ID+":markdown", targetShape.Tooltip, targetShape.FontFamily,
		d2fonts.FONT_SIZE_M, layout.Width, layout.Height,
		libcolor.N1, "", false, false,
		geo.NewPoint(x+positionedTooltipPadding, y+positionedTooltipPadding),
	)
	if err != nil {
		return nil, err
	}
	root.Children = append(root.Children, content)
	return root, nil
}

func positionedTooltipTail(position string, box d2scene.Box, fill d2scene.Paint, stroke *d2scene.Stroke) (*d2scene.Node, error) {
	tailX, tailY := box.X+box.Width/2, box.Y+box.Height
	direction := "bottom"
	switch position {
	case "top-left":
		tailX, tailY = box.X+20, box.Y+box.Height
	case "top-center":
	case "top-right":
		tailX, tailY = box.X+box.Width-20, box.Y+box.Height
	case "center-left":
		direction, tailX, tailY = "right", box.X+box.Width, box.Y+box.Height/2
	case "center-right":
		direction, tailX, tailY = "left", box.X, box.Y+box.Height/2
	case "bottom-left":
		direction, tailX, tailY = "top", box.X+20, box.Y
	case "bottom-center":
		direction, tailX, tailY = "top", box.X+box.Width/2, box.Y
	case "bottom-right":
		direction, tailX, tailY = "top", box.X+box.Width-20, box.Y
	}
	size := float64(positionedTooltipTailSize)
	var commands []d2scene.PathCommand
	switch direction {
	case "top":
		commands = []d2scene.PathCommand{
			d2scene.MoveTo(tailX-size/2, tailY), d2scene.LineTo(tailX+size/2, tailY),
			d2scene.LineTo(tailX, tailY-size), d2scene.ClosePath(),
		}
	case "bottom":
		commands = []d2scene.PathCommand{
			d2scene.MoveTo(tailX-size/2, tailY), d2scene.LineTo(tailX+size/2, tailY),
			d2scene.LineTo(tailX, tailY+size), d2scene.ClosePath(),
		}
	case "left":
		commands = []d2scene.PathCommand{
			d2scene.MoveTo(tailX, tailY-size/2), d2scene.LineTo(tailX, tailY+size/2),
			d2scene.LineTo(tailX-size, tailY), d2scene.ClosePath(),
		}
	case "right":
		commands = []d2scene.PathCommand{
			d2scene.MoveTo(tailX, tailY-size/2), d2scene.LineTo(tailX, tailY+size/2),
			d2scene.LineTo(tailX+size, tailY), d2scene.ClosePath(),
		}
	default:
		return nil, fmt.Errorf("unknown direction %q", direction)
	}
	return d2scene.NewNode(d2scene.Path{Commands: commands, Fill: fill, Stroke: stroke}), nil
}
