package d2scenebuild

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	appendixPadTop     = 50
	appendixPadSides   = 40
	appendixSpacer     = 20
	appendixFontSize   = 16
	appendixIconRadius = 16
)

type appendixRow struct {
	number     int
	text       string
	y          int
	width      int
	height     int
	lineHeight float64
}

type appendixItem struct {
	object string
	field  string
	text   string
}

type appendixLayout struct {
	topLeft     d2target.Point
	bottomRight d2target.Point
	maxWidth    int
	height      int
	rows        []appendixRow
}

// addAppendixItem retains one static representation of metadata that a raster
// container cannot expose interactively. Shape badges are built separately,
// but connection tooltips and Markdown link titles still receive rows so they
// cannot silently disappear from PNG, GIF, PDF, or PPTX pixels.
func (b *builder) addAppendixItem(object, field, text string) error {
	if !b.options.Appendix || text == "" {
		return nil
	}
	budget := b.options.LinkBudget
	if budget.MaxRegions <= 0 || budget.MaxStringBytes <= 0 {
		// compileLinkRegions/addMarkdownLinkRegion reports the canonical missing
		// metadata-budget error before a document can be returned.
		return nil
	}
	maxItems := budget.MaxRegions
	if maxItems <= int(^uint(0)>>1)/2 {
		maxItems *= 2
	} else {
		maxItems = int(^uint(0) >> 1)
	}
	if len(b.appendixItems) >= maxItems {
		return fmt.Errorf("scene: appendix item count exceeds limit %d", maxItems)
	}
	if err := validateLinkString(b.ctx, object, field, text); err != nil {
		return err
	}
	if len(text) > budget.MaxStringBytes-b.appendixStringBytes {
		return fmt.Errorf("scene: appendix string bytes exceed limit %d", budget.MaxStringBytes)
	}
	b.appendixItems = append(b.appendixItems, appendixItem{object: object, field: field, text: text})
	b.appendixStringBytes += len(text)
	return nil
}

// preflightAppendix charges both the numbered badges painted on shapes and
// the rendered appendix strings to the caller's typed metadata budget. A
// shape occupies one link region but may paint both a tooltip and a link
// badge, hence the bounded two-to-one item allowance.
func (b *builder) preflightAppendix() error {
	if !b.options.Appendix {
		return nil
	}
	budget := b.options.LinkBudget
	// compileLinkRegions reports the canonical missing-budget error after all
	// feature preflights have run. Avoid replacing it with a less useful
	// appendix-specific limit error.
	if budget.MaxRegions <= 0 || budget.MaxStringBytes <= 0 {
		return nil
	}
	maxItems := budget.MaxRegions
	if maxItems <= int(^uint(0)>>1)/2 {
		maxItems *= 2
	} else {
		maxItems = int(^uint(0) >> 1)
	}
	items, stringBytes := 0, 0
	for index, targetShape := range b.diagram.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		for _, item := range []struct {
			field string
			value string
			paint bool
		}{
			{field: "tooltip", value: targetShape.Tooltip, paint: targetShape.Tooltip != ""},
			{field: "prettyLink", value: targetShape.PrettyLink, paint: targetShape.Link != ""},
		} {
			if item.paint {
				items++
				if items > maxItems {
					return fmt.Errorf("scene: appendix item count exceeds limit %d", maxItems)
				}
			}
			if item.value == "" {
				continue
			}
			object := fmt.Sprintf("shape[%d] %q", index, targetShape.ID)
			if err := validateLinkString(b.ctx, object, item.field, item.value); err != nil {
				return err
			}
			if len(item.value) > budget.MaxStringBytes-stringBytes {
				return fmt.Errorf("scene: appendix string bytes exceed limit %d", budget.MaxStringBytes)
			}
			stringBytes += len(item.value)
		}
	}
	return b.ctx.Err()
}

