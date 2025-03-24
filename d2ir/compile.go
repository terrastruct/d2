package d2ir

import (
	"html"
	"io/fs"
	"net/url"
	"path"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type globContext struct {
	root   *globContext
	refctx *RefContext

	// Set of BoardIDA that this glob has already applied to.
	appliedFields map[string]struct{}
	// Set of Edge IDs that this glob has already applied to.
	appliedEdges map[string]struct{}
}

type compiler struct {
	err *d2parser.ParseError

	fs      fs.FS
	imports []string
	// importStack is used to detect cyclic imports.
	importStack []string
	seenImports map[string]struct{}
	utf16Pos    bool

	// Stack of globs that must be recomputed at each new object in and below the current scope.
	globContextStack [][]*globContext
	// Used to prevent field globs causing infinite loops.
	globRefContextStack []*RefContext
	// Used to check whether ampersands are allowed in the current map.
	mapRefContextStack   []*RefContext
	lazyGlobBeingApplied bool
}

type CompileOptions struct {
	UTF16Pos bool
	// Pass nil to disable imports.
	FS fs.FS
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func Compile(ast *d2ast.Map, opts *CompileOptions) (*Map, []string, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}
	c := &compiler{
		err: &d2parser.ParseError{},
		fs:  opts.FS,

		seenImports: make(map[string]struct{}),
		utf16Pos:    opts.UTF16Pos,
	}
	m := &Map{}
	m.initRoot()
	m.parent.(*Field).References[0].Context_.Scope = ast
	m.parent.(*Field).References[0].Context_.ScopeAST = ast

	c.pushImportStack(&d2ast.Import{
		Path: []*d2ast.StringBox{d2ast.RawStringBox(ast.GetRange().Path, true)},
	})
	defer c.popImportStack()

	c.compileMap(m, ast, ast)
	c.compileSubstitutions(m, nil)
	c.overlayClasses(m)
	m.removeSuspendedFields()
	if !c.err.Empty() {
		return nil, nil, c.err
	}
	return m, c.imports, nil
}

func (c *compiler) overlayClasses(m *Map) {
	classes := m.GetField(d2ast.FlatUnquotedString("classes"))
	if classes == nil || classes.Map() == nil {
		return
	}

	layersField := m.GetField(d2ast.FlatUnquotedString("layers"))
	if layersField == nil {
		return
	}
	layers := layersField.Map()
	if layers == nil {
		return
	}

	for _, lf := range layers.Fields {
		if lf.Map() == nil || lf.Primary() != nil {
			continue
		}
		l := lf.Map()
		lClasses := l.GetField(d2ast.FlatUnquotedString("classes"))

		if lClasses == nil {
			lClasses = classes.Copy(l).(*Field)
			l.Fields = append(l.Fields, lClasses)
		} else if lClasses.Map() != nil {
			base := classes.Copy(l).(*Field)
			OverlayMap(base.Map(), lClasses.Map())
			l.DeleteField("classes")
			l.Fields = append(l.Fields, base)
		}

		c.overlayClasses(l)
	}
}

func (c *compiler) compileSubstitutions(m *Map, varsStack []*Map) {
	for _, f := range m.Fields {
		if f.Name == nil {
			continue
		}
		if f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() && f.Map() != nil {
			varsStack = append([]*Map{f.Map()}, varsStack...)
		}
	}
	for i := 0; i < len(m.Fields); i++ {
		f := m.Fields[i]
		if f.Primary() != nil {
			removed := c.resolveSubstitutions(varsStack, f)
			if removed {
				i--
			}
		}
		if arr, ok := f.Composite.(*Array); ok {
			for _, val := range arr.Values {
				if scalar, ok := val.(*Scalar); ok {
					removed := c.resolveSubstitutions(varsStack, scalar)
					if removed {
						i--
					}
				}
			}
		} else if f.Map() != nil {
			if f.Name != nil && f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() {
				c.compileSubstitutions(f.Map(), varsStack)
				c.validateConfigs(f.Map().GetField(d2ast.FlatUnquotedString("d2-config")))
			} else {
				c.compileSubstitutions(f.Map(), varsStack)
			}
		}
	}
	for _, e := range m.Edges {
		if e.Primary() != nil {
			c.resolveSubstitutions(varsStack, e)
		}
		if e.Map() != nil {
			c.compileSubstitutions(e.Map(), varsStack)
		}
	}
}

func (c *compiler) validateConfigs(configs *Field) {
	if configs == nil || configs.Map() == nil {
		return
	}

	if NodeBoardKind(ParentMap(ParentMap(configs))) == "" {
		c.errorf(configs.LastRef().AST(), `"%s" can only appear at root vars`, configs.Name.ScalarString())
		return
	}

	for _, f := range configs.Map().Fields {
		var val string
		if f.Primary() == nil {
			if f.Name.ScalarString() != "theme-overrides" && f.Name.ScalarString() != "dark-theme-overrides" && f.Name.ScalarString() != "data" {
				c.errorf(f.LastRef().AST(), `"%s" needs a value`, f.Name.ScalarString())
				continue
			}
		} else {
			val = f.Primary().Value.ScalarString()
		}

		switch f.Name.ScalarString() {
		case "sketch", "center":
			_, err := strconv.ParseBool(val)
			if err != nil {
				c.errorf(f.LastRef().AST(), `expected a boolean for "%s", got "%s"`, f.Name.ScalarString(), val)
				continue
			}
		case "theme-overrides", "dark-theme-overrides", "data":
			if f.Map() == nil {
				c.errorf(f.LastRef().AST(), `"%s" needs a map`, f.Name.ScalarString())
				continue
			}
		case "theme-id", "dark-theme-id":
			valInt, err := strconv.Atoi(val)
			if err != nil {
				c.errorf(f.LastRef().AST(), `expected an integer for "%s", got "%s"`, f.Name.ScalarString(), val)
				continue
			}
			if d2themescatalog.Find(int64(valInt)) == (d2themes.Theme{}) {
				c.errorf(f.LastRef().AST(), `%d is not a valid theme ID`, valInt)
				continue
			}
		case "pad":
			_, err := strconv.Atoi(val)
			if err != nil {
				c.errorf(f.LastRef().AST(), `expected an integer for "%s", got "%s"`, f.Name.ScalarString(), val)
				continue
			}
		case "layout-engine":
		default:
			c.errorf(f.LastRef().AST(), `"%s" is not a valid config`, f.Name.ScalarString())
		}
	}
}

func (c *compiler) resolveSubstitutions(varsStack []*Map, node Node) (removedField bool) {
	var subbed bool
	var resolvedField *Field

	switch s := node.Primary().Value.(type) {
	case *d2ast.UnquotedString:
		for i, box := range s.Value {
			if box.Substitution != nil {
				for i, vars := range varsStack {
					resolvedField = c.resolveSubstitution(vars, node, box.Substitution, i == 0)
					if resolvedField != nil {
						if resolvedField.Primary() != nil {
							if _, ok := resolvedField.Primary().Value.(*d2ast.Null); ok {
								resolvedField = nil
							}
						}
						break
					}
				}
				if resolvedField == nil {
					c.errorf(node.LastRef().AST(), `could not resolve variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
					return
				}
				if box.Substitution.Spread {
					if resolvedField.Composite == nil {
						c.errorf(box.Substitution, "cannot spread non-composite")
						continue
					}
					switch n := node.(type) {
					case *Scalar: // Array value
						resolvedArr, ok := resolvedField.Composite.(*Array)
						if !ok {
							c.errorf(box.Substitution, "cannot spread non-array into array")
							continue
						}
						arr := n.parent.(*Array)
						for i, s := range arr.Values {
							if s == n {
								arr.Values = append(append(arr.Values[:i], resolvedArr.Values...), arr.Values[i+1:]...)
								break
							}
						}
					case *Field:
						m := ParentMap(n)
						if resolvedField.Map() != nil {
							ExpandSubstitution(m, resolvedField.Map(), n)
						}
						// Remove the placeholder field
						for i, f2 := range m.Fields {
							if n == f2 {
								m.Fields = append(m.Fields[:i], m.Fields[i+1:]...)
								removedField = true
								break
							}
						}

						if removedField && len(m.globs) > 0 && !c.lazyGlobBeingApplied {
							origGlobStack := c.globContextStack
							c.globContextStack = append(c.globContextStack, m.globs)
							for _, gctx := range m.globs {
								old := c.lazyGlobBeingApplied
								c.lazyGlobBeingApplied = true
								c.compileKey(gctx.refctx)
								c.lazyGlobBeingApplied = old
							}
							c.globContextStack = origGlobStack
						}

					}
				}
				if resolvedField.Primary() == nil {
					if resolvedField.Composite == nil {
						c.errorf(node.LastRef().AST(), `cannot substitute variable without value: "%s"`, strings.Join(box.Substitution.IDA(), "."))
						return
					}
					if len(s.Value) > 1 {
						c.errorf(node.LastRef().AST(), `cannot substitute composite variable "%s" as part of a string`, strings.Join(box.Substitution.IDA(), "."))
						return
					}
					switch n := node.(type) {
					case *Field:
						n.Primary_ = nil
					case *Edge:
						n.Primary_ = nil
					}
				} else {
					if i == 0 && len(s.Value) == 1 {
						node.Primary().Value = resolvedField.Primary().Value
					} else {
						s.Value[i].String = go2.Pointer(resolvedField.Primary().Value.ScalarString())
						subbed = true
					}
				}
				if resolvedField.Composite != nil {
					switch n := node.(type) {
					case *Field:
						n.Composite = resolvedField.Composite
					case *Edge:
						if resolvedField.Composite.Map() == nil {
							c.errorf(node.LastRef().AST(), `cannot substitute array variable "%s" to an edge`, strings.Join(box.Substitution.IDA(), "."))
							return
						}
						n.Map_ = resolvedField.Composite.Map()
					}
				}
			}
		}
		if subbed {
			s.Coalesce()
		}
	case *d2ast.DoubleQuotedString:
		for i, box := range s.Value {
			if box.Substitution != nil {
				for i, vars := range varsStack {
					resolvedField = c.resolveSubstitution(vars, node, box.Substitution, i == 0)
					if resolvedField != nil {
						break
					}
				}
				if resolvedField == nil {
					c.errorf(node.LastRef().AST(), `could not resolve variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
					return
				}
				if resolvedField.Primary() == nil && resolvedField.Composite != nil {
					c.errorf(node.LastRef().AST(), `cannot substitute map variable "%s" in quotes`, strings.Join(box.Substitution.IDA(), "."))
					return
				}
				s.Value[i].String = go2.Pointer(resolvedField.Primary().Value.ScalarString())
				subbed = true
			}
		}
		if subbed {
			s.Coalesce()
		}
	case *d2ast.BlockString:
		variables := make(map[string]string)
		for _, vars := range varsStack {
			c.collectVariables(vars, variables)
		}
		preprocessedValue := textmeasure.ReplaceSubstitutionsMarkdown(s.Value, variables)

		// Update the block string value
		s.Value = preprocessedValue
	}
	return removedField
}

func (c *compiler) collectVariables(vars *Map, variables map[string]string) {
	if vars == nil {
		return
	}
	for _, f := range vars.Fields {
		if f.Primary() != nil {
			variables[f.Name.ScalarString()] = f.Primary().Value.ScalarString()
		} else if f.Map() != nil {
			nestedVars := make(map[string]string)
			c.collectVariables(f.Map(), nestedVars)
			for k, v := range nestedVars {
				variables[f.Name.ScalarString()+"."+k] = v
			}
			c.collectVariables(f.Map(), variables)
		}
	}
}

func (c *compiler) resolveSubstitution(vars *Map, node Node, substitution *d2ast.Substitution, isCurrentScopeVars bool) *Field {
	if vars == nil {
		return nil
	}

	fieldNode, fok := node.(*Field)
	parent := ParentField(node)

	for i, p := range substitution.Path {
		f := vars.GetField(p.Unbox())
		if f == nil {
			return nil
		}
		// Consider this case:
		//
		// ```
		// vars: {
		//   x: a
		// }
		// hi: {
		//   vars: {
		//     x: ${x}-b
		//   }
		//   yo: ${x}
		// }
		// ```
		//
		// When resolving hi.vars.x, the vars stack includes itself.
		// So this next if clause says, "ignore if we're using the current scope's vars to try to resolve a substitution that requires a var from further in the stack"
		if fok && fieldNode.Name != nil && fieldNode.Name.ScalarString() == p.Unbox().ScalarString() && isCurrentScopeVars && parent.Name.ScalarString() == "vars" && parent.Name.IsUnquoted() {
			return nil
		}

		if i == len(substitution.Path)-1 {
			return f
		}
		vars = f.Map()
	}
	return nil
}

func (c *compiler) overlay(base *Map, f *Field) {
	if f.Map() == nil || f.Primary() != nil {
		c.errorf(f.References[0].Context_.Key, "invalid %s", NodeBoardKind(f))
		return
	}
	base = base.CopyBase(f)
	// Certain fields should never carry forward.
	// If you give your scenario a label, you don't want all steps in a scenario to be labeled the same.
	base.DeleteField("label")
	OverlayMap(base, f.Map())
	f.Composite = base
}

func (g *globContext) copy() *globContext {
	g2 := *g
	g2.refctx = g.root.refctx.Copy()
	return &g2
}

func (g *globContext) copyApplied(from *globContext) {
	g.appliedFields = make(map[string]struct{})
	for k, v := range from.appliedFields {
		g.appliedFields[k] = v
	}
	g.appliedEdges = make(map[string]struct{})
	for k, v := range from.appliedEdges {
		g.appliedEdges[k] = v
	}
}

func (c *compiler) ampersandFilterMap(dst *Map, ast, scopeAST *d2ast.Map) bool {
	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			ok := c.ampersandFilter(&RefContext{
				Key:      n.MapKey,
				Scope:    ast,
				ScopeMap: dst,
				ScopeAST: scopeAST,
			})
			if n.MapKey.NotAmpersand {
				ok = !ok
			}
			if !ok {
				if len(c.mapRefContextStack) == 0 {
					return false
				}
				// Unapply glob if appropriate.
				gctx := c.getGlobContext(c.mapRefContextStack[len(c.mapRefContextStack)-1])
				if gctx == nil {
					return false
				}
				var ks string
				if gctx.refctx.Key.HasMultiGlob() {
					ks = d2format.Format(d2ast.MakeKeyPathString(IDA(dst)))
				} else {
					ks = d2format.Format(d2ast.MakeKeyPathString(BoardIDA(dst)))
				}
				delete(gctx.appliedFields, ks)
				delete(gctx.appliedEdges, ks)
				return false
			}
		}
	}
	return true
}

