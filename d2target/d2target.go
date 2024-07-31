package d2target

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/d2/lib/svg"
)

const (
	DEFAULT_ICON_SIZE = 32
	MAX_ICON_SIZE     = 64

	SHADOW_SIZE_X    = 3
	SHADOW_SIZE_Y    = 5
	THREE_DEE_OFFSET = 15
	MULTIPLE_OFFSET  = 10

	INNER_BORDER_OFFSET = 5

	BG_COLOR = color.N7
	FG_COLOR = color.N1

	MIN_ARROWHEAD_STROKE_WIDTH = 2
	ARROWHEAD_PADDING          = 2.
)

var BorderOffset = geo.NewVector(5, 5)

type Config struct {
	Sketch             *bool           `json:"sketch"`
	ThemeID            *int64          `json:"themeID"`
	DarkThemeID        *int64          `json:"darkThemeID"`
	Pad                *int64          `json:"pad"`
	Center             *bool           `json:"center"`
	LayoutEngine       *string         `json:"layoutEngine"`
	ThemeOverrides     *ThemeOverrides `json:"themeOverrides,omitempty"`
	DarkThemeOverrides *ThemeOverrides `json:"darkThemeOverrides,omitempty"`
}

type ThemeOverrides struct {
	N1  *string `json:"n1"`
	N2  *string `json:"n2"`
	N3  *string `json:"n3"`
	N4  *string `json:"n4"`
	N5  *string `json:"n5"`
	N6  *string `json:"n6"`
	N7  *string `json:"n7"`
	B1  *string `json:"b1"`
	B2  *string `json:"b2"`
	B3  *string `json:"b3"`
	B4  *string `json:"b4"`
	B5  *string `json:"b5"`
	B6  *string `json:"b6"`
	AA2 *string `json:"aa2"`
	AA4 *string `json:"aa4"`
	AA5 *string `json:"aa5"`
	AB4 *string `json:"ab4"`
	AB5 *string `json:"ab5"`
}

type Diagram struct {
	Name   string  `json:"name"`
	Config *Config `json:"config,omitempty"`
	// See docs on the same field in d2graph to understand what it means.
	IsFolderOnly bool                `json:"isFolderOnly"`
	Description  string              `json:"description,omitempty"`
	FontFamily   *d2fonts.FontFamily `json:"fontFamily,omitempty"`

	Shapes      []Shape      `json:"shapes"`
	Connections []Connection `json:"connections"`

	Root Shape `json:"root"`
	// Maybe Icon can be used as a watermark in the root shape

	Layers    []*Diagram `json:"layers,omitempty"`
	Scenarios []*Diagram `json:"scenarios,omitempty"`
	Steps     []*Diagram `json:"steps,omitempty"`
}

func (d *Diagram) GetBoard(boardPath []string) *Diagram {
	if len(boardPath) == 0 {
		return d
	}

	head := boardPath[0]

	if len(boardPath) == 1 && d.Name == head {
		return d
	}

	switch head {
	case "layers":
		if len(boardPath) < 2 {
			return nil
		}
		for _, b := range d.Layers {
			if b.Name == boardPath[1] {
				return b.GetBoard(boardPath[2:])
			}
		}
	case "scenarios":
		if len(boardPath) < 2 {
			return nil
		}
		for _, b := range d.Scenarios {
			if b.Name == boardPath[1] {
				return b.GetBoard(boardPath[2:])
			}
		}
	case "steps":
		if len(boardPath) < 2 {
			return nil
		}
		for _, b := range d.Steps {
			if b.Name == boardPath[1] {
				return b.GetBoard(boardPath[2:])
			}
		}
	}

	for _, b := range d.Layers {
		if b.Name == head {
			return b.GetBoard(boardPath[1:])
		}
	}
	for _, b := range d.Scenarios {
		if b.Name == head {
			return b.GetBoard(boardPath[1:])
		}
	}
	for _, b := range d.Steps {
		if b.Name == head {
			return b.GetBoard(boardPath[1:])
		}
	}
	return nil
}

