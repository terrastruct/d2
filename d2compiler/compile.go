package d2compiler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type CompileOptions struct {
	UTF16Pos bool
	// FS is the file system used for resolving imports in the d2 text.
	// It should correspond to the root path.
	FS fs.FS
}

// Changes for Language Server 'mode'
type LspOutputData struct {
	Ast *d2ast.Map
	Err error
}

var lod LspOutputData

func LspOutput(m bool) {
	if !m {
		return
	}

	jsonOutput, _ := json.Marshal(lod)
	fmt.Print(string(jsonOutput))
	os.Exit(42)
}

func Compile(p string, r io.Reader, opts *CompileOptions) (*d2graph.Graph, *d2target.Config, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}

	lspMode := os.Getenv("D2_LSP_MODE") == "1"
	defer LspOutput(lspMode)

	ast, err := d2parser.Parse(p, r, &d2parser.ParseOptions{
		UTF16Pos: opts.UTF16Pos,
	})

	lod.Ast = ast
	if err != nil {
		lod.Err = err
		return nil, nil, err
	}

	if lspMode {
		return nil, nil, nil
	}

	ir, err := d2ir.Compile(ast, &d2ir.CompileOptions{
		UTF16Pos: opts.UTF16Pos,
		FS:       opts.FS,
	})
	if err != nil {
		return nil, nil, err
	}

	g, err := compileIR(ast, ir)
	if err != nil {
		return nil, nil, err
	}
	g.SortObjectsByAST()
	g.SortEdgesByAST()
	return g, compileConfig(ir), nil
}

func compileIR(ast *d2ast.Map, m *d2ir.Map) (*d2graph.Graph, error) {
	c := &compiler{
		err: &d2parser.ParseError{},
	}

	g := d2graph.NewGraph()
	g.AST = ast
	c.compileBoard(g, m)
	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	c.validateBoardLinks(g)
	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	return g, nil
}

func (c *compiler) compileBoard(g *d2graph.Graph, ir *d2ir.Map) *d2graph.Graph {
	ir = ir.Copy(nil).(*d2ir.Map)
	// c.preprocessSeqDiagrams(ir)
	c.compileMap(g.Root, ir)
	if len(c.err.Errors) == 0 {
		c.validateKeys(g.Root, ir)
	}
	c.validateNear(g)
	c.validateEdges(g)

	c.compileBoardsField(g, ir, "layers")
	c.compileBoardsField(g, ir, "scenarios")
	c.compileBoardsField(g, ir, "steps")
	if d2ir.ParentMap(ir).CopyBase(nil).Equal(ir.CopyBase(nil)) {
		if len(g.Layers) > 0 || len(g.Scenarios) > 0 || len(g.Steps) > 0 {
			g.IsFolderOnly = true
		}
	}
	if len(g.Objects) == 0 {
		g.IsFolderOnly = true
	}
	return g
}

func (c *compiler) compileBoardsField(g *d2graph.Graph, ir *d2ir.Map, fieldName string) {
	layers := ir.GetField(fieldName)
	if layers.Map() == nil {
		return
	}
	for _, f := range layers.Map().Fields {
		if f.Map() == nil {
			continue
		}
		if g.GetBoard(f.Name) != nil {
			c.errorf(f.References[0].AST(), "board name %v already used by another board", f.Name)
			continue
		}
		g2 := d2graph.NewGraph()
		g2.Parent = g
		g2.AST = f.Map().AST().(*d2ast.Map)
		g2.BaseAST = findFieldAST(g.AST, f)
		c.compileBoard(g2, f.Map())
		g2.Name = f.Name
		switch fieldName {
		case "layers":
			g.Layers = append(g.Layers, g2)
		case "scenarios":
			g.Scenarios = append(g.Scenarios, g2)
		case "steps":
			g.Steps = append(g.Steps, g2)
		}
	}
}