func (b *builder) measureAppendix(topLeft, bottomRight d2target.Point) (*appendixLayout, error) {
	if !b.options.Appendix {
		return nil, nil
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if len(b.appendixItems) == 0 {
		return nil, nil
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("scene: appendix text ruler: %w", err)
	}
	if ruler == nil {
		return nil, fmt.Errorf("scene: appendix text ruler is nil")
	}

	layout := &appendixLayout{topLeft: topLeft, bottomRight: bottomRight}
	totalHeight := 0
	number := 1
	for _, item := range b.appendixItems {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		dimensions := d2graph.GetTextDimensions(nil, ruler, &d2target.MText{
			Text: item.text, FontSize: appendixFontSize,
		}, nil)
		if dimensions == nil || dimensions.Width < 0 || dimensions.Height < 0 {
			return nil, fmt.Errorf("scene: %s appendix %s has invalid measured text dimensions", item.object, item.field)
		}
		rowHeight := maxInt(dimensions.Height, appendixIconRadius*2)
		rowOffset, ok := checkedAppendixInt(int64(totalHeight), appendixPadTop*2)
		if !ok {
			return nil, fmt.Errorf("scene: appendix row offset exceeds the platform integer domain")
		}
		y, ok := checkedAppendixInt(int64(bottomRight.Y), rowOffset)
		if !ok {
			return nil, fmt.Errorf("scene: appendix row y position exceeds the platform integer domain")
		}
		if _, ok := checkedAppendixInt(int64(y), 5); !ok {
			return nil, fmt.Errorf("scene: appendix row text baseline exceeds the platform integer domain")
		}
		rowWidth, ok := checkedAppendixInt(int64(dimensions.Width), appendixIconRadius*3)
		if !ok {
			return nil, fmt.Errorf("scene: appendix row width exceeds the platform integer domain")
		}
		layout.rows = append(layout.rows, appendixRow{
			number: number, text: item.text, y: y, width: rowWidth, height: rowHeight,
			lineHeight: float64(dimensions.Height) / float64(maxInt(1, strings.Count(item.text, "\n")+1)),
		})
		layout.maxWidth = maxInt(layout.maxWidth, rowWidth)
		increment, ok := checkedAppendixInt(int64(rowHeight), appendixSpacer)
		if !ok {
			return nil, fmt.Errorf("scene: appendix row height exceeds the platform integer domain")
		}
		totalHeight, ok = checkedAppendixInt(int64(totalHeight), increment)
		if !ok {
			return nil, fmt.Errorf("scene: appendix total height exceeds the platform integer domain")
		}
		number++
	}
	if len(layout.rows) == 0 {
		return nil, nil
	}
	var ok bool
	layout.height, ok = checkedAppendixInt(int64(totalHeight), appendixSpacer)
	if !ok {
		return nil, fmt.Errorf("scene: appendix total height exceeds the platform integer domain")
	}
	return layout, b.ctx.Err()
}

func checkedAppendixInt(value int64, delta int) (int, bool) {
	maxPlatform := int64(^uint(0) >> 1)
	minPlatform := -maxPlatform - 1
	result, ok := checkedAdd(value, int64(delta), minPlatform, maxPlatform)
	return int(result), ok
}

// expandViewBox mirrors appendix.Append's post-serialization arithmetic. Its
// x/y origin remains unchanged; width is widened against absolute row text
// geometry and height always grows by the row block plus PAD_TOP.
func (layout *appendixLayout) expandViewBox(left, top, width, height int) (int, int, int, int, error) {
	if layout == nil {
		return left, top, width, height, nil
	}
	maxPlatform := int64(^uint(0) >> 1)
	minPlatform := -maxPlatform - 1
	requiredWidth, ok := checkedSub(int64(layout.maxWidth), int64(left), minPlatform, maxPlatform)
	if ok {
		requiredWidth, ok = checkedAdd(requiredWidth, appendixPadSides*2, minPlatform, maxPlatform)
	}
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("scene: appendix width expansion exceeds the platform integer domain")
	}
	width64 := int64(width)
	if width64 < requiredWidth {
		width64 = requiredWidth
	}
	additionalHeight, ok := checkedAdd(int64(layout.height), appendixPadTop, minPlatform, maxPlatform)
	if ok {
		additionalHeight, ok = checkedAdd(int64(height), additionalHeight, minPlatform, maxPlatform)
	}
	height64 := additionalHeight
	if !ok || width64 < 0 || height64 < 0 {
		return 0, 0, 0, 0, fmt.Errorf("scene: appendix height expansion exceeds the platform integer domain")
	}
	return left, top, int(width64), int(height64), nil
}

