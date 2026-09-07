package d2ir

import (
	"math/big"

	"github.com/d2lang/d2/d2ast"
)

// The parser AST is mutable: substitution resolution coalesces strings in
// place. Import caches therefore retain an immutable parsed tree and compile a
// deep clone for each importer context.
func cloneASTMap(src *d2ast.Map) *d2ast.Map {
	if src == nil {
		return nil
	}
	dst := &d2ast.Map{Range: src.Range, Nodes: make([]d2ast.MapNodeBox, 0, len(src.Nodes))}
	for _, box := range src.Nodes {
		switch n := box.Unbox().(type) {
		case *d2ast.Comment:
			copy := *n
			dst.Nodes = append(dst.Nodes, d2ast.MakeMapNodeBox(&copy))
		case *d2ast.BlockComment:
			copy := *n
			dst.Nodes = append(dst.Nodes, d2ast.MakeMapNodeBox(&copy))
		case *d2ast.Substitution:
			dst.Nodes = append(dst.Nodes, d2ast.MakeMapNodeBox(cloneASTSubstitution(n)))
		case *d2ast.Import:
			dst.Nodes = append(dst.Nodes, d2ast.MakeMapNodeBox(cloneASTImport(n)))
		case *d2ast.Key:
			dst.Nodes = append(dst.Nodes, d2ast.MakeMapNodeBox(cloneASTKey(n)))
		}
	}
	return dst
}

func cloneASTArray(src *d2ast.Array) *d2ast.Array {
	if src == nil {
		return nil
	}
	dst := &d2ast.Array{Range: src.Range, Nodes: make([]d2ast.ArrayNodeBox, 0, len(src.Nodes))}
	for _, box := range src.Nodes {
		switch n := box.Unbox().(type) {
		case *d2ast.Comment:
			copy := *n
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(&copy))
		case *d2ast.BlockComment:
			copy := *n
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(&copy))
		case *d2ast.Substitution:
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(cloneASTSubstitution(n)))
		case *d2ast.Import:
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(cloneASTImport(n)))
		case *d2ast.Array:
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(cloneASTArray(n)))
		case *d2ast.Map:
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(cloneASTMap(n)))
		case d2ast.Scalar:
			dst.Nodes = append(dst.Nodes, d2ast.MakeArrayNodeBox(cloneASTScalar(n).(d2ast.ArrayNode)))
		}
	}
	return dst
}

func cloneASTKey(src *d2ast.Key) *d2ast.Key {
	if src == nil {
		return nil
	}
	dst := &d2ast.Key{
		Range:        src.Range,
		Ampersand:    src.Ampersand,
		NotAmpersand: src.NotAmpersand,
		Key:          cloneASTKeyPath(src.Key),
		EdgeKey:      cloneASTKeyPath(src.EdgeKey),
		Primary:      cloneASTScalarBox(src.Primary),
		Value:        cloneASTValueBox(src.Value),
	}
	if src.EdgeIndex != nil {
		index := *src.EdgeIndex
		if src.EdgeIndex.Int != nil {
			value := *src.EdgeIndex.Int
			index.Int = &value
		}
		dst.EdgeIndex = &index
	}
	dst.Edges = make([]*d2ast.Edge, len(src.Edges))
	for i, edge := range src.Edges {
		dst.Edges[i] = &d2ast.Edge{
			Range:    edge.Range,
			Src:      cloneASTKeyPath(edge.Src),
			SrcArrow: edge.SrcArrow,
			Dst:      cloneASTKeyPath(edge.Dst),
			DstArrow: edge.DstArrow,
		}
	}
	return dst
}

func cloneASTKeyPath(src *d2ast.KeyPath) *d2ast.KeyPath {
	if src == nil {
		return nil
	}
	dst := &d2ast.KeyPath{Range: src.Range, Path: make([]*d2ast.StringBox, len(src.Path))}
	for i, part := range src.Path {
		dst.Path[i] = cloneASTStringBox(part)
	}
	return dst
}

func cloneASTStringBox(src *d2ast.StringBox) *d2ast.StringBox {
	if src == nil || src.Unbox() == nil {
		return nil
	}
	return d2ast.MakeValueBox(cloneASTScalar(src.Unbox())).StringBox()
}

func cloneASTSubstitution(src *d2ast.Substitution) *d2ast.Substitution {
	if src == nil {
		return nil
	}
	dst := &d2ast.Substitution{Range: src.Range, Spread: src.Spread, Path: make([]*d2ast.StringBox, len(src.Path))}
	for i, part := range src.Path {
		dst.Path[i] = cloneASTStringBox(part)
	}
	return dst
}