func findFieldAST(ast *d2ast.Map, f *d2ir.Field) *d2ast.Map {
	path := []string{}
	var curr *d2ir.Field = f
	for {
		path = append([]string{curr.Name}, path...)
		boardKind := d2ir.NodeBoardKind(curr)
		if boardKind == "" {
			break
		}
		curr = d2ir.ParentField(curr)
	}

	currAST := ast
	for len(path) > 0 {
		head := path[0]
		found := false
		for _, n := range currAST.Nodes {
			if n.MapKey == nil {
				continue
			}
			if n.MapKey.Key == nil {
				continue
			}
			if len(n.MapKey.Key.Path) != 1 {
				continue
			}
			head2 := n.MapKey.Key.Path[0].Unbox().ScalarString()
			if head == head2 {
				currAST = n.MapKey.Value.Map
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		path = path[1:]
	}

	return currAST
}

type compiler struct {
	err *d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func (c *compiler) compileMap(obj *d2graph.Object, m *d2ir.Map) {
	class := m.GetField("class")
	if class != nil {
		var classNames []string
		if class.Primary() != nil {
			classNames = append(classNames, class.Primary().String())
		} else if class.Composite != nil {
			if arr, ok := class.Composite.(*d2ir.Array); ok {
				for _, class := range arr.Values {
					if scalar, ok := class.(*d2ir.Scalar); ok {
						classNames = append(classNames, scalar.Value.ScalarString())
					} else {
						c.errorf(class.LastPrimaryKey(), "invalid value in array")
					}
				}
			}
		} else {
			c.errorf(class.LastRef().AST(), "class missing value")
		}

		for _, className := range classNames {
			classMap := m.GetClassMap(className)
			if classMap != nil {
				c.compileMap(obj, classMap)
			} else {
				if strings.Contains(className, ",") {
					split := strings.Split(className, ",")
					allFound := true
					for _, maybeClassName := range split {
						maybeClassName = strings.TrimSpace(maybeClassName)
						if m.GetClassMap(maybeClassName) == nil {
							allFound = false
							break
						}
					}
					if allFound {
						c.errorf(class.LastRef().AST(), `class "%s" not found. Did you mean to use ";" to separate array items?`, className)
					}
				}
			}
		}
	}
	shape := m.GetField("shape")
	if shape != nil {
		if shape.Composite != nil {
			c.errorf(shape.LastPrimaryKey(), "reserved field shape does not accept composite")
		} else {
			c.compileField(obj, shape)
		}
	}
	for _, f := range m.Fields {
		if f.Name == "shape" {
			continue
		}
		if _, ok := d2graph.BoardKeywords[f.Name]; ok {
			continue
		}
		c.compileField(obj, f)
	}

	if !m.IsClass() {
		switch obj.Shape.Value {
		case d2target.ShapeClass:
			c.compileClass(obj)
		case d2target.ShapeSQLTable:
			c.compileSQLTable(obj)
		}

		for _, e := range m.Edges {
			c.compileEdge(obj, e)
		}
	}
}

func (c *compiler) compileField(obj *d2graph.Object, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name)
	_, isStyleReserved := d2graph.StyleKeywords[keyword]
	if isStyleReserved {
		c.errorf(f.LastRef().AST(), "%v must be style.%v", f.Name, f.Name)
		return
	}
	_, isReserved := d2graph.SimpleReservedKeywords[keyword]
	if f.Name == "classes" {
		if f.Map() != nil {
			if len(f.Map().Edges) > 0 {
				c.errorf(f.Map().Edges[0].LastRef().AST(), "classes cannot contain an edge")
			}
			for _, classesField := range f.Map().Fields {
				if classesField.Map() == nil {
					continue
				}
				for _, cf := range classesField.Map().Fields {
					if _, ok := d2graph.ReservedKeywords[cf.Name]; !ok {
						c.errorf(cf.LastRef().AST(), "%s is an invalid class field, must be reserved keyword", cf.Name)
					}
					if cf.Name == "class" {
						c.errorf(cf.LastRef().AST(), `"class" cannot appear within "classes"`)
					}
				}
			}
		}
		return
	} else if f.Name == "vars" {
		return
	} else if isReserved {
		c.compileReserved(&obj.Attributes, f)
		return
	} else if f.Name == "style" {
		if f.Map() == nil || len(f.Map().Fields) == 0 {
			c.errorf(f.LastRef().AST(), `"style" expected to be set to a map of key-values, or contain an additional keyword like "style.opacity: 0.4"`)
			return
		}
		c.compileStyle(&obj.Attributes, f.Map())
		if obj.Style.Animated != nil {
			c.errorf(obj.Style.Animated.MapKey, `key "animated" can only be applied to edges`)
		}
		return
	}

	if obj.Parent != nil {
		if obj.Parent.Shape.Value == d2target.ShapeSQLTable {
			c.errorf(f.LastRef().AST(), "sql_table columns cannot have children")
			return
		}
		if obj.Parent.Shape.Value == d2target.ShapeClass {
			c.errorf(f.LastRef().AST(), "class fields cannot have children")
			return
		}
	}

	obj = obj.EnsureChild(d2graphIDA([]string{f.Name}))
	if f.Primary() != nil {
		c.compileLabel(&obj.Attributes, f)
	}
	if f.Map() != nil {
		c.compileMap(obj, f.Map())
	}

	if obj.Label.MapKey == nil {
		obj.Label.MapKey = f.LastPrimaryKey()
	}
	for _, fr := range f.References {
		if fr.Primary() {
			if fr.Context.Key.Value.Map != nil {
				obj.Map = fr.Context.Key.Value.Map
			}
		}
		r := d2graph.Reference{
			Key:          fr.KeyPath,
			KeyPathIndex: fr.KeyPathIndex(),

			MapKey:          fr.Context.Key,
			MapKeyEdgeIndex: fr.Context.EdgeIndex(),
			Scope:           fr.Context.Scope,
			ScopeAST:        fr.Context.ScopeAST,
		}
		if fr.Context.ScopeMap != nil && !d2ir.IsVar(fr.Context.ScopeMap) {
			scopeObjIDA := d2graphIDA(d2ir.BoardIDA(fr.Context.ScopeMap))
			r.ScopeObj = obj.Graph.Root.EnsureChild(scopeObjIDA)
		}
		obj.References = append(obj.References, r)
	}
}

func (c *compiler) compileLabel(attrs *d2graph.Attributes, f d2ir.Node) {
	scalar := f.Primary().Value
	switch scalar := scalar.(type) {
	case *d2ast.BlockString:
		if strings.TrimSpace(scalar.ScalarString()) == "" {
			c.errorf(f.LastPrimaryKey(), "block string cannot be empty")
		}
		attrs.Language = scalar.Tag
		fullTag, ok := ShortToFullLanguageAliases[scalar.Tag]
		if ok {
			attrs.Language = fullTag
		}
		switch attrs.Language {
		case "latex":
			attrs.Shape.Value = d2target.ShapeText
		case "markdown":
			rendered, err := textmeasure.RenderMarkdown(scalar.ScalarString())
			if err != nil {
				c.errorf(f.LastPrimaryKey(), "malformed Markdown")
			}
			rendered = "<div>" + rendered + "</div>"
			var xmlParsed interface{}
			err = xml.Unmarshal([]byte(rendered), &xmlParsed)
			if err != nil {
				switch xmlErr := err.(type) {
				case *xml.SyntaxError:
					c.errorf(f.LastPrimaryKey(), "malformed Markdown: %s", xmlErr.Msg)
				default:
					c.errorf(f.LastPrimaryKey(), "malformed Markdown: %s", err.Error())
				}
			}
			attrs.Shape.Value = d2target.ShapeText
		default:
			attrs.Shape.Value = d2target.ShapeCode
		}
		attrs.Label.Value = scalar.ScalarString()
	default:
		attrs.Label.Value = scalar.ScalarString()
	}
	attrs.Label.MapKey = f.LastPrimaryKey()
}

func (c *compiler) compilePosition(attrs *d2graph.Attributes, f *d2ir.Field) {
	name := f.Name
	if f.Map() != nil {
		for _, f := range f.Map().Fields {
			if f.Name == "near" {
				if f.Primary() == nil {
					c.errorf(f.LastPrimaryKey(), `invalid "near" field`)
				} else {
					scalar := f.Primary().Value
					switch scalar := scalar.(type) {
					case *d2ast.Null:
						attrs.LabelPosition = nil
					default:
						if _, ok := d2graph.LabelPositions[scalar.ScalarString()]; !ok {
							c.errorf(f.LastPrimaryKey(), `invalid "near" field`)
						} else {
							switch name {
							case "label":
								attrs.LabelPosition = &d2graph.Scalar{}
								attrs.LabelPosition.Value = scalar.ScalarString()
								attrs.LabelPosition.MapKey = f.LastPrimaryKey()
							case "icon":
								attrs.IconPosition = &d2graph.Scalar{}
								attrs.IconPosition.Value = scalar.ScalarString()
								attrs.IconPosition.MapKey = f.LastPrimaryKey()
							}
						}
					}
				}
			} else {
				if f.LastPrimaryKey() != nil {
					c.errorf(f.LastPrimaryKey(), `unexpected field %s`, f.Name)
				}
			}
		}
		if len(f.Map().Edges) > 0 {
			c.errorf(f.LastPrimaryKey(), "unexpected edges in map")
		}
	}
}

func (c *compiler) compileReserved(attrs *d2graph.Attributes, f *d2ir.Field) {
	if f.Primary() == nil {
		if f.Composite != nil {
			switch f.Name {
			case "class":
				if arr, ok := f.Composite.(*d2ir.Array); ok {
					for _, class := range arr.Values {
						if scalar, ok := class.(*d2ir.Scalar); ok {
							attrs.Classes = append(attrs.Classes, scalar.Value.ScalarString())
						}
					}
				}
			case "constraint":
				if arr, ok := f.Composite.(*d2ir.Array); ok {
					for _, constraint := range arr.Values {
						if scalar, ok := constraint.(*d2ir.Scalar); ok {
							attrs.Constraint = append(attrs.Constraint, scalar.Value.ScalarString())
						}
					}
				}
			case "label", "icon":
				c.compilePosition(attrs, f)
			default:
				c.errorf(f.LastPrimaryKey(), "reserved field %v does not accept composite", f.Name)
			}
		}
		return
	}
	scalar := f.Primary().Value
	switch f.Name {
	case "label":
		c.compileLabel(attrs, f)
		c.compilePosition(attrs, f)
	case "shape":
		in := d2target.IsShape(scalar.ScalarString())
		_, isArrowhead := d2target.Arrowheads[scalar.ScalarString()]
		if !in && !isArrowhead {
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
		c.compilePosition(attrs, f)
	case "near":
		nearKey, err := d2parser.ParseKey(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "bad near key %#v: %s", scalar.ScalarString(), err)
			return
		}
		nearKey.Range = scalar.GetRange()
		attrs.NearKey = nearKey
	case "tooltip":
		attrs.Tooltip = &d2graph.Scalar{}
		attrs.Tooltip.Value = scalar.ScalarString()
		attrs.Tooltip.MapKey = f.LastPrimaryKey()
	case "width":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer width %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.WidthAttr = &d2graph.Scalar{}
		attrs.WidthAttr.Value = scalar.ScalarString()
		attrs.WidthAttr.MapKey = f.LastPrimaryKey()
	case "height":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer height %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.HeightAttr = &d2graph.Scalar{}
		attrs.HeightAttr.Value = scalar.ScalarString()
		attrs.HeightAttr.MapKey = f.LastPrimaryKey()
	case "top":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer top %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v < 0 {
			c.errorf(scalar, "top must be a non-negative integer: %#v", scalar.ScalarString())
			return
		}
		attrs.Top = &d2graph.Scalar{}
		attrs.Top.Value = scalar.ScalarString()
		attrs.Top.MapKey = f.LastPrimaryKey()
	case "left":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer left %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v < 0 {
			c.errorf(scalar, "left must be a non-negative integer: %#v", scalar.ScalarString())
			return
		}
		attrs.Left = &d2graph.Scalar{}
		attrs.Left.Value = scalar.ScalarString()
		attrs.Left.MapKey = f.LastPrimaryKey()
	case "link":
		attrs.Link = &d2graph.Scalar{}
		attrs.Link.Value = scalar.ScalarString()
		attrs.Link.MapKey = f.LastPrimaryKey()
	case "direction":
		dirs := []string{"up", "down", "right", "left"}
		if !go2.Contains(dirs, scalar.ScalarString()) {
			c.errorf(scalar, `direction must be one of %v, got %q`, strings.Join(dirs, ", "), scalar.ScalarString())
			return
		}
		attrs.Direction.Value = scalar.ScalarString()
		attrs.Direction.MapKey = f.LastPrimaryKey()
	case "constraint":
		if _, ok := scalar.(d2ast.String); !ok {
			c.errorf(f.LastPrimaryKey(), "constraint value must be a string")
			return
		}
		attrs.Constraint = append(attrs.Constraint, scalar.ScalarString())
	case "grid-rows":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer grid-rows %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v <= 0 {
			c.errorf(scalar, "grid-rows must be a positive integer: %#v", scalar.ScalarString())
			return
		}
		attrs.GridRows = &d2graph.Scalar{}
		attrs.GridRows.Value = scalar.ScalarString()
		attrs.GridRows.MapKey = f.LastPrimaryKey()
	case "grid-columns":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer grid-columns %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v <= 0 {
			c.errorf(scalar, "grid-columns must be a positive integer: %#v", scalar.ScalarString())
			return
		}
		attrs.GridColumns = &d2graph.Scalar{}
		attrs.GridColumns.Value = scalar.ScalarString()
		attrs.GridColumns.MapKey = f.LastPrimaryKey()
	case "grid-gap":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer grid-gap %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v < 0 {
			c.errorf(scalar, "grid-gap must be a non-negative integer: %#v", scalar.ScalarString())
			return
		}
		attrs.GridGap = &d2graph.Scalar{}
		attrs.GridGap.Value = scalar.ScalarString()
		attrs.GridGap.MapKey = f.LastPrimaryKey()
	case "vertical-gap":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer vertical-gap %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v < 0 {
			c.errorf(scalar, "vertical-gap must be a non-negative integer: %#v", scalar.ScalarString())
			return
		}
		attrs.VerticalGap = &d2graph.Scalar{}
		attrs.VerticalGap.Value = scalar.ScalarString()
		attrs.VerticalGap.MapKey = f.LastPrimaryKey()
	case "horizontal-gap":
		v, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar, "non-integer horizontal-gap %#v: %s", scalar.ScalarString(), err)
			return
		}
		if v < 0 {
			c.errorf(scalar, "horizontal-gap must be a non-negative integer: %#v", scalar.ScalarString())
			return
		}
		attrs.HorizontalGap = &d2graph.Scalar{}
		attrs.HorizontalGap.Value = scalar.ScalarString()
		attrs.HorizontalGap.MapKey = f.LastPrimaryKey()
	case "class":
		attrs.Classes = append(attrs.Classes, scalar.ScalarString())
	case "classes":
	}

	if attrs.Link != nil && attrs.Tooltip != nil {
		u, err := url.ParseRequestURI(attrs.Tooltip.Value)
		if err == nil && u.Host != "" {
			c.errorf(scalar, "Tooltip cannot be set to URL when link is also set (for security)")
		}
	}
}