func (diagram Diagram) Bytes() ([]byte, error) {
	b1, err := json.Marshal(diagram.Shapes)
	if err != nil {
		return nil, err
	}
	b2, err := json.Marshal(diagram.Connections)
	if err != nil {
		return nil, err
	}
	b3, err := json.Marshal(diagram.Root)
	if err != nil {
		return nil, err
	}
	base := append(append(b1, b2...), b3...)

	if diagram.Config != nil {
		b, err := json.Marshal(diagram.Config)
		if err != nil {
			return nil, err
		}
		base = append(base, b...)
	}

	for _, d := range diagram.Layers {
		slices, err := d.Bytes()
		if err != nil {
			return nil, err
		}
		base = append(base, slices...)
	}
	for _, d := range diagram.Scenarios {
		slices, err := d.Bytes()
		if err != nil {
			return nil, err
		}
		base = append(base, slices...)
	}
	for _, d := range diagram.Steps {
		slices, err := d.Bytes()
		if err != nil {
			return nil, err
		}
		base = append(base, slices...)
	}

	return base, nil
}

func (diagram Diagram) HasShape(condition func(Shape) bool) bool {
	for _, d := range diagram.Layers {
		if d.HasShape(condition) {
			return true
		}
	}
	for _, d := range diagram.Scenarios {
		if d.HasShape(condition) {
			return true
		}
	}
	for _, d := range diagram.Steps {
		if d.HasShape(condition) {
			return true
		}
	}
	for _, s := range diagram.Shapes {
		if condition(s) {
			return true
		}
	}
	return false
}

func (diagram Diagram) HashID() (string, error) {
	bytes, err := diagram.Bytes()
	if err != nil {
		return "", err
	}
	h := fnv.New32a()
	h.Write(bytes)
	// CSS names can't start with numbers, so prepend a little something
	return fmt.Sprintf("d2-%d", h.Sum32()), nil
}

func (diagram Diagram) NestedBoundingBox() (topLeft, bottomRight Point) {
	tl, br := diagram.BoundingBox()
	for _, d := range diagram.Layers {
		tl2, br2 := d.NestedBoundingBox()
		tl.X = go2.Min(tl.X, tl2.X)
		tl.Y = go2.Min(tl.Y, tl2.Y)
		br.X = go2.Max(br.X, br2.X)
		br.Y = go2.Max(br.Y, br2.Y)
	}
	for _, d := range diagram.Scenarios {
		tl2, br2 := d.NestedBoundingBox()
		tl.X = go2.Min(tl.X, tl2.X)
		tl.Y = go2.Min(tl.Y, tl2.Y)
		br.X = go2.Max(br.X, br2.X)
		br.Y = go2.Max(br.Y, br2.Y)
	}
	for _, d := range diagram.Steps {
		tl2, br2 := d.NestedBoundingBox()
		tl.X = go2.Min(tl.X, tl2.X)
		tl.Y = go2.Min(tl.Y, tl2.Y)
		br.X = go2.Max(br.X, br2.X)
		br.Y = go2.Max(br.Y, br2.Y)
	}
	return tl, br
}