func (c *compiler) compileMap(dst *Map, ast, scopeAST *d2ast.Map) {
	var globs []*globContext
	if len(c.globContextStack) > 0 {
		previousGlobs := c.globContexts()
		// A root layer with existing glob context stack implies it's an import
		// In which case, the previous globs should be inherited (the else block)
		if NodeBoardKind(dst) == BoardLayer && !dst.Root() {
			for _, g := range previousGlobs {
				if g.refctx.Key.HasTripleGlob() {
					gctx2 := g.copy()
					gctx2.refctx.ScopeMap = dst
					globs = append(globs, gctx2)
				}
			}
		} else if NodeBoardKind(dst) == BoardScenario {
			for _, g := range previousGlobs {
				gctx2 := g.copy()
				gctx2.refctx.ScopeMap = dst
				if !g.refctx.Key.HasMultiGlob() {
					// Triple globs already apply independently to each board
					gctx2.copyApplied(g)
				}
				globs = append(globs, gctx2)
			}
			for _, g := range previousGlobs {
				g2 := g.copy()
				g2.refctx.ScopeMap = dst
				// We don't want globs applied in a given scenario to affect future boards
				// Copying the applied fields and edges keeps the applications scoped to this board
				// Note that this is different from steps, where applications carry over
				if !g.refctx.Key.HasMultiGlob() {
					// Triple globs already apply independently to each board
					g2.copyApplied(g)
				}
				globs = append(globs, g2)
			}
		} else if NodeBoardKind(dst) == BoardStep {
			for _, g := range previousGlobs {
				gctx2 := g.copy()
				gctx2.refctx.ScopeMap = dst
				globs = append(globs, gctx2)
			}
		} else {
			globs = append(globs, previousGlobs...)
		}
	}
	c.globContextStack = append(c.globContextStack, globs)
	defer func() {
		dst.globs = c.globContexts()
		c.globContextStack = c.globContextStack[:len(c.globContextStack)-1]
	}()

	ok := c.ampersandFilterMap(dst, ast, scopeAST)
	if !ok {
		return
	}

	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			c.compileKey(&RefContext{
				Key:      n.MapKey,
				Scope:    ast,
				ScopeMap: dst,
				ScopeAST: scopeAST,
			})
		case n.Substitution != nil:
			// placeholder field to be resolved at the end
			f := &Field{
				parent: dst,
				Primary_: &Scalar{
					Value: &d2ast.UnquotedString{
						Value: []d2ast.InterpolationBox{{Substitution: n.Substitution}},
					},
				},
				References: []*FieldReference{{
					Context_: &RefContext{
						Scope:    ast,
						ScopeMap: dst,
						ScopeAST: scopeAST,
					},
				}},
			}
			dst.Fields = append(dst.Fields, f)
		case n.Import != nil:
			// Spread import
			impn, ok := c._import(n.Import)
			if !ok {
				continue
			}
			if impn.Map() == nil {
				c.errorf(n.Import, "cannot spread import non map into map")
				continue
			}
			impn.(Importable).SetImportAST(n.Import)

			for _, gctx := range impn.Map().globs {
				if !gctx.refctx.Key.HasTripleGlob() {
					continue
				}
				gctx2 := gctx.copy()
				gctx2.refctx.ScopeMap = dst
				c.compileKey(gctx2.refctx)
				c.ensureGlobContext(gctx2.refctx)
			}

			scenariosField := impn.Map().GetField(d2ast.FlatUnquotedString("scenarios"))
			if scenariosField != nil && scenariosField.Map() != nil {
				for _, sf := range scenariosField.Map().Fields {
					c.overlay(dst, sf)
				}
			}

			stepsField := impn.Map().GetField(d2ast.FlatUnquotedString("steps"))
			if stepsField != nil && stepsField.Map() != nil {
				for _, sf := range stepsField.Map().Fields {
					c.overlay(dst, sf)
				}
			}

			OverlayMap(dst, impn.Map())
			impDir := n.Import.Dir()
			c.extendLinks(dst, ParentField(dst), impDir)

			if impnf, ok := impn.(*Field); ok {
				if impnf.Primary_ != nil {
					dstf := ParentField(dst)
					if dstf != nil {
						dstf.Primary_ = impnf.Primary_
					}
				}
			}
		}
	}
}

