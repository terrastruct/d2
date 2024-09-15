package d2graph

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2latex"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const INNER_LABEL_PADDING int = 5
const DEFAULT_SHAPE_SIZE = 100.
const MIN_SHAPE_SIZE = 5

type Graph struct {
	FS     fs.FS  `json:"-"`
	Parent *Graph `json:"-"`
	Name   string `json:"name"`
	// IsFolderOnly indicates a board or scenario itself makes no modifications from its
	// base. Folder only boards do not have a render and are used purely for organizing
	// the board tree.
	IsFolderOnly bool       `json:"isFolderOnly"`
	AST          *d2ast.Map `json:"ast"`
	// BaseAST is the AST of the original graph without inherited fields and edges
	BaseAST *d2ast.Map `json:"-"`

	Root    *Object   `json:"root"`
	Edges   []*Edge   `json:"edges"`
	Objects []*Object `json:"objects"`

	Layers    []*Graph `json:"layers,omitempty"`
	Scenarios []*Graph `json:"scenarios,omitempty"`
	Steps     []*Graph `json:"steps,omitempty"`

	Theme *d2themes.Theme `json:"theme,omitempty"`

	// Object.Level uses the location of a nested graph
	RootLevel int `json:"rootLevel,omitempty"`
}

func NewGraph() *Graph {
	d := &Graph{}
	d.Root = &Object{
		Graph:    d,
		Parent:   nil,
		Children: make(map[string]*Object),
	}
	return d
}

func (g *Graph) RootBoard() *Graph {
	for g.Parent != nil {
		g = g.Parent
	}
	return g
}

type LayoutGraph func(context.Context, *Graph) error
type RouteEdges func(context.Context, *Graph, []*Edge) error

// TODO consider having different Scalar types
// Right now we'll hold any types in Value and just convert, e.g. floats
type Scalar struct {
	Value  string     `json:"value"`
	MapKey *d2ast.Key `json:"-"`
}

// TODO maybe rename to Shape
type Object struct {
	Graph  *Graph  `json:"-"`
	Parent *Object `json:"-"`

	// IDVal is the actual value of the ID whereas ID is the value in d2 syntax.
	// e.g. ID:    "yes'\""
	//      IDVal: yes'"
	//
	// ID allows joining on . naively and construct a valid D2 key path
	ID         string      `json:"id"`
	IDVal      string      `json:"id_val"`
	Map        *d2ast.Map  `json:"-"`
	References []Reference `json:"references,omitempty"`

	*geo.Box      `json:"box,omitempty"`
	LabelPosition *string `json:"labelPosition,omitempty"`
	IconPosition  *string `json:"iconPosition,omitempty"`

	ContentAspectRatio *float64 `json:"contentAspectRatio,omitempty"`

	Class    *d2target.Class    `json:"class,omitempty"`
	SQLTable *d2target.SQLTable `json:"sql_table,omitempty"`

	Children      map[string]*Object `json:"-"`
	ChildrenArray []*Object          `json:"-"`

	Attributes `json:"attributes"`

	ZIndex int `json:"zIndex"`
}

type Attributes struct {
	Label           Scalar                  `json:"label"`
	LabelDimensions d2target.TextDimensions `json:"labelDimensions"`

	Style   Style    `json:"style"`
	Icon    *url.URL `json:"icon,omitempty"`
	Tooltip *Scalar  `json:"tooltip,omitempty"`
	Link    *Scalar  `json:"link,omitempty"`

	WidthAttr  *Scalar `json:"width,omitempty"`
	HeightAttr *Scalar `json:"height,omitempty"`

	Top  *Scalar `json:"top,omitempty"`
	Left *Scalar `json:"left,omitempty"`

	// TODO consider separate Attributes struct for shape-specific and edge-specific
	// Shapes only
	NearKey  *d2ast.KeyPath `json:"near_key"`
	Language string         `json:"language,omitempty"`
	// TODO: default to ShapeRectangle instead of empty string
	Shape Scalar `json:"shape"`

	Direction  Scalar   `json:"direction"`
	Constraint []string `json:"constraint"`

	GridRows      *Scalar `json:"gridRows,omitempty"`
	GridColumns   *Scalar `json:"gridColumns,omitempty"`
	GridGap       *Scalar `json:"gridGap,omitempty"`
	VerticalGap   *Scalar `json:"verticalGap,omitempty"`
	HorizontalGap *Scalar `json:"horizontalGap,omitempty"`

	LabelPosition *Scalar `json:"labelPosition,omitempty"`
	IconPosition  *Scalar `json:"iconPosition,omitempty"`

	// These names are attached to the rendered elements in SVG
	// so that users can target them however they like outside of D2
	Classes []string `json:"classes,omitempty"`
}

// ApplyTextTransform will alter the `Label.Value` of the current object based
// on the specification of the `text-transform` styling option. This function
// has side-effects!
func (a *Attributes) ApplyTextTransform() {
	if a.Style.NoneTextTransform() {
		return
	}

	if a.Style.TextTransform != nil && a.Style.TextTransform.Value == "uppercase" {
		a.Label.Value = strings.ToUpper(a.Label.Value)
	}
	if a.Style.TextTransform != nil && a.Style.TextTransform.Value == "lowercase" {
		a.Label.Value = strings.ToLower(a.Label.Value)
	}
	if a.Style.TextTransform != nil && a.Style.TextTransform.Value == "capitalize" {
		a.Label.Value = cases.Title(language.Und).String(a.Label.Value)
	}
}

func (a *Attributes) ToArrowhead() d2target.Arrowhead {
	var filled *bool
	if a.Style.Filled != nil {
		v, _ := strconv.ParseBool(a.Style.Filled.Value)
		filled = go2.Pointer(v)
	}
	return d2target.ToArrowhead(a.Shape.Value, filled)
}

type Reference struct {
	Key          *d2ast.KeyPath `json:"key"`
	KeyPathIndex int            `json:"key_path_index"`

	MapKey          *d2ast.Key `json:"-"`
	MapKeyEdgeIndex int        `json:"map_key_edge_index"`
	Scope           *d2ast.Map `json:"-"`
	ScopeObj        *Object    `json:"-"`
	ScopeAST        *d2ast.Map `json:"-"`
}

func (r Reference) MapKeyEdgeDest() bool {
	return r.Key == r.MapKey.Edges[r.MapKeyEdgeIndex].Dst
}

func (r Reference) InEdge() bool {
	return r.Key != r.MapKey.Key
}

type Style struct {
	Opacity       *Scalar `json:"opacity,omitempty"`
	Stroke        *Scalar `json:"stroke,omitempty"`
	Fill          *Scalar `json:"fill,omitempty"`
	FillPattern   *Scalar `json:"fillPattern,omitempty"`
	StrokeWidth   *Scalar `json:"strokeWidth,omitempty"`
	StrokeDash    *Scalar `json:"strokeDash,omitempty"`
	BorderRadius  *Scalar `json:"borderRadius,omitempty"`
	Shadow        *Scalar `json:"shadow,omitempty"`
	ThreeDee      *Scalar `json:"3d,omitempty"`
	Multiple      *Scalar `json:"multiple,omitempty"`
	Font          *Scalar `json:"font,omitempty"`
	FontSize      *Scalar `json:"fontSize,omitempty"`
	FontColor     *Scalar `json:"fontColor,omitempty"`
	Animated      *Scalar `json:"animated,omitempty"`
	Bold          *Scalar `json:"bold,omitempty"`
	Italic        *Scalar `json:"italic,omitempty"`
	Underline     *Scalar `json:"underline,omitempty"`
	Filled        *Scalar `json:"filled,omitempty"`
	DoubleBorder  *Scalar `json:"doubleBorder,omitempty"`
	TextTransform *Scalar `json:"textTransform,omitempty"`
}