func cloneASTImport(src *d2ast.Import) *d2ast.Import {
	if src == nil {
		return nil
	}
	dst := &d2ast.Import{Range: src.Range, Spread: src.Spread, Pre: src.Pre, Path: make([]*d2ast.StringBox, len(src.Path))}
	for i, part := range src.Path {
		dst.Path[i] = cloneASTStringBox(part)
	}
	return dst
}

func cloneASTScalarBox(src d2ast.ScalarBox) d2ast.ScalarBox {
	if src.Unbox() == nil {
		return d2ast.ScalarBox{}
	}
	return d2ast.MakeValueBox(cloneASTScalar(src.Unbox())).ScalarBox()
}

func cloneASTValueBox(src d2ast.ValueBox) d2ast.ValueBox {
	switch value := src.Unbox().(type) {
	case nil:
		return d2ast.ValueBox{}
	case *d2ast.Map:
		return d2ast.MakeValueBox(cloneASTMap(value))
	case *d2ast.Array:
		return d2ast.MakeValueBox(cloneASTArray(value))
	case *d2ast.Import:
		return d2ast.MakeValueBox(cloneASTImport(value))
	case d2ast.Scalar:
		return d2ast.MakeValueBox(cloneASTScalar(value))
	default:
		panic("unhandled AST value")
	}
}

func cloneASTScalar(src d2ast.Scalar) d2ast.Scalar {
	if src == nil {
		return nil
	}
	switch value := src.(type) {
	case *d2ast.Null:
		copy := *value
		return &copy
	case *d2ast.Suspension:
		copy := *value
		return &copy
	case *d2ast.Boolean:
		copy := *value
		return &copy
	case *d2ast.Number:
		copy := *value
		if value.Value != nil {
			copy.Value = new(big.Rat).Set(value.Value)
		}
		return &copy
	case *d2ast.UnquotedString:
		copy := *value
		copy.Pattern = append([]string(nil), value.Pattern...)
		copy.Value = cloneASTInterpolation(value.Value)
		return &copy
	case *d2ast.DoubleQuotedString:
		copy := *value
		copy.Value = cloneASTInterpolation(value.Value)
		return &copy
	case *d2ast.SingleQuotedString:
		copy := *value
		return &copy
	case *d2ast.BlockString:
		copy := *value
		return &copy
	default:
		panic("unhandled AST scalar")
	}
}

func cloneASTInterpolation(src []d2ast.InterpolationBox) []d2ast.InterpolationBox {
	dst := make([]d2ast.InterpolationBox, len(src))
	for i, box := range src {
		if box.String != nil {
			value := *box.String
			dst[i].String = &value
		}
		if box.StringRaw != nil {
			value := *box.StringRaw
			dst[i].StringRaw = &value
		}
		dst[i].Substitution = cloneASTSubstitution(box.Substitution)
	}
	return dst
}

// A selective non-spread import only copies the chosen field into its
// destination. Once the complete library has been compiled and validated, its
// unrelated siblings need not be copied again for each use. Keep full copies
// for boards, globs, and maps in arrays, whose retained contexts can refer
// outside that field. Imports into arrays also retain the source ancestry.
type importTemplate struct {
	ir               *Map
	selective        bool
	selectionChecked bool
}

func (t *importTemplate) canSelect() bool {
	if !t.selectionChecked {
		t.selective = canSelectImport(t.ir)
		t.selectionChecked = true
	}
	return t.selective
}

func canSelectImport(n Node) bool {
	return canSelectImportNode(n, false)
}

func canSelectImportNode(n Node, inArray bool) bool {
	switch n := n.(type) {
	case *Map:
		// nilScopeMap does not descend into arrays. Those references retain
		// the complete imported ancestry, so use the complete clone for them.
		if inArray || len(n.globs) != 0 {
			return false
		}
		for _, f := range n.Fields {
			if !canSelectImport(f) {
				return false
			}
		}
		for _, e := range n.Edges {
			if !canSelectImport(e) {
				return false
			}
		}
	case *Field:
		if NodeBoardKind(n) != "" {
			return false
		}
		return n.Composite == nil || canSelectImport(n.Composite)
	case *Edge:
		return n.Map_ == nil || canSelectImport(n.Map_)
	case *Array:
		for _, value := range n.Values {
			if !canSelectImportNode(value, true) {
				return false
			}
		}
	}
	return true
}

func cloneImportField(src *Field) *Field {
	dst := src.Copy(nil).(*Field)
	cloneImportContexts(src, dst)
	return dst
}