func (diagram Diagram) BoundingBox() (topLeft, bottomRight Point) {
	if len(diagram.Shapes) == 0 {
		return Point{0, 0}, Point{0, 0}
	}
	x1 := int(math.MaxInt32)
	y1 := int(math.MaxInt32)
	x2 := int(math.MinInt32)
	y2 := int(math.MinInt32)

	for _, targetShape := range diagram.Shapes {
		x1 = go2.Min(x1, targetShape.Pos.X-int(math.Ceil(float64(targetShape.StrokeWidth)/2.)))
		y1 = go2.Min(y1, targetShape.Pos.Y-int(math.Ceil(float64(targetShape.StrokeWidth)/2.)))
		x2 = go2.Max(x2, targetShape.Pos.X+targetShape.Width+int(math.Ceil(float64(targetShape.StrokeWidth)/2.)))
		y2 = go2.Max(y2, targetShape.Pos.Y+targetShape.Height+int(math.Ceil(float64(targetShape.StrokeWidth)/2.)))

		if targetShape.Tooltip != "" || targetShape.Link != "" {
			// 16 is the icon radius
			y1 = go2.Min(y1, targetShape.Pos.Y-targetShape.StrokeWidth-16)
			x2 = go2.Max(x2, targetShape.Pos.X+targetShape.StrokeWidth+targetShape.Width+16)
		}
		if targetShape.Shadow {
			y2 = go2.Max(y2, targetShape.Pos.Y+targetShape.Height+int(math.Ceil(float64(targetShape.StrokeWidth)/2.))+SHADOW_SIZE_Y)
			x2 = go2.Max(x2, targetShape.Pos.X+targetShape.Width+int(math.Ceil(float64(targetShape.StrokeWidth)/2.))+SHADOW_SIZE_X)
		}

		if targetShape.ThreeDee {
			offsetY := THREE_DEE_OFFSET
			if targetShape.Type == ShapeHexagon {
				offsetY /= 2
			}
			y1 = go2.Min(y1, targetShape.Pos.Y-offsetY-targetShape.StrokeWidth)
			x2 = go2.Max(x2, targetShape.Pos.X+THREE_DEE_OFFSET+targetShape.Width+targetShape.StrokeWidth)
		}
		if targetShape.Multiple {
			y1 = go2.Min(y1, targetShape.Pos.Y-MULTIPLE_OFFSET-targetShape.StrokeWidth)
			x2 = go2.Max(x2, targetShape.Pos.X+MULTIPLE_OFFSET+targetShape.Width+targetShape.StrokeWidth)
		}

		if targetShape.Icon != nil && label.FromString(targetShape.IconPosition).IsOutside() {
			contentBox := geo.NewBox(geo.NewPoint(0, 0), float64(targetShape.Width), float64(targetShape.Height))
			s := shape.NewShape(targetShape.Type, contentBox)
			size := GetIconSize(s.GetInnerBox(), targetShape.IconPosition)

			if strings.HasPrefix(targetShape.IconPosition, "OUTSIDE_TOP") {
				y1 = go2.Min(y1, targetShape.Pos.Y-label.PADDING-size)
			} else if strings.HasPrefix(targetShape.IconPosition, "OUTSIDE_BOTTOM") {
				y2 = go2.Max(y2, targetShape.Pos.Y+targetShape.Height+label.PADDING+size)
			} else if strings.HasPrefix(targetShape.IconPosition, "OUTSIDE_LEFT") {
				x1 = go2.Min(x1, targetShape.Pos.X-label.PADDING-size)
			} else if strings.HasPrefix(targetShape.IconPosition, "OUTSIDE_RIGHT") {
				x2 = go2.Max(x2, targetShape.Pos.X+targetShape.Width+label.PADDING+size)
			}
		}

		if targetShape.Label != "" {
			labelPosition := label.FromString(targetShape.LabelPosition)

			shapeType := DSL_SHAPE_TO_SHAPE_TYPE[targetShape.Type]
			s := shape.NewShape(shapeType,
				geo.NewBox(
					geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
					float64(targetShape.Width),
					float64(targetShape.Height),
				),
			)

			labelTL := labelPosition.GetPointOnBox(s.GetBox(), label.PADDING, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight))
			x1 = go2.Min(x1, int(labelTL.X))
			y1 = go2.Min(y1, int(labelTL.Y))
			x2 = go2.Max(x2, int(labelTL.X)+targetShape.LabelWidth)
			y2 = go2.Max(y2, int(labelTL.Y)+targetShape.LabelHeight)
		}
	}

	for _, connection := range diagram.Connections {
		for _, point := range connection.Route {
			x1 = go2.Min(x1, int(math.Floor(point.X))-int(math.Ceil(float64(connection.StrokeWidth)/2.)))
			y1 = go2.Min(y1, int(math.Floor(point.Y))-int(math.Ceil(float64(connection.StrokeWidth)/2.)))
			x2 = go2.Max(x2, int(math.Ceil(point.X))+int(math.Ceil(float64(connection.StrokeWidth)/2.)))
			y2 = go2.Max(y2, int(math.Ceil(point.Y))+int(math.Ceil(float64(connection.StrokeWidth)/2.)))
		}

		if connection.Label != "" {
			labelTL := connection.GetLabelTopLeft()
			x1 = go2.Min(x1, int(labelTL.X))
			y1 = go2.Min(y1, int(labelTL.Y))
			x2 = go2.Max(x2, int(labelTL.X)+connection.LabelWidth)
			y2 = go2.Max(y2, int(labelTL.Y)+connection.LabelHeight)
		}
		if connection.SrcLabel != nil && connection.SrcLabel.Label != "" {
			labelTL := connection.GetArrowheadLabelPosition(false)
			x1 = go2.Min(x1, int(labelTL.X))
			y1 = go2.Min(y1, int(labelTL.Y))
			x2 = go2.Max(x2, int(labelTL.X)+connection.SrcLabel.LabelWidth)
			y2 = go2.Max(y2, int(labelTL.Y)+connection.SrcLabel.LabelHeight)
		}
		if connection.DstLabel != nil && connection.DstLabel.Label != "" {
			labelTL := connection.GetArrowheadLabelPosition(true)
			x1 = go2.Min(x1, int(labelTL.X))
			y1 = go2.Min(y1, int(labelTL.Y))
			x2 = go2.Max(x2, int(labelTL.X)+connection.DstLabel.LabelWidth)
			y2 = go2.Max(y2, int(labelTL.Y)+connection.DstLabel.LabelHeight)
		}
	}

	return Point{x1, y1}, Point{x2, y2}
}