// NoneTextTransform will return a boolean if the text should not have any
// transformation applied. This should overwrite theme specific transformations
// like `CapsLock` from the `terminal` theme.
func (s Style) NoneTextTransform() bool {
	return s.TextTransform != nil && s.TextTransform.Value == "none"
}

func (s *Style) Apply(key, value string) error {
	switch key {
	case "opacity":
		if s.Opacity == nil {
			break
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || (f < 0 || f > 1) {
			return errors.New(`expected "opacity" to be a number between 0.0 and 1.0`)
		}
		s.Opacity.Value = value
	case "stroke":
		if s.Stroke == nil {
			break
		}
		if !go2.Contains(color.NamedColors, strings.ToLower(value)) && !color.ColorHexRegex.MatchString(value) {
			return errors.New(`expected "stroke" to be a valid named color ("orange") or a hex code ("#f0ff3a")`)
		}
		s.Stroke.Value = value
	case "fill":
		if s.Fill == nil {
			break
		}
		if !go2.Contains(color.NamedColors, strings.ToLower(value)) && !color.ColorHexRegex.MatchString(value) {
			return errors.New(`expected "fill" to be a valid named color ("orange") or a hex code ("#f0ff3a")`)
		}
		s.Fill.Value = value
	case "fill-pattern":
		if s.FillPattern == nil {
			break
		}
		if !go2.Contains(d2ast.FillPatterns, strings.ToLower(value)) {
			return fmt.Errorf(`expected "fill-pattern" to be one of: %s`, strings.Join(d2ast.FillPatterns, ", "))
		}
		s.FillPattern.Value = value
	case "stroke-width":
		if s.StrokeWidth == nil {
			break
		}
		f, err := strconv.Atoi(value)
		if err != nil || (f < 0 || f > 15) {
			return errors.New(`expected "stroke-width" to be a number between 0 and 15`)
		}
		s.StrokeWidth.Value = value
	case "stroke-dash":
		if s.StrokeDash == nil {
			break
		}
		f, err := strconv.Atoi(value)
		if err != nil || (f < 0 || f > 10) {
			return errors.New(`expected "stroke-dash" to be a number between 0 and 10`)
		}
		s.StrokeDash.Value = value
	case "border-radius":
		if s.BorderRadius == nil {
			break
		}
		f, err := strconv.Atoi(value)
		if err != nil || (f < 0) {
			return errors.New(`expected "border-radius" to be a number greater or equal to 0`)
		}
		s.BorderRadius.Value = value
	case "shadow":
		if s.Shadow == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "shadow" to be true or false`)
		}
		s.Shadow.Value = value
	case "3d":
		if s.ThreeDee == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "3d" to be true or false`)
		}
		s.ThreeDee.Value = value
	case "multiple":
		if s.Multiple == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "multiple" to be true or false`)
		}
		s.Multiple.Value = value
	case "font":
		if s.Font == nil {
			break
		}
		if _, ok := d2fonts.D2_FONT_TO_FAMILY[strings.ToLower(value)]; !ok {
			return fmt.Errorf(`"%v" is not a valid font in our system`, value)
		}
		s.Font.Value = strings.ToLower(value)
	case "font-size":
		if s.FontSize == nil {
			break
		}
		f, err := strconv.Atoi(value)
		if err != nil || (f < 8 || f > 100) {
			return errors.New(`expected "font-size" to be a number between 8 and 100`)
		}
		s.FontSize.Value = value
	case "font-color":
		if s.FontColor == nil {
			break
		}
		if !go2.Contains(color.NamedColors, strings.ToLower(value)) && !color.ColorHexRegex.MatchString(value) {
			return errors.New(`expected "font-color" to be a valid named color ("orange") or a hex code ("#f0ff3a")`)
		}
		s.FontColor.Value = value
	case "animated":
		if s.Animated == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "animated" to be true or false`)
		}
		s.Animated.Value = value
	case "bold":
		if s.Bold == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "bold" to be true or false`)
		}
		s.Bold.Value = value
	case "italic":
		if s.Italic == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "italic" to be true or false`)
		}
		s.Italic.Value = value
	case "underline":
		if s.Underline == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "underline" to be true or false`)
		}
		s.Underline.Value = value
	case "filled":
		if s.Filled == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "filled" to be true or false`)
		}
		s.Filled.Value = value
	case "double-border":
		if s.DoubleBorder == nil {
			break
		}
		_, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New(`expected "double-border" to be true or false`)
		}
		s.DoubleBorder.Value = value
	case "text-transform":
		if s.TextTransform == nil {
			break
		}
		if !go2.Contains(d2ast.TextTransforms, strings.ToLower(value)) {
			return fmt.Errorf(`expected "text-transform" to be one of (%s)`, strings.Join(d2ast.TextTransforms, ", "))
		}
		s.TextTransform.Value = value
	default:
		return fmt.Errorf("unknown style key: %s", key)
	}

	return nil
}

type ContainerLevel int

func (l ContainerLevel) LabelSize() int {
	// Largest to smallest
	if l == 1 {
		return d2fonts.FONT_SIZE_XXL
	} else if l == 2 {
		return d2fonts.FONT_SIZE_XL
	} else if l == 3 {
		return d2fonts.FONT_SIZE_L
	}
	return d2fonts.FONT_SIZE_M
}

func (obj *Object) GetFill() string {
	level := int(obj.Level())
	shape := obj.Shape.Value

	if strings.EqualFold(shape, d2target.ShapeSQLTable) || strings.EqualFold(shape, d2target.ShapeClass) {
		return color.N1
	}

	if obj.IsSequenceDiagramNote() {
		return color.N7
	} else if obj.IsSequenceDiagramGroup() {
		return color.N5
	} else if obj.Parent.IsSequenceDiagram() {
		return color.B5
	}

	// fill for spans
	sd := obj.OuterSequenceDiagram()
	if sd != nil {
		level -= int(sd.Level())
		if level == 1 {
			return color.B3
		} else if level == 2 {
			return color.B4
		} else if level == 3 {
			return color.B5
		} else if level == 4 {
			return color.N6
		}
		return color.N7
	}

	if obj.IsSequenceDiagram() {
		return color.N7
	}

	if shape == "" || strings.EqualFold(shape, d2target.ShapeSquare) || strings.EqualFold(shape, d2target.ShapeCircle) || strings.EqualFold(shape, d2target.ShapeOval) || strings.EqualFold(shape, d2target.ShapeRectangle) || strings.EqualFold(shape, d2target.ShapeHierarchy) {
		if level == 1 {
			if !obj.IsContainer() {
				return color.B6
			}
			return color.B4
		} else if level == 2 {
			return color.B5
		} else if level == 3 {
			return color.B6
		}
		return color.N7
	}

	if strings.EqualFold(shape, d2target.ShapeCylinder) || strings.EqualFold(shape, d2target.ShapeStoredData) || strings.EqualFold(shape, d2target.ShapePackage) {
		if level == 1 {
			return color.AA4
		}
		return color.AA5
	}

	if strings.EqualFold(shape, d2target.ShapeStep) || strings.EqualFold(shape, d2target.ShapePage) || strings.EqualFold(shape, d2target.ShapeDocument) {
		if level == 1 {
			return color.AB4
		}
		return color.AB5
	}

	if strings.EqualFold(shape, d2target.ShapePerson) {
		return color.B3
	}
	if strings.EqualFold(shape, d2target.ShapeDiamond) {
		return color.N4
	}
	if strings.EqualFold(shape, d2target.ShapeCloud) || strings.EqualFold(shape, d2target.ShapeCallout) {
		return color.N7
	}
	if strings.EqualFold(shape, d2target.ShapeQueue) || strings.EqualFold(shape, d2target.ShapeParallelogram) || strings.EqualFold(shape, d2target.ShapeHexagon) {
		return color.N5
	}

	return color.N7
}