// cloneImportMap deep-copies mutable IR and remaps reference/glob contexts to
// the copy. The ordinary Map.Copy intentionally shares source contexts; import
// templates need stronger isolation because nilScopeMap and substitutions
// mutate them after each import.
func cloneImportMap(src *Map) *Map {
	dst := src.Copy(nil).(*Map)
	cloneImportContexts(src, dst)
	return dst
}

func cloneImportContexts(src, dst Node) {
	mapCopies := make(map[*Map]*Map)
	collectMapCopies(src, dst, mapCopies)
	contextCopies := make(map[*RefContext]*RefContext)
	cloneContext := func(src *RefContext) *RefContext {
		if src == nil {
			return nil
		}
		if dst := contextCopies[src]; dst != nil {
			return dst
		}
		dst := *src
		if mapped := mapCopies[src.ScopeMap]; mapped != nil {
			dst.ScopeMap = mapped
		}
		contextCopies[src] = &dst
		return &dst
	}

	cloneNodeReferences(src, dst, cloneContext)
	cloneGlobContexts(src, dst, mapCopies, cloneContext)
}

func collectMapCopies(src, dst Node, copies map[*Map]*Map) {
	switch src := src.(type) {
	case *Map:
		dst := dst.(*Map)
		copies[src] = dst
		for i := range src.Fields {
			collectMapCopies(src.Fields[i], dst.Fields[i], copies)
		}
		for i := range src.Edges {
			collectMapCopies(src.Edges[i], dst.Edges[i], copies)
		}
	case *Field:
		dst := dst.(*Field)
		if src.Composite != nil {
			collectMapCopies(src.Composite, dst.Composite, copies)
		}
	case *Edge:
		dst := dst.(*Edge)
		if src.Map_ != nil {
			collectMapCopies(src.Map_, dst.Map_, copies)
		}
	case *Array:
		dst := dst.(*Array)
		for i := range src.Values {
			collectMapCopies(src.Values[i], dst.Values[i], copies)
		}
	}
}

func cloneNodeReferences(src, dst Node, cloneContext func(*RefContext) *RefContext) {
	switch src := src.(type) {
	case *Map:
		dst := dst.(*Map)
		for i := range src.Fields {
			cloneNodeReferences(src.Fields[i], dst.Fields[i], cloneContext)
		}
		for i := range src.Edges {
			cloneNodeReferences(src.Edges[i], dst.Edges[i], cloneContext)
		}
	case *Field:
		dst := dst.(*Field)
		dst.References = make([]*FieldReference, len(src.References))
		for i, ref := range src.References {
			copy := *ref
			copy.Context_ = cloneContext(ref.Context_)
			dst.References[i] = &copy
		}
		if src.Composite != nil {
			cloneNodeReferences(src.Composite, dst.Composite, cloneContext)
		}
	case *Edge:
		dst := dst.(*Edge)
		dst.References = make([]*EdgeReference, len(src.References))
		for i, ref := range src.References {
			copy := *ref
			copy.Context_ = cloneContext(ref.Context_)
			dst.References[i] = &copy
		}
		if src.Map_ != nil {
			cloneNodeReferences(src.Map_, dst.Map_, cloneContext)
		}
	case *Array:
		dst := dst.(*Array)
		for i := range src.Values {
			cloneNodeReferences(src.Values[i], dst.Values[i], cloneContext)
		}
	}
}

func cloneGlobContexts(src, dst Node, mapCopies map[*Map]*Map, cloneContext func(*RefContext) *RefContext) {
	globCopies := make(map[*globContext]*globContext)
	var cloneGlob func(*globContext) *globContext
	cloneGlob = func(src *globContext) *globContext {
		if src == nil {
			return nil
		}
		if dst := globCopies[src]; dst != nil {
			return dst
		}
		dst := *src
		globCopies[src] = &dst
		dst.refctx = cloneContext(src.refctx)
		dst.appliedFields = cloneStringSet(src.appliedFields)
		dst.appliedEdges = cloneStringSet(src.appliedEdges)
		dst.root = cloneGlob(src.root)
		return &dst
	}

	for srcMap, dstMap := range mapCopies {
		dstMap.globs = make([]*globContext, len(srcMap.globs))
		for i, glob := range srcMap.globs {
			dstMap.globs[i] = cloneGlob(glob)
		}
	}
}

func cloneStringSet(src map[string]struct{}) map[string]struct{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]struct{}, len(src))
	for key := range src {
		dst[key] = struct{}{}
	}
	return dst
}
