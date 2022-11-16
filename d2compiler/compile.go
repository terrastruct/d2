package d2compiler

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
)

// TODO: should Parse even be exported? guess not. IR should contain list of files and
// their AST.
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
		if !errors.As(err, &pe) {
			return nil, err
		}
	}

	return compileAST(path, pe, ast)
}

func compileAST(path string, pe d2parser.ParseError, ast *d2ast.Map) (*d2graph.Graph, error) {
	g := d2graph.NewGraph(ast)

	c := &compiler{
		path: path,
		err:  pe,
	}

	c.compileKeys(g.Root, ast)
	if len(c.err.Errors) == 0 {
		c.validateKeys(g.Root, ast)
	}
	c.compileEdges(g.Root, ast)
	// TODO: simplify removeContainer by running before compileEdges
	c.compileShapes(g.Root)
	c.validateNear(g)

	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	return g, nil
}

type compiler struct {
	path string
	err  d2parser.ParseError
}

func (c *compiler) errorf(start d2ast.Position, end d2ast.Position, f string, v ...interface{}) {
	r := d2ast.Range{
		Path:  c.path,
		Start: start,
		End:   end,
	}
	f = "%v: " + f
	v = append([]interface{}{r}, v...)
	c.err.Errors = append(c.err.Errors, d2ast.Error{
		Range:   r,
		Message: fmt.Sprintf(f, v...),
	})
}

func (c *compiler) compileKeys(obj *d2graph.Object, m *d2ast.Map) {
	for _, n := range m.Nodes {
		if n.MapKey != nil && n.MapKey.Key != nil && len(n.MapKey.Edges) == 0 {
			c.compileKey(obj, m, n.MapKey)
		}
	}
}

func (c *compiler) compileEdges(obj *d2graph.Object, m *d2ast.Map) {
	for _, n := range m.Nodes {
		if n.MapKey != nil {
			if len(n.MapKey.Edges) > 0 {
				obj := obj
				if n.MapKey.Key != nil {
					ida := d2graph.Key(n.MapKey.Key)
					parent, resolvedIDA, err := d2graph.ResolveUnderscoreKey(ida, obj)
					if err != nil {
						c.errorf(n.MapKey.Range.Start, n.MapKey.Range.End, err.Error())
						return
					}
					unresolvedObj := obj
					obj = parent.EnsureChild(resolvedIDA)

					parent.AppendReferences(ida, d2graph.Reference{
						Key: n.MapKey.Key,

						MapKey: n.MapKey,
						Scope:  m,
					}, unresolvedObj)
				}
				c.compileEdgeMapKey(obj, m, n.MapKey)
			}
			if n.MapKey.Key != nil && n.MapKey.Value.Map != nil {
				c.compileEdges(obj.EnsureChild(d2graph.Key(n.MapKey.Key)), n.MapKey.Value.Map)
			}
		}
	}
}

// compileArrowheads compiles keywords for edge arrowhead attributes by
// 1. creating a fake, detached parent
// 2. compiling the arrowhead field as a fake object onto that fake parent
// 3. transferring the relevant attributes onto the edge
func (c *compiler) compileArrowheads(edge *d2graph.Edge, m *d2ast.Map, mk *d2ast.Key) bool {
	arrowheadKey := mk.Key
	if mk.EdgeKey != nil {
		arrowheadKey = mk.EdgeKey
	}
	if arrowheadKey == nil || len(arrowheadKey.Path) == 0 {
		return false
	}
	key := arrowheadKey.Path[0].Unbox().ScalarString()
	var field *d2graph.Attributes
	if key == "source-arrowhead" {
		if edge.SrcArrowhead == nil {
			edge.SrcArrowhead = &d2graph.Attributes{}
		}
		field = edge.SrcArrowhead
	} else if key == "target-arrowhead" {
		if edge.DstArrowhead == nil {
			edge.DstArrowhead = &d2graph.Attributes{}
		}
		field = edge.DstArrowhead
	} else {
		return false
	}
	fakeParent := &d2graph.Object{
		Children: make(map[string]*d2graph.Object),
	}
	detachedMK := &d2ast.Key{
		Key:     arrowheadKey,
		Primary: mk.Primary,
		Value:   mk.Value,
	}
	c.compileKey(fakeParent, m, detachedMK)
	fakeObj := fakeParent.ChildrenArray[0]
	c.compileShapes(fakeObj)

	if fakeObj.Attributes.Shape.Value != "" {
		field.Shape = fakeObj.Attributes.Shape
	}
	if fakeObj.Attributes.Label.Value != "" && fakeObj.Attributes.Label.Value != "source-arrowhead" && fakeObj.Attributes.Label.Value != "target-arrowhead" {
		field.Label = fakeObj.Attributes.Label
	}
	if fakeObj.Attributes.Style.Filled != nil {
		field.Style.Filled = fakeObj.Attributes.Style.Filled
	}

	return true
}