func (obj *Object) GetStroke(dashGapSize interface{}) string {
	shape := obj.Shape.Value
	if strings.EqualFold(shape, d2target.ShapeCode) ||
		strings.EqualFold(shape, d2target.ShapeText) {
		return color.N1
	}
	if strings.EqualFold(shape, d2target.ShapeClass) ||
		strings.EqualFold(shape, d2target.ShapeSQLTable) {
		return color.N7
	}
	if dashGapSize != 0.0 {
		return color.B2
	}
	return color.B1
}

func (obj *Object) Level() ContainerLevel {
	if obj.Parent == nil {
		return ContainerLevel(obj.Graph.RootLevel)
	}
	return 1 + obj.Parent.Level()
}

func (obj *Object) IsContainer() bool {
	return len(obj.Children) > 0
}

func (obj *Object) HasOutsideBottomLabel() bool {
	if obj == nil {
		return false
	}
	switch obj.Shape.Value {
	case d2target.ShapeImage, d2target.ShapePerson:
		return true
	default:
		return false
	}
}

func (obj *Object) HasLabel() bool {
	if obj == nil {
		return false
	}
	switch obj.Shape.Value {
	case d2target.ShapeText, d2target.ShapeClass, d2target.ShapeSQLTable, d2target.ShapeCode:
		return false
	default:
		return obj.Label.Value != ""
	}
}

func (obj *Object) HasIcon() bool {
	return obj.Icon != nil && obj.Shape.Value != d2target.ShapeImage
}

func (obj *Object) AbsID() string {
	if obj.Parent != nil && obj.Parent.ID != "" {
		return obj.Parent.AbsID() + "." + obj.ID
	}
	return obj.ID
}

func (obj *Object) AbsIDArray() []string {
	if obj.Parent == nil {
		return nil
	}
	return append(obj.Parent.AbsIDArray(), obj.ID)
}

func (obj *Object) Text() *d2target.MText {
	isBold := !obj.IsContainer() && obj.Shape.Value != "text"
	isItalic := false
	if obj.Style.Bold != nil && obj.Style.Bold.Value == "true" {
		isBold = true
	}
	if obj.Style.Italic != nil && obj.Style.Italic.Value == "true" {
		isItalic = true
	}
	fontSize := d2fonts.FONT_SIZE_M

	if obj.Class != nil || obj.SQLTable != nil {
		fontSize = d2fonts.FONT_SIZE_L
	}

	if obj.OuterSequenceDiagram() == nil {
		// Note: during grid layout when children are temporarily removed `IsContainer` is false
		if (obj.IsContainer() || obj.IsGridDiagram()) && obj.Shape.Value != "text" {
			fontSize = obj.Level().LabelSize()
		}
	} else {
		isBold = false
	}
	if obj.Style.FontSize != nil {
		fontSize, _ = strconv.Atoi(obj.Style.FontSize.Value)
	}
	// Class and Table objects have Label set to header
	if obj.Class != nil || obj.SQLTable != nil {
		fontSize += d2target.HeaderFontAdd
	}
	if obj.Class != nil {
		isBold = false
	}
	return &d2target.MText{
		Text:     obj.Label.Value,
		FontSize: fontSize,
		IsBold:   isBold,
		IsItalic: isItalic,
		Language: obj.Language,
		Shape:    obj.Shape.Value,

		Dimensions: obj.LabelDimensions,
	}
}

func (obj *Object) newObject(id string) *Object {
	idval := id
	k, _ := d2parser.ParseKey(id)
	if k != nil && len(k.Path) > 0 {
		idval = k.Path[0].Unbox().ScalarString()
	}
	child := &Object{
		ID:    id,
		IDVal: idval,
		Attributes: Attributes{
			Label: Scalar{
				Value: idval,
			},
			Shape: Scalar{
				Value: d2target.ShapeRectangle,
			},
		},

		Graph:  obj.Graph,
		Parent: obj,

		Children: make(map[string]*Object),
	}

	obj.Children[strings.ToLower(id)] = child
	obj.ChildrenArray = append(obj.ChildrenArray, child)

	if obj.Graph != nil {
		obj.Graph.Objects = append(obj.Graph.Objects, child)
	}

	return child
}

func (obj *Object) HasChild(ids []string) (*Object, bool) {
	if len(ids) == 0 {
		return obj, true
	}
	if len(ids) == 1 && ids[0] != "style" {
		_, ok := d2ast.ReservedKeywords[ids[0]]
		if ok {
			return obj, true
		}
	}

	id := ids[0]
	ids = ids[1:]

	child, ok := obj.Children[strings.ToLower(id)]
	if !ok {
		return nil, false
	}

	if len(ids) >= 1 {
		return child.HasChild(ids)
	}
	return child, true
}

func (obj *Object) HasEdge(mk *d2ast.Key) (*Edge, bool) {
	ea, ok := obj.FindEdges(mk)
	if !ok {
		return nil, false
	}
	for _, e := range ea {
		if e.Index == *mk.EdgeIndex.Int {
			return e, true
		}
	}
	return nil, false
}

// TODO: remove once not used anywhere
func ResolveUnderscoreKey(ida []string, obj *Object) (resolvedObj *Object, resolvedIDA []string, _ error) {
	if len(ida) > 0 && !obj.IsSequenceDiagram() {
		objSD := obj.OuterSequenceDiagram()
		if objSD != nil {
			referencesActor := false
			for _, c := range objSD.ChildrenArray {
				if c.ID == ida[0] {
					referencesActor = true
					break
				}
			}
			if referencesActor {
				obj = objSD
			}
		}
	}

	resolvedObj = obj
	resolvedIDA = ida

	for i, id := range ida {
		if id != "_" {
			continue
		}
		if i != 0 && ida[i-1] != "_" {
			return nil, nil, errors.New(`parent "_" can only be used in the beginning of paths, e.g. "_.x"`)
		}
		if resolvedObj == obj.Graph.Root {
			return nil, nil, errors.New(`parent "_" cannot be used in the root scope`)
		}
		if i == len(ida)-1 {
			return nil, nil, errors.New(`invalid use of parent "_"`)
		}
		resolvedObj = resolvedObj.Parent
		resolvedIDA = resolvedIDA[1:]
	}

	return resolvedObj, resolvedIDA, nil
}