func (c *compiler) compileStyle(attrs *d2graph.Attributes, m *d2ir.Map) {
	for _, f := range m.Fields {
		c.compileStyleField(attrs, f)
	}
}

func (c *compiler) compileStyleField(attrs *d2graph.Attributes, f *d2ir.Field) {
	if _, ok := d2graph.StyleKeywords[strings.ToLower(f.Name)]; !ok {
		c.errorf(f.LastRef().AST(), `invalid style keyword: "%s"`, f.Name)
		return
	}
	if f.Primary() == nil {
		return
	}
	compileStyleFieldInit(attrs, f)
	scalar := f.Primary().Value
	err := attrs.Style.Apply(f.Name, scalar.ScalarString())
	if err != nil {
		c.errorf(scalar, err.Error())
		return
	}
}

func compileStyleFieldInit(attrs *d2graph.Attributes, f *d2ir.Field) {
	switch f.Name {
	case "opacity":
		attrs.Style.Opacity = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke":
		attrs.Style.Stroke = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "fill":
		attrs.Style.Fill = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "fill-pattern":
		attrs.Style.FillPattern = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
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
		attrs.WidthAttr = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "height":
		attrs.HeightAttr = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "top":
		attrs.Top = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "left":
		attrs.Left = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "double-border":
		attrs.Style.DoubleBorder = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "text-transform":
		attrs.Style.TextTransform = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	}
}