func (diagram Diagram) GetNestedCorpus() string {
	corpus := diagram.GetCorpus()
	for _, d := range diagram.Layers {
		corpus += d.GetNestedCorpus()
	}
	for _, d := range diagram.Scenarios {
		corpus += d.GetNestedCorpus()
	}
	for _, d := range diagram.Steps {
		corpus += d.GetNestedCorpus()
	}

	return corpus
}

func (diagram Diagram) GetCorpus() string {
	var corpus string
	appendixCount := 0
	for _, s := range diagram.Shapes {
		corpus += s.Label
		if s.Tooltip != "" {
			corpus += s.Tooltip
			appendixCount++
			corpus += fmt.Sprint(appendixCount)
		}
		if s.Link != "" {
			corpus += s.Link
			appendixCount++
			corpus += fmt.Sprint(appendixCount)
		}
		corpus += s.PrettyLink
		if s.Type == ShapeClass {
			for _, cf := range s.Fields {
				corpus += cf.Text(0).Text + cf.VisibilityToken()
			}
			for _, cm := range s.Methods {
				corpus += cm.Text(0).Text + cm.VisibilityToken()
			}
		}
		if s.Type == ShapeSQLTable {
			for _, c := range s.Columns {
				for _, t := range c.Texts(0) {
					corpus += t.Text
				}
				corpus += c.ConstraintAbbr()
			}
		}
	}
	for _, c := range diagram.Connections {
		corpus += c.Label
		if c.SrcLabel != nil {
			corpus += c.SrcLabel.Label
		}
		if c.DstLabel != nil {
			corpus += c.DstLabel.Label
		}
	}

	return corpus
}

func NewDiagram() *Diagram {
	return &Diagram{
		Root: Shape{
			Fill: BG_COLOR,
		},
	}
}

type Shape struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	Classes []string `json:"classes,omitempty"`

	Pos    Point `json:"pos"`
	Width  int   `json:"width"`
	Height int   `json:"height"`

	Opacity     float64 `json:"opacity"`
	StrokeDash  float64 `json:"strokeDash"`
	StrokeWidth int     `json:"strokeWidth"`

	BorderRadius int `json:"borderRadius"`

	Fill        string `json:"fill"`
	FillPattern string `json:"fillPattern,omitempty"`
	Stroke      string `json:"stroke"`

	Shadow       bool `json:"shadow"`
	ThreeDee     bool `json:"3d"`
	Multiple     bool `json:"multiple"`
	DoubleBorder bool `json:"double-border"`

	Tooltip      string   `json:"tooltip"`
	Link         string   `json:"link"`
	PrettyLink   string   `json:"prettyLink,omitempty"`
	Icon         *url.URL `json:"icon"`
	IconPosition string   `json:"iconPosition"`

	// Whether the shape should allow shapes behind it to bleed through
	// Currently just used for sequence diagram groups
	Blend bool `json:"blend"`

	Class
	SQLTable

	ContentAspectRatio *float64 `json:"contentAspectRatio,omitempty"`

	Text

	LabelPosition string `json:"labelPosition,omitempty"`

	ZIndex int `json:"zIndex"`
	Level  int `json:"level"`

	// These are used for special shapes, sql_table and class
	PrimaryAccentColor   string `json:"primaryAccentColor,omitempty"`
	SecondaryAccentColor string `json:"secondaryAccentColor,omitempty"`
	NeutralAccentColor   string `json:"neutralAccentColor,omitempty"`
}

