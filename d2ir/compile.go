package d2ir

import (
	"context"
	"errors"
	"html"
	"io/fs"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/d2lang/util-go/go2"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2format"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/textmeasure"
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
	err                *d2parser.ParseError
	ctx                context.Context
	contextErr         error
	expansionErr       error
	globExpansionErr   error
	halted             bool
	variableExpansion  *variableExpansionBudget
	globExpansion      *globExpansionBudget
	edgeExpansion      *edgeExpansionBudget
	edgeExpansionWork  *edgeExpansionWorkBudget
	edgeExpansionPairs map[edgeExpansionPair]struct{}

	fs      fs.FS
	imports []string
	// importStack is used to detect cyclic imports.
	importStack []string
	seenImports map[string]struct{}
	// parsedImports and importTemplates are immutable per-compilation caches.
	// Callers always receive deep copies so importer substitutions, references,
	// and glob state cannot leak between import sites.
	parsedImports   map[string]*d2ast.Map
	importTemplates map[string]*importTemplate
	// Composite maps may be revisited through valid variable aliases. Report a
	// rejected cycle once at its source substitution rather than once per alias.
	reportedCompositeCycles map[*d2ast.Substitution]struct{}
	utf16Pos                bool

	// Stack of globs that must be recomputed at each new object in and below the current scope.
	globContextStack [][]*globContext
	// Used to prevent field globs causing infinite loops.
	globRefContextStack []*RefContext
	// Used to check whether ampersands are allowed in the current map.
	mapRefContextStack   []*RefContext
	lazyGlobBeingApplied bool

	// Lazy field globs are evaluated against newly created fields through a
	// deterministic worklist. This avoids rescanning the entire IR after each
	// insertion while retaining source-order rule application.
	lazyGlobTarget       *Field
	lazyGlobWorklist     []*Field
	lazyGlobQueued       map[*Field]struct{}
	applyingLazyWorklist bool
	lazySettledVersions  map[*Map]uint64
	lazyPostTargets      []*Field
	lazyPostQueued       map[*Field]struct{}
}