func (c *compiler) globContexts() []*globContext {
	return c.globContextStack[len(c.globContextStack)-1]
}

func (c *compiler) getGlobContext(refctx *RefContext) *globContext {
	for _, gctx := range c.globContexts() {
		if gctx.refctx.Equal(refctx) {
			return gctx
		}
	}
	return nil
}

func (c *compiler) ensureGlobContext(refctx *RefContext) *globContext {
	gctx := c.getGlobContext(refctx)
	if gctx != nil {
		return gctx
	}
	gctx = &globContext{
		refctx:        refctx,
		appliedFields: make(map[string]struct{}),
		appliedEdges:  make(map[string]struct{}),
	}
	gctx.root = gctx
	c.globContextStack[len(c.globContextStack)-1] = append(c.globContexts(), gctx)
	return gctx
}

func (c *compiler) compileKey(refctx *RefContext) {
	if refctx.Key.HasGlob() {
		// These printlns are for debugging infinite loops.
		// println("og", refctx.Edge, refctx.Key, refctx.Scope, refctx.ScopeMap, refctx.ScopeAST)
		for _, refctx2 := range c.globRefContextStack {
			// println("st", refctx2.Edge, refctx2.Key, refctx2.Scope, refctx2.ScopeMap, refctx2.ScopeAST)
			if refctx.Equal(refctx2) {
				// Break the infinite loop.
				return
			}
			// println("keys", d2format.Format(refctx2.Key), d2format.Format(refctx.Key))
		}
		c.globRefContextStack = append(c.globRefContextStack, refctx)
		defer func() {
			c.globRefContextStack = c.globRefContextStack[:len(c.globRefContextStack)-1]
		}()
		c.ensureGlobContext(refctx)
	}
	oldFields := refctx.ScopeMap.FieldCountRecursive()
	oldEdges := refctx.ScopeMap.EdgeCountRecursive()
	if len(refctx.Key.Edges) == 0 {
		c.compileField(refctx.ScopeMap, refctx.Key.Key, refctx)
	} else {
		c.compileEdges(refctx)
	}
	if oldFields != refctx.ScopeMap.FieldCountRecursive() || oldEdges != refctx.ScopeMap.EdgeCountRecursive() {
		for _, gctx2 := range c.globContexts() {
			// println(d2format.Format(gctx2.refctx.Key), d2format.Format(refctx.Key))
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx2.refctx)
			c.lazyGlobBeingApplied = old
		}
	}
}