func (s Shape) GetFontColor() string {
	if s.Type == ShapeClass || s.Type == ShapeSQLTable {
		if !color.IsThemeColor(s.Color) {
			return s.Color
		}
		return s.Stroke
	}
	if s.Color != color.Empty {
		return s.Color
	}
	return color.N1
}

// TODO remove this function, just set fields on themeable
func (s Shape) CSSStyle() string {
	out := ""

	out += fmt.Sprintf(`stroke-width:%d;`, s.StrokeWidth)
	if s.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(s.StrokeWidth), s.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

func (s *Shape) SetType(t string) {
	// Some types are synonyms of other types, but with hinting for autolayout
	// They should only have one representation in the final export
	if strings.EqualFold(t, ShapeCircle) {
		t = ShapeOval
	} else if strings.EqualFold(t, ShapeSquare) {
		t = ShapeRectangle
	}
	s.Type = strings.ToLower(t)
}

func (s Shape) GetZIndex() int {
	return s.ZIndex
}

func (s Shape) GetID() string {
	return s.ID
}

type Text struct {
	Label      string `json:"label"`
	FontSize   int    `json:"fontSize"`
	FontFamily string `json:"fontFamily"`
	Language   string `json:"language"`
	Color      string `json:"color"`

	Italic    bool `json:"italic"`
	Bold      bool `json:"bold"`
	Underline bool `json:"underline"`

	LabelWidth  int    `json:"labelWidth"`
	LabelHeight int    `json:"labelHeight"`
	LabelFill   string `json:"labelFill,omitempty"`
}

func BaseShape() *Shape {
	return &Shape{
		Opacity:     1,
		StrokeDash:  0,
		StrokeWidth: 2,
		Text: Text{
			Bold:       true,
			FontFamily: "DEFAULT",
		},
	}
}

type Connection struct {
	ID string `json:"id"`

	Classes []string `json:"classes,omitempty"`

	Src      string    `json:"src"`
	SrcArrow Arrowhead `json:"srcArrow"`
	SrcLabel *Text     `json:"srcLabel,omitempty"`

	Dst      string    `json:"dst"`
	DstArrow Arrowhead `json:"dstArrow"`
	DstLabel *Text     `json:"dstLabel,omitempty"`

	Opacity      float64 `json:"opacity"`
	StrokeDash   float64 `json:"strokeDash"`
	StrokeWidth  int     `json:"strokeWidth"`
	Stroke       string  `json:"stroke"`
	Fill         string  `json:"fill,omitempty"`
	BorderRadius float64 `json:"borderRadius,omitempty"`

	Text
	LabelPosition   string  `json:"labelPosition"`
	LabelPercentage float64 `json:"labelPercentage"`

	Link       string `json:"link"`
	PrettyLink string `json:"prettyLink,omitempty"`

	Route   []*geo.Point `json:"route"`
	IsCurve bool         `json:"isCurve,omitempty"`

	Animated bool     `json:"animated"`
	Tooltip  string   `json:"tooltip"`
	Icon     *url.URL `json:"icon"`

	ZIndex int `json:"zIndex"`
}

func BaseConnection() *Connection {
	return &Connection{
		SrcArrow:     NoArrowhead,
		DstArrow:     NoArrowhead,
		Route:        make([]*geo.Point, 0),
		Opacity:      1,
		StrokeDash:   0,
		StrokeWidth:  2,
		BorderRadius: 10,
		Text: Text{
			Italic:     true,
			FontFamily: "DEFAULT",
		},
	}
}

func (c Connection) GetFontColor() string {
	if c.Color != color.Empty {
		return c.Color
	}
	return color.N1
}

func (c Connection) CSSStyle() string {
	out := ""

	out += fmt.Sprintf(`stroke-width:%d;`, c.StrokeWidth)
	strokeDash := c.StrokeDash
	if strokeDash == 0 && c.Animated {
		strokeDash = 5
	}
	if strokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(c.StrokeWidth), strokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)

		if c.Animated {
			dashOffset := -10
			if c.SrcArrow != NoArrowhead && c.DstArrow == NoArrowhead {
				dashOffset = 10
			}
			out += fmt.Sprintf(`stroke-dashoffset:%f;`, float64(dashOffset)*(dashSize+gapSize))
			out += fmt.Sprintf(`animation: dashdraw %fs linear infinite;`, gapSize*0.5)
		}
	}
	return out
}

