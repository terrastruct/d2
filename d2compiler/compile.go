package d2compiler

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
)

type CompileOptions struct {
	UTF16 bool
}

func Compile(path string, r io.RuneReader, opts *CompileOptions) (*d2graph.Graph, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}

	var pe d2parser.ParseError

	ast, err := d2parser.Parse(path, r, &d2parser.ParseOptions{
		UTF16: opts.UTF16,
	})
	if err != nil {
		return nil, err
	}

	ir, err := d2ir.Compile(ast)
	if err != nil {
		return nil, err
	}

	g, err := compileIR(pe, ir.CopyBase(nil))
	if err != nil {
		return nil, err
	}
	g.AST = ast

	err = compileLayersField(pe, g, ir, "layers")
	if err != nil {
		return nil, err
	}
	err = compileLayersField(pe, g, ir, "scenarios")
	if err != nil {
		return nil, err
	}
	err = compileLayersField(pe, g, ir, "steps")
	return g, err
}

func compileLayersField(pe d2parser.ParseError, g *d2graph.Graph, ir *d2ir.Map, fieldName string) error {
	layers := ir.GetField(fieldName)
	if layers.Map() == nil {
		return nil
	}
	for _, f := range layers.Map().Fields {
		if f.Map() == nil {
			continue
		}
		g2, err := compileIR(pe, f.Map())
		if err != nil {
			return err
		}
		g2.Name = f.Name
		g.Layers = append(g.Layers, g2)
	}
	return nil
}

func compileIR(pe d2parser.ParseError, m *d2ir.Map) (*d2graph.Graph, error) {
	g := d2graph.NewGraph()

	c := &compiler{
		err: pe,
	}

	c.compileMap(g.Root, m)
	if len(c.err.Errors) == 0 {
		c.validateKeys(g.Root, m)
	}
	c.compileShapes(g.Root)
	c.validateNear(g)

	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	return g, nil
}

type compiler struct {
	err d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func (c *compiler) compileMap(obj *d2graph.Object, m *d2ir.Map) {
	for _, f := range m.Fields {
		c.compileField(obj, f)
	}
	for _, e := range m.Edges {
		c.compileEdge(obj, m, e)
	}
}

func (c *compiler) compileField(obj *d2graph.Object, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name)
	_, isReserved := d2graph.ReservedKeywords[keyword]
	if isReserved {
		c.compileReserved(obj.Attributes, f)
		return
	} else if f.Name == "style" {
		if f.Map() == nil {
			return
		}
		c.compileStyle(obj.Attributes, f.Map())
		return
	}

	obj = obj.EnsureChild([]string{f.Name})
	if f.Primary() != nil {
		c.compileLabel(obj, f)
	}
	if f.Map() != nil {
		c.compileMap(obj, f.Map())
	}
}

func (c *compiler) compileLabel(attrs *d2graph.Attributes, f d2ir.Node) {
	scalar := f.Primary().Value
	switch scalar := scalar.(type) {
	case *d2ast.Null:
		// TODO: Delete instaed.
		attrs.Label.Value = scalar.ScalarString()
	case *d2ast.BlockString:
		attrs.Language = scalar.Tag
		fullTag, ok := ShortToFullLanguageAliases[scalar.Tag]
		if ok {
			attrs.Language = fullTag
		}
		if attrs.Language == "markdown" || attrs.Language == "latex" {
			attrs.Shape.Value = d2target.ShapeText
		} else {
			attrs.Shape.Value = d2target.ShapeCode
		}
	default:
		attrs.Label.Value = scalar.ScalarString()
	}
	attrs.Label.MapKey = f.LastPrimaryKey()
}