func (c *compiler) compileField(dst *Map, kp *d2ast.KeyPath, refctx *RefContext) {
	if refctx.Key.Ampersand || refctx.Key.NotAmpersand {
		return
	}

	fa, err := dst.EnsureField(kp, refctx, true, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}

	for _, f := range fa {
		c._compileField(f, refctx)
	}
}

func (c *compiler) ampersandFilter(refctx *RefContext) bool {
	if !refctx.Key.Ampersand && !refctx.Key.NotAmpersand {
		return true
	}
	if len(c.mapRefContextStack) == 0 || !c.mapRefContextStack[len(c.mapRefContextStack)-1].Key.SupportsGlobFilters() {
		c.errorf(refctx.Key, "glob filters cannot be used outside globs")
		return false
	}
	if len(refctx.Key.Edges) > 0 {
		return true
	}

	keyPath := refctx.Key.Key
	if keyPath == nil || len(keyPath.Path) == 0 {
		return false
	}

	firstPart := keyPath.Path[0].Unbox().ScalarString()
	if (firstPart == "src" || firstPart == "dst") && len(keyPath.Path) > 1 {
		if len(c.mapRefContextStack) == 0 {
			return false
		}

		edge := ParentEdge(refctx.ScopeMap)
		if edge == nil {
			return false
		}

		var nodePath []d2ast.String
		if firstPart == "src" {
			nodePath = edge.ID.SrcPath
		} else {
			nodePath = edge.ID.DstPath
		}

		rootMap := RootMap(refctx.ScopeMap)
		node := rootMap.GetField(nodePath...)
		if node == nil || node.Map() == nil {
			return false
		}

		propKeyPath := &d2ast.KeyPath{
			Path: keyPath.Path[1:],
		}

		propKey := &d2ast.Key{
			Key:   propKeyPath,
			Value: refctx.Key.Value,
		}

		propRefCtx := &RefContext{
			Key:      propKey,
			ScopeMap: node.Map(),
			ScopeAST: refctx.ScopeAST,
		}

		fa, err := node.Map().EnsureField(propKeyPath, propRefCtx, false, c)
		if err != nil || len(fa) == 0 {
			return false
		}

		for _, f := range fa {
			if c._ampersandFilter(f, propRefCtx) {
				return true
			}
		}
		return false
	}

	fa, err := refctx.ScopeMap.EnsureField(refctx.Key.Key, refctx, false, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return false
	}
	if len(fa) == 0 {
		if refctx.Key.Value.ScalarBox().Unbox().ScalarString() == "*" {
			return false
		}
		// The field/edge has no value for this filter
		// But the filter might still match default, e.g. opacity 1
		// So we make a fake field for the default
		// NOTE: this does not apply to things that themes control, like stroke and fill
		// Nor does it apply to layout things like width and height
		switch refctx.Key.Key.Last().ScalarString() {
		case "shape":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString("rectangle"),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "border-radius", "stroke-dash":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString("0"),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "opacity":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString("1"),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "stroke-width":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString("2"),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "icon", "tooltip", "link":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString(""),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "shadow", "multiple", "3d", "animated", "filled":
			f := &Field{
				Primary_: &Scalar{
					Value: d2ast.FlatUnquotedString("false"),
				},
			}
			return c._ampersandFilter(f, refctx)
		case "leaf":
			raw := refctx.Key.Value.ScalarBox().Unbox().ScalarString()
			boolVal, err := strconv.ParseBool(raw)
			if err != nil {
				c.errorf(refctx.Key, `&leaf must be "true" or "false", got %q`, raw)
				return false
			}

			f := refctx.ScopeMap.Parent().(*Field)
			isLeaf := f.Map() == nil || !f.Map().IsContainer()
			return isLeaf == boolVal
		case "connected":
			raw := refctx.Key.Value.ScalarBox().Unbox().ScalarString()
			boolVal, err := strconv.ParseBool(raw)
			if err != nil {
				c.errorf(refctx.Key, `&connected must be "true" or "false", got %q`, raw)
				return false
			}
			f := refctx.ScopeMap.Parent().(*Field)
			isConnected := false
			for _, r := range f.References {
				if r.InEdge() {
					isConnected = true
					break
				}
			}
			return isConnected == boolVal
		case "label":
			f := &Field{}
			n := refctx.ScopeMap.Parent()
			if n.Primary() == nil {
				switch n := n.(type) {
				case *Field:
					// The label value for fields is their key value
					f.Primary_ = &Scalar{
						Value: n.Name,
					}
				case *Edge:
					// But for edges, it's nothing
					return false
				}
			} else {
				f.Primary_ = n.Primary()
			}
			return c._ampersandFilter(f, refctx)
		case "src":
			if len(c.mapRefContextStack) == 0 {
				return false
			}

			edge := ParentEdge(refctx.ScopeMap)
			if edge == nil {
				return false
			}

			filterValue := refctx.Key.Value.ScalarBox().Unbox().ScalarString()

			var srcParts []string
			for _, part := range edge.ID.SrcPath {
				srcParts = append(srcParts, part.ScalarString())
			}

			container := ParentField(edge)
			if container != nil && container.Name.ScalarString() != "root" {
				containerPath := []string{}
				curr := container
				for curr != nil && curr.Name.ScalarString() != "root" {
					containerPath = append([]string{curr.Name.ScalarString()}, containerPath...)
					curr = ParentField(curr)
				}

				srcStart := srcParts[0]
				if !strings.EqualFold(srcStart, containerPath[0]) {
					srcParts = append(containerPath, srcParts...)
				}
			}

			srcPath := strings.Join(srcParts, ".")

			return srcPath == filterValue

		case "dst":
			if len(c.mapRefContextStack) == 0 {
				return false
			}

			edge := ParentEdge(refctx.ScopeMap)
			if edge == nil {
				return false
			}

			filterValue := refctx.Key.Value.ScalarBox().Unbox().ScalarString()

			var dstParts []string
			for _, part := range edge.ID.DstPath {
				dstParts = append(dstParts, part.ScalarString())
			}

			// Find the container that holds this edge
			// Build the absolute path by prepending the container's path
			container := ParentField(edge)
			if container != nil && container.Name.ScalarString() != "root" {
				containerPath := []string{}
				curr := container
				for curr != nil && curr.Name.ScalarString() != "root" {
					containerPath = append([]string{curr.Name.ScalarString()}, containerPath...)
					curr = ParentField(curr)
				}

				dstStart := dstParts[0]
				if !strings.EqualFold(dstStart, containerPath[0]) {
					dstParts = append(containerPath, dstParts...)
				}
			}
			dstPath := strings.Join(dstParts, ".")

			return dstPath == filterValue
		default:
			return false
		}
	}
	for _, f := range fa {
		ok := c._ampersandFilter(f, refctx)
		if !ok {
			return false
		}
	}
	return true
}

