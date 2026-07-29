package d2compiler

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type CompileOptions struct {
	UTF16Pos bool
	// FS is the file system used for resolving imports in the d2 text.
	// It should correspond to the root path.
	FS fs.FS
}

func Compile(p string, r io.Reader, opts *CompileOptions) (*d2graph.Graph, *d2target.Config, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}

	ast, err := d2parser.Parse(p, r, &d2parser.ParseOptions{
		UTF16Pos: opts.UTF16Pos,
	})
	if err != nil {
		return nil, nil, err
	}

	ir, _, err := d2ir.Compile(ast, &d2ir.CompileOptions{
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
	g.FS = opts.FS
	g.SortObjectsByAST()
	g.SortEdgesByAST()
	config, err := compileConfig(ir)
	if err != nil {
		return nil, nil, err
	}
	return g, config, nil
}

func compileIR(ast *d2ast.Map, m *d2ir.Map) (*d2graph.Graph, error) {
	c := &compiler{
		err: &d2parser.ParseError{},
	}

	g := d2graph.NewGraph()
	g.AST = ast
	g.BaseAST = ast
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
	c.setDefaultShapes(g)
	if len(c.err.Errors) == 0 {
		c.validateKeys(g.Root, ir)
	}
	c.validateLabels(g)
	c.validateNear(g)
	c.validateEdges(g)
	c.validatePositionsCompatibility(g)

	c.compileLegend(g, ir)

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

func (c *compiler) compileLegend(g *d2graph.Graph, m *d2ir.Map) {
	varsField := m.GetField(d2ast.FlatUnquotedString("vars"))
	if varsField == nil || varsField.Map() == nil {
		return
	}

	legendField := varsField.Map().GetField(d2ast.FlatUnquotedString("d2-legend"))
	if legendField == nil || legendField.Map() == nil {
		return
	}

	legendGraph := d2graph.NewGraph()

	c.compileMap(legendGraph.Root, legendField.Map())
	c.setDefaultShapes(legendGraph)

	objects := make([]*d2graph.Object, 0)
	for _, obj := range legendGraph.Objects {
		if obj.Style.Opacity != nil {
			if opacity, err := strconv.ParseFloat(obj.Style.Opacity.Value, 64); err == nil && opacity == 0 {
				continue
			}
		}
		obj.Box = &geo.Box{}
		obj.TopLeft = geo.NewPoint(10, 10)
		obj.Width = 100
		obj.Height = 100
		objects = append(objects, obj)
	}

	for _, edge := range legendGraph.Edges {
		edge.Route = []*geo.Point{
			{X: 10, Y: 10},
			{X: 110, Y: 10},
		}
	}

	legend := &d2graph.Legend{
		Objects: objects,
		Edges:   legendGraph.Edges,
	}

	if legendField.Primary() != nil && legendField.Primary().Value != nil {
		legend.Label = legendField.Primary().Value.ScalarString()
	}

	if len(legend.Objects) > 0 || len(legend.Edges) > 0 {
		g.Legend = legend
	}
}

func (c *compiler) compileBoardsField(g *d2graph.Graph, ir *d2ir.Map, fieldName string) {
	boards := ir.GetField(d2ast.FlatUnquotedString(fieldName))
	if boards.Map() == nil {
		return
	}
	for _, f := range boards.Map().Fields {
		m := f.Map()
		if f.Map() == nil {
			m = &d2ir.Map{}
		}
		if g.GetBoard(f.Name.ScalarString()) != nil {
			c.errorf(f.References[0].AST(), "board name %v already used by another board", f.Name.ScalarString())
			continue
		}
		g2 := d2graph.NewGraph()
		g2.Parent = g
		g2.AST = m.AST().(*d2ast.Map)
		if g.BaseAST != nil {
			g2.BaseAST = findFieldAST(g.BaseAST, f)
		}
		c.compileBoard(g2, m)
		if f.Primary() != nil {
			c.compileLabel(&g2.Root.Attributes, f)
		}
		g2.Name = f.Name.ScalarString()
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
	curr := f
	for {
		path = append([]string{curr.Name.ScalarString()}, path...)
		boardKind := d2ir.NodeBoardKind(curr)
		if boardKind == "" {
			break
		}
		curr = d2ir.ParentField(curr)
	}

	return _findFieldAST(ast, path)
}

func _findFieldAST(ast *d2ast.Map, path []string) *d2ast.Map {
	if len(path) == 0 {
		return ast
	}

	head := path[0]
	remainingPath := path[1:]

	for i := range ast.Nodes {
		if ast.Nodes[i].MapKey == nil || ast.Nodes[i].MapKey.Key == nil || len(ast.Nodes[i].MapKey.Key.Path) != 1 {
			continue
		}

		head2 := ast.Nodes[i].MapKey.Key.Path[0].Unbox().ScalarString()
		if head == head2 {
			if ast.Nodes[i].MapKey.Value.Map == nil {
				ast.Nodes[i].MapKey.Value.Map = &d2ast.Map{
					Range: d2ast.MakeRange(",1:0:0-1:0:0"),
				}
				if ast.Nodes[i].MapKey.Value.Import != nil {
					imp := &d2ast.Import{
						Range:  d2ast.MakeRange(",1:0:0-1:0:0"),
						Spread: true,
						Pre:    ast.Nodes[i].MapKey.Value.Import.Pre,
						Path:   ast.Nodes[i].MapKey.Value.Import.Path,
					}
					ast.Nodes[i].MapKey.Value.Map.Nodes = append(ast.Nodes[i].MapKey.Value.Map.Nodes, d2ast.MapNodeBox{
						Import: imp,
					})
				}
			}

			if result := _findFieldAST(ast.Nodes[i].MapKey.Value.Map, remainingPath); result != nil {
				return result
			}
		}
	}

	return nil
}

type compiler struct {
	err *d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	err := d2parser.Errorf(n, f, v...).(d2ast.Error)
	if c.err.ErrorsLookup == nil {
		c.err.ErrorsLookup = make(map[d2ast.Error]struct{})
	}
	if _, ok := c.err.ErrorsLookup[err]; !ok {
		c.err.Errors = append(c.err.Errors, err)
		c.err.ErrorsLookup[err] = struct{}{}
	}
}

func (c *compiler) compileMap(obj *d2graph.Object, m *d2ir.Map) {
	class := m.GetField(d2ast.FlatUnquotedString("class"))
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
	shape := m.GetField(d2ast.FlatUnquotedString("shape"))
	if shape != nil {
		if shape.Composite != nil {
			c.errorf(shape.LastPrimaryKey(), "reserved field shape does not accept composite")
		} else {
			c.compileField(obj, shape)
		}
	}
	for _, f := range m.Fields {
		if f.Name.ScalarString() == "shape" && f.Name.IsUnquoted() {
			continue
		}
		if _, ok := d2ast.BoardKeywords[f.Name.ScalarString()]; ok && f.Name.IsUnquoted() {
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
	keyword := strings.ToLower(f.Name.ScalarString())
	_, isStyleReserved := d2ast.StyleKeywords[keyword]
	if isStyleReserved && f.Name.IsUnquoted() {
		c.errorf(f.LastRef().AST(), "%v must be style.%v", f.Name.ScalarString(), f.Name.ScalarString())
		return
	}
	_, isReserved := d2ast.SimpleReservedKeywords[keyword]
	isReserved = isReserved && f.Name.IsUnquoted()
	if f.Name.ScalarString() == "classes" && f.Name.IsUnquoted() {
		if f.Map() != nil {
			if len(f.Map().Edges) > 0 {
				c.errorf(f.Map().Edges[0].LastRef().AST(), "classes cannot contain an edge")
			}
			for _, classesField := range f.Map().Fields {
				if classesField.Map() == nil {
					continue
				}
				for _, cf := range classesField.Map().Fields {
					if _, ok := d2ast.ReservedKeywords[cf.Name.ScalarString()]; !(ok && f.Name.IsUnquoted()) {
						c.errorf(cf.LastRef().AST(), "%s is an invalid class field, must be reserved keyword", cf.Name.ScalarString())
					}
					if cf.Name.ScalarString() == "class" && cf.Name.IsUnquoted() {
						c.errorf(cf.LastRef().AST(), `"class" cannot appear within "classes"`)
					}
				}
			}
		}
		return
	} else if f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() {
		return
	} else if (f.Name.ScalarString() == "source-arrowhead" || f.Name.ScalarString() == "target-arrowhead") && f.Name.IsUnquoted() {
		c.errorf(f.LastRef().AST(), `%#v can only be used on connections`, f.Name.ScalarString())
		return

	} else if isReserved {
		c.compileReserved(&obj.Attributes, f)
		return
	} else if f.Name.ScalarString() == "style" && f.Name.IsUnquoted() {
		if f.Map() == nil || len(f.Map().Fields) == 0 {
			c.errorf(f.LastRef().AST(), `"style" expected to be set to a map of key-values, or contain an additional keyword like "style.opacity: 0.4"`)
			return
		}
		c.compileStyle(&obj.Attributes.Style, f.Map())
		return
	}

	if obj.Parent != nil {
		if strings.EqualFold(obj.Parent.Shape.Value, d2target.ShapeSQLTable) {
			c.errorf(f.LastRef().AST(), "sql_table columns cannot have children")
			return
		}
		if strings.EqualFold(obj.Parent.Shape.Value, d2target.ShapeClass) {
			c.errorf(f.LastRef().AST(), "class fields cannot have children")
			return
		}
	}

	parent := obj
	obj = obj.EnsureChild(([]d2ast.String{f.Name}))
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
			if fr.Context_.Key.Value.Map != nil {
				obj.Map = fr.Context_.Key.Value.Map
			}
		}
		r := d2graph.Reference{
			Key:          fr.KeyPath,
			KeyPathIndex: fr.KeyPathIndex(),

			MapKey:          fr.Context_.Key,
			MapKeyEdgeIndex: fr.Context_.EdgeIndex(),
			Scope:           fr.Context_.Scope,
			ScopeAST:        fr.Context_.ScopeAST,
			ScopeObj:        parent,
			IsVar:           d2ir.IsVar(fr.Context_.ScopeMap),
		}
		if fr.Context_.ScopeMap != nil && !d2ir.IsVar(fr.Context_.ScopeMap) {
			scopeObjIDA := d2ir.BoardIDA(fr.Context_.ScopeMap)
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
			if f.Name.ScalarString() == "near" && f.Name.IsUnquoted() {
				if f.Primary() == nil {
					c.errorf(f.LastPrimaryKey(), `invalid "near" field`)
				} else {
					scalar := f.Primary().Value
					switch scalar := scalar.(type) {
					case *d2ast.Null:
						switch name.ScalarString() {
						case "label":
							attrs.LabelPosition = nil
						case "icon":
							attrs.IconPosition = nil
						case "tooltip":
							attrs.TooltipPosition = nil
						}
					default:
						switch name.ScalarString() {
						case "label", "icon":
							if _, ok := d2ast.LabelPositions[scalar.ScalarString()]; !ok {
								c.errorf(f.LastPrimaryKey(), `invalid "near" field`)
							} else {
								switch name.ScalarString() {
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
						case "tooltip":
							if _, ok := d2ast.TooltipPositions[scalar.ScalarString()]; !ok {
								c.errorf(f.LastPrimaryKey(), `invalid "near" field`)
							} else {
								attrs.TooltipPosition = &d2graph.Scalar{}
								attrs.TooltipPosition.Value = scalar.ScalarString()
								attrs.TooltipPosition.MapKey = f.LastPrimaryKey()
							}
						}
					}
				}
			} else {
				if f.LastPrimaryKey() != nil {
					c.errorf(f.LastPrimaryKey(), `unexpected field %s`, f.Name.ScalarString())
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
			switch f.Name.ScalarString() {
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
							switch scalar.Value.(type) {
							case *d2ast.Null:
								attrs.Constraint = append(attrs.Constraint, "null")
							default:
								attrs.Constraint = append(attrs.Constraint, scalar.Value.ScalarString())
							}
						}
					}
				}
			case "label", "icon", "tooltip":
				c.compilePosition(attrs, f)
			default:
				c.errorf(f.LastPrimaryKey(), "reserved field %v does not accept composite", f.Name.ScalarString())
			}
		} else {
			c.errorf(f.LastRef().AST(), `reserved field "%v" must have a value`, f.Name.ScalarString())
		}
		return
	}
	scalar := f.Primary().Value
	switch f.Name.ScalarString() {
	case "label":
		c.compileLabel(attrs, f)
		c.compilePosition(attrs, f)
	case "shape":
		shapeVal := strings.ToLower(scalar.ScalarString())
		in := d2target.IsShape(shapeVal)
		_, isArrowhead := d2target.Arrowheads[shapeVal]
		if !in && !isArrowhead {
			c.errorf(scalar, "unknown shape %q", scalar.ScalarString())
			return
		}
		attrs.Shape.Value = shapeVal
		if strings.EqualFold(attrs.Shape.Value, d2target.ShapeCode) {
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
		if f.Map() != nil {
			for _, ff := range f.Map().Fields {
				if ff.Name.ScalarString() == "style" && ff.Name.IsUnquoted() {
					if ff.Map() == nil || len(ff.Map().Fields) == 0 {
						c.errorf(f.LastRef().AST(), `"style" expected to be set to a map of key-values, or contain an additional keyword like "style.opacity: 0.4"`)
						return
					}
					c.compileStyle(&attrs.IconStyle, ff.Map())
				}
			}
		}
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

		c.compilePosition(attrs, f)
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
		val := strings.ToLower(scalar.ScalarString())
		if !go2.Contains(dirs, val) {
			c.errorf(scalar, `direction must be one of %v, got %q`, strings.Join(dirs, ", "), scalar.ScalarString())
			return
		}
		attrs.Direction.Value = val
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

	if attrs.Link != nil && attrs.Label.Value != "" {
		u, err := url.ParseRequestURI(attrs.Label.Value)
		if err == nil && u.Host != "" {
			c.errorf(scalar, "Label cannot be set to URL when link is also set (for security)")
		}
	}

	if attrs.Link != nil && attrs.Tooltip != nil {
		u, err := url.ParseRequestURI(attrs.Tooltip.Value)
		if err == nil && u.Host != "" {
			c.errorf(scalar, "Tooltip cannot be set to URL when link is also set (for security)")
		}
	}
}

func (c *compiler) compileStyle(styles *d2graph.Style, m *d2ir.Map) {
	for _, f := range m.Fields {
		c.compileStyleField(styles, f)
	}
}

func (c *compiler) compileStyleField(styles *d2graph.Style, f *d2ir.Field) {
	if _, ok := d2ast.StyleKeywords[strings.ToLower(f.Name.ScalarString())]; !(ok && f.Name.IsUnquoted()) {
		c.errorf(f.LastRef().AST(), `invalid style keyword: "%s"`, f.Name.ScalarString())
		return
	}
	if f.Primary() == nil {
		return
	}
	compileStyleFieldInit(styles, f)
	scalar := f.Primary().Value
	err := styles.Apply(f.Name.ScalarString(), scalar.ScalarString())
	if err != nil {
		c.errorf(scalar, err.Error())
		return
	}
}

func compileStyleFieldInit(styles *d2graph.Style, f *d2ir.Field) {
	switch f.Name.ScalarString() {
	case "opacity":
		styles.Opacity = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke":
		styles.Stroke = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "fill":
		styles.Fill = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "fill-pattern":
		styles.FillPattern = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke-width":
		styles.StrokeWidth = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "stroke-dash":
		styles.StrokeDash = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "border-radius":
		styles.BorderRadius = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "shadow":
		styles.Shadow = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "3d":
		styles.ThreeDee = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "multiple":
		styles.Multiple = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font":
		styles.Font = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font-size":
		styles.FontSize = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "font-color":
		styles.FontColor = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "animated":
		styles.Animated = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "bold":
		styles.Bold = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "italic":
		styles.Italic = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "underline":
		styles.Underline = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "filled":
		styles.Filled = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "double-border":
		styles.DoubleBorder = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	case "text-transform":
		styles.TextTransform = &d2graph.Scalar{MapKey: f.LastPrimaryKey()}
	}
}

func (c *compiler) compileEdge(obj *d2graph.Object, e *d2ir.Edge) {
	edge, err := obj.Connect(e.ID.SrcPath, e.ID.DstPath, e.ID.SrcArrow, e.ID.DstArrow, "")
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
			Edge:            er.Context_.Edge,
			MapKey:          er.Context_.Key,
			MapKeyEdgeIndex: er.Context_.EdgeIndex(),
			Scope:           er.Context_.Scope,
			ScopeAST:        er.Context_.ScopeAST,
			ScopeObj:        obj,
		}
		if er.Context_.ScopeMap != nil && !d2ir.IsVar(er.Context_.ScopeMap) {
			scopeObjIDA := d2ir.BoardIDA(er.Context_.ScopeMap)
			r.ScopeObj = edge.Src.Graph.Root.EnsureChild(scopeObjIDA)
		}
		edge.References = append(edge.References, r)
	}
}

func (c *compiler) compileEdgeMap(edge *d2graph.Edge, m *d2ir.Map) {
	class := m.GetField(d2ast.FlatUnquotedString("class"))
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
		}

		for _, className := range classNames {
			classMap := m.GetClassMap(className)
			if classMap != nil {
				c.compileEdgeMap(edge, classMap)
			}
		}
	}
	for _, f := range m.Fields {
		_, ok := d2ast.ReservedKeywords[f.Name.ScalarString()]
		if !(ok && f.Name.IsUnquoted()) {
			c.errorf(f.References[0].AST(), `edge map keys must be reserved keywords`)
			continue
		}
		c.compileEdgeField(edge, f)
	}
}

func (c *compiler) compileEdgeField(edge *d2graph.Edge, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name.ScalarString())
	_, isStyleReserved := d2ast.StyleKeywords[keyword]
	isStyleReserved = isStyleReserved && f.Name.IsUnquoted()
	if isStyleReserved {
		c.errorf(f.LastRef().AST(), "%v must be style.%v", f.Name.ScalarString(), f.Name.ScalarString())
		return
	}
	_, isReserved := d2ast.SimpleReservedKeywords[keyword]
	if isReserved {
		c.compileReserved(&edge.Attributes, f)
		return
	} else if f.Name.ScalarString() == "style" {
		if f.Map() == nil {
			return
		}
		c.compileStyle(&edge.Attributes.Style, f.Map())
		return
	}

	if (f.Name.ScalarString() == "source-arrowhead" || f.Name.ScalarString() == "target-arrowhead") && f.Name.IsUnquoted() {
		c.compileArrowheads(edge, f)
	}
}

func (c *compiler) compileArrowheads(edge *d2graph.Edge, f *d2ir.Field) {
	var attrs *d2graph.Attributes
	if f.Name.ScalarString() == "source-arrowhead" {
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
			keyword := strings.ToLower(f2.Name.ScalarString())
			_, isReserved := d2ast.SimpleReservedKeywords[keyword]
			isReserved = isReserved && f2.Name.IsUnquoted()
			if isReserved {
				c.compileReserved(attrs, f2)
				continue
			} else if f2.Name.ScalarString() == "style" && f2.Name.IsUnquoted() {
				if f2.Map() == nil {
					continue
				}
				c.compileStyle(&attrs.Style, f2.Map())
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
			underline := f.Attributes.Style.Underline != nil && f.Attributes.Style.Underline.Value == "true"
			obj.Class.Fields = append(obj.Class.Fields, d2target.ClassField{
				Name:       name,
				Type:       typ,
				Visibility: visibility,
				Underline:  underline,
			})
		} else {
			// TODO: Not great, AST should easily allow specifying alternate primary field
			// as an explicit label should change the name.
			returnType := f.Label.Value
			if returnType == f.IDVal {
				returnType = "void"
			}
			underline := f.Attributes.Style.Underline != nil && f.Attributes.Style.Underline.Value == "true"
			obj.Class.Methods = append(obj.Class.Methods, d2target.ClassMethod{
				Name:       name,
				Return:     returnType,
				Visibility: visibility,
				Underline:  underline,
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
		if _, ok := d2ast.BoardKeywords[f.Name.ScalarString()]; ok && f.Name.IsUnquoted() {
			continue
		}
		c.validateKey(obj, f)
	}
}

func (c *compiler) validateKey(obj *d2graph.Object, f *d2ir.Field) {
	keyword := strings.ToLower(f.Name.ScalarString())
	_, isReserved := d2ast.ReservedKeywords[keyword]
	isReserved = isReserved && f.Name.IsUnquoted()
	if isReserved {
		switch obj.Shape.Value {
		case d2target.ShapeCircle, d2target.ShapeSquare:
			checkEqual := (keyword == "width" && obj.HeightAttr != nil) || (keyword == "height" && obj.WidthAttr != nil)
			if checkEqual && obj.WidthAttr.Value != obj.HeightAttr.Value {
				c.errorf(f.LastPrimaryKey(), "width and height must be equal for %s shapes", obj.Shape.Value)
			}
		}

		switch f.Name.ScalarString() {
		case "style":
			if obj.Style.ThreeDee != nil {
				if !strings.EqualFold(obj.Shape.Value, d2target.ShapeSquare) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeRectangle) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeHexagon) {
					c.errorf(obj.Style.ThreeDee.MapKey, `key "3d" can only be applied to squares, rectangles, and hexagons`)
				}
			}
			if obj.Style.DoubleBorder != nil {
				if obj.Shape.Value != "" && !strings.EqualFold(obj.Shape.Value, d2target.ShapeSquare) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeRectangle) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeCircle) && !strings.EqualFold(obj.Shape.Value, d2target.ShapeOval) {
					c.errorf(obj.Style.DoubleBorder.MapKey, `key "double-border" can only be applied to squares, rectangles, circles, ovals`)
				}
			}
		case "shape":
			if strings.EqualFold(obj.Shape.Value, d2target.ShapeImage) && obj.Icon == nil {
				c.errorf(f.LastPrimaryKey(), `image shape must include an "icon" field`)
			}

			in := d2target.IsShape(obj.Shape.Value)
			_, arrowheadIn := d2target.Arrowheads[obj.Shape.Value]
			if !in && arrowheadIn {
				c.errorf(f.LastPrimaryKey(), fmt.Sprintf(`invalid shape, can only set "%s" for arrowheads`, obj.Shape.Value))
			}
		case "constraint":
			if !strings.EqualFold(obj.Shape.Value, d2target.ShapeSQLTable) {
				c.errorf(f.LastPrimaryKey(), `"constraint" keyword can only be used in "sql_table" shapes`)
			}
		}
		return
	}

	if strings.EqualFold(obj.Shape.Value, d2target.ShapeImage) && obj.OuterSequenceDiagram() == nil {
		c.errorf(f.LastRef().AST(), "image shapes cannot have children.")
		return
	}

	obj, ok := obj.HasChild([]string{f.Name.ScalarString()})
	if ok && f.Map() != nil {
		c.validateKeys(obj, f.Map())
	}
}

