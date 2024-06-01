package d2exporter

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
)

func Export(ctx context.Context, g *d2graph.Graph, fontFamily *d2fonts.FontFamily) (*d2target.Diagram, error) {
	diagram := d2target.NewDiagram()
	applyStyles(&diagram.Root, g.Root)
	if g.Root.Label.MapKey == nil {
		diagram.Root.Label = g.Name
	} else {
		diagram.Root.Label = g.Root.Label.Value
	}
	diagram.Name = g.Name
	diagram.IsFolderOnly = g.IsFolderOnly
	if fontFamily == nil {
		fontFamily = go2.Pointer(d2fonts.SourceSansPro)
	}
	if g.Theme != nil && g.Theme.SpecialRules.Mono {
		fontFamily = go2.Pointer(d2fonts.SourceCodePro)
	}
	diagram.FontFamily = fontFamily

	diagram.Shapes = make([]d2target.Shape, len(g.Objects))
	for i := range g.Objects {
		diagram.Shapes[i] = toShape(g.Objects[i], g)
	}

	diagram.Connections = make([]d2target.Connection, len(g.Edges))
	for i := range g.Edges {
		diagram.Connections[i] = toConnection(g.Edges[i], g.Theme)
	}

	return diagram, nil
}

func applyTheme(shape *d2target.Shape, obj *d2graph.Object, theme *d2themes.Theme) {
	shape.Stroke = obj.GetStroke(shape.StrokeDash)
	shape.Fill = obj.GetFill()
	if obj.Shape.Value == d2target.ShapeText {
		shape.Color = color.N1
	}
	if obj.Shape.Value == d2target.ShapeSQLTable || obj.Shape.Value == d2target.ShapeClass {
		shape.PrimaryAccentColor = color.B2
		shape.SecondaryAccentColor = color.AA2
		shape.NeutralAccentColor = color.N2
	}

	// Theme options that change more than color
	if theme != nil {
		if theme.SpecialRules.OuterContainerDoubleBorder {
			if obj.Level() == 1 && len(obj.ChildrenArray) > 0 {
				shape.DoubleBorder = true
			}
		}
		if theme.SpecialRules.ContainerDots {
			if len(obj.ChildrenArray) > 0 {
				shape.FillPattern = "dots"
			}
		} else if theme.SpecialRules.AllPaper {
			shape.FillPattern = "paper"
		}
		if theme.SpecialRules.Mono {
			shape.FontFamily = "mono"
		}
	}
}

func applyStyles(shape *d2target.Shape, obj *d2graph.Object) {
	if obj.Style.Opacity != nil {
		shape.Opacity, _ = strconv.ParseFloat(obj.Style.Opacity.Value, 64)
	}
	if obj.Style.StrokeDash != nil {
		shape.StrokeDash, _ = strconv.ParseFloat(obj.Style.StrokeDash.Value, 64)
	}
	if obj.Style.Fill != nil {
		shape.Fill = obj.Style.Fill.Value
	} else if obj.Shape.Value == d2target.ShapeText {
		shape.Fill = "transparent"
	}
	if obj.Style.FillPattern != nil {
		shape.FillPattern = obj.Style.FillPattern.Value
	}
	if obj.Style.Stroke != nil {
		shape.Stroke = obj.Style.Stroke.Value
	}
	if obj.Style.StrokeWidth != nil {
		shape.StrokeWidth, _ = strconv.Atoi(obj.Style.StrokeWidth.Value)
	}
	if obj.Style.Shadow != nil {
		shape.Shadow, _ = strconv.ParseBool(obj.Style.Shadow.Value)
	}
	if obj.Style.ThreeDee != nil {
		shape.ThreeDee, _ = strconv.ParseBool(obj.Style.ThreeDee.Value)
	}
	if obj.Style.Multiple != nil {
		shape.Multiple, _ = strconv.ParseBool(obj.Style.Multiple.Value)
	}
	if obj.Style.BorderRadius != nil {
		shape.BorderRadius, _ = strconv.Atoi(obj.Style.BorderRadius.Value)
	}

	if obj.Style.FontColor != nil {
		shape.Color = obj.Style.FontColor.Value
	}
	if obj.Style.Italic != nil {
		shape.Italic, _ = strconv.ParseBool(obj.Style.Italic.Value)
	}
	if obj.Style.Bold != nil {
		shape.Bold, _ = strconv.ParseBool(obj.Style.Bold.Value)
	}
	if obj.Style.Underline != nil {
		shape.Underline, _ = strconv.ParseBool(obj.Style.Underline.Value)
	}
	if obj.Style.Font != nil {
		shape.FontFamily = obj.Style.Font.Value
	}
	if obj.Style.DoubleBorder != nil {
		shape.DoubleBorder, _ = strconv.ParseBool(obj.Style.DoubleBorder.Value)
	}
}