// TODO: remove edges []edge and scope each edge inside Object.
func (obj *Object) FindEdges(mk *d2ast.Key) ([]*Edge, bool) {
	if len(mk.Edges) != 1 {
		return nil, false
	}
	if mk.EdgeIndex.Int == nil {
		return nil, false
	}
	ae := mk.Edges[0]

	srcObj, srcID, err := ResolveUnderscoreKey(Key(ae.Src), obj)
	if err != nil {
		return nil, false
	}
	dstObj, dstID, err := ResolveUnderscoreKey(Key(ae.Dst), obj)
	if err != nil {
		return nil, false
	}

	src := strings.Join(srcID, ".")
	dst := strings.Join(dstID, ".")
	if srcObj.Parent != nil {
		src = srcObj.AbsID() + "." + src
	}
	if dstObj.Parent != nil {
		dst = dstObj.AbsID() + "." + dst
	}

	var ea []*Edge
	for _, e := range obj.Graph.Edges {
		if strings.EqualFold(src, e.Src.AbsID()) &&
			((ae.SrcArrow == "<" && e.SrcArrow) || (ae.SrcArrow == "" && !e.SrcArrow)) &&
			strings.EqualFold(dst, e.Dst.AbsID()) &&
			((ae.DstArrow == ">" && e.DstArrow) || (ae.DstArrow == "" && !e.DstArrow)) {
			ea = append(ea, e)
		}
	}
	return ea, true
}

func (obj *Object) ensureChildEdge(ida []string) *Object {
	for i := range ida {
		switch obj.Shape.Value {
		case d2target.ShapeClass, d2target.ShapeSQLTable:
			// This will only be called for connecting edges where we want to truncate to the
			// container.
			return obj
		default:
			obj = obj.EnsureChild(ida[i : i+1])
		}
	}
	return obj
}

// EnsureChild grabs the child by ids or creates it if it does not exist including all
// intermediate nodes.
func (obj *Object) EnsureChild(ida []string) *Object {
	seq := obj.OuterSequenceDiagram()
	if seq != nil {
		for _, c := range seq.ChildrenArray {
			if c.ID == ida[0] {
				if obj.ID == ida[0] {
					// In cases of a.a where EnsureChild is called on the parent a, the second a should
					// be created as a child of a and not as a child of the diagram. This is super
					// unfortunate code but alas.
					break
				}
				obj = seq
				break
			}
		}
	}

	if len(ida) == 0 {
		return obj
	}

	_, is := d2ast.ReservedKeywordHolders[ida[0]]
	if len(ida) == 1 && !is {
		_, ok := d2ast.ReservedKeywords[ida[0]]
		if ok {
			return obj
		}
	}

	id := ida[0]
	ida = ida[1:]

	if id == "_" {
		return obj.Parent.EnsureChild(ida)
	}

	child, ok := obj.Children[strings.ToLower(id)]
	if !ok {
		child = obj.newObject(id)
	}

	if len(ida) >= 1 {
		return child.EnsureChild(ida)
	}
	return child
}

func (obj *Object) AppendReferences(ida []string, ref Reference, unresolvedObj *Object) {
	ref.ScopeObj = unresolvedObj
	numUnderscores := 0
	for i := range ida {
		if ida[i] == "_" {
			numUnderscores++
			continue
		}
		child, ok := obj.HasChild(ida[numUnderscores : i+1])
		if !ok {
			return
		}
		ref.KeyPathIndex = i
		child.References = append(child.References, ref)
	}
}

func (obj *Object) GetLabelSize(mtexts []*d2target.MText, ruler *textmeasure.Ruler, fontFamily *d2fonts.FontFamily) (*d2target.TextDimensions, error) {
	shapeType := strings.ToLower(obj.Shape.Value)

	if obj.Style.Font != nil {
		f := d2fonts.D2_FONT_TO_FAMILY[obj.Style.Font.Value]
		fontFamily = &f
	}

	var dims *d2target.TextDimensions
	switch shapeType {
	case d2target.ShapeText:
		if obj.Language == "latex" {
			width, height, err := d2latex.Measure(obj.Text().Text)
			if err != nil {
				return nil, err
			}
			dims = d2target.NewTextDimensions(width, height)
		} else if obj.Language != "" {
			var err error
			dims, err = getMarkdownDimensions(mtexts, ruler, obj.Text(), fontFamily)
			if err != nil {
				return nil, err
			}
		} else {
			dims = GetTextDimensions(mtexts, ruler, obj.Text(), fontFamily)
		}

	case d2target.ShapeClass:
		dims = GetTextDimensions(mtexts, ruler, obj.Text(), go2.Pointer(d2fonts.SourceCodePro))

	default:
		dims = GetTextDimensions(mtexts, ruler, obj.Text(), fontFamily)
	}

	if shapeType == d2target.ShapeSQLTable && obj.Label.Value == "" {
		// measure with placeholder text to determine height
		placeholder := *obj.Text()
		placeholder.Text = "Table"
		dims = GetTextDimensions(mtexts, ruler, &placeholder, fontFamily)
	}

	if dims == nil {
		if obj.Text().Text == "" {
			return d2target.NewTextDimensions(0, 0), nil
		}
		if shapeType == d2target.ShapeImage {
			dims = d2target.NewTextDimensions(0, 0)
		} else {
			return nil, fmt.Errorf("dimensions for object label %#v not found", obj.Text())
		}
	}

	return dims, nil
}