func (c *compiler) compileAttributes(attrs *d2graph.Attributes, mk *d2ast.Key) {
	var reserved string
	var ok bool

	if mk.EdgeKey != nil {
		_, reserved, ok = c.compileFlatKey(mk.EdgeKey)
	} else if mk.Key != nil {
		_, reserved, ok = c.compileFlatKey(mk.Key)
	}
	if !ok {
		return
	}

	if reserved == "" || reserved == "label" {
		attrs.Label.MapKey = mk
	} else if reserved == "shape" {
		attrs.Shape.MapKey = mk
	} else if reserved == "opacity" {
		attrs.Style.Opacity = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "stroke" {
		attrs.Style.Stroke = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "fill" {
		attrs.Style.Fill = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "stroke-width" {
		attrs.Style.StrokeWidth = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "stroke-dash" {
		attrs.Style.StrokeDash = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "border-radius" {
		attrs.Style.BorderRadius = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "shadow" {
		attrs.Style.Shadow = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "3d" {
		// TODO this should be movd to validateKeys, as shape may not be set yet
		if attrs.Shape.Value != "" && !strings.EqualFold(attrs.Shape.Value, d2target.ShapeSquare) && !strings.EqualFold(attrs.Shape.Value, d2target.ShapeRectangle) {
			c.errorf(mk.Range.Start, mk.Range.End, `key "3d" can only be applied to squares and rectangles`)
			return
		}
		attrs.Style.ThreeDee = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "multiple" {
		attrs.Style.Multiple = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "font" {
		attrs.Style.Font = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "font-size" {
		attrs.Style.FontSize = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "font-color" {
		attrs.Style.FontColor = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "animated" {
		attrs.Style.Animated = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "bold" {
		attrs.Style.Bold = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "italic" {
		attrs.Style.Italic = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "underline" {
		attrs.Style.Underline = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "filled" {
		attrs.Style.Filled = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "width" {
		attrs.Width = &d2graph.Scalar{MapKey: mk}
	} else if reserved == "height" {
		attrs.Height = &d2graph.Scalar{MapKey: mk}
	}
}

func (c *compiler) compileKey(obj *d2graph.Object, m *d2ast.Map, mk *d2ast.Key) {
	ida, reserved, ok := c.compileFlatKey(mk.Key)
	if !ok {
		return
	}
	if reserved == "desc" {
		return
	}

	resolvedObj, resolvedIDA, err := d2graph.ResolveUnderscoreKey(ida, obj)
	if err != nil {
		c.errorf(mk.Range.Start, mk.Range.End, err.Error())
		return
	}
	if resolvedObj != obj {
		obj = resolvedObj
	}

	parent := obj
	if len(resolvedIDA) > 0 {
		unresolvedObj := obj
		obj = parent.EnsureChild(resolvedIDA)
		parent.AppendReferences(ida, d2graph.Reference{
			Key: mk.Key,

			MapKey: mk,
			Scope:  m,
		}, unresolvedObj)
	} else if obj.Parent == nil {
		// Top level reserved key set on root.
		return
	}

	if len(mk.Edges) > 0 {
		return
	}

	c.compileAttributes(&obj.Attributes, mk)
	if obj.Attributes.Style.Animated != nil {
		c.errorf(mk.Range.Start, mk.Range.End, `key "animated" can only be applied to edges`)
		return
	}

	c.applyScalar(&obj.Attributes, reserved, mk.Value.ScalarBox())
	if mk.Value.Map != nil {
		if reserved != "" {
			c.errorf(mk.Range.Start, mk.Range.End, "cannot set reserved key %q to a map", reserved)
			return
		}
		obj.Map = mk.Value.Map
		c.compileKeys(obj, mk.Value.Map)
	}

	c.applyScalar(&obj.Attributes, reserved, mk.Primary)
}

func (c *compiler) applyScalar(attrs *d2graph.Attributes, reserved string, box d2ast.ScalarBox) {
	scalar := box.Unbox()
	if scalar == nil {
		return
	}

	switch reserved {
	case "shape":
		in := d2target.IsShape(scalar.ScalarString())
		_, isArrowhead := d2target.Arrowheads[scalar.ScalarString()]
		if !in && !isArrowhead {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, "unknown shape %q", scalar.ScalarString())
			return
		}
		if box.Null != nil {
			attrs.Shape.Value = ""
		} else {
			attrs.Shape.Value = scalar.ScalarString()
		}
		if attrs.Shape.Value == d2target.ShapeCode {
			// Explicit code shape is plaintext.
			attrs.Language = d2target.ShapeText
		}
		return
	case "icon":
		iconURL, err := url.Parse(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, "bad icon url %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Icon = iconURL
		return
	case "near":
		nearKey, err := d2parser.ParseKey(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, "bad near key %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.NearKey = nearKey
		return
	case "tooltip":
		attrs.Tooltip = scalar.ScalarString()
		return
	case "width":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, "non-integer width %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Width.Value = scalar.ScalarString()
		return
	case "height":
		_, err := strconv.Atoi(scalar.ScalarString())
		if err != nil {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, "non-integer height %#v: %s", scalar.ScalarString(), err)
			return
		}
		attrs.Height.Value = scalar.ScalarString()
		return
	case "link":
		attrs.Link = scalar.ScalarString()
		return
	}

	if _, ok := d2graph.StyleKeywords[reserved]; ok {
		if err := attrs.Style.Apply(reserved, scalar.ScalarString()); err != nil {
			c.errorf(scalar.GetRange().Start, scalar.GetRange().End, err.Error())
		}
		return
	}

	if box.Null != nil {
		// TODO: delete obj
		attrs.Label.Value = ""
	} else {
		attrs.Label.Value = scalar.ScalarString()
	}

	bs := box.BlockString
	if bs != nil && reserved == "" {
		attrs.Language = bs.Tag
		fullTag, ok := ShortToFullLanguageAliases[bs.Tag]
		if ok {
			attrs.Language = fullTag
		}
		if attrs.Language == "markdown" {
			attrs.Shape.Value = d2target.ShapeText
		} else {
			attrs.Shape.Value = d2target.ShapeCode
		}
	}
}

func (c *compiler) compileEdgeMapKey(obj *d2graph.Object, m *d2ast.Map, mk *d2ast.Key) {
	if mk.EdgeIndex != nil {
		edge, ok := obj.HasEdge(mk)
		if ok {
			c.appendEdgeReferences(obj, m, mk)
			edge.References = append(edge.References, d2graph.EdgeReference{
				Edge: mk.Edges[0],

				MapKey:          mk,
				MapKeyEdgeIndex: 0,
				Scope:           m,
				ScopeObj:        obj,
			})
			c.compileEdge(edge, m, mk)
		}
		return
	}
	for i, e := range mk.Edges {
		if e.Src == nil || e.Dst == nil {
			continue
		}
		edge, err := obj.Connect(d2graph.Key(e.Src), d2graph.Key(e.Dst), e.SrcArrow == "<", e.DstArrow == ">", "")
		if err != nil {
			c.errorf(e.Range.Start, e.Range.End, err.Error())
			continue
		}
		edge.References = append(edge.References, d2graph.EdgeReference{
			Edge: e,

			MapKey:          mk,
			MapKeyEdgeIndex: i,
			Scope:           m,
			ScopeObj:        obj,
		})
		c.compileEdge(edge, m, mk)
	}
	c.appendEdgeReferences(obj, m, mk)
}

func (c *compiler) compileEdge(edge *d2graph.Edge, m *d2ast.Map, mk *d2ast.Key) {
	if mk.Key == nil && mk.EdgeKey == nil {
		if len(mk.Edges) == 1 {
			edge.Attributes.Label.MapKey = mk
		}
		c.applyScalar(&edge.Attributes, "", mk.Value.ScalarBox())
		c.applyScalar(&edge.Attributes, "", mk.Primary)
	} else {
		c.compileEdgeKey(edge, m, mk)
	}
	if mk.Value.Map != nil && mk.EdgeKey == nil {
		for _, n := range mk.Value.Map.Nodes {
			if n.MapKey == nil {
				continue
			}
			if len(n.MapKey.Edges) > 0 {
				c.errorf(mk.Range.Start, mk.Range.End, `edges cannot be nested within another edge`)
				continue
			}
			if n.MapKey.Key == nil {
				continue
			}
			for _, p := range n.MapKey.Key.Path {
				_, ok := d2graph.ReservedKeywords[strings.ToLower(p.Unbox().ScalarString())]
				if !ok {
					c.errorf(mk.Range.Start, mk.Range.End, `edge map keys must be reserved keywords`)
					return
				}
			}
			c.compileEdgeKey(edge, m, n.MapKey)
		}
	}
}

func (c *compiler) compileEdgeKey(edge *d2graph.Edge, m *d2ast.Map, mk *d2ast.Key) {
	var r string
	var ok bool

	// Give precedence to EdgeKeys
	// x.(a -> b)[0].style.opacity: 0.4
	// We want to compile the style.opacity, not the x
	if mk.EdgeKey != nil {
		_, r, ok = c.compileFlatKey(mk.EdgeKey)
	} else if mk.Key != nil {
		_, r, ok = c.compileFlatKey(mk.Key)
	}
	if !ok {
		return
	}

	ok = c.compileArrowheads(edge, m, mk)
	if ok {
		return
	}
	c.compileAttributes(&edge.Attributes, mk)
	c.applyScalar(&edge.Attributes, r, mk.Value.ScalarBox())
	if mk.Value.Map != nil {
		for _, n := range mk.Value.Map.Nodes {
			if n.MapKey != nil {
				c.compileEdgeKey(edge, m, n.MapKey)
			}
		}
	}
}

func (c *compiler) appendEdgeReferences(obj *d2graph.Object, m *d2ast.Map, mk *d2ast.Key) {
	for i, e := range mk.Edges {
		if e.Src != nil {
			ida := d2graph.Key(e.Src)

			parent, _, err := d2graph.ResolveUnderscoreKey(ida, obj)
			if err != nil {
				c.errorf(mk.Range.Start, mk.Range.End, err.Error())
				return
			}
			parent.AppendReferences(ida, d2graph.Reference{
				Key: e.Src,

				MapKey:          mk,
				MapKeyEdgeIndex: i,
				Scope:           m,
			}, obj)
		}
		if e.Dst != nil {
			ida := d2graph.Key(e.Dst)

			parent, _, err := d2graph.ResolveUnderscoreKey(ida, obj)
			if err != nil {
				c.errorf(mk.Range.Start, mk.Range.End, err.Error())
				return
			}
			parent.AppendReferences(ida, d2graph.Reference{
				Key: e.Dst,

				MapKey:          mk,
				MapKeyEdgeIndex: i,
				Scope:           m,
			}, obj)
		}
	}
}

func (c *compiler) compileFlatKey(k *d2ast.KeyPath) ([]string, string, bool) {
	k2 := *k
	var reserved string
	for i, s := range k.Path {
		keyword := strings.ToLower(s.Unbox().ScalarString())
		_, isReserved := d2graph.ReservedKeywords[keyword]
		_, isReservedHolder := d2graph.ReservedKeywordHolders[keyword]
		if isReserved && !isReservedHolder {
			reserved = keyword
			k2.Path = k2.Path[:i]
			break
		}
	}
	if len(k2.Path) < len(k.Path)-1 {
		c.errorf(k.Range.Start, k.Range.End, "reserved key %q cannot have children", reserved)
		return nil, "", false
	}
	return d2graph.Key(&k2), reserved, true
}

// TODO add more, e.g. C, bash
var ShortToFullLanguageAliases = map[string]string{
	"md": "markdown",
	"js": "javascript",
	"go": "golang",
	"py": "python",
	"rb": "ruby",
	"ts": "typescript",
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

	for _, obj := range obj.ChildrenArray {
		switch obj.Attributes.Shape.Value {
		case d2target.ShapeClass, d2target.ShapeSQLTable:
			flattenContainer(obj.Graph, obj)
		}
		if obj.IDVal == "style" {
			obj.Parent.Attributes.Style = obj.Attributes.Style
			if obj.Graph != nil {
				flattenContainer(obj.Graph, obj)
				removeObject(obj.Graph, obj)
			}
		}
	}
}

func (c *compiler) compileImage(obj *d2graph.Object) {
	if obj.Attributes.Icon == nil {
		c.errorf(obj.Attributes.Shape.MapKey.Range.Start, obj.Attributes.Shape.MapKey.Range.End, `image shape must include an "icon" field`)
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
	tableID := obj.AbsID()
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
			Name: col.IDVal,
			Type: typ,
		}
		// The only map a sql table field could have is to specify constraint
		if col.Map != nil {
			for _, n := range col.Map.Nodes {
				if n.MapKey.Key == nil || len(n.MapKey.Key.Path) == 0 {
					continue
				}
				if n.MapKey.Key.Path[0].Unbox().ScalarString() == "constraint" {
					d2Col.Constraint = n.MapKey.Value.StringBox().Unbox().ScalarString()
				}
			}
		}

		absID := col.AbsID()
		for _, e := range obj.Graph.Edges {
			srcID := e.Src.AbsID()
			dstID := e.Dst.AbsID()
			// skip edges between columns of the same table
			if strings.HasPrefix(srcID, tableID) && strings.HasPrefix(dstID, tableID) {
				continue
			}
			if srcID == absID {
				d2Col.Reference = strings.TrimPrefix(dstID, parentID+".")
				e.FromTableColumnIndex = new(int)
				*e.FromTableColumnIndex = len(obj.SQLTable.Columns)
			} else if dstID == absID {
				e.ToTableColumnIndex = new(int)
				*e.ToTableColumnIndex = len(obj.SQLTable.Columns)
			}
		}

		obj.SQLTable.Columns = append(obj.SQLTable.Columns, d2Col)
	}
}

// TODO too similar to flattenContainer, should reconcile in a refactor
func removeObject(g *d2graph.Graph, obj *d2graph.Object) {
	for i := 0; i < len(obj.Graph.Objects); i++ {
		if obj.Graph.Objects[i] == obj {
			obj.Graph.Objects = append(obj.Graph.Objects[:i], obj.Graph.Objects[i+1:]...)
			break
		}
	}
	delete(obj.Parent.Children, obj.ID)
	for i, child := range obj.Parent.ChildrenArray {
		if obj == child {
			obj.Parent.ChildrenArray = append(obj.Parent.ChildrenArray[:i], obj.Parent.ChildrenArray[i+1:]...)
			break
		}
	}
}

func flattenContainer(g *d2graph.Graph, obj *d2graph.Object) {
	absID := obj.AbsID()

	toRemove := map[*d2graph.Edge]struct{}{}
	toAdd := []*d2graph.Edge{}
	for i := 0; i < len(g.Edges); i++ {
		e := g.Edges[i]
		srcID := e.Src.AbsID()
		dstID := e.Dst.AbsID()

		srcIsChild := strings.HasPrefix(srcID, absID+".")
		dstIsChild := strings.HasPrefix(dstID, absID+".")
		if srcIsChild && dstIsChild {
			toRemove[e] = struct{}{}
		} else if srcIsChild {
			toRemove[e] = struct{}{}
			if dstID == absID {
				continue
			}
			toAdd = append(toAdd, e)
		} else if dstIsChild {
			toRemove[e] = struct{}{}
			if srcID == absID {
				continue
			}
			toAdd = append(toAdd, e)
		}
	}
	for _, e := range toAdd {
		var newEdge *d2graph.Edge
		if strings.HasPrefix(e.Src.AbsID(), absID+".") {
			newEdge, _ = g.Root.Connect(obj.AbsIDArray(), e.Dst.AbsIDArray(), e.SrcArrow, e.DstArrow, e.Attributes.Label.Value)
		} else {
			newEdge, _ = g.Root.Connect(e.Src.AbsIDArray(), obj.AbsIDArray(), e.SrcArrow, e.DstArrow, e.Attributes.Label.Value)
		}
		// TODO more attributes
		newEdge.FromTableColumnIndex = new(int)
		*newEdge.FromTableColumnIndex = *e.FromTableColumnIndex
		newEdge.ToTableColumnIndex = new(int)
		*newEdge.ToTableColumnIndex = *e.ToTableColumnIndex
		newEdge.Attributes.Label = e.Attributes.Label
		newEdge.References = e.References
	}
	updatedEdges := []*d2graph.Edge{}
	for _, e := range g.Edges {
		if _, is := toRemove[e]; is {
			continue
		}
		updatedEdges = append(updatedEdges, e)
	}
	g.Edges = updatedEdges

	for i := 0; i < len(g.Objects); i++ {
		child := g.Objects[i]
		if strings.HasPrefix(child.AbsID(), absID+".") {
			g.Objects = append(g.Objects[:i], g.Objects[i+1:]...)
			i--
			delete(obj.Children, child.ID)
			for i, child2 := range obj.ChildrenArray {
				if child == child2 {
					obj.ChildrenArray = append(obj.ChildrenArray[:i], obj.ChildrenArray[i+1:]...)
					break
				}
			}
		}
	}
}

func (c *compiler) validateKey(obj *d2graph.Object, m *d2ast.Map, mk *d2ast.Key) {
	ida, reserved, ok := c.compileFlatKey(mk.Key)
	if !ok {
		return
	}

	if reserved == "" && obj.Attributes.Shape.Value == d2target.ShapeImage {
		c.errorf(mk.Range.Start, mk.Range.End, "image shapes cannot have children.")
	}

	if reserved == "width" && obj.Attributes.Shape.Value != d2target.ShapeImage {
		c.errorf(mk.Range.Start, mk.Range.End, "width is only applicable to image shapes.")
	}
	if reserved == "height" && obj.Attributes.Shape.Value != d2target.ShapeImage {
		c.errorf(mk.Range.Start, mk.Range.End, "height is only applicable to image shapes.")
	}

	in := d2target.IsShape(obj.Attributes.Shape.Value)
	_, arrowheadIn := d2target.Arrowheads[obj.Attributes.Shape.Value]
	if !in && arrowheadIn {
		c.errorf(mk.Range.Start, mk.Range.End, fmt.Sprintf(`invalid shape, can only set "%s" for arrowheads`, obj.Attributes.Shape.Value))
	}

	resolvedObj, resolvedIDA, err := d2graph.ResolveUnderscoreKey(ida, obj)
	if err != nil {
		c.errorf(mk.Range.Start, mk.Range.End, err.Error())
		return
	}
	if resolvedObj != obj {
		obj = resolvedObj
	}

	parent := obj
	if len(resolvedIDA) > 0 {
		obj, _ = parent.HasChild(resolvedIDA)
	} else if obj.Parent == nil {
		return
	}

	if len(mk.Edges) > 0 {
		return
	}

	if mk.Value.Map != nil {
		c.validateKeys(obj, mk.Value.Map)
	}
}

func (c *compiler) validateKeys(obj *d2graph.Object, m *d2ast.Map) {
	for _, n := range m.Nodes {
		if n.MapKey != nil && n.MapKey.Key != nil && len(n.MapKey.Edges) == 0 {
			c.validateKey(obj, m, n.MapKey)
		}
	}
}

func (c *compiler) validateNear(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.Attributes.NearKey != nil {
			_, ok := g.Root.HasChild(d2graph.Key(obj.Attributes.NearKey))
			if !ok {
				c.errorf(obj.Attributes.NearKey.GetRange().Start, obj.Attributes.NearKey.GetRange().End, "near key %#v does not exist. It must be the absolute path to a shape.", d2format.Format(obj.Attributes.NearKey))
				continue
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