func toShape(obj *d2graph.Object, g *d2graph.Graph) d2target.Shape {
	shape := d2target.BaseShape()
	shape.SetType(obj.Shape.Value)
	shape.ID = obj.AbsID()
	shape.Classes = obj.Classes
	shape.ZIndex = obj.ZIndex
	shape.Level = int(obj.Level())
	shape.Pos = d2target.NewPoint(int(obj.TopLeft.X), int(obj.TopLeft.Y))
	shape.Width = int(obj.Width)
	shape.Height = int(obj.Height)

	text := obj.Text()
	shape.Bold = text.IsBold
	shape.Italic = text.IsItalic
	shape.FontSize = text.FontSize

	if obj.IsSequenceDiagram() {
		shape.StrokeWidth = 0
	}

	if obj.IsSequenceDiagramGroup() {
		shape.StrokeWidth = 0
		shape.Blend = true
	}

	applyStyles(shape, obj)
	applyTheme(shape, obj, g.Theme)
	shape.Color = text.GetColor(shape.Italic)
	applyStyles(shape, obj)

	switch obj.Shape.Value {
	case d2target.ShapeCode, d2target.ShapeText:
		shape.Language = obj.Language
		shape.Label = obj.Label.Value
	case d2target.ShapeClass:
		shape.Class = *obj.Class
		// The label is the header for classes and tables, which is set in client to be 4 px larger than the object's set font size
		shape.FontSize -= d2target.HeaderFontAdd
	case d2target.ShapeSQLTable:
		shape.SQLTable = *obj.SQLTable
		shape.FontSize -= d2target.HeaderFontAdd
	case d2target.ShapeCloud:
		if obj.ContentAspectRatio != nil {
			shape.ContentAspectRatio = go2.Pointer(*obj.ContentAspectRatio)
		}
	}
	shape.Label = text.Text
	shape.LabelWidth = text.Dimensions.Width

	shape.LabelHeight = text.Dimensions.Height
	if obj.LabelPosition != nil {
		shape.LabelPosition = *obj.LabelPosition
		if obj.IsSequenceDiagramGroup() {
			shape.LabelFill = shape.Fill
		}
	}

	if obj.Tooltip != nil {
		shape.Tooltip = obj.Tooltip.Value
	}
	if obj.Link != nil {
		shape.Link = obj.Link.Value
		shape.PrettyLink = toPrettyLink(g, obj.Link.Value)
	}

	shape.LinkIcon = obj.LinkIcon
	shape.Icon = obj.Icon
	if obj.IconPosition != nil {
		shape.IconPosition = *obj.IconPosition
	}

	return *shape
}

func toPrettyLink(g *d2graph.Graph, link string) string {
	u, err := url.ParseRequestURI(link)
	if err == nil && u.Host != "" && len(u.RawPath) > 30 {
		return u.Scheme + "://" + u.Host + u.RawPath[:10] + "..." + u.RawPath[len(u.RawPath)-10:]
	} else if err != nil {
		linkKey, err := d2parser.ParseKey(link)
		if err != nil {
			return link
		}
		rootG := g
		for rootG.Parent != nil {
			rootG = rootG.Parent
		}
		var prettyLink []string
	FOR:
		for i := 0; i < len(linkKey.Path); i++ {
			p := linkKey.Path[i].Unbox().ScalarString()
			if i > 0 {
				switch p {
				case "layers", "scenarios", "steps":
					continue FOR
				}
				rootG = rootG.GetBoard(p)
				if rootG == nil {
					return link
				}
			}
			if rootG.Root.Label.MapKey != nil {
				prettyLink = append(prettyLink, rootG.Root.Label.Value)
			} else {
				prettyLink = append(prettyLink, rootG.Name)
			}
		}
		for _, l := range prettyLink {
			// If any part of it is blank, "x > > y" looks stupid, so just use the last
			if l == "" {
				return prettyLink[len(prettyLink)-1]
			}
		}
		return strings.Join(prettyLink, " > ")
	}
	return link
}