func (c *compiler) _ampersandFilter(f *Field, refctx *RefContext) bool {
	if refctx.Key.Value.ScalarBox().Unbox() == nil {
		c.errorf(refctx.Key, "glob filters cannot be composites")
		return false
	}

	if a, ok := f.Composite.(*Array); ok {
		for _, v := range a.Values {
			if s, ok := v.(*Scalar); ok {
				if refctx.Key.Value.ScalarBox().Unbox().ScalarString() == s.Value.ScalarString() {
					return true
				}
			}
		}
	}

	if f.Primary_ == nil {
		return false
	}

	us, ok := refctx.Key.Value.ScalarBox().Unbox().(*d2ast.UnquotedString)

	if ok && us.Pattern != nil {
		return matchPattern(f.Primary_.Value.ScalarString(), us.Pattern)
	} else {
		if refctx.Key.Value.ScalarBox().Unbox().ScalarString() != f.Primary_.Value.ScalarString() {
			return false
		}
	}

	return true
}

func (c *compiler) _compileField(f *Field, refctx *RefContext) {
	// In case of filters, we need to pass filters before continuing
	if refctx.Key.Value.Map != nil && refctx.Key.Value.Map.HasFilter() {
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		c.mapRefContextStack = append(c.mapRefContextStack, refctx)
		ok := c.ampersandFilterMap(f.Map(), refctx.Key.Value.Map, refctx.ScopeAST)
		c.mapRefContextStack = c.mapRefContextStack[:len(c.mapRefContextStack)-1]
		if !ok {
			return
		}
	}

	if len(refctx.Key.Edges) == 0 && (refctx.Key.Primary.Null != nil || refctx.Key.Value.Null != nil) {
		// For vars, if we delete the field, it may just resolve to an outer scope var of the same name
		// Instead we keep it around, so that resolveSubstitutions can find it
		if !IsVar(ParentMap(f)) {
			ParentMap(f).DeleteField(f.Name.ScalarString())
			return
		}
	}

	if len(refctx.Key.Edges) == 0 && (refctx.Key.Primary.Suspension != nil || refctx.Key.Value.Suspension != nil) {
		if !c.lazyGlobBeingApplied {
			if refctx.Key.Primary.Suspension != nil {
				f.suspended = refctx.Key.Primary.Suspension.Value
			} else {
				f.suspended = refctx.Key.Value.Suspension.Value
			}
		}
		return
	}

	if refctx.Key.Primary.Unbox() != nil {
		if c.ignoreLazyGlob(f) {
			return
		}
		f.Primary_ = &Scalar{
			parent: f,
			Value:  refctx.Key.Primary.Unbox(),
		}
	}

	if refctx.Key.Value.Array != nil {
		a := &Array{
			parent: f,
		}
		c.compileArray(a, refctx.Key.Value.Array, refctx.ScopeAST)
		f.Composite = a
	} else if refctx.Key.Value.Map != nil {
		scopeAST := refctx.Key.Value.Map
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
			switch NodeBoardKind(f) {
			case BoardScenario:
				c.overlay(ParentBoard(f).Map(), f)
			case BoardStep:
				stepsMap := ParentMap(f)
				for i := range stepsMap.Fields {
					if stepsMap.Fields[i] == f {
						if i == 0 {
							c.overlay(ParentBoard(f).Map(), f)
						} else {
							c.overlay(stepsMap.Fields[i-1].Map(), f)
						}
						break
					}
				}
			case BoardLayer:
			default:
				// If new board type, use that as the new scope AST, otherwise, carry on
				scopeAST = refctx.ScopeAST
			}
		} else {
			scopeAST = refctx.ScopeAST
		}
		c.mapRefContextStack = append(c.mapRefContextStack, refctx)
		c.compileMap(f.Map(), refctx.Key.Value.Map, scopeAST)
		c.mapRefContextStack = c.mapRefContextStack[:len(c.mapRefContextStack)-1]
		switch NodeBoardKind(f) {
		case BoardScenario, BoardStep:
			c.overlayClasses(f.Map())
		}
	} else if refctx.Key.Value.Import != nil {
		// Non-spread import
		n, ok := c._import(refctx.Key.Value.Import)
		if !ok {
			return
		}
		n.(Importable).SetImportAST(refctx.Key.Value.Import)
		switch n := n.(type) {
		case *Field:
			if n.Primary_ != nil {
				f.Primary_ = n.Primary_.Copy(f).(*Scalar)
			}
			if n.Composite != nil {
				f.Composite = n.Composite.Copy(f).(Composite)
			}
		case *Map:
			f.Composite = &Map{
				parent: f,
			}
			switch NodeBoardKind(f) {
			case BoardScenario:
				c.overlay(ParentBoard(f).Map(), f)
			case BoardStep:
				stepsMap := ParentMap(f)
				for i := range stepsMap.Fields {
					if stepsMap.Fields[i] == f {
						if i == 0 {
							c.overlay(ParentBoard(f).Map(), f)
						} else {
							c.overlay(stepsMap.Fields[i-1].Map(), f)
						}
						break
					}
				}
			}
			OverlayMap(f.Map(), n)
			impDir := refctx.Key.Value.Import.Dir()
			c.extendLinks(f.Map(), f, impDir)
			switch NodeBoardKind(f) {
			case BoardScenario, BoardStep:
				c.overlayClasses(f.Map())
			}
		}
	} else if refctx.Key.Value.ScalarBox().Unbox() != nil {
		if c.ignoreLazyGlob(f) {
			return
		}
		f.Primary_ = &Scalar{
			parent: f,
			Value:  refctx.Key.Value.ScalarBox().Unbox(),
		}
		// If the link is a board, we need to transform it into an absolute path.
		if f.Name.ScalarString() == "link" && f.Name.IsUnquoted() {
			c.compileLink(f, refctx)
		}
	}
}