func (obj *Object) GetDefaultSize(mtexts []*d2target.MText, ruler *textmeasure.Ruler, fontFamily *d2fonts.FontFamily, labelDims d2target.TextDimensions, withLabelPadding bool) (*d2target.TextDimensions, error) {
	dims := d2target.TextDimensions{}
	dslShape := strings.ToLower(obj.Shape.Value)

	if dslShape == d2target.ShapeCode {
		fontSize := obj.Text().FontSize
		// 0.5em padding on each side
		labelDims.Width += fontSize
		labelDims.Height += fontSize
	} else if withLabelPadding {
		labelDims.Width += INNER_LABEL_PADDING
		labelDims.Height += INNER_LABEL_PADDING
	}

	switch dslShape {
	default:
		return d2target.NewTextDimensions(labelDims.Width, labelDims.Height), nil
	case d2target.ShapeText:
		w := labelDims.Width
		if w < MIN_SHAPE_SIZE {
			w = MIN_SHAPE_SIZE
		}
		h := labelDims.Height
		if h < MIN_SHAPE_SIZE {
			h = MIN_SHAPE_SIZE
		}
		return d2target.NewTextDimensions(w, h), nil

	case d2target.ShapeImage:
		return d2target.NewTextDimensions(128, 128), nil

	case d2target.ShapeClass:
		maxWidth := go2.Max(12, labelDims.Width)

		fontSize := d2fonts.FONT_SIZE_L
		if obj.Style.FontSize != nil {
			fontSize, _ = strconv.Atoi(obj.Style.FontSize.Value)
		}

		for _, f := range obj.Class.Fields {
			fdims := GetTextDimensions(mtexts, ruler, f.Text(fontSize), go2.Pointer(d2fonts.SourceCodePro))
			if fdims == nil {
				return nil, fmt.Errorf("dimensions for class field %#v not found", f.Text(fontSize))
			}
			maxWidth = go2.Max(maxWidth, fdims.Width)
		}
		for _, m := range obj.Class.Methods {
			mdims := GetTextDimensions(mtexts, ruler, m.Text(fontSize), go2.Pointer(d2fonts.SourceCodePro))
			if mdims == nil {
				return nil, fmt.Errorf("dimensions for class method %#v not found", m.Text(fontSize))
			}
			maxWidth = go2.Max(maxWidth, mdims.Width)
		}
		//    ┌─PrefixWidth ┌─CenterPadding
		// ┌─┬─┬───────┬──────┬───┬──┐
		// │ + getJobs()      Job[]  │
		// └─┴─┴───────┴──────┴───┴──┘
		//  └─PrefixPadding        └──TypePadding
		//     ├───────┤   +  ├───┤  = maxWidth
		dims.Width = d2target.PrefixPadding + d2target.PrefixWidth + maxWidth + d2target.CenterPadding + d2target.TypePadding

		// All rows should be the same height
		var anyRowText *d2target.MText
		if len(obj.Class.Fields) > 0 {
			anyRowText = obj.Class.Fields[0].Text(fontSize)
		} else if len(obj.Class.Methods) > 0 {
			anyRowText = obj.Class.Methods[0].Text(fontSize)
		}
		if anyRowText != nil {
			rowHeight := GetTextDimensions(mtexts, ruler, anyRowText, go2.Pointer(d2fonts.SourceCodePro)).Height + d2target.VerticalPadding
			dims.Height = rowHeight*(len(obj.Class.Fields)+len(obj.Class.Methods)) + go2.Max(2*rowHeight, labelDims.Height+2*label.PADDING)
		} else {
			dims.Height = 2*go2.Max(12, labelDims.Height) + d2target.VerticalPadding
		}

	case d2target.ShapeSQLTable:
		maxNameWidth := 0
		maxTypeWidth := 0
		maxConstraintWidth := 0

		colFontSize := d2fonts.FONT_SIZE_L
		if obj.Style.FontSize != nil {
			colFontSize, _ = strconv.Atoi(obj.Style.FontSize.Value)
		}

		for i := range obj.SQLTable.Columns {
			// Note: we want to set dimensions of actual column not the for loop copy of the struct
			c := &obj.SQLTable.Columns[i]

			ctexts := c.Texts(colFontSize)

			nameDims := GetTextDimensions(mtexts, ruler, ctexts[0], fontFamily)
			if nameDims == nil {
				return nil, fmt.Errorf("dimensions for sql_table name %#v not found", ctexts[0].Text)
			}
			c.Name.LabelWidth = nameDims.Width
			c.Name.LabelHeight = nameDims.Height
			maxNameWidth = go2.Max(maxNameWidth, nameDims.Width)

			typeDims := GetTextDimensions(mtexts, ruler, ctexts[1], fontFamily)
			if typeDims == nil {
				return nil, fmt.Errorf("dimensions for sql_table type %#v not found", ctexts[1].Text)
			}
			c.Type.LabelWidth = typeDims.Width
			c.Type.LabelHeight = typeDims.Height
			maxTypeWidth = go2.Max(maxTypeWidth, typeDims.Width)

			if l := len(c.Constraint); l > 0 {
				constraintDims := GetTextDimensions(mtexts, ruler, ctexts[2], fontFamily)
				if constraintDims == nil {
					return nil, fmt.Errorf("dimensions for sql_table constraint %#v not found", ctexts[2].Text)
				}
				maxConstraintWidth = go2.Max(maxConstraintWidth, constraintDims.Width)
			}
		}

		// The rows get padded a little due to header font being larger than row font
		dims.Height = go2.Max(12, labelDims.Height*(len(obj.SQLTable.Columns)+1))
		headerWidth := d2target.HeaderPadding + labelDims.Width + d2target.HeaderPadding
		rowsWidth := d2target.NamePadding + maxNameWidth + d2target.TypePadding + maxTypeWidth + d2target.TypePadding + maxConstraintWidth
		if maxConstraintWidth != 0 {
			rowsWidth += d2target.ConstraintPadding
		}
		dims.Width = go2.Max(12, go2.Max(headerWidth, rowsWidth))
	}

	return &dims, nil
}

// resizes the object to fit content of the given width and height in its inner box with the given padding.
// this accounts for the shape of the object, and if there is a desired width or height set for the object
func (obj *Object) SizeToContent(contentWidth, contentHeight, paddingX, paddingY float64) {
	dslShape := strings.ToLower(obj.Shape.Value)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]
	s := shape.NewShape(shapeType, geo.NewBox(geo.NewPoint(0, 0), contentWidth, contentHeight))

	var fitWidth, fitHeight float64
	if shapeType == shape.PERSON_TYPE {
		fitWidth = contentWidth + paddingX
		fitHeight = contentHeight + paddingY
	} else {
		fitWidth, fitHeight = s.GetDimensionsToFit(contentWidth, contentHeight, paddingX, paddingY)
	}

	var desiredWidth int
	if obj.WidthAttr != nil {
		desiredWidth, _ = strconv.Atoi(obj.WidthAttr.Value)
		obj.Width = float64(desiredWidth)
	} else {
		obj.Width = fitWidth
	}

	var desiredHeight int
	if obj.HeightAttr != nil {
		desiredHeight, _ = strconv.Atoi(obj.HeightAttr.Value)
		obj.Height = float64(desiredHeight)
	} else {
		obj.Height = fitHeight
	}

	if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
		obj.Width = math.Max(float64(desiredWidth), fitWidth)
		obj.Height = math.Max(float64(desiredHeight), fitHeight)
	}

	if s.AspectRatio1() {
		sideLength := math.Max(obj.Width, obj.Height)
		obj.Width = sideLength
		obj.Height = sideLength
	} else if desiredHeight == 0 || desiredWidth == 0 {
		switch shapeType {
		case shape.PERSON_TYPE:
			obj.Width, obj.Height = shape.LimitAR(obj.Width, obj.Height, shape.PERSON_AR_LIMIT)
		case shape.OVAL_TYPE:
			obj.Width, obj.Height = shape.LimitAR(obj.Width, obj.Height, shape.OVAL_AR_LIMIT)
		}
	}
	if shapeType == shape.CLOUD_TYPE {
		innerBox := s.GetInnerBoxForContent(contentWidth, contentHeight)
		obj.ContentAspectRatio = go2.Pointer(innerBox.Width / innerBox.Height)
	}
}

func (obj *Object) OuterNearContainer() *Object {
	for obj != nil {
		if obj.NearKey != nil {
			return obj
		}
		obj = obj.Parent
	}
	return nil
}

func (obj *Object) IsConstantNear() bool {
	if obj.NearKey == nil {
		return false
	}
	keyPath := Key(obj.NearKey)

	// interesting if there is a shape with id=top-left, then top-left isn't treated a constant near
	_, isKey := obj.Graph.Root.HasChild(keyPath)
	if isKey {
		return false
	}
	_, isConst := d2ast.NearConstants[keyPath[0]]
	return isConst
}

type Edge struct {
	Index int `json:"index"`

	SrcTableColumnIndex *int `json:"srcTableColumnIndex,omitempty"`
	DstTableColumnIndex *int `json:"dstTableColumnIndex,omitempty"`

	LabelPosition   *string  `json:"labelPosition,omitempty"`
	LabelPercentage *float64 `json:"labelPercentage,omitempty"`

	IsCurve bool         `json:"isCurve"`
	Route   []*geo.Point `json:"route,omitempty"`

	Src          *Object     `json:"-"`
	SrcArrow     bool        `json:"src_arrow"`
	SrcArrowhead *Attributes `json:"srcArrowhead,omitempty"`
	Dst          *Object     `json:"-"`
	// TODO alixander (Mon Sep 12 2022): deprecate SrcArrow and DstArrow and just use SrcArrowhead and DstArrowhead
	DstArrow     bool        `json:"dst_arrow"`
	DstArrowhead *Attributes `json:"dstArrowhead,omitempty"`

	References []EdgeReference `json:"references,omitempty"`
	Attributes `json:"attributes,omitempty"`

	ZIndex int `json:"zIndex"`
}