func (c *compiler) compileEdge(obj *d2graph.Object, e *d2ir.Edge) {
	edge, err := obj.Connect(d2graphIDA(e.ID.SrcPath), d2graphIDA(e.ID.DstPath), e.ID.SrcArrow, e.ID.DstArrow, "")
	if err != nil {
		c.errorf(e.References[0].AST(), err.Error())
		return
	}

	if e.Primary() != nil {
		c.compileLabel(&edge.Attributes, e)
	}
	if e.Map() != nil {
		c.compileEdgeMap(edge, e.Map())
	}

	edge.Label.MapKey = e.LastPrimaryKey()
	for _, er := range e.References {
		r := d2graph.EdgeReference{
			Edge:            er.Context.Edge,
			MapKey:          er.Context.Key,
			MapKeyEdgeIndex: er.Context.EdgeIndex(),
			Scope:           er.Context.Scope,
			ScopeAST:        er.Context.ScopeAST,
		}
		if er.Context.ScopeMap != nil && !d2ir.IsVar(er.Context.ScopeMap) {
			scopeObjIDA := d2graphIDA(d2ir.BoardIDA(er.Context.ScopeMap))
			r.ScopeObj = edge.Src.Graph.Root.EnsureChild(scopeObjIDA)
		}
		edge.References = append(edge.References, r)
	}
}