func (c *Connection) GetLabelTopLeft() *geo.Point {
	point, _ := label.FromString(c.LabelPosition).GetPointOnRoute(
		c.Route,
		float64(c.StrokeWidth),
		c.LabelPercentage,
		float64(c.LabelWidth),
		float64(c.LabelHeight),
	)
	return point
}

func (connection *Connection) GetArrowheadLabelPosition(isDst bool) *geo.Point {
	var width, height float64
	if isDst {
		width = float64(connection.DstLabel.LabelWidth)
		height = float64(connection.DstLabel.LabelHeight)
	} else {
		width = float64(connection.SrcLabel.LabelWidth)
		height = float64(connection.SrcLabel.LabelHeight)
	}

	// get the start/end points of edge segment with arrowhead
	index := 0
	if isDst {
		index = len(connection.Route) - 2
	}
	start, end := connection.Route[index], connection.Route[index+1]
	// Note: end to start to get normal towards unlocked top position
	normalX, normalY := geo.GetUnitNormalVector(end.X, end.Y, start.X, start.Y)

	// determine how much to move the label back from the very end of the edge
	// e.g. if normal points up {x: 0, y:1}, shift width/2 + padding to fit
	shift := math.Abs(normalX)*(height/2.+label.PADDING) +
		math.Abs(normalY)*(width/2.+label.PADDING)

	length := geo.Route(connection.Route).Length()
	var position float64
	if isDst {
		position = 1.
		if length > 0 {
			position -= shift / length
		}
	} else {
		position = 0.
		if length > 0 {
			position = shift / length
		}
	}

	strokeWidth := float64(connection.StrokeWidth)

	labelTL, _ := label.UnlockedTop.GetPointOnRoute(connection.Route, strokeWidth, position, width, height)

	var arrowSize float64
	if isDst && connection.DstArrow != NoArrowhead {
		// Note: these dimensions are for rendering arrowheads on their side so we want the height
		_, arrowSize = connection.DstArrow.Dimensions(strokeWidth)
	} else if connection.SrcArrow != NoArrowhead {
		_, arrowSize = connection.SrcArrow.Dimensions(strokeWidth)
	}

	if arrowSize > 0 {
		// labelTL already accounts for strokeWidth and padding, we only want to shift further if the arrow is larger than this
		offset := (arrowSize/2 + ARROWHEAD_PADDING) - strokeWidth/2 - label.PADDING
		if offset > 0 {
			labelTL.X += normalX * offset
			labelTL.Y += normalY * offset
		}
	}

	return labelTL
}

func (c Connection) GetZIndex() int {
	return c.ZIndex
}

func (c Connection) GetID() string {
	return c.ID
}

type Arrowhead string

const (
	NoArrowhead               Arrowhead = "none"
	ArrowArrowhead            Arrowhead = "arrow"
	UnfilledTriangleArrowhead Arrowhead = "unfilled-triangle"
	TriangleArrowhead         Arrowhead = "triangle"
	DiamondArrowhead          Arrowhead = "diamond"
	FilledDiamondArrowhead    Arrowhead = "filled-diamond"
	CircleArrowhead           Arrowhead = "circle"
	FilledCircleArrowhead     Arrowhead = "filled-circle"

	// For fat arrows
	LineArrowhead Arrowhead = "line"

	// Crows feet notation
	CfOne          Arrowhead = "cf-one"
	CfMany         Arrowhead = "cf-many"
	CfOneRequired  Arrowhead = "cf-one-required"
	CfManyRequired Arrowhead = "cf-many-required"

	DefaultArrowhead Arrowhead = TriangleArrowhead
)

// valid values for arrowhead.shape
var Arrowheads = map[string]struct{}{
	string(NoArrowhead):       {},
	string(ArrowArrowhead):    {},
	string(TriangleArrowhead): {},
	string(DiamondArrowhead):  {},
	string(CircleArrowhead):   {},
	string(CfOne):             {},
	string(CfMany):            {},
	string(CfOneRequired):     {},
	string(CfManyRequired):    {},
}

