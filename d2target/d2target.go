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
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/d2/lib/svg"
)

const (
	DEFAULT_ICON_SIZE = 32
	MAX_ICON_SIZE     = 64

	THREE_DEE_OFFSET = 15
	MULTIPLE_OFFSET  = 10

	INNER_BORDER_OFFSET = 5
)

var BorderOffset = geo.NewVector(5, 5)

type Diagram struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	FontFamily  *d2fonts.FontFamily `json:"fontFamily,omitempty"`

	Shapes      []Shape      `json:"shapes"`
	Connections []Connection `json:"connections"`

	Layers    []*Diagram `json:"layers,omitempty"`
	Scenarios []*Diagram `json:"scenarios,omitempty"`
	Steps     []*Diagram `json:"steps,omitempty"`
}

func (diagram Diagram) HashID() (string, error) {
	b1, err := json.Marshal(diagram.Shapes)
	if err != nil {
		return "", err
	}
	b2, err := json.Marshal(diagram.Connections)
	if err != nil {
		return "", err
	}
	h := fnv.New32a()
	h.Write(append(b1, b2...))
	return fmt.Sprint(h.Sum32()), nil
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
		x1 = go2.Min(x1, targetShape.Pos.X-targetShape.StrokeWidth)
		y1 = go2.Min(y1, targetShape.Pos.Y-targetShape.StrokeWidth)
		x2 = go2.Max(x2, targetShape.Pos.X+targetShape.Width+targetShape.StrokeWidth)
		y2 = go2.Max(y2, targetShape.Pos.Y+targetShape.Height+targetShape.StrokeWidth)

		if targetShape.Tooltip != "" || targetShape.Link != "" {
			// 16 is the icon radius
			y1 = go2.Min(y1, targetShape.Pos.Y-targetShape.StrokeWidth-16)
			x2 = go2.Max(x2, targetShape.Pos.X+targetShape.StrokeWidth+targetShape.Width+16)
		}

		if targetShape.ThreeDee {
			y1 = go2.Min(y1, targetShape.Pos.Y-THREE_DEE_OFFSET-targetShape.StrokeWidth)
			x2 = go2.Max(x2, targetShape.Pos.X+THREE_DEE_OFFSET+targetShape.Width+targetShape.StrokeWidth)
		}
		if targetShape.Multiple {
			y1 = go2.Min(y1, targetShape.Pos.Y-MULTIPLE_OFFSET-targetShape.StrokeWidth)
			x2 = go2.Max(x2, targetShape.Pos.X+MULTIPLE_OFFSET+targetShape.Width+targetShape.StrokeWidth)
		}

		if targetShape.Label != "" {
			labelPosition := label.Position(targetShape.LabelPosition)
			if !labelPosition.IsOutside() {
				continue
			}

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
			x1 = go2.Min(x1, int(math.Floor(point.X)))
			y1 = go2.Min(y1, int(math.Floor(point.Y)))
			x2 = go2.Max(x2, int(math.Ceil(point.X)))
			y2 = go2.Max(y2, int(math.Ceil(point.Y)))
		}

		if connection.Label != "" {
			labelTL := connection.GetLabelTopLeft()
			x1 = go2.Min(x1, int(labelTL.X))
			y1 = go2.Min(y1, int(labelTL.Y))
			x2 = go2.Max(x2, int(labelTL.X)+connection.LabelWidth)
			y2 = go2.Max(y2, int(labelTL.Y)+connection.LabelHeight)
		}
	}

	return Point{x1, y1}, Point{x2, y2}
}

func NewDiagram() *Diagram {
	return &Diagram{}
}