type EdgeReference struct {
	Edge *d2ast.Edge `json:"-"`

	MapKey          *d2ast.Key `json:"-"`
	MapKeyEdgeIndex int        `json:"map_key_edge_index"`
	Scope           *d2ast.Map `json:"-"`
	ScopeObj        *Object    `json:"-"`
	ScopeAST        *d2ast.Map `json:"-"`
}

func (e *Edge) GetAstEdge() *d2ast.Edge {
	return e.References[0].Edge
}

func (e *Edge) GetStroke(dashGapSize interface{}) string {
	if dashGapSize != 0.0 {
		return color.B2
	}
	return color.B1
}

func (e *Edge) ArrowString() string {
	if e.SrcArrow && e.DstArrow {
		return "<->"
	}
	if e.SrcArrow {
		return "<-"
	}
	if e.DstArrow {
		return "->"
	}
	return "--"
}

func (e *Edge) Text() *d2target.MText {
	fontSize := d2fonts.FONT_SIZE_M
	if e.Style.FontSize != nil {
		fontSize, _ = strconv.Atoi(e.Style.FontSize.Value)
	}
	isBold := false
	if e.Style.Bold != nil {
		isBold, _ = strconv.ParseBool(e.Style.Bold.Value)
	}
	return &d2target.MText{
		Text:     e.Label.Value,
		FontSize: fontSize,
		IsBold:   isBold,
		IsItalic: true,

		Dimensions: e.LabelDimensions,
	}
}

func (e *Edge) Move(dx, dy float64) {
	for _, p := range e.Route {
		p.X += dx
		p.Y += dy
	}
}

func (e *Edge) AbsID() string {
	srcIDA := e.Src.AbsIDArray()
	dstIDA := e.Dst.AbsIDArray()

	var commonIDA []string
	for len(srcIDA) > 1 && len(dstIDA) > 1 {
		if !strings.EqualFold(srcIDA[0], dstIDA[0]) {
			break
		}
		commonIDA = append(commonIDA, srcIDA[0])
		srcIDA = srcIDA[1:]
		dstIDA = dstIDA[1:]
	}

	commonKey := ""
	if len(commonIDA) > 0 {
		commonKey = strings.Join(commonIDA, ".") + "."
	}

	return fmt.Sprintf("%s(%s %s %s)[%d]", commonKey, strings.Join(srcIDA, "."), e.ArrowString(), strings.Join(dstIDA, "."), e.Index)
}

func (obj *Object) Connect(srcID, dstID []string, srcArrow, dstArrow bool, label string) (*Edge, error) {
	for _, id := range [][]string{srcID, dstID} {
		for _, p := range id {
			if _, ok := d2ast.ReservedKeywords[p]; ok {
				return nil, errors.New("cannot connect to reserved keyword")
			}
		}
	}

	src := obj.ensureChildEdge(srcID)
	dst := obj.ensureChildEdge(dstID)

	e := &Edge{
		Attributes: Attributes{
			Label: Scalar{
				Value: label,
			},
		},
		Src:      src,
		SrcArrow: srcArrow,
		Dst:      dst,
		DstArrow: dstArrow,
	}
	e.initIndex()

	addSQLTableColumnIndices(e, srcID, dstID, obj, src, dst)

	obj.Graph.Edges = append(obj.Graph.Edges, e)
	return e, nil
}

func addSQLTableColumnIndices(e *Edge, srcID, dstID []string, obj, src, dst *Object) {
	if src.Shape.Value == d2target.ShapeSQLTable {
		if src == dst {
			// Ignore edge to column inside table.
			return
		}
		objAbsID := obj.AbsIDArray()
		srcAbsID := src.AbsIDArray()
		if len(objAbsID)+len(srcID) > len(srcAbsID) {
			for i, d2col := range src.SQLTable.Columns {
				if d2col.Name.Label == srcID[len(srcID)-1] {
					d2col.Reference = dst.AbsID()
					e.SrcTableColumnIndex = new(int)
					*e.SrcTableColumnIndex = i
					break
				}
			}
		}
	}
	if dst.Shape.Value == d2target.ShapeSQLTable {
		objAbsID := obj.AbsIDArray()
		dstAbsID := dst.AbsIDArray()
		if len(objAbsID)+len(dstID) > len(dstAbsID) {
			for i, d2col := range dst.SQLTable.Columns {
				if d2col.Name.Label == dstID[len(dstID)-1] {
					d2col.Reference = dst.AbsID()
					e.DstTableColumnIndex = new(int)
					*e.DstTableColumnIndex = i
					break
				}
			}
		}
	}
}

// TODO: Treat undirectional/bidirectional edge here and in HasEdge flipped. Same with
// SrcArrow.
func (e *Edge) initIndex() {
	for _, e2 := range e.Src.Graph.Edges {
		if e.Src == e2.Src &&
			e.SrcArrow == e2.SrcArrow &&
			e.Dst == e2.Dst &&
			e.DstArrow == e2.DstArrow {
			e.Index++
		}
	}
}

func findMeasured(mtexts []*d2target.MText, t1 *d2target.MText) *d2target.TextDimensions {
	for i, t2 := range mtexts {
		if t1.Text != t2.Text {
			continue
		}
		if t1.FontSize != t2.FontSize {
			continue
		}
		if t1.IsBold != t2.IsBold {
			continue
		}
		if t1.IsItalic != t2.IsItalic {
			continue
		}
		if t1.Language != t2.Language {
			continue
		}
		return &mtexts[i].Dimensions
	}
	return nil
}

func getMarkdownDimensions(mtexts []*d2target.MText, ruler *textmeasure.Ruler, t *d2target.MText, fontFamily *d2fonts.FontFamily) (*d2target.TextDimensions, error) {
	if dims := findMeasured(mtexts, t); dims != nil {
		return dims, nil
	}

	if ruler != nil {
		width, height, err := textmeasure.MeasureMarkdown(t.Text, ruler, fontFamily, t.FontSize)
		if err != nil {
			return nil, err
		}
		return d2target.NewTextDimensions(width, height), nil
	}

	if strings.TrimSpace(t.Text) == "" {
		return d2target.NewTextDimensions(1, 1), nil
	}

	return nil, fmt.Errorf("text not pre-measured and no ruler provided")
}

func GetTextDimensions(mtexts []*d2target.MText, ruler *textmeasure.Ruler, t *d2target.MText, fontFamily *d2fonts.FontFamily) *d2target.TextDimensions {
	if dims := findMeasured(mtexts, t); dims != nil {
		return dims
	}

	if ruler != nil {
		var w int
		var h int
		if t.Language != "" {
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = textmeasure.CODE_LINE_HEIGHT
			w, h = ruler.MeasureMono(d2fonts.SourceCodePro.Font(t.FontSize, d2fonts.FONT_STYLE_REGULAR), t.Text)
			ruler.LineHeightFactor = originalLineHeight

			// count empty leading and trailing lines since ruler will not be able to measure it
			lines := strings.Split(t.Text, "\n")
			hasLeading := false
			if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
				hasLeading = true
			}
			numTrailing := 0
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.TrimSpace(lines[i]) == "" {
					numTrailing++
				} else {
					break
				}
			}
			if hasLeading && numTrailing < len(lines) {
				h += t.FontSize
			}
			h += int(math.Ceil(textmeasure.CODE_LINE_HEIGHT * float64(t.FontSize*numTrailing)))
		} else {
			style := d2fonts.FONT_STYLE_REGULAR
			if t.IsBold {
				style = d2fonts.FONT_STYLE_BOLD
			} else if t.IsItalic {
				style = d2fonts.FONT_STYLE_ITALIC
			}
			if fontFamily == nil {
				fontFamily = go2.Pointer(d2fonts.SourceSansPro)
			}
			w, h = ruler.Measure(fontFamily.Font(t.FontSize, style), t.Text)
		}
		return d2target.NewTextDimensions(w, h)
	}

	return nil
}