type CompileOptions struct {
	// Context stops parsing and compilation when canceled. A nil Context is
	// treated as context.Background().
	Context  context.Context
	UTF16Pos bool
	// MaxVariableExpansion bounds work added by substitutions and automatic
	// copies. Zero uses DefaultMaxVariableExpansion.
	MaxVariableExpansion int64
	// MaxGlobExpansion bounds work performed by glob matching and
	// materialization. Zero uses DefaultMaxGlobExpansion. Explicit source fields
	// are not counted as materialization work.
	MaxGlobExpansion int64
	// MaxEdgeExpansion bounds distinct edge-segment and endpoint combinations
	// considered by edge globs. Zero uses DefaultMaxEdgeExpansion. Explicit edges
	// do not consume this budget.
	MaxEdgeExpansion int64
	// MaxEdgeExpansionWork bounds all endpoint-pair examinations performed by
	// edge globs, including lazy replays. Zero uses DefaultMaxEdgeExpansionWork.
	MaxEdgeExpansionWork int64
	// FS resolves imports. Nil disables imports. The lib/localfile package
	// provides rooted and explicit unrestricted host-filesystem policies.
	FS fs.FS
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func Compile(ast *d2ast.Map, opts *CompileOptions) (*Map, []string, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	variableExpansion, err := newVariableExpansionBudget(opts.MaxVariableExpansion)
	if err != nil {
		return nil, nil, err
	}
	globExpansion, err := newGlobExpansionBudget(opts.MaxGlobExpansion)
	if err != nil {
		return nil, nil, err
	}
	edgeExpansion, err := newEdgeExpansionBudget(opts.MaxEdgeExpansion)
	if err != nil {
		return nil, nil, err
	}
	edgeExpansionWork, err := newEdgeExpansionWorkBudget(opts.MaxEdgeExpansionWork)
	if err != nil {
		return nil, nil, err
	}
	c := &compiler{
		err:               &d2parser.ParseError{},
		ctx:               ctx,
		fs:                opts.FS,
		variableExpansion: variableExpansion,
		globExpansion:     globExpansion,
		edgeExpansion:     edgeExpansion,
		edgeExpansionWork: edgeExpansionWork,

		seenImports:             make(map[string]struct{}),
		parsedImports:           make(map[string]*d2ast.Map),
		importTemplates:         make(map[string]*importTemplate),
		reportedCompositeCycles: make(map[*d2ast.Substitution]struct{}),
		utf16Pos:                opts.UTF16Pos,
	}
	m := &Map{}
	m.initRoot()
	m.variableExpansion = variableExpansion
	m.parent.(*Field).References[0].Context_.Scope = ast
	m.parent.(*Field).References[0].Context_.ScopeAST = ast

	c.pushImportStack(&d2ast.Import{
		Path: []*d2ast.StringBox{d2ast.RawStringBox(ast.GetRange().Path, true)},
	})
	defer c.popImportStack()

	c.compileMap(m, ast, ast)
	if c.contextErr != nil {
		return nil, nil, c.contextErr
	}
	if err := c.compileLimitError(); err != nil {
		return nil, nil, err
	}
	c.compileSubstitutions(m, nil)
	if c.contextErr != nil {
		return nil, nil, c.contextErr
	}
	if err := c.compileLimitError(); err != nil {
		return nil, nil, err
	}
	c.overlayClasses(m)
	if c.contextErr != nil {
		return nil, nil, c.contextErr
	}
	if err := c.compileLimitError(); err != nil {
		return nil, nil, err
	}
	// Substitutions can grow shared nodes after an earlier alias inserted them
	// (for example through a forward scalar chain in an array spread). Recheck
	// the fully resolved IR before cleanup or any public caller can observe it.
	if err := ReserveVariableExpansionAliases(ctx, m); err != nil {
		return nil, nil, err
	}
	m.removeSuspendedFields()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := c.compileLimitError(); err != nil {
		return nil, nil, err
	}
	if !c.err.Empty() {
		return nil, nil, c.err
	}
	return m, c.imports, nil
}

func (c *compiler) compileLimitError() error {
	if c.expansionErr == nil && c.globExpansionErr == nil {
		return nil
	}
	if !c.err.Empty() {
		return c.err
	}
	if c.expansionErr != nil {
		return c.expansionErr
	}
	return c.globExpansionErr
}

func (c *compiler) overlayClasses(m *Map) {
	if c.stopped() {
		return
	}
	classes := m.getFieldIndexed(d2ast.FlatUnquotedString("classes"))
	if classes == nil || classes.Map() == nil {
		return
	}

	layersField := m.getFieldIndexed(d2ast.FlatUnquotedString("layers"))
	if layersField == nil {
		return
	}
	layers := layersField.Map()
	if layers == nil {
		return
	}

	for _, lf := range layers.Fields {
		if c.stopped() {
			return
		}
		if lf.Map() == nil || lf.Primary() != nil {
			continue
		}
		l := lf.Map()
		lClasses := l.getFieldIndexed(d2ast.FlatUnquotedString("classes"))

		if lClasses == nil {
			if !c.reserveVariableCopy(lf.LastRef().AST(), classes) {
				return
			}
			lClasses = classes.Copy(l).(*Field)
			l.appendField(lClasses)
		} else if lClasses.Map() != nil {
			if !c.reserveVariableCopy(lf.LastRef().AST(), classes) {
				return
			}
			base := classes.Copy(l).(*Field)
			if !c.reserveVariableCopy(lf.LastRef().AST(), lClasses.Map()) {
				return
			}
			overlayMapIndexed(base.Map(), lClasses.Map())
			l.DeleteField("classes")
			l.appendField(base)
		}

		c.overlayClasses(l)
	}
}

func (c *compiler) compileSubstitutions(m *Map, varsStack []*Map) {
	c.compileSubstitutionsWalk(m, varsStack, make(map[Node]struct{}), nil)
}

func (c *compiler) compileSubstitutionsWalk(m *Map, varsStack []*Map, seen map[Node]struct{}, source d2ast.Node) {
	if c.stopped() {
		return
	}
	if !c.visitSubstitutionNode(m, source, seen) {
		return
	}
	for _, f := range m.Fields {
		if c.stopped() {
			return
		}
		if f.Name == nil {
			continue
		}
		if f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() && f.Map() != nil {
			varsStack = append([]*Map{f.Map()}, varsStack...)
		}
	}
	for i := 0; i < len(m.Fields); i++ {
		if c.stopped() {
			return
		}
		f := m.Fields[i]
		if !c.visitSubstitutionNode(f, f.LastRef().AST(), seen) {
			return
		}
		if f.Primary() != nil {
			if !c.visitSubstitutionNode(f.Primary(), f.LastRef().AST(), seen) {
				return
			}
			removed := c.resolveSubstitutions(varsStack, f)
			if removed {
				i--
			}
		}
		if arr, ok := f.Composite.(*Array); ok {
			if !c.visitSubstitutionNode(arr, f.LastRef().AST(), seen) {
				return
			}
			for _, val := range arr.Values {
				if !c.visitSubstitutionNode(val, f.LastRef().AST(), seen) {
					return
				}
				if scalar, ok := val.(*Scalar); ok {
					removed := c.resolveSubstitutions(varsStack, scalar)
					if removed {
						i--
					}
				}
			}
		} else if f.Map() != nil {
			if f.Name != nil && f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() {
				c.compileSubstitutionsWalk(f.Map(), varsStack, seen, f.LastRef().AST())
				c.validateConfigs(f.Map().getFieldIndexed(d2ast.FlatUnquotedString("d2-config")))
			} else {
				c.compileSubstitutionsWalk(f.Map(), varsStack, seen, f.LastRef().AST())
			}
		}
	}
	for _, e := range m.Edges {
		if c.stopped() {
			return
		}
		if !c.visitSubstitutionNode(e, e.LastRef().AST(), seen) {
			return
		}
		if e.Primary() != nil {
			if !c.visitSubstitutionNode(e.Primary(), e.LastRef().AST(), seen) {
				return
			}
			c.resolveSubstitutions(varsStack, e)
		}
		if e.Map() != nil {
			c.compileSubstitutionsWalk(e.Map(), varsStack, seen, e.LastRef().AST())
		}
	}
}

func (c *compiler) visitSubstitutionNode(node Node, source d2ast.Node, seen map[Node]struct{}) bool {
	if node == nil {
		return true
	}
	if _, ok := seen[node]; ok {
		return c.reserveVariableExpansion(source, variableExpansionNodeUnits(node))
	}
	seen[node] = struct{}{}
	return true
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
		case "sketch", "isometric", "center":
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
	if c.stopped() {
		return false
	}
	var subbed bool
	var resolvedField *Field

	switch s := node.Primary().Value.(type) {
	case *d2ast.UnquotedString:
		for i, box := range s.Value {
			if c.stopped() {
				return
			}
			if box.Substitution != nil {
				for i, vars := range varsStack {
					if c.stopped() {
						return
					}
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
				if resolvedField.Composite != nil && substitutionUsesComposite(node, box.Substitution.Spread) {
					if compositeContainsNode(resolvedField.Composite, node) ||
						(box.Substitution.Spread && c.spreadCompositeReferencesNode(resolvedField.Composite, node, varsStack)) {
						c.reportCompositeCycle(box.Substitution)
						return
					}
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
						// The values remain shared in the IR, but public IR consumers may
						// later copy or marshal every logical occurrence. Charge the full
						// recursively reachable value cost before inserting the aliases.
						if !c.reserveVariableCopy(box.Substitution, resolvedArr) {
							return
						}
						for i, s := range arr.Values {
							if s == n {
								arr.Values = append(append(arr.Values[:i], resolvedArr.Values...), arr.Values[i+1:]...)
								break
							}
						}
					case *Field:
						m := ParentMap(n)
						if resolvedField.Map() != nil {
							if hasUnresolvedMapSpread(resolvedField.Map()) {
								c.errorf(box.Substitution, `cannot spread composite variable "%s" before its spread substitutions are resolved`, strings.Join(box.Substitution.IDA(), "."))
								return
							}
							if !c.reserveVariableCopy(box.Substitution, resolvedField.Map()) {
								return
							}
							expandSubstitutionIndexed(m, resolvedField.Map(), n)
						}
						// Remove the placeholder field
						for i, f2 := range m.Fields {
							if n == f2 {
								m.removeField(i)
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
						if !c.reserveVariableExpansion(box.Substitution, int64(len(resolvedField.Primary().Value.ScalarString()))) {
							return
						}
						node.Primary().Value = resolvedField.Primary().Value
					} else {
						s.Value[i].String = go2.Pointer(resolvedField.Primary().Value.ScalarString())
						subbed = true
					}
				}
				if resolvedField.Composite != nil {
					switch n := node.(type) {
					case *Field:
						if n.Composite != nil {
							n.Composite = n.Composite.Copy(resolvedField.Composite).(Composite)
						} else {
							n.Composite = resolvedField.Composite
						}
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
			if !c.reserveVariableExpansion(node.LastRef().AST(), interpolationBytes(s.Value)) {
				return
			}
			s.Coalesce()
		}
	case *d2ast.DoubleQuotedString:
		for i, box := range s.Value {
			if c.stopped() {
				return
			}
			if box.Substitution != nil {
				for i, vars := range varsStack {
					if c.stopped() {
						return
					}
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
			if !c.reserveVariableExpansion(node.LastRef().AST(), interpolationBytes(s.Value)) {
				return
			}
			s.Coalesce()
		}
	case *d2ast.BlockString:
		if !strings.Contains(s.Value, "${") {
			return
		}
		variables := make(map[string]string)
		seen := make(map[Node]struct{})
		for _, vars := range varsStack {
			c.collectVariables(vars, variables, seen, node.LastRef().AST())
		}
		if c.stopped() {
			return
		}
		preprocessedValue, expandedBytes, err := textmeasure.ReplaceSubstitutionsMarkdownBounded(c.ctx, s.Value, variables, c.variableExpansion.remaining())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				c.contextErr = err
				c.halted = true
				return
			}
			c.errorf(node.LastRef().AST(), "variable substitution expansion exceeds limit of %d work units", c.variableExpansion.limit)
			c.halted = true
			return
		}
		if expandedBytes > 0 && !c.reserveVariableExpansion(node.LastRef().AST(), expandedBytes) {
			return
		}

		// Update the block string value
		s.Value = preprocessedValue
	}
	return removedField
}

func interpolationBytes(values []d2ast.InterpolationBox) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	var total int64
	for _, box := range values {
		if box.Substitution == nil || box.String == nil {
			continue
		}
		n := int64(len(*box.String))
		if n > maxInt64-total {
			return maxInt64
		}
		total += n
	}
	return total
}

func (c *compiler) reportCompositeCycle(substitution *d2ast.Substitution) {
	if _, reported := c.reportedCompositeCycles[substitution]; reported {
		return
	}
	if c.reportedCompositeCycles == nil {
		c.reportedCompositeCycles = make(map[*d2ast.Substitution]struct{})
	}
	c.reportedCompositeCycles[substitution] = struct{}{}
	c.errorf(substitution, `cyclic composite variable reference "%s"`, strings.Join(substitution.IDA(), "."))
}

func substitutionUsesComposite(node Node, spread bool) bool {
	switch node.(type) {
	case *Field, *Edge:
		return true
	case *Scalar:
		return spread
	default:
		return false
	}
}

// spreadCompositeReferencesNode follows unresolved spread substitutions
// anywhere below composite without mutating it. Map spread expansion copies
// fields, while array spread expansion shares values, so a pointer-only
// containment check cannot see an indirect cycle until after the first
// expansion. Following the spread dependencies first keeps cycle rejection
// ahead of every mutation.
func (c *compiler) spreadCompositeReferencesNode(composite Composite, target Node, varsStack []*Map) bool {
	stack := []Composite{composite}
	seen := make(map[Composite]struct{})
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		if compositeContainsNode(current, target) {
			return true
		}

		for _, candidate := range compositeSpreadNodes(current) {
			primary := candidate.Primary()
			if primary == nil {
				continue
			}
			unquoted, ok := primary.Value.(*d2ast.UnquotedString)
			if !ok {
				continue
			}
			for _, box := range unquoted.Value {
				if box.Substitution == nil || !box.Substitution.Spread {
					continue
				}
				for i, vars := range varsStack {
					resolved := c.resolveSubstitution(vars, candidate, box.Substitution, i == 0)
					if resolved == nil {
						continue
					}
					if resolved.Composite != nil {
						stack = append(stack, resolved.Composite)
					}
					break
				}
			}
		}
	}
	return false
}

func compositeSpreadNodes(composite Composite) (nodes []Node) {
	walkComposite(composite, func(n Node) bool {
		switch n := n.(type) {
		case *Field:
			if n != nil && n.Name == nil {
				nodes = append(nodes, n)
			}
		case *Scalar:
			if n != nil {
				if _, ok := n.Parent().(*Array); ok {
					nodes = append(nodes, n)
				}
			}
		}
		return false
	})
	return nodes
}

func hasUnresolvedMapSpread(m *Map) bool {
	for _, field := range m.Fields {
		if field == nil || field.Name == nil {
			return true
		}
	}
	return false
}

// compositeContainsNode reports whether target is reachable through a
// composite's child structure. Composite substitutions deliberately share the
// resolved value, so attaching a composite below one of its own descendants
// would turn the IR tree into a cycle. Use an iterative walk with a visited set
// because earlier valid substitutions can make the structure a DAG, and the
// check must also terminate defensively if handed an already-cyclic IR.
func compositeContainsNode(composite Composite, target Node) bool {
	return walkComposite(composite, func(n Node) bool {
		return n == target
	})
}

func walkComposite(composite Composite, visit func(Node) bool) bool {
	stack := []Node{composite}
	seen := make(map[Node]struct{})
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		if visit(n) {
			return true
		}

		switch n := n.(type) {
		case *Map:
			if n == nil {
				continue
			}
			for _, f := range n.Fields {
				stack = append(stack, f)
			}
			for _, e := range n.Edges {
				stack = append(stack, e)
			}
		case *Field:
			if n != nil && n.Composite != nil {
				stack = append(stack, n.Composite)
			}
		case *Edge:
			if n != nil && n.Map_ != nil {
				stack = append(stack, n.Map_)
			}
		case *Array:
			if n == nil {
				continue
			}
			for _, value := range n.Values {
				stack = append(stack, value)
			}
		}
	}
	return false
}

func (c *compiler) collectVariables(vars *Map, variables map[string]string, seen map[Node]struct{}, source d2ast.Node) {
	if vars == nil || c.stopped() {
		return
	}
	if !c.visitSubstitutionNode(vars, source, seen) {
		return
	}
	for _, f := range vars.Fields {
		if c.stopped() {
			return
		}
		if !c.visitSubstitutionNode(f, source, seen) {
			return
		}
		if f.Primary() != nil {
			name := f.Name.ScalarString()
			if !c.reserveVariableExpansion(source, markdownVariablePatternBytes(int64(len(name)))) {
				return
			}
			variables[name] = f.Primary().Value.ScalarString()
		} else if f.Map() != nil {
			nestedVars := make(map[string]string)
			c.collectVariables(f.Map(), nestedVars, seen, source)
			if c.stopped() {
				return
			}
			name := f.Name.ScalarString()
			for k, v := range nestedVars {
				if !c.reserveVariableExpansion(source, markdownVariablePatternBytes(joinedVariableNameBytes(name, k))) {
					return
				}
				variables[name+"."+k] = v
			}
			c.collectVariables(f.Map(), variables, seen, source)
			if c.stopped() {
				return
			}
		}
	}
}

func markdownVariablePatternBytes(nameBytes int64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if nameBytes >= maxInt64-3 {
		return maxInt64
	}
	return nameBytes + 3 // "${" + name + "}"
}

func joinedVariableNameBytes(prefix, suffix string) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	total := int64(len(prefix))
	if total == maxInt64 || int64(len(suffix)) >= maxInt64-total {
		return maxInt64
	}
	return total + 1 + int64(len(suffix))
}

func (c *compiler) resolveSubstitution(vars *Map, node Node, substitution *d2ast.Substitution, isCurrentScopeVars bool) *Field {
	if vars == nil {
		return nil
	}

	fieldNode, fok := node.(*Field)
	parent := ParentField(node)

	for i, p := range substitution.Path {
		f := vars.getFieldIndexed(p.Unbox())
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
	if f.Map() == nil {
		c.errorf(f.References[0].Context_.Key, "invalid %s", NodeBoardKind(f))
		return
	}

	if !c.reserveVariableCopy(f.LastRef().AST(), base) {
		return
	}
	base = base.CopyBase(f)
	// Certain fields should never carry forward.
	// If you give your scenario a label, you don't want all steps in a scenario to be labeled the same.
	base.DeleteField("label")
	if !c.reserveVariableCopy(f.LastRef().AST(), f.Map()) {
		return
	}
	overlayMapIndexed(base, f.Map())
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
				if gctx.refctx.Key.HasTripleGlob() {
					ks = d2format.FormatKeyPath(IDA(dst))
				} else {
					ks = d2format.FormatKeyPath(BoardIDA(dst))
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
	if c.stopped() {
		return
	}
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
				if !g.refctx.Key.HasTripleGlob() {
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
				if !g.refctx.Key.HasTripleGlob() {
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
		if c.stopped() {
			return
		}
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
			if len(c.globRefContextStack) > 0 {
				if !c.reserveGlobGeneratedFieldWork(dst, n.Substitution) || !c.reserveGlobField(n.Substitution) {
					return
				}
			}
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
			dst.appendField(f)
		case n.Import != nil:
			// Spread import
			impn, ok := c._import(n.Import, dst)
			if !ok {
				if c.stopped() {
					return
				}
				continue
			}
			if impn.Map() == nil {
				c.errorf(n.Import, "cannot spread import non map into map")
				continue
			}
			impn.(Importable).SetImportAST(n.Import)

			for _, gctx := range impn.Map().globs {
				if c.stopped() {
					return
				}
				if !gctx.refctx.Key.HasTripleGlob() {
					continue
				}
				gctx2 := gctx.copy()
				gctx2.refctx.ScopeMap = dst
				c.compileKey(gctx2.refctx)
				c.ensureGlobContext(gctx2.refctx)
			}

			scenariosField := impn.Map().getFieldIndexed(d2ast.FlatUnquotedString("scenarios"))
			if scenariosField != nil && scenariosField.Map() != nil {
				for _, sf := range scenariosField.Map().Fields {
					if c.stopped() {
						return
					}
					c.overlay(dst, sf)
				}
			}

			stepsField := impn.Map().getFieldIndexed(d2ast.FlatUnquotedString("steps"))
			if stepsField != nil && stepsField.Map() != nil {
				for _, sf := range stepsField.Map().Fields {
					if c.stopped() {
						return
					}
					c.overlay(dst, sf)
				}
			}

			if !c.reserveVariableCopy(n.Import, impn.Map()) {
				return
			}
			overlayMapIndexed(dst, impn.Map())
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
	if c.stopped() {
		return
	}
	postTargetStart := len(c.lazyPostTargets)
	if refctx.Key.HasGlob() || len(c.globRefContextStack) > 0 {
		if !c.reserveGlobWork(refctx.Key, 1) {
			return
		}
	}
	if refctx.Key.HasGlob() {
		for _, refctx2 := range c.globRefContextStack {
			if refctx.Equal(refctx2) {
				// Break the infinite loop.
				return
			}
		}
		c.globRefContextStack = append(c.globRefContextStack, refctx)
		defer func() {
			c.globRefContextStack = c.globRefContextStack[:len(c.globRefContextStack)-1]
		}()
		c.ensureGlobContext(refctx)
	}
	oldVersion := refctx.ScopeMap.structureVersion
	if len(refctx.Key.Edges) == 0 {
		c.compileField(refctx.ScopeMap, refctx.Key.Key, refctx)
	} else {
		c.compileEdges(refctx)
	}
	root := RootMap(refctx.ScopeMap)
	settled := c.lazySettledVersions != nil && c.lazySettledVersions[root] == root.structureVersion
	if oldVersion != refctx.ScopeMap.structureVersion && !c.applyingLazyWorklist && len(c.lazyPostTargets) > postTargetStart {
		targets := c.takeLazyPostTargets(postTargetStart)
		c.applyLazyGlobs(targets)
		settled = true
	}
	if oldVersion != refctx.ScopeMap.structureVersion && !c.applyingLazyWorklist && !settled {
		for _, gctx2 := range c.globContexts() {
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx2.refctx)
			c.lazyGlobBeingApplied = old
		}
	}
}

func (c *compiler) takeLazyPostTargets(start int) []*Field {
	targets := append([]*Field(nil), c.lazyPostTargets[start:]...)
	c.lazyPostTargets = c.lazyPostTargets[:start]
	for _, target := range targets {
		delete(c.lazyPostQueued, target)
	}
	if len(c.lazyPostQueued) == 0 {
		c.lazyPostQueued = nil
	}
	return targets
}

func (c *compiler) enqueueLazyPostTargets(fields ...*Field) {
	if c.lazyPostQueued == nil {
		c.lazyPostQueued = make(map[*Field]struct{})
	}
	for _, f := range fields {
		if f == nil {
			continue
		}
		if _, exists := c.lazyPostQueued[f]; exists {
			continue
		}
		c.lazyPostQueued[f] = struct{}{}
		c.lazyPostTargets = append(c.lazyPostTargets, f)
	}
}

func (c *compiler) enqueueLazyGlobFields(fields ...*Field) {
	if len(fields) == 0 {
		return
	}
	if c.lazyGlobQueued == nil {
		c.lazyGlobQueued = make(map[*Field]struct{})
	}
	for _, f := range fields {
		if f == nil {
			continue
		}
		if _, queued := c.lazyGlobQueued[f]; queued {
			continue
		}
		c.lazyGlobQueued[f] = struct{}{}
		c.lazyGlobWorklist = append(c.lazyGlobWorklist, f)
	}
}

func (c *compiler) applyLazyGlobs(created []*Field) {
	if len(created) == 0 {
		for _, gctx := range c.globContexts() {
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx.refctx)
			c.lazyGlobBeingApplied = old
		}
		return
	}

	if c.applyingLazyWorklist {
		c.enqueueLazyGlobFields(created...)
		return
	}
	root := RootMap(ParentMap(created[0]))
	if len(c.globContexts()) == 0 {
		// Preserve the settled version and pending post-targets: a later key or
		// nested value can introduce globs. There is no worklist to run yet.
		if c.lazySettledVersions == nil {
			c.lazySettledVersions = make(map[*Map]uint64)
		}
		c.lazySettledVersions[root] = root.structureVersion
		return
	}

	c.applyingLazyWorklist = true
	defer func() {
		if c.lazySettledVersions == nil {
			c.lazySettledVersions = make(map[*Map]uint64)
		}
		c.lazySettledVersions[root] = root.structureVersion
		c.applyingLazyWorklist = false
		c.lazyGlobTarget = nil
		c.lazyGlobWorklist = nil
		c.lazyGlobQueued = nil
	}()
	c.enqueueLazyGlobFields(created...)

	for len(c.lazyGlobWorklist) > 0 {
		if c.stopped() {
			return
		}
		target := c.lazyGlobWorklist[0]
		c.lazyGlobWorklist = c.lazyGlobWorklist[1:]

		var edgeGlobs []*globContext
		for _, gctx := range c.globContexts() {
			if c.stopped() {
				return
			}
			if len(gctx.refctx.Key.Edges) > 0 {
				edgeGlobs = append(edgeGlobs, gctx)
				continue
			}
			c.lazyGlobTarget = target
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx.refctx)
			c.lazyGlobBeingApplied = old
		}
		c.lazyGlobTarget = nil

		// Edge globs can depend on edges emitted by earlier glob rules. Preserve
		// the old fixed-point behavior, but only for edge rules; field rules have
		// already been applied to the precise changed fields above.
		root := RootMap(target.parent.(*Map))
		for len(edgeGlobs) > 0 {
			if c.stopped() {
				return
			}
			before := root.structureVersion
			for _, gctx := range edgeGlobs {
				if c.stopped() {
					return
				}
				old := c.lazyGlobBeingApplied
				c.lazyGlobBeingApplied = true
				c.compileKey(gctx.refctx)
				c.lazyGlobBeingApplied = old
			}
			if before == root.structureVersion {
				break
			}
		}
	}
}

func (c *compiler) compileField(dst *Map, kp *d2ast.KeyPath, refctx *RefContext) {
	if c.stopped() {
		return
	}
	if refctx.Key.Ampersand || refctx.Key.NotAmpersand {
		return
	}

	fa, err := dst.ensureFieldIndexed(kp, refctx, true, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}

	for _, f := range fa {
		if c.stopped() {
			return
		}
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
		node := rootMap.getFieldIndexed(nodePath...)
		if node == nil || node.Map() == nil {
			return false
		}

		secondPart := keyPath.Path[1].Unbox().ScalarString()
		value := refctx.Key.Value.ScalarBox().Unbox().ScalarString()

		if len(keyPath.Path) == 2 && c._ampersandPropertyFilter(secondPart, value, node, refctx.Key) {
			return true
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

		fa, err := node.Map().ensureFieldIndexed(propKeyPath, propRefCtx, false, c)
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

	fa, err := refctx.ScopeMap.ensureFieldIndexed(refctx.Key.Key, refctx, false, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return false
	}
	if len(fa) == 0 {
		if refctx.Key.Value.ScalarBox().Unbox() != nil && refctx.Key.Value.ScalarBox().Unbox().ScalarString() == "*" {
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
			parent := refctx.ScopeMap.Parent()
			if field, ok := parent.(*Field); ok {
				propName := refctx.Key.Key.Last().ScalarString()
				value := refctx.Key.Value.ScalarBox().Unbox().ScalarString()
				return c._ampersandPropertyFilter(propName, value, field, refctx.Key)
			}
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

// handles filters that are not based on fields
func (c *compiler) _ampersandPropertyFilter(propName string, value string, node *Field, key *d2ast.Key) bool {
	switch propName {
	case "level":
		levelVal, err := strconv.Atoi(value)
		if err != nil {
			c.errorf(key, `&level must be a non-negative integer, got %q`, value)
			return false
		}
		if levelVal < 0 {
			c.errorf(key, `&level must be a non-negative integer, got %d`, levelVal)
			return false
		}

		level := 0
		parent := ParentField(node)
		for parent != nil && parent.Name.ScalarString() != "root" && NodeBoardKind(parent) == "" {
			level++
			parent = ParentField(parent)
		}
		return level == levelVal
	case "leaf":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			c.errorf(key, `&leaf must be "true" or "false", got %q`, value)
			return false
		}
		isLeaf := node.Map() == nil || !c.IsContainer(node.Map())
		return isLeaf == boolVal
	case "connected":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			c.errorf(key, `&connected must be "true" or "false", got %q`, value)
			return false
		}
		isConnected := false
		for _, r := range node.References {
			if r.InEdge() {
				isConnected = true
				break
			}
		}
		return isConnected == boolVal
	case "label":
		f := &Field{}
		if node.Primary() == nil {
			f.Primary_ = &Scalar{
				Value: node.Name,
			}
		} else {
			f.Primary_ = node.Primary()
		}
		propKey := &d2ast.Key{
			Key:   key.Key,
			Value: key.Value,
		}
		propRefCtx := &RefContext{
			Key: propKey,
		}
		return c._ampersandFilter(f, propRefCtx)
	}
	return false
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
	if c.stopped() {
		return
	}
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
		if c.stopped() {
			return
		}
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
		if c.stopped() {
			return
		}
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
				if c.stopped() {
					return
				}
			case BoardStep:
				stepsMap := ParentMap(f)
				for i := range stepsMap.Fields {
					if stepsMap.Fields[i] == f {
						if i == 0 {
							c.overlay(ParentBoard(f).Map(), f)
						} else {
							c.overlay(stepsMap.Fields[i-1].Map(), f)
						}
						if c.stopped() {
							return
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
		if c.stopped() {
			return
		}
		switch NodeBoardKind(f) {
		case BoardScenario, BoardStep:
			c.overlayClasses(f.Map())
			if c.stopped() {
				return
			}
		}
	} else if refctx.Key.Value.Import != nil {
		// Non-spread import
		n, ok := c._import(refctx.Key.Value.Import, f)
		if !ok {
			return
		}
		n.(Importable).SetImportAST(refctx.Key.Value.Import)
		var existingEdges []*Edge
		if f.Map() != nil {
			existingEdges = f.Map().Edges
		}
		if !c.reserveVariableCopy(refctx.Key.Value.Import, f) {
			return
		}
		originalF := f.Copy(refctx.ScopeMap).(*Field)
		switch n := n.(type) {
		case *Field:
			if n.Primary_ != nil {
				f.Primary_ = n.Primary_.Copy(f).(*Scalar)
			}
			if n.Composite != nil {
				if !c.reserveVariableCopy(refctx.Key.Value.Import, n.Composite) {
					return
				}
				beforeFields, beforeEdges := structureCounts(f.Map())
				f.Composite = n.Composite.Copy(f).(Composite)
				markStructureCountDelta(ParentMap(f), beforeFields, beforeEdges, f.Map())
			}
		case *Map:
			f.Composite = &Map{
				parent: f,
			}
			switch NodeBoardKind(f) {
			case BoardScenario:
				c.overlay(ParentBoard(f).Map(), f)
				if c.stopped() {
					return
				}
			case BoardStep:
				stepsMap := ParentMap(f)
				for i := range stepsMap.Fields {
					if stepsMap.Fields[i] == f {
						if i == 0 {
							c.overlay(ParentBoard(f).Map(), f)
						} else {
							c.overlay(stepsMap.Fields[i-1].Map(), f)
						}
						if c.stopped() {
							return
						}
						break
					}
				}
			}
			if !c.reserveVariableCopy(refctx.Key.Value.Import, n) {
				return
			}
			overlayMapIndexed(f.Map(), n)
			impDir := refctx.Key.Value.Import.Dir()
			c.extendLinks(f.Map(), f, impDir)
			switch NodeBoardKind(f) {
			case BoardScenario, BoardStep:
				c.overlayClasses(f.Map())
				if c.stopped() {
					return
				}
			}
		}
		if !c.reserveVariableCopy(refctx.Key.Value.Import, originalF) {
			return
		}
		overlayFieldIndexed(f, originalF)
		if existingEdges != nil && f.Map() != nil {
			for _, edge := range existingEdges {
				exists := false
				for _, currentEdge := range f.Map().Edges {
					if currentEdge.ID.Match(edge.ID) {
						exists = true
						break
					}
				}
				if !exists {
					f.Map().appendEdge(edge)
				}
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
	if c.stopped() {
		return
	}
	if refctx.Key.Key == nil {
		c._compileEdges(refctx)
		return
	}

	fa, err := refctx.ScopeMap.ensureFieldIndexed(refctx.Key.Key, refctx, true, c)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}
	for _, f := range fa {
		if c.stopped() {
			return
		}
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
	if c.stopped() {
		return
	}
	eida := NewEdgeIDs(refctx.Key)
	for i, eid := range eida {
		if c.stopped() {
			return
		}
		if !eid.Glob && (refctx.Key.Primary.Null != nil || refctx.Key.Value.Null != nil) {
			refctx.ScopeMap.DeleteEdge(eid)
			continue
		}

		refctx = refctx.Copy()
		refctx.Edge = refctx.Key.Edges[i]

		var ea []*Edge
		if eid.Index != nil || eid.Glob {
			ea = refctx.ScopeMap.getEdgesForCompile(eid, refctx, c)
			if len(ea) == 0 {
				if !eid.Glob {
					c.errorf(refctx.Edge, "indexed edge does not exist")
				}
				continue
			}
			for _, e := range ea {
				if c.stopped() {
					return
				}
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
					if c.stopped() {
						return
					}
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
							if c.stopped() {
								return
							}
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
							srcObj := rootMap.getFieldIndexed(srcPath...)
							dstObj := rootMap.getFieldIndexed(dstPath...)

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

				e.appendReference(&EdgeReference{
					Context_:       refctx,
					DueToGlob_:     len(c.globRefContextStack) > 0,
					DueToLazyGlob_: c.lazyGlobBeingApplied,
				})
				refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Src, refctx, c)
				refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Dst, refctx, c)
			}
		} else {
			var err error
			ea, err = refctx.ScopeMap.createEdgeForCompile(eid, refctx, c)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
		}

		for _, e := range ea {
			if c.stopped() {
				return
			}
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
	if c.stopped() {
		return
	}
	for _, an := range a.Nodes {
		if c.stopped() {
			return
		}
		arrayNode := an.Unbox()
		if len(c.globRefContextStack) > 0 && !c.reserveGlobWork(arrayNode, 1) {
			return
		}
		var irv Value
		switch v := arrayNode.(type) {
		case *d2ast.Array:
			ira := &Array{
				parent: dst,
			}
			c.compileArray(ira, v, scopeAST)
			if c.stopped() {
				return
			}
			irv = ira
		case *d2ast.Map:
			irm := &Map{
				parent: dst,
			}
			c.compileMap(irm, v, scopeAST)
			if c.stopped() {
				return
			}
			irv = irm
		case d2ast.Scalar:
			irv = &Scalar{
				parent: dst,
				Value:  v,
			}
		case *d2ast.Import:
			n, ok := c._import(v, dst)
			if !ok {
				if c.stopped() {
					return
				}
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
					if !c.reserveVariableCopy(v, a) {
						return
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

		if c.stopped() {
			return
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