type Shape struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	Pos    Point `json:"pos"`
	Width  int   `json:"width"`
	Height int   `json:"height"`

	Opacity     float64 `json:"opacity"`
	StrokeDash  float64 `json:"strokeDash"`
	StrokeWidth int     `json:"strokeWidth"`

	BorderRadius int `json:"borderRadius"`

	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`

	Shadow       bool `json:"shadow"`
	ThreeDee     bool `json:"3d"`
	Multiple     bool `json:"multiple"`
	DoubleBorder bool `json:"double-border"`

	Tooltip      string   `json:"tooltip"`
	Link         string   `json:"link"`
	Icon         *url.URL `json:"icon"`
	IconPosition string   `json:"iconPosition"`

	// Whether the shape should allow shapes behind it to bleed through
	// Currently just used for sequence diagram groups
	Blend bool `json:"blend"`

	Class
	SQLTable

	Text

	LabelPosition string `json:"labelPosition,omitempty"`

	ZIndex int `json:"zIndex"`
	Level  int `json:"level"`

	// These are used for special shapes, sql_table and class
	PrimaryAccentColor   string `json:"primaryAccentColor,omitempty"`
	SecondaryAccentColor string `json:"secondaryAccentColor,omitempty"`
	NeutralAccentColor   string `json:"neutralAccentColor,omitempty"`
}

func (s Shape) CSSStyle() string {
	out := ""

	if s.Type == ShapeSQLTable || s.Type == ShapeClass {
		// Fill is used for header fill in these types
		// This fill property is just background of rows
		out += fmt.Sprintf(`fill:%s;`, s.Stroke)
		// Stroke (border) of these shapes should match the header fill
		out += fmt.Sprintf(`stroke:%s;`, s.Fill)
	} else {
		out += fmt.Sprintf(`fill:%s;`, s.Fill)
		out += fmt.Sprintf(`stroke:%s;`, s.Stroke)
	}
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

	Src      string    `json:"src"`
	SrcArrow Arrowhead `json:"srcArrow"`
	SrcLabel string    `json:"srcLabel"`

	Dst      string    `json:"dst"`
	DstArrow Arrowhead `json:"dstArrow"`
	DstLabel string    `json:"dstLabel"`

	Opacity     float64 `json:"opacity"`
	StrokeDash  float64 `json:"strokeDash"`
	StrokeWidth int     `json:"strokeWidth"`
	Stroke      string  `json:"stroke"`
	Fill        string  `json:"fill,omitempty"`

	Text
	LabelPosition   string  `json:"labelPosition"`
	LabelPercentage float64 `json:"labelPercentage"`

	Route   []*geo.Point `json:"route"`
	IsCurve bool         `json:"isCurve,omitempty"`

	Animated bool     `json:"animated"`
	Tooltip  string   `json:"tooltip"`
	Icon     *url.URL `json:"icon"`

	ZIndex int `json:"zIndex"`
}

func BaseConnection() *Connection {
	return &Connection{
		SrcArrow:    NoArrowhead,
		DstArrow:    NoArrowhead,
		Route:       make([]*geo.Point, 0),
		Opacity:     1,
		StrokeDash:  0,
		StrokeWidth: 2,
		Text: Text{
			Italic:     true,
			FontFamily: "DEFAULT",
		},
	}
}

func (c Connection) CSSStyle() string {
	out := ""

	out += fmt.Sprintf(`stroke:%s;`, c.Stroke)
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
	return label.Position(c.LabelPosition).GetPointOnRoute(
		c.Route,
		float64(c.StrokeWidth),
		c.LabelPercentage,
		float64(c.LabelWidth),
		float64(c.LabelHeight),
	)
}

func (c Connection) GetZIndex() int {
	return c.ZIndex
}

func (c Connection) GetID() string {
	return c.ID
}

type Arrowhead string

const (
	NoArrowhead            Arrowhead = "none"
	ArrowArrowhead         Arrowhead = "arrow"
	TriangleArrowhead      Arrowhead = "triangle"
	DiamondArrowhead       Arrowhead = "diamond"
	FilledDiamondArrowhead Arrowhead = "filled-diamond"
	CircleArrowhead        Arrowhead = "circle"
	FilledCircleArrowhead  Arrowhead = "filled-circle"

	// For fat arrows
	LineArrowhead Arrowhead = "line"

	// Crows feet notation
	CfOne          Arrowhead = "cf-one"
	CfMany         Arrowhead = "cf-many"
	CfOneRequired  Arrowhead = "cf-one-required"
	CfManyRequired Arrowhead = "cf-many-required"
)

var Arrowheads = map[string]struct{}{
	string(NoArrowhead):            {},
	string(ArrowArrowhead):         {},
	string(TriangleArrowhead):      {},
	string(DiamondArrowhead):       {},
	string(FilledDiamondArrowhead): {},
	string(CircleArrowhead):        {},
	string(FilledCircleArrowhead):  {},
	string(CfOne):                  {},
	string(CfMany):                 {},
	string(CfOneRequired):          {},
	string(CfManyRequired):         {},
}

func ToArrowhead(arrowheadType string, filled bool) Arrowhead {
	switch arrowheadType {
	case string(DiamondArrowhead):
		if filled {
			return FilledDiamondArrowhead
		}
		return DiamondArrowhead
	case string(CircleArrowhead):
		if filled {
			return FilledCircleArrowhead
		}
		return CircleArrowhead
	case string(ArrowArrowhead):
		return ArrowArrowhead
	case string(CfOne):
		return CfOne
	case string(CfMany):
		return CfMany
	case string(CfOneRequired):
		return CfOneRequired
	case string(CfManyRequired):
		return CfManyRequired
	default:
		return TriangleArrowhead
	}
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

func (text MText) GetColor(theme *d2themes.Theme, isItalic bool) string {
	if isItalic {
		return theme.Colors.Neutrals.N2
	}
	return theme.Colors.Neutrals.N1
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
}

var SHAPE_TYPE_TO_DSL_SHAPE map[string]string

func init() {
	SHAPE_TYPE_TO_DSL_SHAPE = make(map[string]string, len(DSL_SHAPE_TO_SHAPE_TYPE))
	for k, v := range DSL_SHAPE_TO_SHAPE_TYPE {
		SHAPE_TYPE_TO_DSL_SHAPE[v] = k
	}
}

func GetIconSize(box *geo.Box, position string) int {
	iconPosition := label.Position(position)

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