func (c *compiler) compileEdgeMap(edge *d2graph.Edge, m *d2ir.Map) {
	class := m.GetField("class")
	if class != nil {
		var classNames []string
		if class.Primary() != nil {
			classNames = append(classNames, class.Primary().String())
		} else if class.Composite != nil {
			if arr, ok := class.Composite.(*d2ir.Array); ok {
				for _, class := range arr.Values {
					if scalar, ok := class.(*d2ir.Scalar); ok {
						classNames = append(classNames, scalar.Value.ScalarString())
					} else {
						c.errorf(class.LastPrimaryKey(), "invalid value in array")
					}
				}
			}
		} else {
			c.errorf(class.LastRef().AST(), "class missing value")
		}

		for _, className := range classNames {
			classMap := m.GetClassMap(className)
			if classMap != nil {
				c.compileEdgeMap(edge, classMap)
			}
		}
	}
	for _, f := range m.Fields {
		_, ok := d2graph.ReservedKeywords[f.Name]
		if !ok {
			c.errorf(f.References[0].AST(), `edge map keys must be reserved keywords`)
			continue
		}
		c.compileEdgeField(edge, f)
	}
}

func (c *compiler) compileEdgeField(edge *d2graph.Edge, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name)
	_, isStyleReserved := d2graph.StyleKeywords[keyword]
	if isStyleReserved {
		c.errorf(f.LastRef().AST(), "%v must be style.%v", f.Name, f.Name)
		return
	}
	_, isReserved := d2graph.SimpleReservedKeywords[keyword]
	if isReserved {
		c.compileReserved(&edge.Attributes, f)
		return
	} else if f.Name == "style" {
		if f.Map() == nil {
			return
		}
		c.compileStyle(&edge.Attributes, f.Map())
		return
	}

	if f.Name == "source-arrowhead" || f.Name == "target-arrowhead" {
		c.compileArrowheads(edge, f)
	}
}

