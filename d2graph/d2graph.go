package d2graph

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2latex"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const INNER_LABEL_PADDING int = 5
const DEFAULT_SHAPE_SIZE = 100.
const MIN_SHAPE_SIZE = 5

type Graph struct {
	Name string     `json:"name"`
	AST  *d2ast.Map `json:"ast"`

	Root    *Object   `json:"root"`
	Edges   []*Edge   `json:"edges"`
	Objects []*Object `json:"objects"`

	Layers    []*Graph `json:"layers,omitempty"`
	Scenarios []*Graph `json:"scenarios,omitempty"`
	Steps     []*Graph `json:"steps,omitempty"`
}

func NewGraph() *Graph {
	d := &Graph{}
	d.Root = &Object{
		Graph:      d,
		Parent:     nil,
		Children:   make(map[string]*Object),
		Attributes: &Attributes{},
	}
	return d
}

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
	ID              string                  `json:"id"`
	IDVal           string                  `json:"id_val"`
	Map             *d2ast.Map              `json:"-"`
	LabelDimensions d2target.TextDimensions `json:"label_dimensions"`
	References      []Reference             `json:"references,omitempty"`

	*geo.Box      `json:"box,omitempty"`
	LabelPosition *string `json:"labelPosition,omitempty"`
	LabelWidth    *int    `json:"labelWidth,omitempty"`
	LabelHeight   *int    `json:"labelHeight,omitempty"`
	IconPosition  *string `json:"iconPosition,omitempty"`

	Class    *d2target.Class    `json:"class,omitempty"`
	SQLTable *d2target.SQLTable `json:"sql_table,omitempty"`

	Children      map[string]*Object `json:"-"`
	ChildrenArray []*Object          `json:"-"`

	Attributes *Attributes `json:"attributes,omitempty"`

	ZIndex int `json:"zIndex"`
}

type Attributes struct {
	Label   Scalar   `json:"label"`
	Style   Style    `json:"style"`
	Icon    *url.URL `json:"icon,omitempty"`
	Tooltip string   `json:"tooltip,omitempty"`
	Link    string   `json:"link,omitempty"`

	// Only applicable for images right now
	Width  *Scalar `json:"width,omitempty"`
	Height *Scalar `json:"height,omitempty"`

	// TODO consider separate Attributes struct for shape-specific and edge-specific
	// Shapes only
	NearKey  *d2ast.KeyPath `json:"near_key"`
	Language string         `json:"language,omitempty"`
	// TODO: default to ShapeRectangle instead of empty string
	Shape Scalar `json:"shape"`

	Direction  Scalar `json:"direction"`
	Constraint Scalar `json:"constraint"`
}

// TODO references at the root scope should have their Scope set to root graph AST
type Reference struct {
	Key          *d2ast.KeyPath `json:"key"`
	KeyPathIndex int            `json:"key_path_index"`

	MapKey          *d2ast.Key `json:"-"`
	MapKeyEdgeIndex int        `json:"map_key_edge_index"`
	Scope           *d2ast.Map `json:"-"`
	ScopeObj        *Object    `json:"-"`
}

func (r Reference) MapKeyEdgeDest() bool {
	return r.Key == r.MapKey.Edges[r.MapKeyEdgeIndex].Dst
}

func (r Reference) InEdge() bool {
	return r.Key != r.MapKey.Key
}