func (b *builder) buildAppendixIcons(layout *appendixLayout, targetShape d2target.Shape, shapeIndex, tooltipNumber, linkNumber int) ([]*d2scene.Node, error) {
	if layout == nil || targetShape.Tooltip == "" && targetShape.Link == "" {
		return nil, nil
	}
	tooltipCenter, linkCenter, err := appendixIconCenters(targetShape)
	if err != nil {
		return nil, fmt.Errorf("scene: shape[%d] %q appendix icons: %w", shapeIndex, targetShape.ID, err)
	}
	nodes := make([]*d2scene.Node, 0, 2)
	if targetShape.Tooltip != "" && targetShape.TooltipPosition == "" {
		node, err := b.buildNumberedAppendixIcon(fmt.Sprintf("appendix:shape:%d:tooltip", shapeIndex), tooltipNumber, *tooltipCenter)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if targetShape.Link != "" {
		node, err := b.buildNumberedAppendixIcon(fmt.Sprintf("appendix:shape:%d:link", shapeIndex), linkNumber, *linkCenter)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func appendixIconCenters(targetShape d2target.Shape) (tooltip, link *d2scene.Point, err error) {
	geometry := targetGeometry(targetShape)
	if geometry == nil {
		return nil, nil, fmt.Errorf("shape geometry is nil")
	}
	bothIcons := targetShape.Tooltip != "" && targetShape.Link != ""
	corner := geo.NewPoint(float64(targetShape.Pos.X+targetShape.Width), float64(targetShape.Pos.Y))
	center := geo.NewPoint(
		float64(targetShape.Pos.X)+float64(targetShape.Width)/2,
		float64(targetShape.Pos.Y)+float64(targetShape.Height)/2,
	)
	offset := geo.NewVector(-2*appendixIconRadius, 0)
	leftOnShape := false
	switch geometry.GetType() {
	case shape.STEP_TYPE, shape.HEXAGON_TYPE, shape.QUEUE_TYPE, shape.PAGE_TYPE:
		center.Y = float64(targetShape.Pos.Y)
	case shape.PACKAGE_TYPE:
		center.X = float64(targetShape.Pos.X + targetShape.Width)
	case shape.CIRCLE_TYPE, shape.OVAL_TYPE, shape.DIAMOND_TYPE,
		shape.PERSON_TYPE, shape.CLOUD_TYPE, shape.CYLINDER_TYPE:
		if bothIcons {
			leftOnShape = true
			corner = corner.AddVector(offset)
		}
	}
	v1 := center.VectorTo(corner)
	p1 := shape.TraceToShapeBorder(geometry, corner, corner.AddVector(v1))
	if p1 == nil || !finite(p1.X) || !finite(p1.Y) {
		return nil, nil, fmt.Errorf("shape border trace did not resolve to a finite point")
	}
	p2 := p1
	if bothIcons {
		if leftOnShape {
			p2 = p1.AddVector(offset.Reverse())
			p1, p2 = p2, p1
		} else {
			p2 = p1.AddVector(offset)
		}
	}
	tooltip = &d2scene.Point{X: math.Ceil(p1.X), Y: math.Ceil(p1.Y)}
	link = &d2scene.Point{X: math.Ceil(p2.X), Y: math.Ceil(p2.Y)}
	return tooltip, link, nil
}

func (b *builder) buildNumberedAppendixIcon(id string, number int, center d2scene.Point) (*d2scene.Node, error) {
	white, err := b.paint("#ffffff", id+" fill")
	if err != nil {
		return nil, err
	}
	borderPaint, err := b.paint("#DEE1EB", id+" border")
	if err != nil {
		return nil, err
	}
	textPaint, err := b.paint("#000000", id+" number")
	if err != nil {
		return nil, err
	}
	font, err := b.font(d2target.Text{FontSize: appendixFontSize, Bold: true})
	if err != nil {
		return nil, fmt.Errorf("scene: %s: %w", id, err)
	}
	group := d2scene.NewNode(nil)
	group.ID = id
	group.Classes = []string{"appendix-icon"}
	group.Filters = []d2scene.Filter{d2scene.DropShadow{
		SigmaX: 32, SigmaY: 32, Color: color.NRGBA{R: 31, G: 36, B: 58, A: 26},
	}}
	circle := d2scene.NewNode(d2scene.Ellipse{
		Center: center, RadiusX: appendixIconRadius, RadiusY: appendixIconRadius, Fill: white,
		Stroke: &d2scene.Stroke{Paint: borderPaint, Width: 1, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4},
	})
	circle.ID = id + ":circle"
	baseline := center.Y + 5
	text := strconv.Itoa(number)
	textNode := d2scene.NewNode(d2scene.TextRun{
		Text: text, Origin: d2scene.Point{X: center.X, Y: baseline}, Anchor: d2scene.AnchorMiddle,
		Font: font, Fill: textPaint,
		Ink: d2scene.NewBounds(center.X-appendixIconRadius, center.Y-appendixIconRadius, center.X+appendixIconRadius, center.Y+appendixIconRadius),
	})
	textNode.ID = id + ":number"
	group.Children = []*d2scene.Node{circle, textNode}
	return group, nil
}

func (b *builder) buildAppendix(layout *appendixLayout) (*d2scene.Node, error) {
	if layout == nil {
		return nil, nil
	}
	separatorPaint, err := b.paint("B2", "appendix separator")
	if err != nil {
		return nil, err
	}
	textPaint, err := b.paint("N1", "appendix text")
	if err != nil {
		return nil, err
	}
	regular, err := b.font(d2target.Text{FontSize: appendixFontSize})
	if err != nil {
		return nil, fmt.Errorf("scene: appendix text: %w", err)
	}
	root := d2scene.NewNode(nil)
	root.ID = "appendix"
	root.Classes = []string{"appendix"}
	separatorY, ok := checkedAppendixInt(int64(layout.bottomRight.Y), appendixPadTop)
	if !ok {
		return nil, fmt.Errorf("scene: appendix separator y exceeds the platform integer domain")
	}
	separatorLeft, ok := checkedAppendixInt(int64(layout.topLeft.X), -appendixPadSides)
	if !ok {
		return nil, fmt.Errorf("scene: appendix separator left edge exceeds the platform integer domain")
	}
	separatorRight, ok := checkedAppendixInt(int64(maxInt(layout.maxWidth, layout.bottomRight.X)), appendixPadSides)
	if !ok {
		return nil, fmt.Errorf("scene: appendix separator right edge exceeds the platform integer domain")
	}
	separator := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(float64(separatorLeft), float64(separatorY)),
			d2scene.LineTo(float64(separatorRight), float64(separatorY)),
		},
		Stroke: &d2scene.Stroke{Paint: separatorPaint, Width: 1, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4},
	})
	separator.ID = "appendix:separator"
	root.Children = append(root.Children, separator)
	for _, row := range layout.rows {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		rowNode := d2scene.NewNode(nil)
		rowNode.ID = fmt.Sprintf("appendix:row:%d", row.number)
		icon, err := b.buildNumberedAppendixIcon(rowNode.ID+":icon", row.number, d2scene.Point{X: appendixIconRadius, Y: float64(row.y)})
		if err != nil {
			return nil, err
		}
		rowNode.Children = append(rowNode.Children, icon)
		lines := strings.Split(row.text, "\n")
		for lineIndex, line := range lines {
			if err := b.ctx.Err(); err != nil {
				return nil, err
			}
			rendered := legendRenderedText(line)
			if rendered == "" && len(lines) > 1 {
				rendered = " "
			}
			baseline := float64(row.y) + 5 + float64(lineIndex)*row.lineHeight
			lineTop := float64(row.y) + 5 - float64(row.height) + float64(lineIndex)*row.lineHeight
			lineBottom := lineTop + math.Max(1, row.lineHeight)
			text := d2scene.NewNode(d2scene.TextRun{
				Text: rendered, Origin: d2scene.Point{X: appendixIconRadius * 3, Y: baseline}, Anchor: d2scene.AnchorStart,
				Font: regular, Fill: textPaint,
				Ink: d2scene.NewBounds(appendixIconRadius*3, lineTop, float64(maxInt(appendixIconRadius*3+1, row.width)), lineBottom),
			})
			text.ID = fmt.Sprintf("%s:text:%d", rowNode.ID, lineIndex)
			rowNode.Children = append(rowNode.Children, text)
		}
		root.Children = append(root.Children, rowNode)
	}
	return root, b.ctx.Err()
}

func appendixIconNumbers(diagram *d2target.Diagram) (tooltips, links []int) {
	tooltips = make([]int, len(diagram.Shapes))
	links = make([]int, len(diagram.Shapes))
	number := 1
	for index, targetShape := range diagram.Shapes {
		if targetShape.Tooltip != "" {
			tooltips[index] = number
			number++
		}
		if targetShape.Link != "" {
			links[index] = number
			number++
		}
	}
	return tooltips, links
}