func (c *compiler) compileArrowheads(edge *d2graph.Edge, f *d2ir.Field) {
	var attrs *d2graph.Attributes
	if f.Name == "source-arrowhead" {
		if edge.SrcArrowhead == nil {
			edge.SrcArrowhead = &d2graph.Attributes{}
		}
		attrs = edge.SrcArrowhead
	} else {
		if edge.DstArrowhead == nil {
			edge.DstArrowhead = &d2graph.Attributes{}
		}
		attrs = edge.DstArrowhead
	}

	if f.Primary() != nil {
		c.compileLabel(attrs, f)
	}

	if f.Map() != nil {
		for _, f2 := range f.Map().Fields {
			keyword := strings.ToLower(f2.Name)
			_, isReserved := d2graph.SimpleReservedKeywords[keyword]
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
				c.errorf(f2.LastRef().AST(), `source-arrowhead/target-arrowhead map keys must be reserved keywords`)
				continue
			}
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

func (c *compiler) compileClass(obj *d2graph.Object) {
	obj.Class = &d2target.Class{}
	for _, f := range obj.ChildrenArray {
		visibility := "public"
		name := f.IDVal
		// See https://www.uml-diagrams.org/visibility.html
		if name != "" {
			switch name[0] {
			case '+':
				name = name[1:]
			case '-':
				visibility = "private"
				name = name[1:]
			case '#':
				visibility = "protected"
				name = name[1:]
			}
		}

		if !strings.Contains(f.IDVal, "(") {
			typ := f.Label.Value
			if typ == f.IDVal {
				typ = ""
			}
			obj.Class.Fields = append(obj.Class.Fields, d2target.ClassField{
				Name:       name,
				Type:       typ,
				Visibility: visibility,
			})
		} else {
			// TODO: Not great, AST should easily allow specifying alternate primary field
			// as an explicit label should change the name.
			returnType := f.Label.Value
			if returnType == f.IDVal {
				returnType = "void"
			}
			obj.Class.Methods = append(obj.Class.Methods, d2target.ClassMethod{
				Name:       name,
				Return:     returnType,
				Visibility: visibility,
			})
		}
	}

	for _, ch := range obj.ChildrenArray {
		for i := 0; i < len(obj.Graph.Objects); i++ {
			if obj.Graph.Objects[i] == ch {
				obj.Graph.Objects = append(obj.Graph.Objects[:i], obj.Graph.Objects[i+1:]...)
				i--
			}
		}
	}
	obj.Children = nil
	obj.ChildrenArray = nil
}

func (c *compiler) compileSQLTable(obj *d2graph.Object) {
	obj.SQLTable = &d2target.SQLTable{}
	for _, col := range obj.ChildrenArray {
		typ := col.Label.Value
		if typ == col.IDVal {
			// Not great, AST should easily allow specifying alternate primary field
			// as an explicit label should change the name.
			typ = ""
		}
		d2Col := d2target.SQLColumn{
			Name:       d2target.Text{Label: col.IDVal},
			Type:       d2target.Text{Label: typ},
			Constraint: col.Constraint,
		}
		obj.SQLTable.Columns = append(obj.SQLTable.Columns, d2Col)
	}

	for _, ch := range obj.ChildrenArray {
		for i := 0; i < len(obj.Graph.Objects); i++ {
			if obj.Graph.Objects[i] == ch {
				obj.Graph.Objects = append(obj.Graph.Objects[:i], obj.Graph.Objects[i+1:]...)
				i--
			}
		}
	}
	obj.Children = nil
	obj.ChildrenArray = nil
}

func (c *compiler) validateKeys(obj *d2graph.Object, m *d2ir.Map) {
	for _, f := range m.Fields {
		if _, ok := d2graph.BoardKeywords[f.Name]; ok {
			continue
		}
		c.validateKey(obj, f)
	}
}

func (c *compiler) validateKey(obj *d2graph.Object, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name)
	_, isReserved := d2graph.ReservedKeywords[keyword]
	if isReserved {
		switch obj.Shape.Value {
		case d2target.ShapeCircle, d2target.ShapeSquare:
			checkEqual := (keyword == "width" && obj.HeightAttr != nil) || (keyword == "height" && obj.WidthAttr != nil)
			if checkEqual && obj.WidthAttr.Value != obj.HeightAttr.Value {
				c.errorf(f.LastPrimaryKey(), "width and height must be equal for %s shapes", obj.Shape.Value)
			}
		}

		switch f.Name {
		case "style":
			if obj.Style.ThreeDee != nil {
				if !strings.EqualFold(obj.Shape.Value, d2target.ShapeSquare) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeRectangle) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeHexagon) {
					c.errorf(obj.Style.ThreeDee.MapKey, `key "3d" can only be applied to squares, rectangles, and hexagons`)
				}
			}
			if obj.Style.DoubleBorder != nil {
				if obj.Shape.Value != "" && obj.Shape.Value != d2target.ShapeSquare && obj.Shape.Value != d2target.ShapeRectangle && obj.Shape.Value != d2target.ShapeCircle && obj.Shape.Value != d2target.ShapeOval {
					c.errorf(obj.Style.DoubleBorder.MapKey, `key "double-border" can only be applied to squares, rectangles, circles, ovals`)
				}
			}
		case "shape":
			if obj.Shape.Value == d2target.ShapeImage && obj.Icon == nil {
				c.errorf(f.LastPrimaryKey(), `image shape must include an "icon" field`)
			}

			in := d2target.IsShape(obj.Shape.Value)
			_, arrowheadIn := d2target.Arrowheads[obj.Shape.Value]
			if !in && arrowheadIn {
				c.errorf(f.LastPrimaryKey(), fmt.Sprintf(`invalid shape, can only set "%s" for arrowheads`, obj.Shape.Value))
			}
		case "constraint":
			if obj.Shape.Value != d2target.ShapeSQLTable {
				c.errorf(f.LastPrimaryKey(), `"constraint" keyword can only be used in "sql_table" shapes`)
			}
		}
		return
	}

	if obj.Shape.Value == d2target.ShapeImage {
		c.errorf(f.LastRef().AST(), "image shapes cannot have children.")
		return
	}

	obj, ok := obj.HasChild([]string{f.Name})
	if ok && f.Map() != nil {
		c.validateKeys(obj, f.Map())
	}
}