type Style struct {
	Opacity      *Scalar `json:"opacity,omitempty"`
	Stroke       *Scalar `json:"stroke,omitempty"`
	Fill         *Scalar `json:"fill,omitempty"`
	StrokeWidth  *Scalar `json:"strokeWidth,omitempty"`
	StrokeDash   *Scalar `json:"strokeDash,omitempty"`
	BorderRadius *Scalar `json:"borderRadius,omitempty"`
	Shadow       *Scalar `json:"shadow,omitempty"`
	ThreeDee     *Scalar `json:"3d,omitempty"`
	Multiple     *Scalar `json:"multiple,omitempty"`
	Font         *Scalar `json:"font,omitempty"`
	FontSize     *Scalar `json:"fontSize,omitempty"`
	FontColor    *Scalar `json:"fontColor,omitempty"`
	Animated     *Scalar `json:"animated,omitempty"`
	Bold         *Scalar `json:"bold,omitempty"`
	Italic       *Scalar `json:"italic,omitempty"`
	Underline    *Scalar `json:"underline,omitempty"`
	Filled       *Scalar `json:"filled,omitempty"`
	DoubleBorder *Scalar `json:"doubleBorder,omitempty"`
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
		if !go2.Contains(namedColors, strings.ToLower(value)) && !colorHexRegex.MatchString(value) {
			return errors.New(`expected "stroke" to be a valid named color ("orange") or a hex code ("#f0ff3a")`)
		}
		s.Stroke.Value = value
	case "fill":
		if s.Fill == nil {
			break
		}
		if !go2.Contains(namedColors, strings.ToLower(value)) && !colorHexRegex.MatchString(value) {
			return errors.New(`expected "fill" to be a valid named color ("orange") or a hex code ("#f0ff3a")`)
		}
		s.Fill.Value = value
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
		if err != nil || (f < 0 || f > 20) {
			return errors.New(`expected "border-radius" to be a number between 0 and 20`)
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
		if !go2.Contains(systemFonts, strings.ToUpper(value)) {
			return fmt.Errorf(`"%v" is not a valid font in our system`, value)
		}
		s.Font.Value = strings.ToUpper(value)
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
		if !go2.Contains(namedColors, strings.ToLower(value)) && !colorHexRegex.MatchString(value) {
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

func (obj *Object) GetFill(theme *d2themes.Theme) string {
	level := int(obj.Level())
	if obj.IsSequenceDiagramNote() {
		return theme.Colors.Neutrals.N7
	} else if obj.IsSequenceDiagramGroup() {
		return theme.Colors.Neutrals.N5
	} else if obj.Parent.IsSequenceDiagram() {
		return theme.Colors.B5
	}

	// fill for spans
	sd := obj.OuterSequenceDiagram()
	if sd != nil {
		level -= int(sd.Level())
		if level == 1 {
			return theme.Colors.B3
		} else if level == 2 {
			return theme.Colors.B4
		} else if level == 3 {
			return theme.Colors.B5
		} else if level == 4 {
			return theme.Colors.Neutrals.N6
		}
		return theme.Colors.Neutrals.N7
	}

	if obj.IsSequenceDiagram() {
		return theme.Colors.Neutrals.N7
	}

	shape := obj.Attributes.Shape.Value

	if shape == "" || strings.EqualFold(shape, d2target.ShapeSquare) || strings.EqualFold(shape, d2target.ShapeCircle) || strings.EqualFold(shape, d2target.ShapeOval) || strings.EqualFold(shape, d2target.ShapeRectangle) {
		if level == 1 {
			if !obj.IsContainer() {
				return theme.Colors.B6
			}
			return theme.Colors.B4
		} else if level == 2 {
			return theme.Colors.B5
		} else if level == 3 {
			return theme.Colors.B6
		}
		return theme.Colors.Neutrals.N7
	}

	if strings.EqualFold(shape, d2target.ShapeCylinder) || strings.EqualFold(shape, d2target.ShapeStoredData) || strings.EqualFold(shape, d2target.ShapePackage) {
		if level == 1 {
			return theme.Colors.AA4
		}
		return theme.Colors.AA5
	}

	if strings.EqualFold(shape, d2target.ShapeStep) || strings.EqualFold(shape, d2target.ShapePage) || strings.EqualFold(shape, d2target.ShapeDocument) {
		if level == 1 {
			return theme.Colors.AB4
		}
		return theme.Colors.AB5
	}

	if strings.EqualFold(shape, d2target.ShapePerson) {
		return theme.Colors.B3
	}
	if strings.EqualFold(shape, d2target.ShapeDiamond) {
		return theme.Colors.Neutrals.N4
	}
	if strings.EqualFold(shape, d2target.ShapeCloud) || strings.EqualFold(shape, d2target.ShapeCallout) {
		return theme.Colors.Neutrals.N7
	}
	if strings.EqualFold(shape, d2target.ShapeQueue) || strings.EqualFold(shape, d2target.ShapeParallelogram) || strings.EqualFold(shape, d2target.ShapeHexagon) {
		return theme.Colors.Neutrals.N5
	}

	if strings.EqualFold(shape, d2target.ShapeSQLTable) || strings.EqualFold(shape, d2target.ShapeClass) {
		return theme.Colors.Neutrals.N1
	}

	return theme.Colors.Neutrals.N7
}

func (obj *Object) GetStroke(theme *d2themes.Theme, dashGapSize interface{}) string {
	shape := obj.Attributes.Shape.Value
	if strings.EqualFold(shape, d2target.ShapeCode) ||
		strings.EqualFold(shape, d2target.ShapeText) {
		return theme.Colors.Neutrals.N1
	}
	if strings.EqualFold(shape, d2target.ShapeClass) ||
		strings.EqualFold(shape, d2target.ShapeSQLTable) {
		return theme.Colors.Neutrals.N7
	}
	if dashGapSize != 0.0 {
		return theme.Colors.B2
	}
	return theme.Colors.B1
}

func (obj *Object) Level() ContainerLevel {
	if obj.Parent == nil {
		return 0
	}
	return 1 + obj.Parent.Level()
}

func (obj *Object) IsContainer() bool {
	return len(obj.Children) > 0
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
	isBold := !obj.IsContainer() && obj.Attributes.Shape.Value != "text"
	isItalic := false
	if obj.Attributes.Style.Bold != nil && obj.Attributes.Style.Bold.Value == "true" {
		isBold = true
	}
	if obj.Attributes.Style.Italic != nil && obj.Attributes.Style.Italic.Value == "true" {
		isItalic = true
	}
	fontSize := d2fonts.FONT_SIZE_M

	if obj.Class != nil || obj.SQLTable != nil {
		fontSize = d2fonts.FONT_SIZE_L
	}

	if obj.OuterSequenceDiagram() == nil {
		if obj.IsContainer() {
			fontSize = obj.Level().LabelSize()
		}
	} else {
		isBold = false
	}
	if obj.Attributes.Style.FontSize != nil {
		fontSize, _ = strconv.Atoi(obj.Attributes.Style.FontSize.Value)
	}
	// Class and Table objects have Label set to header
	if obj.Class != nil || obj.SQLTable != nil {
		fontSize += d2target.HeaderFontAdd
	}
	if obj.Class != nil {
		isBold = false
	}
	return &d2target.MText{
		Text:     obj.Attributes.Label.Value,
		FontSize: fontSize,
		IsBold:   isBold,
		IsItalic: isItalic,
		Language: obj.Attributes.Language,
		Shape:    obj.Attributes.Shape.Value,

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
		Attributes: &Attributes{
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
		_, ok := ReservedKeywords[ids[0]]
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
		switch obj.Attributes.Shape.Value {
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

	_, is := ReservedKeywordHolders[ida[0]]
	if len(ida) == 1 && !is {
		_, ok := ReservedKeywords[ida[0]]
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
	shapeType := strings.ToLower(obj.Attributes.Shape.Value)

	var dims *d2target.TextDimensions
	switch shapeType {
	case d2target.ShapeText:
		if obj.Attributes.Language == "latex" {
			width, height, err := d2latex.Measure(obj.Text().Text)
			if err != nil {
				return nil, err
			}
			dims = d2target.NewTextDimensions(width, height)
		} else if obj.Attributes.Language != "" {
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

	if shapeType == d2target.ShapeSQLTable && obj.Attributes.Label.Value == "" {
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

	if withLabelPadding {
		labelDims.Width += INNER_LABEL_PADDING
		labelDims.Height += INNER_LABEL_PADDING
	}

	switch strings.ToLower(obj.Attributes.Shape.Value) {
	default:
		return d2target.NewTextDimensions(labelDims.Width, labelDims.Height), nil

	case d2target.ShapeImage:
		return d2target.NewTextDimensions(128, 128), nil

	case d2target.ShapeClass:
		maxWidth := go2.Max(12, labelDims.Width)

		fontSize := d2fonts.FONT_SIZE_L
		if obj.Attributes.Style.FontSize != nil {
			fontSize, _ = strconv.Atoi(obj.Attributes.Style.FontSize.Value)
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
			dims.Height = rowHeight * (len(obj.Class.Fields) + len(obj.Class.Methods) + 2)
		} else {
			dims.Height = 2*go2.Max(12, labelDims.Height) + d2target.VerticalPadding
		}

	case d2target.ShapeSQLTable:
		maxNameWidth := 0
		maxTypeWidth := 0
		constraintWidth := 0

		colFontSize := d2fonts.FONT_SIZE_L
		if obj.Attributes.Style.FontSize != nil {
			colFontSize, _ = strconv.Atoi(obj.Attributes.Style.FontSize.Value)
		}

		for i := range obj.SQLTable.Columns {
			// Note: we want to set dimensions of actual column not the for loop copy of the struct
			c := &obj.SQLTable.Columns[i]

			ctexts := c.Texts(colFontSize)

			nameDims := GetTextDimensions(mtexts, ruler, ctexts[0], fontFamily)
			if nameDims == nil {
				return nil, fmt.Errorf("dimensions for sql_table name %#v not found", ctexts[0].Text)
			}
			c.Label.LabelWidth = nameDims.Width
			c.Label.LabelHeight = nameDims.Height
			maxNameWidth = go2.Max(maxNameWidth, nameDims.Width)

			typeDims := GetTextDimensions(mtexts, ruler, ctexts[1], fontFamily)
			if typeDims == nil {
				return nil, fmt.Errorf("dimensions for sql_table type %#v not found", ctexts[1].Text)
			}
			c.Type.LabelWidth = typeDims.Width
			c.Type.LabelHeight = typeDims.Height
			if maxTypeWidth < typeDims.Width {
				maxTypeWidth = typeDims.Width
			}
			maxTypeWidth = go2.Max(maxTypeWidth, typeDims.Width)

			if c.Constraint != "" {
				// covers UNQ constraint with padding
				constraintWidth = 60
			}
		}

		// The rows get padded a little due to header font being larger than row font
		dims.Height = go2.Max(12, labelDims.Height*(len(obj.SQLTable.Columns)+1))
		headerWidth := d2target.HeaderPadding + labelDims.Width + d2target.HeaderPadding
		rowsWidth := d2target.NamePadding + maxNameWidth + d2target.TypePadding + maxTypeWidth + d2target.TypePadding + constraintWidth
		dims.Width = go2.Max(12, go2.Max(headerWidth, rowsWidth))
	}

	return &dims, nil
}

type Edge struct {
	Index int `json:"index"`

	MinWidth  int `json:"minWidth"`
	MinHeight int `json:"minHeight"`

	SrcTableColumnIndex *int `json:"srcTableColumnIndex,omitempty"`
	DstTableColumnIndex *int `json:"dstTableColumnIndex,omitempty"`

	LabelDimensions d2target.TextDimensions `json:"label_dimensions"`
	LabelPosition   *string                 `json:"labelPosition,omitempty"`
	LabelPercentage *float64                `json:"labelPercentage,omitempty"`

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
	Attributes *Attributes     `json:"attributes,omitempty"`

	ZIndex int `json:"zIndex"`
}

type EdgeReference struct {
	Edge *d2ast.Edge `json:"-"`

	MapKey          *d2ast.Key `json:"-"`
	MapKeyEdgeIndex int        `json:"map_key_edge_index"`
	Scope           *d2ast.Map `json:"-"`
	ScopeObj        *Object    `json:"-"`
}

func (e *Edge) GetStroke(theme *d2themes.Theme, dashGapSize interface{}) string {
	if dashGapSize != 0.0 {
		return theme.Colors.B2
	}
	return theme.Colors.B1
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
	if e.Attributes.Style.FontSize != nil {
		fontSize, _ = strconv.Atoi(e.Attributes.Style.FontSize.Value)
	}
	return &d2target.MText{
		Text:     e.Attributes.Label.Value,
		FontSize: fontSize,
		IsBold:   false,
		IsItalic: true,

		Dimensions: e.LabelDimensions,
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
			if _, ok := ReservedKeywords[p]; ok {
				return nil, errors.New("cannot connect to reserved keyword")
			}
		}
	}

	src := obj.ensureChildEdge(srcID)
	dst := obj.ensureChildEdge(dstID)

	if src.OuterSequenceDiagram() != dst.OuterSequenceDiagram() {
		return nil, errors.New("connections within sequence diagrams can connect only to other objects within the same sequence diagram")
	}

	e := &Edge{
		Attributes: &Attributes{
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
	if src.Attributes.Shape.Value == d2target.ShapeSQLTable {
		if src == dst {
			// Ignore edge to column inside table.
			return
		}
		objAbsID := obj.AbsIDArray()
		srcAbsID := src.AbsIDArray()
		if len(objAbsID)+len(srcID) > len(srcAbsID) {
			for i, d2col := range src.SQLTable.Columns {
				if d2col.Name == srcID[len(srcID)-1] {
					d2col.Reference = dst.AbsID()
					e.SrcTableColumnIndex = new(int)
					*e.SrcTableColumnIndex = i
					break
				}
			}
		}
	}
	if dst.Attributes.Shape.Value == d2target.ShapeSQLTable {
		objAbsID := obj.AbsIDArray()
		dstAbsID := dst.AbsIDArray()
		if len(objAbsID)+len(dstID) > len(dstAbsID) {
			for i, d2col := range dst.SQLTable.Columns {
				if d2col.Name == dstID[len(dstID)-1] {
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
		width, height, err := textmeasure.MeasureMarkdown(t.Text, ruler, fontFamily)
		if err != nil {
			return nil, err
		}
		return d2target.NewTextDimensions(width, height), nil
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
			w, h = ruler.Measure(d2fonts.SourceCodePro.Font(t.FontSize, d2fonts.FONT_STYLE_REGULAR), t.Text)

			// count empty leading and trailing lines since ruler will not be able to measure it
			lines := strings.Split(t.Text, "\n")
			leadingLines := 0
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					leadingLines++
				} else {
					break
				}
			}
			trailingLines := 0
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.TrimSpace(lines[i]) == "" {
					trailingLines++
				} else {
					break
				}
			}
			h += t.FontSize * (leadingLines + trailingLines)

			// padding
			w += 12
			h += 12
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

	for _, obj := range g.Objects {
		obj.Box = &geo.Box{}

		var desiredWidth int
		var desiredHeight int
		if obj.Attributes.Width != nil {
			desiredWidth, _ = strconv.Atoi(obj.Attributes.Width.Value)
		}
		if obj.Attributes.Height != nil {
			desiredHeight, _ = strconv.Atoi(obj.Attributes.Height.Value)
		}

		dslShape := strings.ToLower(obj.Attributes.Shape.Value)

		if obj.Attributes.Label.Value == "" &&
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

		labelDims, err := obj.GetLabelSize(mtexts, ruler, fontFamily)
		if err != nil {
			return err
		}
		obj.LabelDimensions = *labelDims

		switch dslShape {
		case d2target.ShapeText, d2target.ShapeClass, d2target.ShapeSQLTable, d2target.ShapeCode:
			// no labels
		default:
			if obj.Attributes.Label.Value != "" {
				obj.LabelWidth = go2.Pointer(labelDims.Width)
				obj.LabelHeight = go2.Pointer(labelDims.Height)
			}
		}

		// if there is a desired width or height, fit to content box without inner label padding for smallest minimum size
		withInnerLabelPadding := desiredWidth == 0 && desiredHeight == 0 &&
			dslShape != d2target.ShapeText && obj.Attributes.Label.Value != ""
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
		if obj.Attributes.Icon != nil {
			labelHeight := float64(labelDims.Height + INNER_LABEL_PADDING)
			// Evenly pad enough to fit label above icon
			if desiredWidth == 0 {
				paddingX += labelHeight
			}
			if desiredHeight == 0 {
				paddingY += labelHeight
			}
		}
		if desiredWidth == 0 {
			switch shapeType {
			case shape.TABLE_TYPE, shape.CLASS_TYPE, shape.CODE_TYPE, shape.IMAGE_TYPE:
			default:
				if obj.Attributes.Link != "" {
					paddingX += 32
				}
				if obj.Attributes.Tooltip != "" {
					paddingX += 32
				}
			}
		}

		fitWidth, fitHeight := s.GetDimensionsToFit(contentBox.Width, contentBox.Height, paddingX, paddingY)
		obj.Width = math.Max(float64(desiredWidth), fitWidth)
		obj.Height = math.Max(float64(desiredHeight), fitHeight)
		if s.AspectRatio1() {
			sideLength := math.Max(obj.Width, obj.Height)
			obj.Width = sideLength
			obj.Height = sideLength
		} else if desiredHeight == 0 || desiredWidth == 0 {
			if s.GetType() == shape.PERSON_TYPE {
				obj.Width, obj.Height = shape.LimitAR(obj.Width, obj.Height, shape.PERSON_AR_LIMIT)
			}
		}
	}
	for _, edge := range g.Edges {
		endpointLabels := []string{}
		if edge.SrcArrowhead != nil && edge.SrcArrowhead.Label.Value != "" {
			endpointLabels = append(endpointLabels, edge.SrcArrowhead.Label.Value)
		}
		if edge.DstArrowhead != nil && edge.DstArrowhead.Label.Value != "" {
			endpointLabels = append(endpointLabels, edge.DstArrowhead.Label.Value)
		}

		for _, label := range endpointLabels {
			t := edge.Text()
			t.Text = label
			dims := GetTextDimensions(mtexts, ruler, t, fontFamily)
			edge.MinWidth += dims.Width
			// Some padding as it's not totally near the end
			edge.MinHeight += dims.Height + 5
		}

		if edge.Attributes.Label.Value == "" {
			continue
		}

		dims := GetTextDimensions(mtexts, ruler, edge.Text(), fontFamily)
		if dims == nil {
			return fmt.Errorf("dimensions for edge label %#v not found", edge.Text())
		}

		edge.LabelDimensions = *dims
		edge.MinWidth += dims.Width
		edge.MinHeight += dims.Height
	}
	return nil
}

func (g *Graph) Texts() []*d2target.MText {
	var texts []*d2target.MText

	for _, obj := range g.Objects {
		if obj.Attributes.Label.Value != "" {
			texts = appendTextDedup(texts, obj.Text())
		}
		if obj.Class != nil {
			fontSize := d2fonts.FONT_SIZE_L
			if obj.Attributes.Style.FontSize != nil {
				fontSize, _ = strconv.Atoi(obj.Attributes.Style.FontSize.Value)
			}
			for _, field := range obj.Class.Fields {
				texts = appendTextDedup(texts, field.Text(fontSize))
			}
			for _, method := range obj.Class.Methods {
				texts = appendTextDedup(texts, method.Text(fontSize))
			}
		} else if obj.SQLTable != nil {
			colFontSize := d2fonts.FONT_SIZE_L
			if obj.Attributes.Style.FontSize != nil {
				colFontSize, _ = strconv.Atoi(obj.Attributes.Style.FontSize.Value)
			}
			for _, column := range obj.SQLTable.Columns {
				for _, t := range column.Texts(colFontSize) {
					texts = appendTextDedup(texts, t)
				}
			}
		}
	}
	for _, edge := range g.Edges {
		if edge.Attributes.Label.Value != "" {
			texts = appendTextDedup(texts, edge.Text())
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

	return texts
}

func Key(k *d2ast.KeyPath) []string {
	return d2format.KeyPath(k)
}

// All reserved keywords. See init below.
var ReservedKeywords map[string]struct{}

// All reserved keywords not including style keywords.
var ReservedKeywords2 map[string]struct{}

// Non Style/Holder keywords.
var SimpleReservedKeywords = map[string]struct{}{
	"label":      {},
	"desc":       {},
	"shape":      {},
	"icon":       {},
	"constraint": {},
	"tooltip":    {},
	"link":       {},
	"near":       {},
	"width":      {},
	"height":     {},
	"direction":  {},
}

// ReservedKeywordHolders are reserved keywords that are meaningless on its own and exist solely to hold a set of reserved keywords
var ReservedKeywordHolders = map[string]struct{}{
	"style":            {},
	"source-arrowhead": {},
	"target-arrowhead": {},
}

// StyleKeywords are reserved keywords which cannot exist outside of the "style" keyword
var StyleKeywords = map[string]struct{}{
	"opacity":       {},
	"stroke":        {},
	"fill":          {},
	"stroke-width":  {},
	"stroke-dash":   {},
	"border-radius": {},

	// Only for text
	"font":       {},
	"font-size":  {},
	"font-color": {},
	"bold":       {},
	"italic":     {},
	"underline":  {},

	// Only for shapes
	"shadow":        {},
	"multiple":      {},
	"double-border": {},

	// Only for squares
	"3d": {},

	// Only for edges
	"animated": {},
	"filled":   {},
}

// TODO maybe autofmt should allow other values, and transform them to conform
// e.g. left-center becomes center-left
var NearConstantsArray = []string{
	"top-left",
	"top-center",
	"top-right",

	"center-left",
	"center-right",

	"bottom-left",
	"bottom-center",
	"bottom-right",
}
var NearConstants map[string]struct{}

// BoardKeywords contains the keywords that create new boards.
var BoardKeywords = map[string]struct{}{
	"layers":    {},
	"scenarios": {},
	"steps":     {},
}

func init() {
	ReservedKeywords = make(map[string]struct{})
	for k, v := range SimpleReservedKeywords {
		ReservedKeywords[k] = v
	}
	for k, v := range StyleKeywords {
		ReservedKeywords[k] = v
	}
	for k, v := range ReservedKeywordHolders {
		ReservedKeywords[k] = v
	}
	for k, v := range BoardKeywords {
		ReservedKeywords[k] = v
	}

	ReservedKeywords2 = make(map[string]struct{})
	for k, v := range SimpleReservedKeywords {
		ReservedKeywords2[k] = v
	}
	for k, v := range ReservedKeywordHolders {
		ReservedKeywords2[k] = v
	}
	for k, v := range BoardKeywords {
		ReservedKeywords2[k] = v
	}

	NearConstants = make(map[string]struct{}, len(NearConstantsArray))
	for _, k := range NearConstantsArray {
		NearConstants[k] = struct{}{}
	}
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