func (c *compiler) validateLabels(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if strings.EqualFold(obj.Shape.Value, d2target.ShapeText) {
			if obj.Attributes.Language != "" {
				// blockstrings have already been validated
				continue
			}
			if strings.TrimSpace(obj.Label.Value) == "" {
				c.errorf(obj.Label.MapKey, "shape text must have a non-empty label")
			}
		} else if strings.EqualFold(obj.Shape.Value, d2target.ShapeSQLTable) {
			if strings.Contains(obj.Label.Value, "\n") {
				c.errorf(obj.Label.MapKey, "shape sql_table cannot have newlines in label")
			}
		}
	}
}

func (c *compiler) validateNear(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.NearKey != nil {
			nearObj, isKey := g.Root.HasChild(d2graph.Key(obj.NearKey))
			_, isConst := d2ast.NearConstants[d2graph.Key(obj.NearKey)[0]]
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
					_, nearObjNearIsConst := d2ast.NearConstants[d2graph.Key(nearObj.NearKey)[0]]
					if nearObjNearIsConst {
						c.errorf(obj.NearKey, "near keys cannot be set to an object with a constant near key")
						continue
					}
				}
				if nearObj.ClosestGridDiagram() != nil {
					c.errorf(obj.NearKey, "near keys cannot be set to descendants of special objects, like grid cells")
					continue
				}
				if nearObj.OuterSequenceDiagram() != nil {
					c.errorf(obj.NearKey, "near keys cannot be set to descendants of special objects, like sequence diagram actors")
					continue
				}
			} else if isConst {
				if obj.Parent != g.Root {
					c.errorf(obj.NearKey, "constant near keys can only be set on root level shapes")
					continue
				}
			} else {
				c.errorf(obj.NearKey, "near key %#v must be the absolute path to a shape or one of the following constants: %s", d2format.Format(obj.NearKey), strings.Join(d2ast.NearConstantsArray, ", "))
				continue
			}
		}
	}

	for _, edge := range g.Edges {
		if edge.Src.IsConstantNear() && edge.Dst.IsDescendantOf(edge.Src) {
			c.errorf(edge.GetAstEdge(), "edge from constant near %#v cannot enter itself", edge.Src.AbsID())
			continue
		}
		if edge.Dst.IsConstantNear() && edge.Src.IsDescendantOf(edge.Dst) {
			c.errorf(edge.GetAstEdge(), "edge from constant near %#v cannot enter itself", edge.Dst.AbsID())
			continue
		}
	}

}