// Whether the current lazy glob being applied should not override the field
// if already set by a non glob key.
func (c *compiler) ignoreLazyGlob(n Node) bool {
	if c.lazyGlobBeingApplied && n.Primary() != nil {
		lastPrimaryRef := n.LastPrimaryRef()
		if lastPrimaryRef != nil && !lastPrimaryRef.DueToLazyGlob() {
			return true
		}
	}
	return false
}

// When importing a file, all of its board and icon links need to be extended to reflect their new path
func (c *compiler) extendLinks(m *Map, importF *Field, importDir string) {
	nodeBoardKind := NodeBoardKind(m)
	importIDA := IDA(importF)
	for _, f := range m.Fields {
		// A substitute or such
		if f.Name == nil {
			continue
		}
		if f.Name.ScalarString() == "link" && f.Name.IsUnquoted() {
			if nodeBoardKind != "" {
				c.errorf(f.LastRef().AST(), "a board itself cannot be linked; only objects within a board can be linked")
				continue
			}
			val := f.Primary().Value.ScalarString()

			u, err := url.Parse(html.UnescapeString(val))
			isRemote := err == nil && (u.Scheme != "" || strings.HasPrefix(u.Path, "/"))
			if isRemote {
				continue
			}

			link, err := d2parser.ParseKey(val)
			if err != nil {
				continue
			}
			linkIDA := link.IDA()
			if len(linkIDA) == 0 {
				continue
			}

			for _, id := range linkIDA[1:] {
				if id.ScalarString() == "_" && id.IsUnquoted() {
					if len(linkIDA) < 2 || len(importIDA) < 2 {
						break
					}
					linkIDA = append([]d2ast.String{linkIDA[0]}, linkIDA[2:]...)
					importIDA = importIDA[:len(importIDA)-2]
				} else {
					break
				}
			}

			extendedIDA := append(importIDA, linkIDA[1:]...)
			kp := d2ast.MakeKeyPathString(extendedIDA)
			s := d2format.Format(kp)
			f.Primary_.Value = d2ast.MakeValueBox(d2ast.FlatUnquotedString(s)).ScalarBox().Unbox()
		}
		if f.Name.ScalarString() == "icon" && f.Name.IsUnquoted() && f.Primary() != nil {
			val := f.Primary().Value.ScalarString()
			// It's likely a substitution
			if val == "" {
				continue
			}
			u, err := url.Parse(html.UnescapeString(val))
			isRemoteImg := err == nil && (u.Scheme != "" || strings.HasPrefix(u.Path, "/"))
			if isRemoteImg {
				continue
			}
			val = path.Join(importDir, val)
			f.Primary_.Value = d2ast.MakeValueBox(d2ast.FlatUnquotedString(val)).ScalarBox().Unbox()
		}
		if f.Map() != nil {
			c.extendLinks(f.Map(), importF, importDir)
		}
	}
}

