package d2target

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

const (
	DEFAULT_ICON_SIZE = 32
	MAX_ICON_SIZE     = 64
)

type Diagram struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	Shapes      []Shape      `json:"shapes"`
	Connections []Connection `json:"connections"`
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
	x1 := int64(math.MaxInt64)
	y1 := int64(math.MaxInt64)
	x2 := int64(-math.MaxInt64)
	y2 := int64(-math.MaxInt64)

	for _, targetShape := range diagram.Shapes {
		x1 = go2.Min(x1, targetShape.Pos.X)
		y1 = go2.Min(y1, targetShape.Pos.Y)
		x2 = go2.Max(x2, targetShape.Pos.X+targetShape.Width)
		y2 = go2.Max(y2, targetShape.Pos.Y+targetShape.Height)

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
			x1 = go2.Min(x1, int64(labelTL.X))
			y1 = go2.Min(y1, int64(labelTL.Y))
			x2 = go2.Max(x2, int64(labelTL.X)+targetShape.LabelWidth)
			y2 = go2.Max(y2, int64(labelTL.Y)+targetShape.LabelHeight)
		}
	}

	for _, connection := range diagram.Connections {
		for _, point := range connection.Route {
			x1 = go2.Min(x1, int64(math.Floor(point.X)))
			y1 = go2.Min(y1, int64(math.Floor(point.Y)))
			x2 = go2.Max(x2, int64(math.Ceil(point.X)))
			y2 = go2.Max(y2, int64(math.Ceil(point.Y)))
		}

		if connection.Label != "" {
			labelTL := connection.GetLabelTopLeft()
			x1 = go2.Min(x1, int64(labelTL.X))
			y1 = go2.Min(y1, int64(labelTL.Y))
			x2 = go2.Max(x2, int64(labelTL.X)+connection.LabelWidth)
			y2 = go2.Max(y2, int64(labelTL.Y)+connection.LabelHeight)
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
	Width  int64 `json:"width"`
	Height int64 `json:"height"`

	Opacity     float64 `json:"opacity"`
	StrokeDash  float64 `json:"strokeDash"`
	StrokeWidth int64   `json:"strokeWidth"`

	BorderRadius int `json:"borderRadius"`

	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`

	Shadow   bool `json:"shadow"`
	ThreeDee bool `json:"3d"`
	Multiple bool `json:"multiple"`

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

	ZIndex int64 `json:"zIndex"`
	Level  int64 `json:"level"`
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

func (s Shape) GetZIndex() int64 {
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

	LabelWidth  int64 `json:"labelWidth"`
	LabelHeight int64 `json:"labelHeight"`
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
	StrokeWidth int64   `json:"strokeWidth"`
	Stroke      string  `json:"stroke"`

	Text
	LabelPosition   string  `json:"labelPosition"`
	LabelPercentage float64 `json:"labelPercentage"`

	Route   []*geo.Point `json:"route"`
	IsCurve bool         `json:"isCurve,omitempty"`

	Animated bool     `json:"animated"`
	Tooltip  string   `json:"tooltip"`
	Icon     *url.URL `json:"icon"`

	ZIndex int64 `json:"zIndex"`
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

func (c *Connection) GetLabelTopLeft() *geo.Point {
	return label.Position(c.LabelPosition).GetPointOnRoute(
		c.Route,
		float64(c.StrokeWidth),
		c.LabelPercentage,
		float64(c.LabelWidth),
		float64(c.LabelHeight),
	)
}

func (c Connection) GetZIndex() int64 {
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

	// For fat arrows
	LineArrowhead Arrowhead = "line"
)

var Arrowheads = map[string]struct{}{
	string(NoArrowhead):            {},
	string(ArrowArrowhead):         {},
	string(TriangleArrowhead):      {},
	string(DiamondArrowhead):       {},
	string(FilledDiamondArrowhead): {},
}

func ToArrowhead(arrowheadType string, filled bool) Arrowhead {
	switch arrowheadType {
	case string(DiamondArrowhead):
		if filled {
			return FilledDiamondArrowhead
		}
		return DiamondArrowhead
	case string(ArrowArrowhead):
		return ArrowArrowhead
	default:
		return TriangleArrowhead
	}
}

type Point struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

func NewPoint(x, y int64) Point {
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
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

func NewTextDimensions(w, h int64) *TextDimensions {
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

func (s *Shape) GetIconSize(box *geo.Box) int64 {
	iconPosition := label.Position(s.IconPosition)

	minDimension := int64(math.Min(box.Width, box.Height))
	halfMinDimension := int64(math.Ceil(0.5 * float64(minDimension)))

	var size int64

	if iconPosition == label.InsideMiddleCenter {
		size = halfMinDimension
	} else {
		size = int64(math.Min(
			float64(minDimension),
			math.Max(float64(DEFAULT_ICON_SIZE), float64(halfMinDimension)),
		))
	}

	size = int64(math.Min(float64(size), float64(MAX_ICON_SIZE)))

	if !iconPosition.IsOutside() {
		size = int64(math.Min(float64(size),
			math.Min(
				math.Max(float64(int64(box.Width)-2*label.PADDING), 0),
				math.Max(float64(int64(box.Height)-2*label.PADDING), 0),
			),
		))
	}

	return size
}