func appendTextDedup(texts []*d2target.MText, t *d2target.MText) []*d2target.MText {
	if GetTextDimensions(texts, nil, t, nil) == nil {
		return append(texts, t)
	}
	return texts
}

func (g *Graph) SetDimensions(mtexts []*d2target.MText, ruler *textmeasure.Ruler, fontFamily *d2fonts.FontFamily) error {
	if ruler != nil && fontFamily != nil {
		if ok := ruler.HasFontFamilyLoaded(fontFamily); !ok {
			return fmt.Errorf("ruler does not have entire font family %s loaded, is a style missing?", *fontFamily)
		}
	}

	if g.Theme != nil && g.Theme.SpecialRules.Mono {
		tmp := d2fonts.SourceCodePro
		fontFamily = &tmp
	}

	for _, obj := range g.Objects {
		obj.Box = &geo.Box{}

		// user-specified label/icon positions
		if obj.HasLabel() && obj.Attributes.LabelPosition != nil {
			scalar := *obj.Attributes.LabelPosition
			position := d2ast.LabelPositionsMapping[scalar.Value]
			obj.LabelPosition = go2.Pointer(position.String())
		}
		if obj.Icon != nil && obj.Attributes.IconPosition != nil {
			scalar := *obj.Attributes.IconPosition
			position := d2ast.LabelPositionsMapping[scalar.Value]
			obj.IconPosition = go2.Pointer(position.String())
		}

		var desiredWidth int
		var desiredHeight int
		if obj.WidthAttr != nil {
			desiredWidth, _ = strconv.Atoi(obj.WidthAttr.Value)
		}
		if obj.HeightAttr != nil {
			desiredHeight, _ = strconv.Atoi(obj.HeightAttr.Value)
		}

		dslShape := strings.ToLower(obj.Shape.Value)

		if obj.Label.Value == "" &&
			dslShape != d2target.ShapeImage &&
			dslShape != d2target.ShapeSQLTable &&
			dslShape != d2target.ShapeClass {

			if dslShape == d2target.ShapeCircle || dslShape == d2target.ShapeSquare {
				sideLength := DEFAULT_SHAPE_SIZE
				if desiredWidth != 0 || desiredHeight != 0 {
					sideLength = float64(go2.Max(desiredWidth, desiredHeight))
				}
				obj.Width = sideLength
				obj.Height = sideLength
			} else {
				obj.Width = DEFAULT_SHAPE_SIZE
				obj.Height = DEFAULT_SHAPE_SIZE
				if desiredWidth != 0 {
					obj.Width = float64(desiredWidth)
				}
				if desiredHeight != 0 {
					obj.Height = float64(desiredHeight)
				}
			}

			continue
		}

		if g.Theme != nil && g.Theme.SpecialRules.CapsLock && !strings.EqualFold(obj.Shape.Value, d2target.ShapeCode) {
			if obj.Language != "latex" && !obj.Style.NoneTextTransform() {
				obj.Label.Value = strings.ToUpper(obj.Label.Value)
			}
		}
		obj.ApplyTextTransform()

		labelDims, err := obj.GetLabelSize(mtexts, ruler, fontFamily)
		if err != nil {
			return err
		}
		obj.LabelDimensions = *labelDims

		// if there is a desired width or height, fit to content box without inner label padding for smallest minimum size
		withInnerLabelPadding := desiredWidth == 0 && desiredHeight == 0 &&
			dslShape != d2target.ShapeText && obj.Label.Value != ""
		defaultDims, err := obj.GetDefaultSize(mtexts, ruler, fontFamily, *labelDims, withInnerLabelPadding)
		if err != nil {
			return err
		}

		if dslShape == d2target.ShapeImage {
			if desiredWidth == 0 {
				desiredWidth = defaultDims.Width
			}
			if desiredHeight == 0 {
				desiredHeight = defaultDims.Height
			}
			obj.Width = float64(go2.Max(MIN_SHAPE_SIZE, desiredWidth))
			obj.Height = float64(go2.Max(MIN_SHAPE_SIZE, desiredHeight))
			// images don't need further processing
			continue
		}

		contentBox := geo.NewBox(geo.NewPoint(0, 0), float64(defaultDims.Width), float64(defaultDims.Height))
		shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]
		s := shape.NewShape(shapeType, contentBox)
		paddingX, paddingY := s.GetDefaultPadding()
		if desiredWidth != 0 {
			paddingX = 0.
		}
		if desiredHeight != 0 {
			paddingY = 0.
		}

		// give shapes with icons extra padding to fit their label
		if obj.Icon != nil {
			switch shapeType {
			case shape.TABLE_TYPE, shape.CLASS_TYPE, shape.CODE_TYPE, shape.TEXT_TYPE:
			default:
				labelHeight := float64(labelDims.Height + INNER_LABEL_PADDING)
				// Evenly pad enough to fit label above icon
				if desiredWidth == 0 {
					paddingX += labelHeight
				}
				if desiredHeight == 0 {
					paddingY += labelHeight
				}
			}
		}
		if desiredWidth == 0 {
			switch shapeType {
			case shape.TABLE_TYPE, shape.CLASS_TYPE, shape.CODE_TYPE:
			default:
				if obj.Link != nil {
					paddingX += 32
				}
				if obj.Tooltip != nil {
					paddingX += 32
				}
			}
		}

		obj.SizeToContent(contentBox.Width, contentBox.Height, paddingX, paddingY)
	}
	for _, edge := range g.Edges {
		usedFont := fontFamily
		if edge.Style.Font != nil {
			f := d2fonts.D2_FONT_TO_FAMILY[edge.Style.Font.Value]
			usedFont = &f
		}

		if edge.SrcArrowhead != nil && edge.SrcArrowhead.Label.Value != "" {
			t := edge.Text()
			t.Text = edge.SrcArrowhead.Label.Value
			dims := GetTextDimensions(mtexts, ruler, t, usedFont)
			edge.SrcArrowhead.LabelDimensions = *dims
		}
		if edge.DstArrowhead != nil && edge.DstArrowhead.Label.Value != "" {
			t := edge.Text()
			t.Text = edge.DstArrowhead.Label.Value
			dims := GetTextDimensions(mtexts, ruler, t, usedFont)
			edge.DstArrowhead.LabelDimensions = *dims
		}

		if edge.Label.Value == "" {
			continue
		}

		if g.Theme != nil && g.Theme.SpecialRules.CapsLock && !edge.Style.NoneTextTransform() {
			edge.Label.Value = strings.ToUpper(edge.Label.Value)
		}
		edge.ApplyTextTransform()

		dims := GetTextDimensions(mtexts, ruler, edge.Text(), usedFont)
		if dims == nil {
			return fmt.Errorf("dimensions for edge label %#v not found", edge.Text())
		}

		edge.LabelDimensions = *dims
	}
	return nil
}