func (c *compiler) compileLink(f *Field, refctx *RefContext) {
	val := refctx.Key.Value.ScalarBox().Unbox().ScalarString()
	link, err := d2parser.ParseKey(val)
	if err != nil {
		return
	}

	scopeIDA := IDA(refctx.ScopeMap)

	if len(scopeIDA) == 0 {
		return
	}

	linkIDA := link.IDA()
	if len(linkIDA) == 0 {
		return
	}

	if !linkIDA[0].IsUnquoted() {
		return
	}

	// If it doesn't start with one of these reserved words, the link is definitely not a board link.
	if !strings.EqualFold(linkIDA[0].ScalarString(), "layers") && !strings.EqualFold(linkIDA[0].ScalarString(), "scenarios") && !strings.EqualFold(linkIDA[0].ScalarString(), "steps") && linkIDA[0].ScalarString() != "_" {
		return
	}

	// Chop off the non-board portion of the scope, like if this is being defined on a nested object (e.g. `x.y.z`)
	for i := len(scopeIDA) - 1; i > 0; i-- {
		if scopeIDA[i-1].IsUnquoted() && (strings.EqualFold(scopeIDA[i-1].ScalarString(), "layers") || strings.EqualFold(scopeIDA[i-1].ScalarString(), "scenarios") || strings.EqualFold(scopeIDA[i-1].ScalarString(), "steps")) {
			scopeIDA = scopeIDA[:i+1]
			break
		}
		if scopeIDA[i-1].ScalarString() == "root" && scopeIDA[i-1].IsUnquoted() {
			scopeIDA = scopeIDA[:i]
			break
		}
	}

	// Resolve underscores
	for len(linkIDA) > 0 && linkIDA[0].ScalarString() == "_" && linkIDA[0].IsUnquoted() {
		if len(scopeIDA) < 2 {
			// Leave the underscore. It will fail in compiler as a standalone board,
			// but if imported, will get further resolved in extendLinks
			break
		}
		// pop 2 off path per one underscore
		scopeIDA = scopeIDA[:len(scopeIDA)-2]
		linkIDA = linkIDA[1:]
	}
	if len(scopeIDA) == 0 {
		scopeIDA = []d2ast.String{d2ast.FlatUnquotedString("root")}
	}

	// Create the absolute path by appending scope path with value specified
	scopeIDA = append(scopeIDA, linkIDA...)
	kp := d2ast.MakeKeyPathString(scopeIDA)
	f.Primary_.Value = d2ast.FlatUnquotedString(d2format.Format(kp))
}

func (c *compiler) compileEdges(refctx *RefContext) {
	if refctx.Key.Key == nil {
		c._compileEdges(refctx)
		return
	}

	fa, err := refctx.ScopeMap.EnsureField(refctx.Key.Key, refctx, true, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}
	for _, f := range fa {
		if _, ok := f.Composite.(*Array); ok {
			c.errorf(refctx.Key.Key, "cannot index into array")
			return
		}
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		refctx2 := *refctx
		refctx2.ScopeMap = f.Map()
		c._compileEdges(&refctx2)
	}
}