func (c *compiler) validatePositionsCompatibility(g *d2graph.Graph) {
	for _, o := range g.Objects {
		for _, pos := range []*d2graph.Scalar{o.Top, o.Left} {
			if pos != nil {
				if o.Parent != nil {
					if strings.EqualFold(o.Parent.Shape.Value, d2target.ShapeHierarchy) {
						c.errorf(pos.MapKey, `position keywords cannot be used with shape "hierarchy"`)
					}
					if o.OuterSequenceDiagram() != nil {
						c.errorf(pos.MapKey, `position keywords cannot be used inside shape "sequence_diagram"`)
					}
					if o.Parent.GridColumns != nil || o.Parent.GridRows != nil {
						c.errorf(pos.MapKey, `position keywords cannot be used with grids`)
					}
				}
			}
		}
	}
}

func (c *compiler) validateEdges(g *d2graph.Graph) {
	for _, edge := range g.Edges {
		// edges from a grid to something outside is ok
		//   grid -> outside : ok
		//   grid -> grid.cell : not ok
		//   grid -> grid.cell.inner : not ok
		if edge.Src.IsGridDiagram() && edge.Dst.IsDescendantOf(edge.Src) {
			c.errorf(edge.GetAstEdge(), "edge from grid diagram %#v cannot enter itself", edge.Src.AbsID())
			continue
		}
		if edge.Dst.IsGridDiagram() && edge.Src.IsDescendantOf(edge.Dst) {
			c.errorf(edge.GetAstEdge(), "edge from grid diagram %#v cannot enter itself", edge.Dst.AbsID())
			continue
		}
		if edge.Src.Parent.IsGridDiagram() && edge.Dst.IsDescendantOf(edge.Src) {
			c.errorf(edge.GetAstEdge(), "edge from grid cell %#v cannot enter itself", edge.Src.AbsID())
			continue
		}
		if edge.Dst.Parent.IsGridDiagram() && edge.Src.IsDescendantOf(edge.Dst) {
			c.errorf(edge.GetAstEdge(), "edge from grid cell %#v cannot enter itself", edge.Dst.AbsID())
			continue
		}
		if edge.Src.IsSequenceDiagram() && edge.Dst.IsDescendantOf(edge.Src) {
			c.errorf(edge.GetAstEdge(), "edge from sequence diagram %#v cannot enter itself", edge.Src.AbsID())
			continue
		}
		if edge.Dst.IsSequenceDiagram() && edge.Src.IsDescendantOf(edge.Dst) {
			c.errorf(edge.GetAstEdge(), "edge from sequence diagram %#v cannot enter itself", edge.Dst.AbsID())
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

		u, err := url.Parse(html.UnescapeString(obj.Link.Value))
		isRemote := err == nil && (u.Scheme != "" || strings.HasPrefix(u.Path, "/"))
		if isRemote {
			continue
		}

		if linkKey.Path[0].Unbox().ScalarString() != "root" {
			obj.Link = nil
			continue
		}

		if !hasBoard(g.RootBoard(), linkKey.IDA()) {
			obj.Link = nil
			continue
		}

		if slices.Equal(linkKey.StringIDA(), obj.Graph.IDA()) {
			obj.Link = nil
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

func hasBoard(root *d2graph.Graph, ida []d2ast.String) bool {
	if len(ida) == 0 {
		return true
	}
	if ida[0].ScalarString() == "root" && ida[0].IsUnquoted() {
		return hasBoard(root, ida[1:])
	}
	id := ida[0]
	if len(ida) == 1 {
		return root.Name == id.ScalarString()
	}
	nextID := ida[1]
	switch id.ScalarString() {
	case "layers":
		for _, b := range root.Layers {
			if b.Name == nextID.ScalarString() {
				return hasBoard(b, ida[2:])
			}
		}
	case "scenarios":
		for _, b := range root.Scenarios {
			if b.Name == nextID.ScalarString() {
				return hasBoard(b, ida[2:])
			}
		}
	case "steps":
		for _, b := range root.Steps {
			if b.Name == nextID.ScalarString() {
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

// Unused for now until shape: edge_group
func (c *compiler) preprocessSeqDiagrams(m *d2ir.Map) {
	for _, f := range m.Fields {
		if f.Name.ScalarString() == "shape" && f.Name.IsUnquoted() && f.Primary_.Value.ScalarString() == d2target.ShapeSequenceDiagram {
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
					e.ID.SrcPath = append([]d2ast.String{d2ast.FlatUnquotedString("_")}, e.ID.SrcPath...)
					e.ID.DstPath = append([]d2ast.String{d2ast.FlatUnquotedString("_")}, e.ID.DstPath...)
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
		d2ir.ParentMap(f).DeleteField(f.Name.ScalarString())
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
			if f.Name.ScalarString() == "shape" && f.Name.IsUnquoted() && f.Primary_.Value.ScalarString() == d2target.ShapeSequenceDiagram {
				return m
			}
		}
		n = m
	}
}

func compileConfig(ir *d2ir.Map) (*d2target.Config, error) {
	f := ir.GetField(d2ast.FlatUnquotedString("vars"), d2ast.FlatUnquotedString("d2-config"))
	if f == nil || f.Map() == nil {
		return nil, nil
	}

	configMap := f.Map()

	config := &d2target.Config{}

	f = configMap.GetField(d2ast.FlatUnquotedString("sketch"))
	if f != nil {
		val, _ := strconv.ParseBool(f.Primary().Value.ScalarString())
		config.Sketch = &val
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("theme-id"))
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.ThemeID = go2.Pointer(int64(val))
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("dark-theme-id"))
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.DarkThemeID = go2.Pointer(int64(val))
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("pad"))
	if f != nil {
		val, _ := strconv.Atoi(f.Primary().Value.ScalarString())
		config.Pad = go2.Pointer(int64(val))
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("layout-engine"))
	if f != nil {
		config.LayoutEngine = go2.Pointer(f.Primary().Value.ScalarString())
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("center"))
	if f != nil {
		val, _ := strconv.ParseBool(f.Primary().Value.ScalarString())
		config.Center = &val
	}

	f = configMap.GetField(d2ast.FlatUnquotedString("theme-overrides"))
	if f != nil {
		overrides, err := compileThemeOverrides(f.Map())
		if err != nil {
			return nil, err
		}
		config.ThemeOverrides = overrides
	}
	f = configMap.GetField(d2ast.FlatUnquotedString("dark-theme-overrides"))
	if f != nil {
		overrides, err := compileThemeOverrides(f.Map())
		if err != nil {
			return nil, err
		}
		config.DarkThemeOverrides = overrides
	}
	f = configMap.GetField(d2ast.FlatUnquotedString("data"))
	if f != nil && f.Map() != nil {
		config.Data = make(map[string]interface{})
		for _, f := range f.Map().Fields {
			if f.Primary() != nil {
				config.Data[f.Name.ScalarString()] = f.Primary().Value.ScalarString()
			} else if f.Composite != nil {
				var arr []interface{}
				switch c := f.Composite.(type) {
				case *d2ir.Array:
					for _, f := range c.Values {
						switch c := f.(type) {
						case *d2ir.Scalar:
							arr = append(arr, c.String())
						}
					}
				}
				config.Data[f.Name.ScalarString()] = arr
			}
		}
	}

	return config, nil
}

func compileThemeOverrides(m *d2ir.Map) (*d2target.ThemeOverrides, error) {
	if m == nil {
		return nil, nil
	}
	themeOverrides := d2target.ThemeOverrides{}

	err := &d2parser.ParseError{}
FOR:
	for _, f := range m.Fields {
		switch strings.ToUpper(f.Name.ScalarString()) {
		case "N1":
			themeOverrides.N1 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N2":
			themeOverrides.N2 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N3":
			themeOverrides.N3 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N4":
			themeOverrides.N4 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N5":
			themeOverrides.N5 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N6":
			themeOverrides.N6 = go2.Pointer(f.Primary().Value.ScalarString())
		case "N7":
			themeOverrides.N7 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B1":
			themeOverrides.B1 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B2":
			themeOverrides.B2 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B3":
			themeOverrides.B3 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B4":
			themeOverrides.B4 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B5":
			themeOverrides.B5 = go2.Pointer(f.Primary().Value.ScalarString())
		case "B6":
			themeOverrides.B6 = go2.Pointer(f.Primary().Value.ScalarString())
		case "AA2":
			themeOverrides.AA2 = go2.Pointer(f.Primary().Value.ScalarString())
		case "AA4":
			themeOverrides.AA4 = go2.Pointer(f.Primary().Value.ScalarString())
		case "AA5":
			themeOverrides.AA5 = go2.Pointer(f.Primary().Value.ScalarString())
		case "AB4":
			themeOverrides.AB4 = go2.Pointer(f.Primary().Value.ScalarString())
		case "AB5":
			themeOverrides.AB5 = go2.Pointer(f.Primary().Value.ScalarString())
		default:
			err.Errors = append(err.Errors, d2parser.Errorf(f.LastPrimaryKey(), fmt.Sprintf(`"%s" is not a valid theme code`, f.Name.ScalarString())).(d2ast.Error))
			continue FOR
		}
		if !go2.Contains(color.NamedColors, strings.ToLower(f.Primary().Value.ScalarString())) && !color.ColorHexRegex.MatchString(f.Primary().Value.ScalarString()) {
			err.Errors = append(err.Errors, d2parser.Errorf(f.LastPrimaryKey(), fmt.Sprintf(`expected "%s" to be a valid named color ("orange") or a hex code ("#f0ff3a")`, f.Name.ScalarString())).(d2ast.Error))
		}
	}

	if !err.Empty() {
		return nil, err
	}

	if themeOverrides != (d2target.ThemeOverrides{}) {
		return &themeOverrides, nil
	}
	return nil, nil
}

func (c *compiler) setDefaultShapes(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.Shape.Value == "" {
			if obj.OuterSequenceDiagram() != nil {
				obj.Shape.Value = d2target.ShapeRectangle
			} else if obj.Language == "latex" {
				obj.Shape.Value = d2target.ShapeText
			} else if obj.Language == "markdown" {
				obj.Shape.Value = d2target.ShapeText
			} else if obj.Language != "" {
				obj.Shape.Value = d2target.ShapeCode
			} else {
				obj.Shape.Value = d2target.ShapeRectangle
			}
		}
	}
}