func (g *Graph) Texts() []*d2target.MText {
	var texts []*d2target.MText

	capsLock := g.Theme != nil && g.Theme.SpecialRules.CapsLock

	for _, obj := range g.Objects {
		if obj.Label.Value != "" {
			obj.ApplyTextTransform()
			text := obj.Text()
			if capsLock && !strings.EqualFold(obj.Shape.Value, d2target.ShapeCode) {
				if obj.Language != "latex" && !obj.Style.NoneTextTransform() {
					text.Text = strings.ToUpper(text.Text)
				}
			}
			texts = appendTextDedup(texts, text)
		}
		if obj.Class != nil {
			fontSize := d2fonts.FONT_SIZE_L
			if obj.Style.FontSize != nil {
				fontSize, _ = strconv.Atoi(obj.Style.FontSize.Value)
			}
			for _, field := range obj.Class.Fields {
				texts = appendTextDedup(texts, field.Text(fontSize))
			}
			for _, method := range obj.Class.Methods {
				texts = appendTextDedup(texts, method.Text(fontSize))
			}
		} else if obj.SQLTable != nil {
			colFontSize := d2fonts.FONT_SIZE_L
			if obj.Style.FontSize != nil {
				colFontSize, _ = strconv.Atoi(obj.Style.FontSize.Value)
			}
			for _, column := range obj.SQLTable.Columns {
				for _, t := range column.Texts(colFontSize) {
					texts = appendTextDedup(texts, t)
				}
			}
		}
	}
	for _, edge := range g.Edges {
		if edge.Label.Value != "" {
			edge.ApplyTextTransform()
			text := edge.Text()
			if capsLock && !edge.Style.NoneTextTransform() {
				text.Text = strings.ToUpper(text.Text)
			}
			texts = appendTextDedup(texts, text)
		}
		if edge.SrcArrowhead != nil && edge.SrcArrowhead.Label.Value != "" {
			t := edge.Text()
			t.Text = edge.SrcArrowhead.Label.Value
			texts = appendTextDedup(texts, t)
		}
		if edge.DstArrowhead != nil && edge.DstArrowhead.Label.Value != "" {
			t := edge.Text()
			t.Text = edge.DstArrowhead.Label.Value
			texts = appendTextDedup(texts, t)
		}
	}

	for _, board := range g.Layers {
		for _, t := range board.Texts() {
			texts = appendTextDedup(texts, t)
		}
	}

	for _, board := range g.Scenarios {
		for _, t := range board.Texts() {
			texts = appendTextDedup(texts, t)
		}
	}

	for _, board := range g.Steps {
		for _, t := range board.Texts() {
			texts = appendTextDedup(texts, t)
		}
	}

	return texts
}

func Key(k *d2ast.KeyPath) []string {
	return d2format.KeyPath(k)
}

func (g *Graph) GetBoard(name string) *Graph {
	for _, l := range g.Layers {
		if l.Name == name {
			return l
		}
	}
	for _, l := range g.Scenarios {
		if l.Name == name {
			return l
		}
	}
	for _, l := range g.Steps {
		if l.Name == name {
			return l
		}
	}
	return nil
}

func (g *Graph) SortObjectsByAST() {
	objects := append([]*Object(nil), g.Objects...)
	sort.Slice(objects, func(i, j int) bool {
		o1 := objects[i]
		o2 := objects[j]
		if len(o1.References) == 0 || len(o2.References) == 0 {
			return i < j
		}
		r1 := o1.References[0]
		r2 := o2.References[0]
		return r1.Key.Path[r1.KeyPathIndex].Unbox().GetRange().Before(r2.Key.Path[r2.KeyPathIndex].Unbox().GetRange())
	})
	g.Objects = objects
}

func (g *Graph) SortEdgesByAST() {
	edges := append([]*Edge(nil), g.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		e1 := edges[i]
		e2 := edges[j]
		if len(e1.References) == 0 || len(e2.References) == 0 {
			return i < j
		}
		return e1.References[0].Edge.Range.Before(e2.References[0].Edge.Range)
	})
	g.Edges = edges
}

func (obj *Object) IsDescendantOf(ancestor *Object) bool {
	if obj == ancestor {
		return true
	}
	if obj.Parent == nil {
		return false
	}
	return obj.Parent.IsDescendantOf(ancestor)
}

// ApplyTheme applies themes on the graph level
// This is different than on the render level, which only changes colors
// A theme applied on the graph level applies special rules that change the graph
func (g *Graph) ApplyTheme(themeID int64) error {
	theme := d2themescatalog.Find(themeID)
	if theme == (d2themes.Theme{}) {
		return fmt.Errorf("theme %d not found", themeID)
	}
	g.Theme = &theme
	return nil
}

func (g *Graph) PrintString() string {
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "Objects: [")
	for _, obj := range g.Objects {
		fmt.Fprintf(buf, "%v, ", obj.AbsID())
	}
	fmt.Fprint(buf, "]")
	return buf.String()
}

func (obj *Object) IterDescendants(apply func(parent, child *Object)) {
	for _, c := range obj.ChildrenArray {
		apply(obj, c)
		c.IterDescendants(apply)
	}
}

func (obj *Object) IsMultiple() bool {
	return obj.Style.Multiple != nil && obj.Style.Multiple.Value == "true"
}

func (obj *Object) Is3D() bool {
	return obj.Style.ThreeDee != nil && obj.Style.ThreeDee.Value == "true"
}

func (obj *Object) Spacing() (margin, padding geo.Spacing) {
	return obj.SpacingOpt(2*label.PADDING, 2*label.PADDING, true)
}

func (obj *Object) SpacingOpt(labelPadding, iconPadding float64, maxIconSize bool) (margin, padding geo.Spacing) {
	if obj.HasLabel() {
		var position label.Position
		if obj.LabelPosition != nil {
			position = label.FromString(*obj.LabelPosition)
		}

		var labelWidth, labelHeight float64
		if obj.LabelDimensions.Width > 0 {
			labelWidth = float64(obj.LabelDimensions.Width) + labelPadding
		}
		if obj.LabelDimensions.Height > 0 {
			labelHeight = float64(obj.LabelDimensions.Height) + labelPadding
		}

		switch position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			margin.Top = labelHeight
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			margin.Bottom = labelHeight
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			margin.Left = labelWidth
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			margin.Right = labelWidth
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			padding.Top = labelHeight
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			padding.Bottom = labelHeight
		case label.InsideMiddleLeft:
			padding.Left = labelWidth
		case label.InsideMiddleRight:
			padding.Right = labelWidth
		}
	}

	if obj.HasIcon() {
		var position label.Position
		if obj.IconPosition != nil {
			position = label.FromString(*obj.IconPosition)
		}

		iconSize := float64(d2target.MAX_ICON_SIZE + iconPadding)
		if !maxIconSize {
			iconSize = float64(d2target.GetIconSize(obj.Box, position.String())) + iconPadding
		}
		switch position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			margin.Top = math.Max(margin.Top, iconSize)
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			margin.Bottom = math.Max(margin.Bottom, iconSize)
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			margin.Left = math.Max(margin.Left, iconSize)
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			margin.Right = math.Max(margin.Right, iconSize)
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			padding.Top = math.Max(padding.Top, iconSize)
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			padding.Bottom = math.Max(padding.Bottom, iconSize)
		case label.InsideMiddleLeft:
			padding.Left = math.Max(padding.Left, iconSize)
		case label.InsideMiddleRight:
			padding.Right = math.Max(padding.Right, iconSize)
		}
	}

	dx, dy := obj.GetModifierElementAdjustments()
	margin.Right += dx
	margin.Top += dy

	return
}