func (c *compiler) validateNear(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.NearKey != nil {
			nearObj, isKey := g.Root.HasChild(d2graph.Key(obj.NearKey))
			_, isConst := d2graph.NearConstants[d2graph.Key(obj.NearKey)[0]]
			if isKey {
				// Doesn't make sense to set near to an ancestor or descendant
				nearIsAncestor := false
				for curr := obj; curr != nil; curr = curr.Parent {
					if curr == nearObj {
						nearIsAncestor = true
						break
					}
				}
				if nearIsAncestor {
					c.errorf(obj.NearKey, "near keys cannot be set to an ancestor")
					continue
				}
				nearIsDescendant := false
				for curr := nearObj; curr != nil; curr = curr.Parent {
					if curr == obj {
						nearIsDescendant = true
						break
					}
				}
				if nearIsDescendant {
					c.errorf(obj.NearKey, "near keys cannot be set to an descendant")
					continue
				}
				if nearObj.OuterSequenceDiagram() != nil {
					c.errorf(obj.NearKey, "near keys cannot be set to an object within sequence diagrams")
					continue
				}
				if nearObj.NearKey != nil {
					_, nearObjNearIsConst := d2graph.NearConstants[d2graph.Key(nearObj.NearKey)[0]]
					if nearObjNearIsConst {
						c.errorf(obj.NearKey, "near keys cannot be set to an object with a constant near key")
						continue
					}
				}
			} else if isConst {
				if obj.Parent != g.Root {
					c.errorf(obj.NearKey, "constant near keys can only be set on root level shapes")
					continue
				}
			} else {
				c.errorf(obj.NearKey, "near key %#v must be the absolute path to a shape or one of the following constants: %s", d2format.Format(obj.NearKey), strings.Join(d2graph.NearConstantsArray, ", "))
				continue
			}
		}
	}

	for _, edge := range g.Edges {
		srcNearContainer := edge.Src.OuterNearContainer()
		dstNearContainer := edge.Dst.OuterNearContainer()

		var isSrcNearConst, isDstNearConst bool

		if srcNearContainer != nil {
			_, isSrcNearConst = d2graph.NearConstants[d2graph.Key(srcNearContainer.NearKey)[0]]
		}
		if dstNearContainer != nil {
			_, isDstNearConst = d2graph.NearConstants[d2graph.Key(dstNearContainer.NearKey)[0]]
		}

		if (isSrcNearConst || isDstNearConst) && srcNearContainer != dstNearContainer {
			c.errorf(edge.References[0].Edge, "cannot connect objects from within a container, that has near constant set, to objects outside that container")
		}
	}

}

func (c *compiler) validateEdges(g *d2graph.Graph) {
	for _, edge := range g.Edges {
		if gd := edge.Src.Parent.ClosestGridDiagram(); gd != nil {
			c.errorf(edge.GetAstEdge(), "edges in grid diagrams are not supported yet")
			continue
		}
		if gd := edge.Dst.Parent.ClosestGridDiagram(); gd != nil {
			c.errorf(edge.GetAstEdge(), "edges in grid diagrams are not supported yet")
			continue
		}
	}
}

func (c *compiler) validateBoardLinks(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.Link == nil {
			continue
		}

		linkKey, err := d2parser.ParseKey(obj.Link.Value)
		if err != nil {
			continue
		}

		if linkKey.Path[0].Unbox().ScalarString() != "root" {
			continue
		}

		if !hasBoard(g.RootBoard(), linkKey.IDA()) {
			c.errorf(obj.Link.MapKey, "linked board not found")
			continue
		}
	}
	for _, b := range g.Layers {
		c.validateBoardLinks(b)
	}
	for _, b := range g.Scenarios {
		c.validateBoardLinks(b)
	}
	for _, b := range g.Steps {
		c.validateBoardLinks(b)
	}
}

func hasBoard(root *d2graph.Graph, ida []string) bool {
	if len(ida) == 0 {
		return true
	}
	if ida[0] == "root" {
		return hasBoard(root, ida[1:])
	}
	id := ida[0]
	if len(ida) == 1 {
		return root.Name == id
	}
	nextID := ida[1]
	switch id {
	case "layers":
		for _, b := range root.Layers {
			if b.Name == nextID {
				return hasBoard(b, ida[2:])
			}
		}
	case "scenarios":
		for _, b := range root.Scenarios {
			if b.Name == nextID {
				return hasBoard(b, ida[2:])
			}
		}
	case "steps":
		for _, b := range root.Steps {
			if b.Name == nextID {
				return hasBoard(b, ida[2:])
			}
		}
	}
	return false
}

func init() {
	FullToShortLanguageAliases = make(map[string]string, len(ShortToFullLanguageAliases))
	for k, v := range ShortToFullLanguageAliases {
		FullToShortLanguageAliases[v] = k
	}
}