func ToArrowhead(arrowheadType string, filled *bool) Arrowhead {
	switch arrowheadType {
	case string(DiamondArrowhead):
		if filled != nil && *filled {
			return FilledDiamondArrowhead
		}
		return DiamondArrowhead
	case string(CircleArrowhead):
		if filled != nil && *filled {
			return FilledCircleArrowhead
		}
		return CircleArrowhead
	case string(NoArrowhead):
		return NoArrowhead
	case string(ArrowArrowhead):
		return ArrowArrowhead
	case string(TriangleArrowhead):
		if filled != nil && !(*filled) {
			return UnfilledTriangleArrowhead
		}
		return TriangleArrowhead
	case string(CfOne):
		return CfOne
	case string(CfMany):
		return CfMany
	case string(CfOneRequired):
		return CfOneRequired
	case string(CfManyRequired):
		return CfManyRequired
	default:
		if DefaultArrowhead == TriangleArrowhead &&
			filled != nil && !(*filled) {
			return UnfilledTriangleArrowhead
		}
		return DefaultArrowhead
	}
}

func (arrowhead Arrowhead) Dimensions(strokeWidth float64) (width, height float64) {
	var baseWidth, baseHeight float64
	var widthMultiplier, heightMultiplier float64
	switch arrowhead {
	case ArrowArrowhead:
		baseWidth = 4
		baseHeight = 4
		widthMultiplier = 4
		heightMultiplier = 4
	case TriangleArrowhead:
		baseWidth = 4
		baseHeight = 4
		widthMultiplier = 3
		heightMultiplier = 4
	case UnfilledTriangleArrowhead:
		baseWidth = 7
		baseHeight = 7
		widthMultiplier = 3
		heightMultiplier = 4
	case LineArrowhead:
		widthMultiplier = 5
		heightMultiplier = 8
	case FilledDiamondArrowhead:
		baseWidth = 11
		baseHeight = 7
		widthMultiplier = 5.5
		heightMultiplier = 3.5
	case DiamondArrowhead:
		baseWidth = 11
		baseHeight = 9
		widthMultiplier = 5.5
		heightMultiplier = 4.5
	case FilledCircleArrowhead, CircleArrowhead:
		baseWidth = 8
		baseHeight = 8
		widthMultiplier = 5
		heightMultiplier = 5
	case CfOne, CfMany, CfOneRequired, CfManyRequired:
		baseWidth = 9
		baseHeight = 9
		widthMultiplier = 4.5
		heightMultiplier = 4.5
	}

	clippedStrokeWidth := go2.Max(MIN_ARROWHEAD_STROKE_WIDTH, strokeWidth)
	return baseWidth + clippedStrokeWidth*widthMultiplier, baseHeight + clippedStrokeWidth*heightMultiplier
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func NewPoint(x, y int) Point {
	return Point{X: x, Y: y}
}

const (
	ShapeRectangle       = "rectangle"
	ShapeSquare          = "square"
	ShapePage            = "page"
	ShapeParallelogram   = "parallelogram"
	ShapeDocument        = "document"
	ShapeCylinder        = "cylinder"
	ShapeQueue           = "queue"
	ShapePackage         = "package"
	ShapeStep            = "step"
	ShapeCallout         = "callout"
	ShapeStoredData      = "stored_data"
	ShapePerson          = "person"
	ShapeDiamond         = "diamond"
	ShapeOval            = "oval"
	ShapeCircle          = "circle"
	ShapeHexagon         = "hexagon"
	ShapeCloud           = "cloud"
	ShapeText            = "text"
	ShapeCode            = "code"
	ShapeClass           = "class"
	ShapeSQLTable        = "sql_table"
	ShapeImage           = "image"
	ShapeSequenceDiagram = "sequence_diagram"
	ShapeHierarchy       = "hierarchy"
)

var Shapes = []string{
	ShapeRectangle,
	ShapeSquare,
	ShapePage,
	ShapeParallelogram,
	ShapeDocument,
	ShapeCylinder,
	ShapeQueue,
	ShapePackage,
	ShapeStep,
	ShapeCallout,
	ShapeStoredData,
	ShapePerson,
	ShapeDiamond,
	ShapeOval,
	ShapeCircle,
	ShapeHexagon,
	ShapeCloud,
	ShapeText,
	ShapeCode,
	ShapeClass,
	ShapeSQLTable,
	ShapeImage,
	ShapeSequenceDiagram,
	ShapeHierarchy,
}

func IsShape(s string) bool {
	if s == "" {
		// Default shape is rectangle.
		return true
	}
	for _, s2 := range Shapes {
		if strings.EqualFold(s, s2) {
			return true
		}
	}
	return false
}

type MText struct {
	Text     string `json:"text"`
	FontSize int    `json:"fontSize"`
	IsBold   bool   `json:"isBold"`
	IsItalic bool   `json:"isItalic"`
	Language string `json:"language"`
	Shape    string `json:"shape"`

	Dimensions TextDimensions `json:"dimensions,omitempty"`
}

type TextDimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func NewTextDimensions(w, h int) *TextDimensions {
	return &TextDimensions{Width: w, Height: h}
}

func (text MText) GetColor(isItalic bool) string {
	if isItalic {
		return color.N2
	}
	return color.N1
}

var DSL_SHAPE_TO_SHAPE_TYPE = map[string]string{
	"":                   shape.SQUARE_TYPE,
	ShapeRectangle:       shape.SQUARE_TYPE,
	ShapeSquare:          shape.REAL_SQUARE_TYPE,
	ShapePage:            shape.PAGE_TYPE,
	ShapeParallelogram:   shape.PARALLELOGRAM_TYPE,
	ShapeDocument:        shape.DOCUMENT_TYPE,
	ShapeCylinder:        shape.CYLINDER_TYPE,
	ShapeQueue:           shape.QUEUE_TYPE,
	ShapePackage:         shape.PACKAGE_TYPE,
	ShapeStep:            shape.STEP_TYPE,
	ShapeCallout:         shape.CALLOUT_TYPE,
	ShapeStoredData:      shape.STORED_DATA_TYPE,
	ShapePerson:          shape.PERSON_TYPE,
	ShapeDiamond:         shape.DIAMOND_TYPE,
	ShapeOval:            shape.OVAL_TYPE,
	ShapeCircle:          shape.CIRCLE_TYPE,
	ShapeHexagon:         shape.HEXAGON_TYPE,
	ShapeCloud:           shape.CLOUD_TYPE,
	ShapeText:            shape.TEXT_TYPE,
	ShapeCode:            shape.CODE_TYPE,
	ShapeClass:           shape.CLASS_TYPE,
	ShapeSQLTable:        shape.TABLE_TYPE,
	ShapeImage:           shape.IMAGE_TYPE,
	ShapeSequenceDiagram: shape.SQUARE_TYPE,
	ShapeHierarchy:       shape.SQUARE_TYPE,
}

var SHAPE_TYPE_TO_DSL_SHAPE map[string]string

func init() {
	SHAPE_TYPE_TO_DSL_SHAPE = make(map[string]string, len(DSL_SHAPE_TO_SHAPE_TYPE))
	for k, v := range DSL_SHAPE_TO_SHAPE_TYPE {
		SHAPE_TYPE_TO_DSL_SHAPE[v] = k
	}
	// SQUARE_TYPE is defined twice in the map, make sure it doesn't get set to the empty string one
	SHAPE_TYPE_TO_DSL_SHAPE[shape.SQUARE_TYPE] = ShapeRectangle
}

func GetIconSize(box *geo.Box, position string) int {
	iconPosition := label.FromString(position)

	minDimension := int(math.Min(box.Width, box.Height))
	halfMinDimension := int(math.Ceil(0.5 * float64(minDimension)))

	var size int

	if iconPosition == label.InsideMiddleCenter {
		size = halfMinDimension
	} else {
		size = go2.Min(
			minDimension,
			go2.Max(DEFAULT_ICON_SIZE, halfMinDimension),
		)
	}

	size = go2.Min(size, MAX_ICON_SIZE)

	if !iconPosition.IsOutside() {
		size = go2.Min(size,
			go2.Min(
				go2.Max(int(box.Width)-2*label.PADDING, 0),
				go2.Max(int(box.Height)-2*label.PADDING, 0),
			),
		)
	}

	return size
}