func toConnection(edge *d2graph.Edge, theme *d2themes.Theme) d2target.Connection {
	connection := d2target.BaseConnection()
	connection.ID = edge.AbsID()
	connection.Classes = edge.Classes
	connection.ZIndex = edge.ZIndex
	text := edge.Text()

	if edge.SrcArrow {
		connection.SrcArrow = d2target.DefaultArrowhead
		if edge.SrcArrowhead != nil {
			connection.SrcArrow = edge.SrcArrowhead.ToArrowhead()
		}
	}
	if edge.SrcArrowhead != nil {
		if edge.SrcArrowhead.Label.Value != "" {
			connection.SrcLabel = &d2target.Text{
				Label:       edge.SrcArrowhead.Label.Value,
				LabelWidth:  edge.SrcArrowhead.LabelDimensions.Width,
				LabelHeight: edge.SrcArrowhead.LabelDimensions.Height,
			}
			if edge.SrcArrowhead.Style.FontColor != nil {
				connection.SrcLabel.Color = edge.SrcArrowhead.Style.FontColor.Value
			}
		}
	}
	if edge.DstArrow {
		connection.DstArrow = d2target.DefaultArrowhead
		if edge.DstArrowhead != nil {
			connection.DstArrow = edge.DstArrowhead.ToArrowhead()
		}
	}
	if edge.DstArrowhead != nil {
		if edge.DstArrowhead.Label.Value != "" {
			connection.DstLabel = &d2target.Text{
				Label:       edge.DstArrowhead.Label.Value,
				LabelWidth:  edge.DstArrowhead.LabelDimensions.Width,
				LabelHeight: edge.DstArrowhead.LabelDimensions.Height,
			}
			if edge.DstArrowhead.Style.FontColor != nil {
				connection.DstLabel.Color = edge.DstArrowhead.Style.FontColor.Value
			}
		}
	}
	if theme != nil && theme.SpecialRules.NoCornerRadius {
		connection.BorderRadius = 0
	}
	if edge.Style.BorderRadius != nil {
		connection.BorderRadius, _ = strconv.ParseFloat(edge.Style.BorderRadius.Value, 64)
	}

	if edge.Style.Opacity != nil {
		connection.Opacity, _ = strconv.ParseFloat(edge.Style.Opacity.Value, 64)
	}

	if edge.Style.StrokeDash != nil {
		connection.StrokeDash, _ = strconv.ParseFloat(edge.Style.StrokeDash.Value, 64)
	}
	connection.Stroke = edge.GetStroke(connection.StrokeDash)
	if edge.Style.Stroke != nil {
		connection.Stroke = edge.Style.Stroke.Value
	}

	if edge.Style.StrokeWidth != nil {
		connection.StrokeWidth, _ = strconv.Atoi(edge.Style.StrokeWidth.Value)
	}

	if edge.Style.Fill != nil {
		connection.Fill = edge.Style.Fill.Value
	}

	connection.FontSize = text.FontSize
	if edge.Style.FontSize != nil {
		connection.FontSize, _ = strconv.Atoi(edge.Style.FontSize.Value)
	}

	if edge.Style.Animated != nil {
		connection.Animated, _ = strconv.ParseBool(edge.Style.Animated.Value)
	}

	if edge.Tooltip != nil {
		connection.Tooltip = edge.Tooltip.Value
	}
	connection.Icon = edge.Icon

	if edge.Style.Italic != nil {
		connection.Italic, _ = strconv.ParseBool(edge.Style.Italic.Value)
	}

	connection.Color = text.GetColor(connection.Italic)
	if edge.Style.FontColor != nil {
		connection.Color = edge.Style.FontColor.Value
	}
	if edge.Style.Bold != nil {
		connection.Bold, _ = strconv.ParseBool(edge.Style.Bold.Value)
	}
	if edge.Style.Underline != nil {
		connection.Underline, _ = strconv.ParseBool(edge.Style.Underline.Value)
	}
	if theme != nil && theme.SpecialRules.Mono {
		connection.FontFamily = "mono"
	}
	if edge.Style.Font != nil {
		connection.FontFamily = edge.Style.Font.Value
	}
	connection.Label = text.Text
	connection.LabelWidth = text.Dimensions.Width
	connection.LabelHeight = text.Dimensions.Height

	if edge.LabelPosition != nil {
		connection.LabelPosition = *edge.LabelPosition
	}
	if edge.LabelPercentage != nil {
		connection.LabelPercentage = float64(float32(*edge.LabelPercentage))
	}
	connection.Route = make([]*geo.Point, 0, len(edge.Route))
	for i := range edge.Route {
		p := edge.Route[i].Copy()
		p.TruncateDecimals()
		p.TruncateFloat32()
		connection.Route = append(connection.Route, p)
	}

	connection.IsCurve = edge.IsCurve

	connection.Src = edge.Src.AbsID()
	connection.Dst = edge.Dst.AbsID()

	return *connection
}