func (c *compiler) compileReserved(attrs *d2graph.Attributes, f d2ir.Node) {
	scalar := f.Primary().Value
	switch f.Name {
	case "label":
		c.compileLabel(obj, f)
	case "shape":
		in := d2target.IsShape(scalar.ScalarString())
		if !in {
			c.errorf(scalar, "unknown shape %q", scalar.ScalarString())
			return
		}
		attrs.Shape.Value = scalar.ScalarString()
		if attrs.Shape.Value == d2target.ShapeCode {
			// Explicit code shape is plaintext.
			attrs.Language = d2target.ShapeText
		}
		attrs.Shape.MapKey = f.LastPrimaryKey()
	case "icon":
		iconURL, err := url.Parse(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "bad icon url %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Icon = iconURL
	case "near":
		nearKey, err := d2parser.ParseKey(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "bad near key %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.NearKey = nearKey
	case "tooltip":
		attrs.Tooltip = scalar.ScalarString()
	case "width":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer width %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Width = &d2graph.Scalar{}
		attrs.Width.Value = scalar.ScalarString()
		attrs.Width.MapKey = f.LastPrimaryKey()
	case "height":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer height %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Height = &d2graph.Scalar{}
		attrs.Height.Value = scalar.ScalarString()
		attrs.Height.MapKey = f.LastPrimaryKey()
	case "link":
		attrs.Link = scalar.ScalarString()
	case "direction":
		dirs := []string{"up", "down", "right", "left"}
		if !go2.Contains(dirs, scalar.ScalarString()) {
			c.errorf(scalar, `direction must be one of %v, got %q`, strings.Join(dirs, ", "), scalar.ScalarString())
			return
		}
		attrs.Direction.Value = scalar.ScalarString()
		attrs.Direction.MapKey = f.LastPrimaryKey()
	case "constraint":
		// Compilation for shape-specific keywords happens elsewhere
	}
}

func (c *compiler) compileStyle(attrs *d2graph.Attributes, f d2ir.Node) {
	scalar := f.Primary().Value
	err := attrs.Style.Apply(f.Name, scalar.ScalarString())
	if err != nil {
		c.errorf(scalar, err.Error())
		return
	}

	switch f.Name {
	case "opacity":
		attrs.Style.Opacity = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke":
		attrs.Style.Stroke = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "fill":
		attrs.Style.Fill = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke-width":
		attrs.Style.StrokeWidth = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke-dash":
		attrs.Style.StrokeDash = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "border-radius":
		attrs.Style.BorderRadius = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "shadow":
		attrs.Style.Shadow = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "3d":
		attrs.Style.ThreeDee = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "multiple":
		attrs.Style.Multiple = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font":
		attrs.Style.Font = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font-size":
		attrs.Style.FontSize = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font-color":
		attrs.Style.FontColor = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "animated":
		attrs.Style.Animated = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "bold":
		attrs.Style.Bold = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "italic":
		attrs.Style.Italic = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "underline":
		attrs.Style.Underline = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "filled":
		attrs.Style.Filled = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "width":
		attrs.Width = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "height":
		attrs.Height = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	}
}

func (c *compiler) compileEdge(obj *d2graph.Object, e *d2ir.Edge) {
	edge, err := obj.Connect(e.ID.SrcPath, e.ID.DstPath, e.ID.SrcArrow, e.ID.DstArrow, "")
	if err != nil {
		c.errorf(e, err.Error())
		return
	}

	if e.Primary() != nil {
		c.compileLabel(edge.Attributes, e)
	}
	if e.Map() != nil {
		for _, f := range e.Map().Fields {
			_, ok := d2graph.ReservedKeywords[f.Name]
			if !ok {
				c.errorf(mk, `edge map keys must be reserved keywords`)
				continue
			}
			c.compileEdgeField(edge, f)
		}
	}
}

func (c *compiler) compileEdgeField(edge *d2graph.Edge, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name)
	_, isReserved := d2graph.ReservedKeywords[keyword]
	if isReserved {
		c.compileReserved(edge.Attributes, f)
		return
	} else if f.Name == "style" {
		if f.Map() == nil {
			return
		}
		c.compileStyle(edge.Attributes, f.Map())
		return
	}

	if f.Primary() != nil {
		c.compileLabel(edge, f)
	}

	if f.Name == "source-arrowhead" || f.Name == "target-arrowhead" {
		if f.Map() != nil {
			c.compileArrowheads(edge, f)
		}
	}
}

func (c *compiler) compileArrowheads(edge *d2graph.Edge, f *d2ir.Field) {
	var attrs *d2graph.Attributes
	if f.Name == "source-arrowhead" {
		edge.SrcArrowhead = &d2graph.Attributes{}
		attrs = edge.SrcArrowhead
	} else {
		edge.DstArrowhead = &d2graph.Attributes{}
		attrs = edge.DstArrowhead
	}

	for _, f2 := range f.Map().Fields {
		_, isReserved := d2graph.ReservedKeywords[keyword]
		if isReserved {
			c.compileReserved(attrs, f2)
			continue
		} else if f2.Name == "style" {
			if f2.Map() == nil {
				continue
			}
			c.compileStyle(attrs, f2.Map())
			continue
		} else {
			c.errorf(mk, `source-arrowhead/target-arrowhead map keys must be reserved keywords`)
			continue
		}
	}
}

// TODO add more, e.g. C, bash
var ShortToFullLanguageAliases = map[string]string{
	"md":  "markdown",
	"tex": "latex",
	"js":  "javascript",
	"go":  "golang",
	"py":  "python",
	"rb":  "ruby",
	"ts":  "typescript",
}
var FullToShortLanguageAliases map[string]string

func (c *compiler) compileShapes(obj *d2graph.Object) {
	for _, obj := range obj.ChildrenArray {
		switch obj.Attributes.Shape.Value {
		case d2target.ShapeClass:
			c.compileClass(obj)
		case d2target.ShapeSQLTable:
			c.compileSQLTable(obj)
		case d2target.ShapeImage:
			c.compileImage(obj)
		}
		c.compileShapes(obj)
	}
}

func (c *compiler) compileImage(obj *d2graph.Object) {
	if obj.Attributes.Icon == nil {
		c.errorf(obj.Attributes.Shape.MapKey, `image shape must include an "icon" field`)
	}
}

func (c *compiler) compileClass(obj *d2graph.Object) {
	obj.Class = &d2target.Class{}

	for _, f := range obj.ChildrenArray {
		if f.IDVal == "style" {
			continue
		}
		visiblity := "public"
		name := f.IDVal
		// See https://www.uml-diagrams.org/visibility.html
		if name != "" {
			switch name[0] {
			case '+':
				name = name[1:]
			case '-':
				visiblity = "private"
				name = name[1:]
			case '#':
				visiblity = "protected"
				name = name[1:]
			}
		}

		if !strings.Contains(f.IDVal, "(") {
			typ := f.Attributes.Label.Value
			if typ == f.IDVal {
				typ = ""
			}
			obj.Class.Fields = append(obj.Class.Fields, d2target.ClassField{
				Name:       name,
				Type:       typ,
				Visibility: visiblity,
			})
		} else {
			// TODO: Not great, AST should easily allow specifying alternate primary field
			// as an explicit label should change the name.
			returnType := f.Attributes.Label.Value
			if returnType == f.IDVal {
				returnType = "void"
			}
			obj.Class.Methods = append(obj.Class.Methods, d2target.ClassMethod{
				Name:       name,
				Return:     returnType,
				Visibility: visiblity,
			})
		}
	}
}

func (c *compiler) compileSQLTable(obj *d2graph.Object) {
	obj.SQLTable = &d2target.SQLTable{}

	parentID := obj.Parent.AbsID()
	tableIDPrefix := obj.AbsID() + "."
	for _, col := range obj.ChildrenArray {
		if col.IDVal == "style" {
			continue
		}
		typ := col.Attributes.Label.Value
		if typ == col.IDVal {
			// Not great, AST should easily allow specifying alternate primary field
			// as an explicit label should change the name.
			typ = ""
		}
		d2Col := d2target.SQLColumn{
			Name: d2target.Text{Label: col.IDVal},
			Type: d2target.Text{Label: typ},
		}
		// The only map a sql table field could have is to specify constraint
		if col.Map != nil {
			for _, n := range col.Map.Nodes {
				if n.MapKey.Key == nil || len(n.MapKey.Key.Path) == 0 {
					continue
				}
				if n.MapKey.Key.Path[0].Unbox().ScalarString() == "constraint" {
					if n.MapKey.Value.StringBox().Unbox() == nil {
						c.errorf(n.MapKey, "constraint value must be a string")
						return
					}
					d2Col.Constraint = n.MapKey.Value.StringBox().Unbox().ScalarString()
				}
			}
		}

		absID := col.AbsID()
		for _, e := range obj.Graph.Edges {
			srcID := e.Src.AbsID()
			dstID := e.Dst.AbsID()
			// skip edges between columns of the same table
			if strings.HasPrefix(srcID, tableIDPrefix) && strings.HasPrefix(dstID, tableIDPrefix) {
				continue
			}
			if srcID == absID {
				d2Col.Reference = strings.TrimPrefix(dstID, parentID+".")
				e.SrcTableColumnIndex = new(int)
				*e.SrcTableColumnIndex = len(obj.SQLTable.Columns)
			} else if dstID == absID {
				e.DstTableColumnIndex = new(int)
				*e.DstTableColumnIndex = len(obj.SQLTable.Columns)
			}
		}

		obj.SQLTable.Columns = append(obj.SQLTable.Columns, d2Col)
	}
}

func (c *compiler) validateKeys(obj *d2graph.Object, m *d2ir.Map) {
	for _, n := range m.Fields {
		c.validateKey(obj, f)
	}
}

func (c *compiler) validateKey(obj *d2graph.Object, f *d2ir.Field) {
	_, isReserved := d2graph.ReservedKeywords[keyword]
	if isReserved {
		switch obj.Attributes.Shape.Value {
		case d2target.ShapeSQLTable, d2target.ShapeClass:
		default:
			if len(obj.Children) > 0 && (f.Name == "width" || f.Name == "height") {
				c.errorf(f.LastPrimaryKey(), mk.Range.End, fmt.Sprintf("%s cannot be used on container: %s", f.Name, obj.AbsID()))
			}
		}

		switch obj.Attributes.Shape.Value {
		case d2target.ShapeCircle, d2target.ShapeSquare:
			checkEqual := (reserved == "width" && obj.Attributes.Height != nil) || (reserved == "height" && obj.Attributes.Width != nil)
			if checkEqual && obj.Attributes.Width.Value != obj.Attributes.Height.Value {
				c.errorf(f.LastPrimaryKey(), "width and height must be equal for %s shapes", obj.Attributes.Shape.Value)
			}
		}

		switch f.Name {
		case "width":
			if obj.Attributes.Shape.Value != d2target.ShapeImage {
				c.errorf(f.LastPrimaryKey(), "width is only applicable to image shapes.")
			}
		case "height":
			if obj.Attributes.Shape.Value != d2target.ShapeImage {
				c.errorf(f.LastPrimaryKey(), "height is only applicable to image shapes.")
			}
		case "3d":
			if obj.Attributes.Shape.Value != "" && !strings.EqualFold(obj.Attributes.Shape.Value, d2target.ShapeSquare) && !strings.EqualFold(obj.Attributes.Shape.Value, d2target.ShapeRectangle) {
				c.errorf(f.LastPrimaryKey(), `key "3d" can only be applied to squares and rectangles`)
			}
		case "shape":
			in := d2target.IsShape(obj.Attributes.Shape.Value)
			_, arrowheadIn := d2target.Arrowheads[obj.Attributes.Shape.Value]
			if !in && arrowheadIn {
				c.errorf(f.LastPrimaryKey(), fmt.Sprintf(`invalid shape, can only set "%s" for arrowheads`, obj.Attributes.Shape.Value))
			}
		}
		return
	}

	if obj.Attributes.Shape.Value == d2target.ShapeImage {
		c.errorf(mk, "image shapes cannot have children.")
		return
	}

	obj = obj.HasChild([]string{f.Name})
	if f.Map() != nil {
		c.validateKeys(obj, f.Map())
	}
}

func (c *compiler) validateNear(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.Attributes.NearKey != nil {
			_, isKey := g.Root.HasChild(d2graph.Key(obj.Attributes.NearKey))
			_, isConst := d2graph.NearConstants[d2graph.Key(obj.Attributes.NearKey)[0]]
			if !isKey && !isConst {
				c.errorf(obj.Attributes.NearKey, "near key %#v must be the absolute path to a shape or one of the following constants: %s", d2format.Format(obj.Attributes.NearKey), strings.Join(d2graph.NearConstantsArray, ", "))
				continue
			}
			if !isKey && isConst && obj.Parent != g.Root {
				c.errorf(obj.Attributes.NearKey, "constant near keys can only be set on root level shapes")
				continue
			}
			if !isKey && isConst && len(obj.ChildrenArray) > 0 {
				c.errorf(obj.Attributes.NearKey, "constant near keys cannot be set on shapes with children")
				continue
			}
			if !isKey && isConst {
				is := false
				for _, e := range g.Edges {
					if e.Src == obj || e.Dst == obj {
						is = true
						break
					}
				}
				if is {
					c.errorf(obj.Attributes.NearKey, "constant near keys cannot be set on connected shapes")
					continue
				}
			}
		}
	}
}

func init() {
	FullToShortLanguageAliases = make(map[string]string, len(ShortToFullLanguageAliases))
	for k, v := range ShortToFullLanguageAliases {
		FullToShortLanguageAliases[v] = k
	}
}