func d2graphIDA(irIDA []string) (ida []string) {
	for _, el := range irIDA {
		n := &d2ast.KeyPath{
			Path: []*d2ast.StringBox{d2ast.MakeValueBox(d2ast.RawString(el, true)).StringBox()},
		}
		ida = append(ida, d2format.Format(n))
	}
	return ida
}

// Unused for now until shape: edge_group
func (c *compiler) preprocessSeqDiagrams(m *d2ir.Map) {
	for _, f := range m.Fields {
		if f.Name == "shape" && f.Primary_.Value.ScalarString() == d2target.ShapeSequenceDiagram {
			c.preprocessEdgeGroup(m, m)
			return
		}
		if f.Map() != nil {
			c.preprocessSeqDiagrams(f.Map())
		}
	}
}

func (c *compiler) preprocessEdgeGroup(seqDiagram, m *d2ir.Map) {
	// Any child of a sequence diagram can be either an actor, edge group or a span.
	// 1. Actors are shapes without edges inside them defined at the top level scope of a
	//    sequence diagram.
	// 2. Spans are the children of actors. For our purposes we can ignore them.
	// 3. Edge groups are defined as having at least one connection within them and also not
	//    being connected to anything. All direct children of an edge group are either edge
	//    groups or top level actors.

	// Go through all the fields and hoist actors from edge groups while also processing
	// the edge groups recursively.
	for _, f := range m.Fields {
		if isEdgeGroup(f) {
			if f.Map() != nil {
				c.preprocessEdgeGroup(seqDiagram, f.Map())
			}
		} else {
			if m == seqDiagram {
				// Ignore for root.
				continue
			}
			hoistActor(seqDiagram, f)
		}
	}

	// We need to adjust all edges recursively to point to actual actors instead.
	for _, e := range m.Edges {
		if isCrossEdgeGroupEdge(m, e) {
			c.errorf(e.References[0].AST(), "illegal edge between edge groups")
			continue
		}

		if m == seqDiagram {
			// Root edges between actors directly do not require hoisting.
			continue
		}

		srcParent := seqDiagram
		for i, el := range e.ID.SrcPath {
			f := srcParent.GetField(el)
			if !isEdgeGroup(f) {
				for j := 0; j < i+1; j++ {
					e.ID.SrcPath = append([]string{"_"}, e.ID.SrcPath...)
					e.ID.DstPath = append([]string{"_"}, e.ID.DstPath...)
				}
				break
			}
			srcParent = f.Map()
		}
	}
}

func hoistActor(seqDiagram *d2ir.Map, f *d2ir.Field) {
	f2 := seqDiagram.GetField(f.Name)
	if f2 == nil {
		seqDiagram.Fields = append(seqDiagram.Fields, f.Copy(seqDiagram).(*d2ir.Field))
	} else {
		d2ir.OverlayField(f2, f)
		d2ir.ParentMap(f).DeleteField(f.Name)
	}
}

func isCrossEdgeGroupEdge(m *d2ir.Map, e *d2ir.Edge) bool {
	srcParent := m
	for _, el := range e.ID.SrcPath {
		f := srcParent.GetField(el)
		if f == nil {
			// Hoisted already.
			break
		}
		if isEdgeGroup(f) {
			return true
		}
		srcParent = f.Map()
	}

	dstParent := m
	for _, el := range e.ID.DstPath {
		f := dstParent.GetField(el)
		if f == nil {
			// Hoisted already.
			break
		}
		if isEdgeGroup(f) {
			return true
		}
		dstParent = f.Map()
	}

	return false
}

func isEdgeGroup(n d2ir.Node) bool {
	return n.Map().EdgeCountRecursive() > 0
}

func parentSeqDiagram(n d2ir.Node) *d2ir.Map {
	for {
		m := d2ir.ParentMap(n)
		if m == nil {
			return nil
		}
		for _, f := range m.Fields {
			if f.Name == "shape" && f.Primary_.Value.ScalarString() == d2target.ShapeSequenceDiagram {
				return m
			}
		}
		n = m
	}
}

func compileConfig(ir *d2ir.Map) *d2target.Config {
	f := ir.GetField("vars", "d2-config")
	if f == nil || f.Map() == nil {
		return nil
	}

	configMap := f.Map()

	config := &d2target.Config{}

	f = configMap.GetField("sketch")
	if f != nil {
		val, _ := strconv.ParseBool(f.Primary().Value.ScalarString())
		config.Sketch = &val
	}

	f = configMap.GetField("theme-id")
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.ThemeID = go2.Pointer(int64(val))
	}

	f = configMap.GetField("dark-theme-id")
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.DarkThemeID = go2.Pointer(int64(val))
	}

	f = configMap.GetField("pad")
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.Pad = go2.Pointer(int64(val))
	}

	f = configMap.GetField("layout-engine")
	if f != nil {
		config.LayoutEngine = go2.Pointer(f.Primary().Value.ScalarString())
	}

	return config
}