func (c *compiler) _compileEdges(refctx *RefContext) {
	eida := NewEdgeIDs(refctx.Key)
	for i, eid := range eida {
		if !eid.Glob && (refctx.Key.Primary.Null != nil || refctx.Key.Value.Null != nil) {
			refctx.ScopeMap.DeleteEdge(eid)
			continue
		}

		refctx = refctx.Copy()
		refctx.Edge = refctx.Key.Edges[i]

		var ea []*Edge
		if eid.Index != nil || eid.Glob {
			ea = refctx.ScopeMap.GetEdges(eid, refctx, c)
			if len(ea) == 0 {
				if !eid.Glob {
					c.errorf(refctx.Edge, "indexed edge does not exist")
				}
				continue
			}
			for _, e := range ea {
				if refctx.Key.Primary.Null != nil || refctx.Key.Value.Null != nil {
					refctx.ScopeMap.DeleteEdge(e.ID)
					continue
				}

				if refctx.Key.Value.Map != nil && refctx.Key.Value.Map.HasFilter() {
					if e.Map_ == nil {
						e.Map_ = &Map{
							parent: e,
						}
					}
					c.mapRefContextStack = append(c.mapRefContextStack, refctx)
					ok := c.ampersandFilterMap(e.Map_, refctx.Key.Value.Map, refctx.ScopeAST)
					c.mapRefContextStack = c.mapRefContextStack[:len(c.mapRefContextStack)-1]
					if !ok {
						continue
					}
				}

				if refctx.Key.Primary.Suspension != nil || refctx.Key.Value.Suspension != nil {
					if !c.lazyGlobBeingApplied {
						// Check if edge passes filter before applying suspension
						if refctx.Key.Value.Map != nil && refctx.Key.Value.Map.HasFilter() {
							if e.Map_ == nil {
								e.Map_ = &Map{
									parent: e,
								}
							}
							c.mapRefContextStack = append(c.mapRefContextStack, refctx)
							ok := c.ampersandFilterMap(e.Map_, refctx.Key.Value.Map, refctx.ScopeAST)
							c.mapRefContextStack = c.mapRefContextStack[:len(c.mapRefContextStack)-1]
							if !ok {
								continue
							}
						}

						var suspensionValue bool
						if refctx.Key.Primary.Suspension != nil {
							suspensionValue = refctx.Key.Primary.Suspension.Value
						} else {
							suspensionValue = refctx.Key.Value.Suspension.Value
						}
						e.suspended = suspensionValue

						// If we're unsuspending an edge, we should also unsuspend its src and dst objects
						// And their ancestors
						if !suspensionValue {
							srcPath, dstPath := e.ID.SrcPath, e.ID.DstPath

							// Make paths absolute if they're relative
							container := ParentField(e)
							if container != nil && container.Name.ScalarString() != "root" {
								containerPath := []d2ast.String{}
								curr := container
								for curr != nil && curr.Name.ScalarString() != "root" {
									containerPath = append([]d2ast.String{curr.Name}, containerPath...)
									curr = ParentField(curr)
								}

								if len(srcPath) > 0 && !strings.EqualFold(srcPath[0].ScalarString(), containerPath[0].ScalarString()) {
									absSrcPath := append([]d2ast.String{}, containerPath...)
									srcPath = append(absSrcPath, srcPath...)
								}

								if len(dstPath) > 0 && !strings.EqualFold(dstPath[0].ScalarString(), containerPath[0].ScalarString()) {
									absDstPath := append([]d2ast.String{}, containerPath...)
									dstPath = append(absDstPath, dstPath...)
								}
							}

							rootMap := RootMap(refctx.ScopeMap)
							srcObj := rootMap.GetField(srcPath...)
							dstObj := rootMap.GetField(dstPath...)

							// Unsuspend source node and all its ancestors
							if srcObj != nil {
								srcObj.suspended = false
								parent := ParentField(srcObj)
								for parent != nil && parent.Name.ScalarString() != "root" {
									parent.suspended = false
									parent = ParentField(parent)
								}
							}

							// Unsuspend destination node and all its ancestors
							if dstObj != nil {
								dstObj.suspended = false
								parent := ParentField(dstObj)
								for parent != nil && parent.Name.ScalarString() != "root" {
									parent.suspended = false
									parent = ParentField(parent)
								}
							}
						}
					}
				}

				e.References = append(e.References, &EdgeReference{
					Context_:       refctx,
					DueToGlob_:     len(c.globRefContextStack) > 0,
					DueToLazyGlob_: c.lazyGlobBeingApplied,
				})
				refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Src, refctx, c)
				refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Dst, refctx, c)
			}
		} else {
			var err error
			ea, err = refctx.ScopeMap.CreateEdge(eid, refctx, c)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
		}

		for _, e := range ea {
			if refctx.Key.EdgeKey != nil {
				if e.Map_ == nil {
					e.Map_ = &Map{
						parent: e,
					}
				}
				c.compileField(e.Map_, refctx.Key.EdgeKey, refctx)
			} else {
				if refctx.Key.Primary.Unbox() != nil && refctx.Key.Primary.Suspension == nil {
					if c.ignoreLazyGlob(e) {
						return
					}
					e.Primary_ = &Scalar{
						parent: e,
						Value:  refctx.Key.Primary.Unbox(),
					}
				}
				if refctx.Key.Value.Array != nil {
					c.errorf(refctx.Key.Value.Unbox(), "edges cannot be assigned arrays")
					continue
				} else if refctx.Key.Value.Map != nil {
					if e.Map_ == nil {
						e.Map_ = &Map{
							parent: e,
						}
					}
					c.mapRefContextStack = append(c.mapRefContextStack, refctx)
					c.compileMap(e.Map_, refctx.Key.Value.Map, refctx.ScopeAST)
					c.mapRefContextStack = c.mapRefContextStack[:len(c.mapRefContextStack)-1]
				} else if refctx.Key.Value.ScalarBox().Unbox() != nil && refctx.Key.Value.Suspension == nil {
					if c.ignoreLazyGlob(e) {
						return
					}
					e.Primary_ = &Scalar{
						parent: e,
						Value:  refctx.Key.Value.ScalarBox().Unbox(),
					}
				}
			}
		}
	}
}

func (c *compiler) compileArray(dst *Array, a *d2ast.Array, scopeAST *d2ast.Map) {
	for _, an := range a.Nodes {
		var irv Value
		switch v := an.Unbox().(type) {
		case *d2ast.Array:
			ira := &Array{
				parent: dst,
			}
			c.compileArray(ira, v, scopeAST)
			irv = ira
		case *d2ast.Map:
			irm := &Map{
				parent: dst,
			}
			c.compileMap(irm, v, scopeAST)
			irv = irm
		case d2ast.Scalar:
			irv = &Scalar{
				parent: dst,
				Value:  v,
			}
		case *d2ast.Import:
			n, ok := c._import(v)
			if !ok {
				continue
			}
			n.(Importable).SetImportAST(v)
			switch n := n.(type) {
			case *Field:
				if v.Spread {
					a, ok := n.Composite.(*Array)
					if !ok {
						c.errorf(v, "can only spread import array into array")
						continue
					}
					dst.Values = append(dst.Values, a.Values...)
					continue
				}
				if n.Composite != nil {
					irv = n.Composite
				} else {
					irv = n.Primary_
				}
			case *Map:
				if v.Spread {
					c.errorf(v, "can only spread import array into array")
					continue
				}
				irv = n
			}
		case *d2ast.Substitution:
			irv = &Scalar{
				parent: dst,
				Value: &d2ast.UnquotedString{
					Value: []d2ast.InterpolationBox{{Substitution: an.Substitution}},
				},
			}
		case *d2ast.Comment:
			continue
		}

		dst.Values = append(dst.Values, irv)
	}
}

func (m *Map) removeSuspendedFields() {
	if m == nil {
		return
	}

	for _, f := range m.Fields {
		if f.Map() != nil {
			f.Map().removeSuspendedFields()
		}
	}

	for i := len(m.Fields) - 1; i >= 0; i-- {
		if m.Fields[i].Name == nil {
			continue
		}
		_, isReserved := d2ast.ReservedKeywords[m.Fields[i].Name.ScalarString()]
		if isReserved {
			continue
		}
		if m.Fields[i].suspended {
			m.DeleteField(m.Fields[i].Name.ScalarString())
		}
	}

	for _, e := range m.Edges {
		if e.Map() != nil {
			e.Map().removeSuspendedFields()
		}
	}
	for i := len(m.Edges) - 1; i >= 0; i-- {
		if m.Edges[i].suspended {
			m.DeleteEdge(m.Edges[i].ID)
		}
	}
}
