package d2oracle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/xrand"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
)

type OutsideScopeError struct{}

func (e OutsideScopeError) Error() string {
	return "operation would modify AST outside of given scope"
}

func Create(g *d2graph.Graph, boardPath []string, key string) (_ *d2graph.Graph, newKey string, err error) {
	defer xdefer.Errorf(&err, "failed to create %#v", key)

	boardG := g
	baseAST := g.AST

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, "", fmt.Errorf("board %v not found", boardPath)
		}
		// TODO beter name
		baseAST = boardG.BaseAST
	}

	newKey, edge, err := generateUniqueKey(boardG, key, nil, nil)
	if err != nil {
		return nil, "", err
	}

	if edge {
		err = _set(boardG, baseAST, key, nil, nil)
	} else {
		err = _set(boardG, baseAST, newKey, nil, nil)
	}

	if len(boardPath) > 0 {
		replaced := ReplaceBoardNode(g.AST, baseAST, boardPath)
		if !replaced {
			return nil, "", fmt.Errorf("board %v AST not found", boardPath)
		}
	}

	if err != nil {
		return nil, "", err
	}
	g, err = recompile(g)
	if err != nil {
		return nil, "", err
	}
	return g, newKey, nil
}

// TODO: update graph in place when compiler can accept single modifications
// TODO: go through all references to decide best spot to insert something
func Set(g *d2graph.Graph, boardPath []string, key string, tag, value *string) (_ *d2graph.Graph, err error) {
	var valueHelp string
	if value == nil {
		valueHelp = fmt.Sprintf("%#v", value)
	} else {
		valueHelp = fmt.Sprintf("%#v", *value)
	}
	if tag != nil {
		defer xdefer.Errorf(&err, "failed to set %#v to %#v %#v", key, *tag, valueHelp)
	} else {
		defer xdefer.Errorf(&err, "failed to set %#v to %#v", key, valueHelp)
	}

	boardG := g
	baseAST := g.AST

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
		// TODO beter name
		baseAST = boardG.BaseAST
	}

	err = _set(boardG, baseAST, key, tag, value)
	if err != nil {
		return nil, err
	}

	if len(boardPath) > 0 {
		replaced := ReplaceBoardNode(g.AST, baseAST, boardPath)
		if !replaced {
			return nil, fmt.Errorf("board %v AST not found", boardPath)
		}
	}

	return recompile(g)
}

func ReconnectEdge(g *d2graph.Graph, boardPath []string, edgeKey string, srcKey, dstKey *string) (_ *d2graph.Graph, err error) {
	mk, err := d2parser.ParseMapKey(edgeKey)
	if err != nil {
		return nil, err
	}

	if len(mk.Edges) == 0 {
		return nil, errors.New("edgeKey must be an edge")
	}

	if mk.EdgeIndex == nil {
		return nil, errors.New("edgeKey must refer to an existing edge")
	}

	edgeTrimCommon(mk)

	boardG := g
	baseAST := g.AST

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
		// TODO beter name
		baseAST = boardG.BaseAST
	}

	obj := boardG.Root
	if mk.Key != nil {
		var ok bool
		obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return nil, errors.New("edge not found")
		}
	}

	edge, ok := obj.HasEdge(mk)
	if !ok {
		return nil, errors.New("edge not found")
	}

	if srcKey != nil {
		if edge.Src.AbsID() == *srcKey {
			srcKey = nil
		}
	}

	if dstKey != nil {
		if edge.Dst.AbsID() == *dstKey {
			dstKey = nil
		}
	}

	if srcKey == nil && dstKey == nil {
		return g, nil
	}

	var src *d2graph.Object
	var dst *d2graph.Object
	if srcKey != nil {
		srcmk, err := d2parser.ParseMapKey(*srcKey)
		if err != nil {
			return nil, err
		}
		src, ok = boardG.Root.HasChild(d2graph.Key(srcmk.Key))
		if !ok {
			return nil, errors.New("newSrc not found")
		}
	}
	if dstKey != nil {
		dstmk, err := d2parser.ParseMapKey(*dstKey)
		if err != nil {
			return nil, err
		}
		dst, ok = boardG.Root.HasChild(d2graph.Key(dstmk.Key))
		if !ok {
			return nil, errors.New("newDst not found")
		}
	}

	refs := edge.References
	if baseAST != g.AST {
		refs = GetWriteableEdgeRefs(edge, baseAST)
		if len(refs) == 0 || refs[0].ScopeAST != baseAST {
			// TODO null
			return nil, OutsideScopeError{}
		}
	}
	ref := refs[0]

	// for loops where only one end is changing, node is always ensured
	if edge.Src != edge.Dst && (srcKey == nil || dstKey == nil) {
		var refEdges []*d2ast.Edge
		for _, ref := range refs {
			refEdges = append(refEdges, ref.Edge)
		}

		if srcKey != nil {
			ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, ref.MapKey.Edges[ref.MapKeyEdgeIndex].Src, true)
		}
		if dstKey != nil {
			ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, ref.MapKey.Edges[ref.MapKeyEdgeIndex].Dst, false)
		}
	}

	for i := range refs {
		ref := refs[i]
		// it's a chain
		if len(ref.MapKey.Edges) > 1 && ref.MapKey.EdgeIndex == nil {
			splitChain := true
			// Changing the start of a chain is okay
			if ref.MapKeyEdgeIndex == 0 && dstKey == nil {
				splitChain = false
			}
			// Changing the end of a chain is okay
			if ref.MapKeyEdgeIndex == len(ref.MapKey.Edges)-1 && srcKey == nil {
				splitChain = false
			}
			if splitChain {
				tmp := *ref.MapKey
				mk2 := &tmp
				mk2.Edges = []*d2ast.Edge{ref.MapKey.Edges[ref.MapKeyEdgeIndex]}
				ref.Scope.InsertAfter(ref.MapKey, mk2)

				if ref.MapKeyEdgeIndex < len(ref.MapKey.Edges)-1 {
					tmp := *ref.MapKey
					mk2 := &tmp
					mk2.Edges = ref.MapKey.Edges[ref.MapKeyEdgeIndex+1:]
					ref.Scope.InsertAfter(ref.MapKey, mk2)
				}
				ref.MapKey.Edges = ref.MapKey.Edges[:ref.MapKeyEdgeIndex]
			}
		}

		if src != nil {
			srcmk, _ := d2parser.ParseMapKey(*srcKey)
			ref.Edge.Src = srcmk.Key
			newPath, err := pathFromScopeObj(boardG, srcmk, ref.ScopeObj)
			if err != nil {
				return nil, err
			}
			ref.Edge.Src.Path = newPath
		}
		if dst != nil {
			dstmk, _ := d2parser.ParseMapKey(*dstKey)
			ref.Edge.Dst = dstmk.Key
			newPath, err := pathFromScopeObj(boardG, dstmk, ref.ScopeObj)
			if err != nil {
				return nil, err
			}
			ref.Edge.Dst.Path = newPath
		}
	}

	return recompile(g)
}

func pathFromScopeKey(g *d2graph.Graph, key *d2ast.Key, scopeak []string) ([]*d2ast.StringBox, error) {
	ak2 := d2graph.Key(key.Key)

	commonPath := getCommonPath(scopeak, ak2)

	var newPath []*d2ast.StringBox
	// Move out to most common scope
	for i := len(commonPath); i < len(scopeak); i++ {
		newPath = append(newPath, d2ast.MakeValueBox(d2ast.RawString("_", true)).StringBox())
	}
	// From most common scope, target the toKey
	newPath = append(newPath, key.Key.Path[len(commonPath):]...)

	return newPath, nil
}

func pathFromScopeObj(g *d2graph.Graph, key *d2ast.Key, fromScope *d2graph.Object) ([]*d2ast.StringBox, error) {
	// We don't want this to be underscore-resolved scope. We want to ignore underscores
	var scopeak []string
	if fromScope != g.Root {
		scopek, err := d2parser.ParseKey(fromScope.AbsID())
		if err != nil {
			return nil, err
		}
		scopeak = d2graph.Key(scopek)
	}
	return pathFromScopeKey(g, key, scopeak)
}

func recompile(g *d2graph.Graph) (*d2graph.Graph, error) {
	s := d2format.Format(g.AST)
	g2, _, err := d2compiler.Compile(g.AST.Range.Path, strings.NewReader(s), &d2compiler.CompileOptions{
		FS: g.FS,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to recompile:\n%s\n%w", s, err)
	}
	return g2, nil
}

// TODO merge flat styles
func _set(g *d2graph.Graph, baseAST *d2ast.Map, key string, tag, value *string) error {
	if tag != nil {
		if hasSpace(*tag) {
			return fmt.Errorf("spaces are not allowed in blockstring tags")
		}
	}

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return err
	}

	if len(mk.Edges) > 1 {
		return errors.New("can only set one edge at a time")
	}

	if value != nil {
		mk.Value = d2ast.MakeValueBox(d2ast.RawString(*value, false))
	} else {
		mk.Value = d2ast.ValueBox{}
	}
	if tag != nil && value != nil {
		mk.Value = d2ast.MakeValueBox(&d2ast.BlockString{
			Tag:   *tag,
			Value: *value,
		})
	}

	scope := baseAST
	edgeTrimCommon(mk)
	obj := g.Root
	toSkip := 1

	reserved := false
	imported := false

	// If you're setting `(x -> y)[0].style.opacity`
	// There's 3 cases you need to handle:
	// 1. The edge has no map.
	// 2. The edge has a style map with opacity not existing
	// 3. The edge has a style map with opacity existing
	//
	// How each case is handled:
	// 1. Append that mapkey to edge.
	// 2. Append opacity to the style map
	// 3. Set opacity
	//
	// There's certainly cleaner code to achieve this, but currently, there's a lot of logic to correctly scope, merge, append.
	// The tests should be comprehensive enough for a safe refactor someday
	//
	// reservedKey = "style"
	// reservedTargetKey = "opacity"
	reservedKey := ""
	reservedTargetKey := ""
	if mk.Key != nil {
		found := true
		for _, idel := range d2graph.Key(mk.Key) {
			_, ok := d2ast.ReservedKeywords[idel]
			if ok {
				reserved = true
				break
			}
			o, ok := obj.HasChild([]string{idel})
			if !ok {
				found = false
				break
			}
			obj = o
			imported = IsImportedObj(baseAST, obj)

			var maybeNewScope *d2ast.Map
			if baseAST != g.AST || imported {
				writeableRefs := GetWriteableRefs(obj, baseAST)
				for _, ref := range writeableRefs {
					if ref.MapKey != nil && ref.MapKey.Value.Map != nil {
						maybeNewScope = ref.MapKey.Value.Map
					}
				}
			} else {
				maybeNewScope = obj.Map
			}

			if maybeNewScope == nil {
				// If we find a deeper obj.Map we need to skip this key too.
				toSkip++
				continue
			}

			scope = maybeNewScope
			mk.Key.Path = mk.Key.Path[toSkip:]
			toSkip = 1
			if len(mk.Key.Path) == 0 {
				mk.Key = nil
			}
		}

		if mk.Key != nil && len(mk.Key.Path) == 2 {
			boardType := mk.Key.Path[0].Unbox().ScalarString()
			if boardType == "layers" || boardType == "scenarios" || boardType == "steps" {
				// Force map structure
				var containerMap *d2ast.Map
				for _, n := range scope.Nodes {
					if n.MapKey != nil && n.MapKey.Key != nil && len(n.MapKey.Key.Path) == 1 &&
						n.MapKey.Key.Path[0].Unbox().ScalarString() == boardType {
						containerMap = n.MapKey.Value.Map
						break
					}
				}

				if containerMap == nil {
					containerMap = &d2ast.Map{
						Range: d2ast.MakeRange(",1:0:0-1:0:0"),
					}
					containerMK := &d2ast.Key{
						Key: &d2ast.KeyPath{
							Path: []*d2ast.StringBox{
								d2ast.MakeValueBox(d2ast.RawString(boardType, true)).StringBox(),
							},
						},
						Value: d2ast.MakeValueBox(containerMap),
					}
					appendMapKey(scope, containerMK)
				}

				itemMK := &d2ast.Key{
					Key: &d2ast.KeyPath{
						Path: []*d2ast.StringBox{
							d2ast.MakeValueBox(d2ast.RawString(mk.Key.Path[1].Unbox().ScalarString(), true)).StringBox(),
						},
					},
					Value: mk.Value,
				}
				appendMapKey(containerMap, itemMK)
				return nil
			}
		}

		writeableLabelMK := true
		var objK *d2ast.Key
		if baseAST != g.AST || imported {
			writeableRefs := GetWriteableRefs(obj, baseAST)
			if len(writeableRefs) > 0 {
				objK = writeableRefs[0].MapKey
			}
			if objK == nil {
				appendMapKey(scope, mk)
				return nil
			}
			writeableLabelMK = false
			for _, ref := range writeableRefs {
				if ref.MapKey == obj.Label.MapKey {
					writeableLabelMK = true
					break
				}
			}
		} else {
			// Even if not imported or different board, a label can be not writeable if it's in a class or var or glob
			// In those cases, the label is not a direct object reference
			found := false
			for _, ref := range obj.References {
				if ref.MapKey == obj.Label.MapKey {
					found = true
					break
				}
			}
			if !found {
				writeableLabelMK = false
			}
		}
		var m *d2ast.Map
		if objK != nil {
			m = objK.Value.Map
		} else {
			m = obj.Map
		}

		if (obj.Label.MapKey != nil && writeableLabelMK) && m == nil && (!found || reserved || len(mk.Edges) > 0) {
			m2 := &d2ast.Map{
				Range: d2ast.MakeRange(",1:0:0-1:0:0"),
			}
			if objK == nil {
				obj.Map = m2
				objK = obj.Label.MapKey
			} else {
				objK.Value.Map = m2
			}
			objK.Primary = objK.Value.ScalarBox()
			objK.Value = d2ast.MakeValueBox(m2)
			scope = m2

			mk.Key.Path = mk.Key.Path[toSkip-1:]
			toSkip = 1
			if len(mk.Key.Path) == 0 {
				mk.Key = nil
			}
		}

		if !found {
			appendMapKey(scope, mk)
			return nil
		}
	}
	ir, _, err := d2ir.Compile(g.AST, &d2ir.CompileOptions{
		FS: g.FS,
	})
	if err != nil {
		return err
	}
	attrs := obj.Attributes
	var edge *d2graph.Edge
	if len(mk.Edges) == 1 {
		if mk.EdgeIndex == nil {
			appendMapKey(scope, mk)
			return nil
		}
		var ok bool
		edge, ok = obj.HasEdge(mk)
		if !ok {
			return errors.New("edge not found")
		}
		imported = IsImportedEdge(baseAST, edge)
		refs := edge.References
		if baseAST != g.AST || imported {
			refs = GetWriteableEdgeRefs(edge, baseAST)
		}
		onlyInChain := true
		var earliestRef *d2graph.EdgeReference
		for i, ref := range refs {
			if earliestRef == nil || ref.MapKey.Range.Before(earliestRef.MapKey.Range) {
				earliestRef = &refs[i]
			}
			// TODO merge flat edgekeys
			// E.g. this can group into a map
			// (y -> z)[0].style.opacity: 0.4
			// (y -> z)[0].style.animated: true
			if len(ref.MapKey.Edges) == 1 {
				if ref.MapKey.EdgeIndex == nil || ref.MapKey.Value.Map != nil {
					onlyInChain = false
				}
			}

			if ref.MapKey.EdgeIndex == nil || !ref.MapKey.EdgeIndex.Glob {
				// If a ref has an exact match on this key, just change the value
				tmp1 := *ref.MapKey
				tmp2 := *mk
				noVal1 := &tmp1
				noVal2 := &tmp2
				noVal1.Value = d2ast.ValueBox{}
				noVal2.Value = d2ast.ValueBox{}
				if noVal1.D2OracleEquals(noVal2) {
					ref.MapKey.Value = mk.Value
					return nil
				}
			}
		}
		if onlyInChain {
			if earliestRef != nil && scope.Range.Before(earliestRef.MapKey.Range) {
				// Since the original mk was trimmed to common, we set to the edge that
				// the ref's scope is in
				mk.Edges[0] = earliestRef.Edge
				// We can't reference an edge before it's been defined
				earliestRef.Scope.InsertAfter(earliestRef.MapKey, mk)
			} else {
				appendMapKey(scope, mk)
			}
			return nil
		}
		attrs = edge.Attributes

		if mk.EdgeKey != nil {
			if _, ok := d2ast.ReservedKeywords[mk.EdgeKey.Path[0].Unbox().ScalarString()]; !ok {
				return errors.New("edge key must be reserved")
			}
			reserved = true

			toSkip = 1
			mk = &d2ast.Key{
				Key:   cloneKey(mk.EdgeKey),
				Value: mk.Value,
			}

			foundMap := false
			for _, ref := range refs {
				// TODO get the most nested one
				if ref.MapKey.Value.Map != nil {
					foundMap = true
					scope = ref.MapKey.Value.Map
					for _, n := range scope.Nodes {
						if n.MapKey == nil || n.MapKey.Value.Map == nil {
							continue
						}
						if n.MapKey.Key == nil || len(n.MapKey.Key.Path) != 1 {
							continue
						}
						if n.MapKey.Key.Path[0].Unbox().ScalarString() == mk.Key.Path[toSkip-1].Unbox().ScalarString() {
							scope = n.MapKey.Value.Map
							if mk.Key.Path[0].Unbox().ScalarString() == "source-arrowhead" && edge.SrcArrowhead != nil {
								attrs = *edge.SrcArrowhead
							}
							if mk.Key.Path[0].Unbox().ScalarString() == "target-arrowhead" && edge.DstArrowhead != nil {
								attrs = *edge.DstArrowhead
							}
							reservedKey = mk.Key.Path[0].Unbox().ScalarString()
							mk.Key.Path = mk.Key.Path[1:]
							reservedTargetKey = mk.Key.Path[0].Unbox().ScalarString()
							break
						}
					}
					break
				}
			}
			if !foundMap && attrs.Label.MapKey != nil {
				attrs.Label.MapKey.Primary = attrs.Label.MapKey.Value.ScalarBox()
				edgeMap := &d2ast.Map{
					Range: d2ast.MakeRange(",1:0:0-1:0:0"),
				}
				attrs.Label.MapKey.Value = d2ast.MakeValueBox(edgeMap)
				scope = edgeMap
			}
		}
	}

	if reserved {
		inlined := func(s *d2graph.Scalar) bool {
			if s != nil && s.MapKey != nil {
				// The value was set outside of what's writeable
				if s.MapKey.Range.Path != baseAST.Range.Path {
					return false
				}
				// Globs are also not writeable
				if s.MapKey.HasGlob() {
					return false
				}
			}
			return s != nil && s.MapKey != nil && !ir.InClass(s.MapKey)
		}
		reservedIndex := toSkip - 1
		if mk.Key != nil && len(mk.Key.Path) > 0 {
			if reservedKey == "" {
				reservedKey = mk.Key.Path[reservedIndex].Unbox().ScalarString()
			}
			switch reservedKey {
			case "shape":
				if inlined(&attrs.Shape) {
					attrs.Shape.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "link":
				if inlined(attrs.Link) {
					attrs.Link.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "tooltip":
				if inlined(attrs.Tooltip) {
					attrs.Tooltip.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "width":
				if inlined(attrs.WidthAttr) {
					attrs.WidthAttr.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "height":
				if inlined(attrs.HeightAttr) {
					attrs.HeightAttr.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "top":
				if inlined(attrs.Top) {
					attrs.Top.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "left":
				if inlined(attrs.Left) {
					attrs.Left.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "grid-rows":
				if inlined(attrs.GridRows) {
					attrs.GridRows.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "grid-columns":
				if inlined(attrs.GridColumns) {
					attrs.GridColumns.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "grid-gap":
				if inlined(attrs.GridGap) {
					attrs.GridGap.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "vertical-gap":
				if inlined(attrs.VerticalGap) {
					attrs.VerticalGap.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "horizontal-gap":
				if inlined(attrs.HorizontalGap) {
					attrs.HorizontalGap.MapKey.SetScalar(mk.Value.ScalarBox())
					return nil
				}
			case "source-arrowhead", "target-arrowhead":
				var arrowhead *d2graph.Attributes
				if reservedKey == "source-arrowhead" {
					if edge.SrcArrowhead != nil {
						attrs = *edge.SrcArrowhead
					}
					arrowhead = edge.SrcArrowhead
				} else {
					if edge.DstArrowhead != nil {
						attrs = *edge.DstArrowhead
					}
					arrowhead = edge.DstArrowhead
				}
				if arrowhead != nil {
					if reservedTargetKey == "" {
						if len(mk.Key.Path[reservedIndex:]) < 2 {
							return errors.New("malformed style setting, expected >= 2 part path")
						}
						reservedTargetKey = mk.Key.Path[reservedIndex+1].Unbox().ScalarString()
					}
					switch reservedTargetKey {
					case "shape":
						if inlined(&arrowhead.Shape) {
							arrowhead.Shape.MapKey.SetScalar(mk.Value.ScalarBox())
							return nil
						}
					case "label":
						if inlined(&arrowhead.Label) {
							arrowhead.Label.MapKey.SetScalar(mk.Value.ScalarBox())
							return nil
						}
					case "style":
						reservedTargetKey = mk.Key.Path[len(mk.Key.Path)-1].Unbox().ScalarString()
						if inlined(attrs.Style.Filled) {
							attrs.Style.Filled.MapKey.SetScalar(mk.Value.ScalarBox())
							return nil
						}
					}
				}
			case "style":
				if reservedTargetKey == "" {
					if len(mk.Key.Path[reservedIndex:]) != 2 {
						return errors.New("malformed style setting, expected 2 part path")
					}
					reservedTargetKey = mk.Key.Path[reservedIndex+1].Unbox().ScalarString()
				}
				switch reservedTargetKey {
				case "opacity":
					if inlined(attrs.Style.Opacity) {
						attrs.Style.Opacity.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "stroke":
					if inlined(attrs.Style.Stroke) {
						attrs.Style.Stroke.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "fill":
					if inlined(attrs.Style.Fill) {
						attrs.Style.Fill.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "stroke-width":
					if inlined(attrs.Style.StrokeWidth) {
						attrs.Style.StrokeWidth.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "stroke-dash":
					if inlined(attrs.Style.StrokeDash) {
						attrs.Style.StrokeDash.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "border-radius":
					if inlined(attrs.Style.BorderRadius) {
						attrs.Style.BorderRadius.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "shadow":
					if inlined(attrs.Style.Shadow) {
						attrs.Style.Shadow.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "3d":
					if inlined(attrs.Style.ThreeDee) {
						attrs.Style.ThreeDee.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "multiple":
					if inlined(attrs.Style.Multiple) {
						attrs.Style.Multiple.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "double-border":
					if inlined(attrs.Style.DoubleBorder) {
						attrs.Style.DoubleBorder.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "font":
					if inlined(attrs.Style.Font) {
						attrs.Style.Font.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "font-size":
					if inlined(attrs.Style.FontSize) {
						attrs.Style.FontSize.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "font-color":
					if inlined(attrs.Style.FontColor) {
						attrs.Style.FontColor.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "animated":
					if inlined(attrs.Style.Animated) {
						attrs.Style.Animated.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "bold":
					if inlined(attrs.Style.Bold) {
						attrs.Style.Bold.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "italic":
					if inlined(attrs.Style.Italic) {
						attrs.Style.Italic.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "underline":
					if inlined(attrs.Style.Underline) {
						attrs.Style.Underline.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				case "fill-pattern":
					if inlined(attrs.Style.FillPattern) {
						attrs.Style.FillPattern.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				}
			case "label":
				if len(mk.Key.Path[reservedIndex:]) > 1 {
					reservedTargetKey = mk.Key.Path[reservedIndex+1].Unbox().ScalarString()
					switch reservedTargetKey {
					case "near":
						if inlined(attrs.LabelPosition) {
							attrs.LabelPosition.MapKey.SetScalar(mk.Value.ScalarBox())
							return nil
						}
					}
				} else {
					if inlined(&attrs.Label) {
						attrs.Label.MapKey.SetScalar(mk.Value.ScalarBox())
						return nil
					}
				}
			}
		}
	} else if attrs.Label.MapKey != nil {
		attrs.Label.MapKey.SetScalar(mk.Value.ScalarBox())
		return nil
	}
	appendMapKey(scope, mk)
	return nil
}

func appendUniqueMapKey(m *d2ast.Map, mk *d2ast.Key) {
	for _, n := range m.Nodes {
		if n.MapKey != nil && n.MapKey.D2OracleEquals(mk) {
			return
		}
	}
	appendMapKey(m, mk)
}

func appendMapKey(m *d2ast.Map, mk *d2ast.Key) {
	m.Nodes = append(m.Nodes, d2ast.MapNodeBox{
		MapKey: mk,
	})
	if len(m.Nodes) == 1 &&
		mk.Key != nil &&
		len(mk.Key.Path) > 0 {
		_, ok := d2ast.ReservedKeywords[mk.Key.Path[0].Unbox().ScalarString()]
		if ok {
			// Allow one line reserved key (like shape) maps.
			// TODO: This needs to be smarter as certain keys are only reserved in context.
			// e.g. all keys under style are reserved. And constraint is only reserved
			// under sql_table shapes.
			return
		}
	}
	if !m.IsFileMap() && m.Range.OneLine() {
		// This doesn't require any shenanigans to prevent consuming sibling spacing because
		// d2format will use the mapkey's range to determine whether to insert extra newlines.
		// See TestCreate/make_scope_multiline_spacing_2
		m.Range.End.Line++
	}
}

func Delete(g *d2graph.Graph, boardPath []string, key string) (_ *d2graph.Graph, err error) {
	defer xdefer.Errorf(&err, "failed to delete %#v", key)

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}

	if len(mk.Edges) > 1 {
		return nil, errors.New("can only delete one edge at a time")
	}

	if len(mk.Edges) == 1 {
		edgeTrimCommon(mk)
	}

	boardG := g
	baseAST := g.AST

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
		// TODO beter name
		baseAST = boardG.BaseAST
	}

	g2, err := deleteReserved(g, boardPath, baseAST, mk)
	if err != nil {
		return nil, err
	}
	if g != g2 {
		return g2, nil
	}

	if len(mk.Edges) == 1 {
		obj := boardG.Root
		if mk.Key != nil {
			var ok bool
			obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
			if !ok {
				return g, nil
			}
		}
		e, ok := obj.HasEdge(mk)
		if !ok {
			return g, nil
		}

		imported := IsImportedEdge(baseAST, e)

		if imported {
			mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
			appendMapKey(baseAST, mk)
		} else {
			refs := e.References
			if len(boardPath) > 0 {
				refs := GetWriteableEdgeRefs(e, baseAST)
				if len(refs) != len(e.References) {
					mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
				}
			}

			if _, ok := mk.Value.Unbox().(*d2ast.Null); !ok {
				ref := refs[0]
				var refEdges []*d2ast.Edge
				for _, ref := range refs {
					refEdges = append(refEdges, ref.Edge)
				}
				ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, ref.MapKey.Edges[ref.MapKeyEdgeIndex].Src, true)
				ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, ref.MapKey.Edges[ref.MapKeyEdgeIndex].Dst, false)

				for i := len(e.References) - 1; i >= 0; i-- {
					ref := e.References[i]
					// Leave glob setters alone
					if !(ref.MapKey.EdgeIndex != nil && ref.MapKey.EdgeIndex.Glob) {
						deleteEdge(g, ref.Scope, ref.MapKey, ref.MapKeyEdgeIndex)
					}
				}

				edges, ok := obj.FindEdges(mk)
				if ok {
					for _, e2 := range edges {
						if e2.Index <= e.Index {
							continue
						}
						for i := len(e2.References) - 1; i >= 0; i-- {
							ref := e2.References[i]
							if ref.MapKey.EdgeIndex != nil {
								*ref.MapKey.EdgeIndex.Int--
							}
						}
					}
				}
			} else {
				// NOTE: it only needs to be after the last ref, but perhaps simplest and cleanest to append all nulls at the end
				appendMapKey(baseAST, mk)
			}
		}
		if len(boardPath) > 0 {
			replaced := ReplaceBoardNode(g.AST, baseAST, boardPath)
			if !replaced {
				return nil, fmt.Errorf("board %v AST not found", boardPath)
			}
			return recompile(g)
		}
		return recompile(boardG)
	}

	prevG, err := recompile(boardG)
	if err != nil {
		return nil, err
	}

	obj, ok := boardG.Root.HasChild(d2graph.Key(mk.Key))
	if !ok {
		return g, nil
	}

	imported := IsImportedObj(baseAST, obj)

	if imported {
		mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
		appendMapKey(baseAST, mk)
	} else {
		boardG, err = renameConflictsToParent(boardG, mk.Key)
		if err != nil {
			return nil, err
		}
		obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return g, nil
		}
		if len(boardPath) > 0 {
			writeableRefs := GetWriteableRefs(obj, baseAST)
			if len(writeableRefs) != len(obj.References) {
				mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
			}
		}

		if _, ok := mk.Value.Unbox().(*d2ast.Null); !ok {
			boardG, err = deleteObject(boardG, baseAST, mk.Key, obj)
			if err != nil {
				return nil, err
			}

			if err := updateNear(prevG, boardG, &key, nil, false); err != nil {
				return nil, err
			}
		} else {
			appendMapKey(baseAST, mk)
		}
	}

	if len(boardPath) > 0 {
		replaced := ReplaceBoardNode(g.AST, baseAST, boardPath)
		if !replaced {
			return nil, fmt.Errorf("board %v AST not found", boardPath)
		}
		return recompile(g)
	}

	return recompile(boardG)
}

func bumpChildrenUnderscores(m *d2ast.Map) {
	for _, n := range m.Nodes {
		if n.MapKey == nil {
			continue
		}
		if n.MapKey.Key != nil {
			if n.MapKey.Key.Path[0].Unbox().ScalarString() == "_" {
				n.MapKey.Key.Path = n.MapKey.Key.Path[1:]
			}
		}
		for _, e := range n.MapKey.Edges {
			if e.Src.Path[0].Unbox().ScalarString() == "_" {
				e.Src.Path = e.Src.Path[1:]
			}
			if e.Dst.Path[0].Unbox().ScalarString() == "_" {
				e.Dst.Path = e.Dst.Path[1:]
			}
		}
		if n.MapKey.Value.Map != nil {
			bumpChildrenUnderscores(n.MapKey.Value.Map)
		}
	}
}

func hoistRefChildren(g *d2graph.Graph, key *d2ast.KeyPath, ref d2graph.Reference) {
	if ref.MapKey == nil || ref.MapKey.Value.Map == nil {
		return
	}

	bumpChildrenUnderscores(ref.MapKey.Value.Map)
	scopeKey, scope := findNearestParentScope(g, key)
	for i := 0; i < len(ref.MapKey.Value.Map.Nodes); i++ {
		n := ref.MapKey.Value.Map.Nodes[i]
		if n.MapKey == nil {
			continue
		}
		if n.MapKey.Key != nil {
			_, ok := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
			if ok {
				continue
			}
		}
		scopeKey := cloneKey(scopeKey)
		scopeKey.Path = scopeKey.Path[:len(scopeKey.Path)-1]
		if n.MapKey.Key != nil {
			scopeKey.Path = append(scopeKey.Path, n.MapKey.Key.Path...)
		}
		if len(scopeKey.Path) > 0 {
			n.MapKey.Key = scopeKey
		}
		scope.InsertBefore(ref.MapKey, n.Unbox())
	}
}

// renameConflictsToParent renames would-be ID conflicts.
func renameConflictsToParent(g *d2graph.Graph, key *d2ast.KeyPath) (*d2graph.Graph, error) {
	obj, ok := g.Root.HasChild(d2graph.Key(key))
	if !ok {
		return g, nil
	}
	if obj.Shape.Value == d2target.ShapeSQLTable || obj.Shape.Value == d2target.ShapeClass {
		return g, nil
	}

	// Usually ignore the object when generating, but if a sibling has the same ID, can't ignore
	ignored := obj
	for _, ch := range obj.ChildrenArray {
		if ch.ID == obj.ID {
			ignored = nil
			break
		}
	}

	// Keep a list of newly generated IDs, so that generateUniqueKey considers them for conflict
	var newIDs []string
	// If we already renamed the key from another reference, no need to touch
	dedupedRenames := map[string]struct{}{}
	for _, ref := range obj.References {
		var absKeys []*d2ast.KeyPath

		if len(ref.Key.Path)-1 == ref.KeyPathIndex {
			if ref.MapKey == nil || ref.MapKey.Value.Map == nil {
				continue
			}
			var mapKeys []*d2ast.KeyPath
			for _, n := range ref.MapKey.Value.Map.Nodes {
				if n.MapKey == nil {
					continue
				}
				if n.MapKey.Key != nil {
					_, ok := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
					if ok {
						continue
					}
					mapKeys = append(mapKeys, n.MapKey.Key)
				}
				for _, e := range n.MapKey.Edges {
					mapKeys = append(mapKeys, e.Src)
					mapKeys = append(mapKeys, e.Dst)
				}
			}
			for _, k := range mapKeys {
				absKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
				if err != nil {
					absKey = &d2ast.KeyPath{}
				}
				absKey.Path = append(absKey.Path, ref.Key.Path...)
				absKey.Path = append(absKey.Path, k.Path[0])
				absKeys = append(absKeys, absKey)
			}
		} else if _, ok := d2ast.ReservedKeywords[ref.Key.Path[len(ref.Key.Path)-1].Unbox().ScalarString()]; !ok {
			absKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
			if err != nil {
				absKey = &d2ast.KeyPath{}
			}
			absKey.Path = append(absKey.Path, ref.Key.Path[:ref.KeyPathIndex+2]...)
			absKeys = append(absKeys, absKey)
		}

		renames := make(map[string]string)
		for _, absKey := range absKeys {
			ida := d2graph.Key(absKey)
			absKeyStr := strings.Join(ida, ".")
			if _, ok := dedupedRenames[absKeyStr]; ok {
				continue
			}
			// Stale reference
			dedupedRenames[absKeyStr] = struct{}{}
			// Do not consider the parent for conflicts, assume the parent will be deleted
			if ida[len(ida)-1] == ida[len(ida)-2] {
				continue
			}

			hoistedAbsKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
			if err != nil {
				hoistedAbsKey = &d2ast.KeyPath{}
			}
			hoistedAbsKey.Path = append(hoistedAbsKey.Path, ref.Key.Path[:ref.KeyPathIndex]...)
			hoistedAbsKey.Path = append(hoistedAbsKey.Path, absKey.Path[len(absKey.Path)-1])

			// Can't generate a key that'd conflict with sibling
			var siblingHoistedIDs []string
			for _, otherAbsKey := range absKeys {
				if absKey == otherAbsKey {
					continue
				}
				ida := d2graph.Key(otherAbsKey)
				absKeyStr := strings.Join(ida, ".")
				if _, ok := dedupedRenames[absKeyStr]; ok {
					continue
				}
				hoistedAbsKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
				if err != nil {
					hoistedAbsKey = &d2ast.KeyPath{}
				}
				hoistedAbsKey.Path = append(hoistedAbsKey.Path, ref.Key.Path[:ref.KeyPathIndex]...)
				hoistedAbsKey.Path = append(hoistedAbsKey.Path, otherAbsKey.Path[len(otherAbsKey.Path)-1])
				siblingHoistedIDs = append(siblingHoistedIDs, strings.Join(d2graph.Key(hoistedAbsKey), "."))
			}

			uniqueKeyStr, _, err := generateUniqueKey(g, strings.Join(d2graph.Key(hoistedAbsKey), "."), ignored, append(newIDs, siblingHoistedIDs...))
			if err != nil {
				return nil, err
			}
			newIDs = append(newIDs, uniqueKeyStr)
			uniqueKey, err := d2parser.ParseKey(uniqueKeyStr)
			if err != nil {
				return nil, err
			}

			renamedKey := cloneKey(absKey)
			renamedKey.Path[len(renamedKey.Path)-1].Unbox().SetString(uniqueKey.Path[len(uniqueKey.Path)-1].Unbox().ScalarString())

			renamedKeyStr := strings.Join(d2graph.Key(renamedKey), ".")
			if absKeyStr != renamedKeyStr {
				renames[absKeyStr] = renamedKeyStr
			}
			dedupedRenames[renamedKeyStr] = struct{}{}
		}
		// We need to rename in a conflict-free order
		// E.g. imagine you have children `Text 4` and `Text`.
		// `Text 4` would get renamed to `Text` and `Text` gets renamed to `Text 2`
		// But if we follow that order, then both would get named to `Text 2`
		// So order such that the ones that have a conflict are done last, after the no-conflict ones are done
		// A cycle would never occur, as the uniqueness constraint is guaranteed
		var renameOrder []string
		for k, v := range renames {
			// conflict
			if _, ok := renames[v]; ok {
				renameOrder = append(renameOrder, k)
			} else {
				renameOrder = append([]string{k}, renameOrder...)
			}
		}
		for _, k := range renameOrder {
			var err error
			// TODO boardPath
			g, err = move(g, nil, k, renames[k], false)
			if err != nil {
				return nil, err
			}
		}
	}
	return g, nil
}

func deleteReserved(g *d2graph.Graph, boardPath []string, baseAST *d2ast.Map, mk *d2ast.Key) (*d2graph.Graph, error) {
	targetKey := mk.Key
	if len(mk.Edges) == 1 {
		if mk.EdgeKey == nil {
			return g, nil
		}
		targetKey = mk.EdgeKey
	}
	_, ok := d2ast.ReservedKeywords[targetKey.Path[len(targetKey.Path)-1].Unbox().ScalarString()]
	if !ok {
		return g, nil
	}

	var e *d2graph.Edge
	obj := g.Root
	if len(boardPath) > 0 {
		boardG := GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
		obj = boardG.Root
	}
	if len(mk.Edges) == 1 {
		if mk.Key != nil {
			var ok bool
			obj, ok = obj.HasChild(d2graph.Key(mk.Key))
			if !ok {
				return g, nil
			}
		}
		e, ok = obj.HasEdge(mk)
		if !ok {
			return g, nil
		}
		imported := IsImportedEdge(baseAST, e)

		deleted, err := deleteEdgeField(g, baseAST, e, targetKey.Path[len(targetKey.Path)-1].Unbox().ScalarString())
		if err != nil {
			return nil, err
		}
		if !deleted && imported {
			mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
			appendMapKey(baseAST, mk)
		}
		return recompile(g)
	}

	isNestedKey := false
	imported := false
	parts := d2graph.Key(targetKey)
	for i, id := range parts {
		_, ok := d2ast.ReservedKeywords[id]
		if ok {
			if id == "style" {
				isNestedKey = true
				continue
			}
			if id == "label" || id == "icon" {
				if i < len(parts)-1 {
					isNestedKey = true
					continue
				}
			}
			if isNestedKey {
				deleted, err := deleteObjField(g, baseAST, obj, id)
				if err != nil {
					return nil, err
				}
				if !deleted && imported {
					mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
					appendMapKey(baseAST, mk)
				}
				continue
			}

			if id == "near" ||
				id == "tooltip" ||
				id == "icon" ||
				id == "width" ||
				id == "height" ||
				id == "left" ||
				id == "top" ||
				id == "link" {
				deleted, err := deleteObjField(g, baseAST, obj, id)
				if err != nil {
					return nil, err
				}
				if !deleted && imported {
					mk.Value = d2ast.MakeValueBox(&d2ast.Null{})
					appendMapKey(baseAST, mk)
				} else {
				}
			}
			break
		}
		obj, ok = obj.HasChild([]string{id})
		if !ok {
			return nil, fmt.Errorf("object not found")
		}
		imported = IsImportedObj(baseAST, obj)
	}

	return recompile(g)
}

func deleteMapField(m *d2ast.Map, field string) (deleted bool) {
	for i := 0; i < len(m.Nodes); i++ {
		n := m.Nodes[i]
		if n.MapKey != nil && n.MapKey.Key != nil {
			if n.MapKey.Key.Path[0].Unbox().ScalarString() == field {
				deleteFromMap(m, n.MapKey)
			} else if n.MapKey.Key.Path[0].Unbox().ScalarString() == "style" ||
				n.MapKey.Key.Path[0].Unbox().ScalarString() == "label" ||
				n.MapKey.Key.Path[0].Unbox().ScalarString() == "icon" ||
				n.MapKey.Key.Path[0].Unbox().ScalarString() == "source-arrowhead" ||
				n.MapKey.Key.Path[0].Unbox().ScalarString() == "target-arrowhead" {
				if n.MapKey.Value.Map != nil {
					deleted2 := deleteMapField(n.MapKey.Value.Map, field)
					if deleted2 {
						deleted = true
					}
					if len(n.MapKey.Value.Map.Nodes) == 0 {
						deleted2 := deleteFromMap(m, n.MapKey)
						if deleted2 {
							deleted = true
						}
					}
				} else if len(n.MapKey.Key.Path) == 2 && n.MapKey.Key.Path[1].Unbox().ScalarString() == field {
					deleted2 := deleteFromMap(m, n.MapKey)
					if deleted2 {
						deleted = true
					}
				}
			}
		}
	}
	return deleted
}

func deleteEdgeField(g *d2graph.Graph, ast *d2ast.Map, e *d2graph.Edge, field string) (deleted bool, _ error) {
	for _, ref := range e.References {
		// Edge chains can't have fields
		if len(ref.MapKey.Edges) > 1 {
			continue
		}
		if ref.MapKey.Range.Path != ast.Range.Path {
			continue
		}
		if ref.MapKey.Value.Map != nil {
			deleted2 := deleteMapField(ref.MapKey.Value.Map, field)
			if deleted2 {
				deleted = true
			}
		} else if ref.MapKey.EdgeKey != nil && ref.MapKey.EdgeKey.Path[len(ref.MapKey.EdgeKey.Path)-1].Unbox().ScalarString() == field {
			// It's always safe to delete, since edge references must coexist with edge definition elsewhere
			deleted2 := deleteFromMap(ref.Scope, ref.MapKey)
			if deleted2 {
				deleted = true
			}
		}
	}
	return deleted, nil
}

func deleteObjField(g *d2graph.Graph, ast *d2ast.Map, obj *d2graph.Object, field string) (deleted bool, _ error) {
	objK, err := d2parser.ParseKey(obj.AbsID())
	if err != nil {
		return false, err
	}
	objGK := d2graph.Key(objK)
	for _, ref := range obj.References {
		if ref.InEdge() {
			continue
		}
		if ref.Key.Range.Path != ast.Range.Path {
			continue
		}
		if ref.MapKey.Value.Map != nil {
			deleteMapField(ref.MapKey.Value.Map, field)
		} else if (len(ref.Key.Path) >= 2 &&
			ref.Key.Path[len(ref.Key.Path)-1].Unbox().ScalarString() == field &&
			ref.Key.Path[len(ref.Key.Path)-2].Unbox().ScalarString() == obj.ID) ||
			(len(ref.Key.Path) >= 3 &&
				ref.Key.Path[len(ref.Key.Path)-1].Unbox().ScalarString() == field &&
				(ref.Key.Path[len(ref.Key.Path)-2].Unbox().ScalarString() == "style" ||
					ref.Key.Path[len(ref.Key.Path)-2].Unbox().ScalarString() == "label" ||
					ref.Key.Path[len(ref.Key.Path)-2].Unbox().ScalarString() == "icon") &&
				ref.Key.Path[len(ref.Key.Path)-3].Unbox().ScalarString() == obj.ID) {
			tmpNodes := make([]d2ast.MapNodeBox, len(ref.Scope.Nodes))
			copy(tmpNodes, ref.Scope.Nodes)
			// If I delete this, will the object still exist?
			deleted2 := deleteFromMap(ref.Scope, ref.MapKey)
			if deleted2 {
				deleted = true
			}
			g2, err := recompile(g)
			if err != nil {
				return false, err
			}
			if _, ok := g2.Root.HasChild(objGK); !ok {
				// Nope, so can't delete it, just remove the field then
				ref.Scope.Nodes = tmpNodes
				ref.MapKey.Value = d2ast.ValueBox{}
				ref.Key.Path = ref.Key.Path[:ref.KeyPathIndex+1]
			}

		}
	}
	return deleted, nil
}

func deleteObject(g *d2graph.Graph, baseAST *d2ast.Map, key *d2ast.KeyPath, obj *d2graph.Object) (*d2graph.Graph, error) {
	var refEdges []*d2ast.Edge
	for _, ref := range obj.References {
		if ref.InEdge() {
			refEdges = append(refEdges, ref.MapKey.Edges[ref.MapKeyEdgeIndex])
		}
	}

	for i := len(obj.References) - 1; i >= 0; i-- {
		ref := obj.References[i]

		if len(ref.MapKey.Edges) == 0 {
			isSuffix := ref.KeyPathIndex == len(ref.Key.Path)-1
			if isSuffix && ref.MapKey != nil {
				ref.MapKey.Primary = d2ast.ScalarBox{}
			}
			ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex], ref.Key.Path[ref.KeyPathIndex+1:]...)
			withoutSpecial := go2.Filter(ref.Key.Path, func(x *d2ast.StringBox) bool {
				_, isReserved := d2ast.ReservedKeywords[x.Unbox().ScalarString()]
				isSpecial := isReserved || x.Unbox().ScalarString() == "_"
				return !isSpecial
			})
			if obj.Shape.Value == d2target.ShapeSQLTable || obj.Shape.Value == d2target.ShapeClass {
				deleteFromMap(ref.Scope, ref.MapKey)
			} else if len(withoutSpecial) == 0 {
				hoistRefChildren(g, key, ref)
				deleteFromMap(ref.Scope, ref.MapKey)
			} else if ref.MapKey.Value.Unbox() == nil &&
				obj.Parent != nil &&
				isSuffix &&
				len(obj.Parent.References) > 1 {
				// Redundant key.
				deleteFromMap(ref.Scope, ref.MapKey)
			} else if ref.MapKey.Value.Map != nil && isSuffix {
				for i := 0; i < len(ref.MapKey.Value.Map.Nodes); i++ {
					n := ref.MapKey.Value.Map.Nodes[i]
					if n.MapKey != nil && n.MapKey.Key != nil {
						_, ok := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
						if ok {
							deleteFromMap(ref.MapKey.Value.Map, n.MapKey)
							i--
							continue
						}
					}
				}
			} else if isSuffix {
				ref.MapKey.Value = d2ast.ValueBox{}
			}
		} else if ref.InEdge() {
			edge := ref.MapKey.Edges[ref.MapKeyEdgeIndex]

			if obj.Shape.Value == d2target.ShapeSQLTable || obj.Shape.Value == d2target.ShapeClass {
				if ref.MapKeyEdgeDest() {
					ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, edge.Src, true)
				} else {
					ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, edge.Dst, false)
				}
				deleteEdge(g, ref.Scope, ref.MapKey, ref.MapKeyEdgeIndex)
			} else if ref.KeyPathIndex == len(ref.Key.Path)-1 {
				if ref.MapKeyEdgeDest() {
					ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, edge.Src, true)
				} else {
					ensureNode(g, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, edge.Dst, false)
				}
				deleteEdge(g, ref.Scope, ref.MapKey, ref.MapKeyEdgeIndex)
			} else {
				ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex], ref.Key.Path[ref.KeyPathIndex+1:]...)

				// Skip visiting the same middle key in an edge chain
				if !ref.MapKeyEdgeDest() && i > 0 {
					nextRef := obj.References[i-1]
					if nextRef.InEdge() && nextRef.MapKey == ref.MapKey {
						i--
					}
				}
			}
		} else {
			// MapKey.Key with edge.
			ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex], ref.Key.Path[ref.KeyPathIndex+1:]...)
			if len(ref.Key.Path) == 0 {
				ref.MapKey.Key = nil
			}
		}
	}

	return g, nil
}

func findNearestParentScope(g *d2graph.Graph, k *d2ast.KeyPath) (prefix *d2ast.KeyPath, _ *d2ast.Map) {
	for i := 1; i < len(k.Path); i++ {
		scopeKey := cloneKey(k)
		scopeKey.Path = scopeKey.Path[:len(k.Path)-i]
		obj, ok := g.Root.HasChild(d2graph.Key(scopeKey))
		if ok && obj.Map != nil {
			prefix := cloneKey(k)
			prefix.Path = prefix.Path[len(k.Path)-i:]
			return prefix, obj.Map
		}
	}
	return k, g.AST
}

func deleteEdge(g *d2graph.Graph, scope *d2ast.Map, mk *d2ast.Key, i int) {
	edgesAfter := mk.Edges[i+1:]
	mk.Edges = mk.Edges[:i]

	for _, obj := range g.Objects {
		for j := range obj.References {
			ref := obj.References[j]
			if ref.InEdge() {
				if ref.MapKey == mk && ref.MapKeyEdgeIndex >= i {
					obj.References[j].MapKeyEdgeIndex -= i
				}
			}
		}
	}

	if len(edgesAfter) > 0 {
		tmp := *mk
		mk2 := &tmp
		mk2.Edges = edgesAfter
		scope.InsertAfter(mk, mk2)
	}
	if len(mk.Edges) == 0 {
		deleteFromMap(scope, mk)
	}
}

// ensureNode ensures that `k` exists in `scope` if `excludedEdges` were removed
func ensureNode(g *d2graph.Graph, excludedEdges []*d2ast.Edge, scopeObj *d2graph.Object, scope *d2ast.Map, cursor *d2ast.Key, k *d2ast.KeyPath, before bool) {
	if k == nil || len(k.Path) == 0 {
		return
	}
	if cursor.Key != nil && len(cursor.Key.Path) > 0 {
		k = cloneKey(k)
		k.Path = append(cursor.Key.Path, k.Path...)
	}

	obj, ok := scopeObj.HasChild(d2graph.Key(k))
	if ok {
		// If this key only exists as part of excludedEdges (edges that'll be deleted), we need to make a new one
		hasPersistingRef := false
		for _, ref := range obj.References {
			if !ref.InEdge() {
				hasPersistingRef = true
				break
			}
			if len(ref.MapKey.Edges) == 0 {
				continue
			}
			if !go2.Contains(excludedEdges, ref.MapKey.Edges[ref.MapKeyEdgeIndex]) {
				hasPersistingRef = true
				break
			}
		}
		if hasPersistingRef {
			return
		}
	}
	mk := &d2ast.Key{
		Key: k,
	}

	for _, n := range scope.Nodes {
		if n.MapKey != nil && n.MapKey.D2OracleEquals(mk) {
			return
		}
	}

	if before {
		scope.InsertBefore(cursor, mk)
	} else {
		scope.InsertAfter(cursor, mk)
	}
}

func Rename(g *d2graph.Graph, boardPath []string, key, newName string) (_ *d2graph.Graph, newKey string, err error) {
	defer xdefer.Errorf(&err, "failed to rename %#v to %#v", key, newName)

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, "", err
	}

	boardG := g

	if len(boardPath) > 0 {
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, "", fmt.Errorf("board %v not found", boardPath)
		}
	}

	if len(mk.Edges) > 0 && mk.EdgeKey == nil {
		// TODO: Not a fan of this dual interpretation depending on mk.Edges.
		// Maybe we remove Rename and just have Move.
		mk2, err := d2parser.ParseMapKey(newName)
		if err != nil {
			return nil, "", err
		}

		mk2.Key = mk.Key
		mk = mk2
	} else {
		_, ok := d2ast.ReservedKeywords[newName]
		if ok {
			return nil, "", fmt.Errorf("cannot rename to reserved keyword: %#v", newName)
		}
		if mk.Key != nil {
			obj, ok := boardG.Root.HasChild(d2graph.Key(mk.Key))
			if !ok {
				return nil, "", fmt.Errorf("key does not exist")
			}
			// If attempt to name something "x", but "x" already exists, rename it "x 2" instead
			generatedName, _, err := generateUniqueKey(boardG, newName, obj, nil)
			if err == nil {
				newName = generatedName
			}
		}
		// TODO: Handle mk.EdgeKey
		mk.Key.Path[len(mk.Key.Path)-1] = d2ast.MakeValueBox(d2ast.RawString(newName, true)).StringBox()
	}

	g, err = move(g, boardPath, key, d2format.Format(mk), false)
	return g, newName, err
}

func trimReservedSuffix(path []*d2ast.StringBox) []*d2ast.StringBox {
	for i, p := range path {
		if _, ok := d2ast.ReservedKeywords[p.Unbox().ScalarString()]; ok {
			return path[:i]
		}
	}
	return path
}

// Does not handle edge keys, on account of edge keys can only be reserved, e.g. (a->b).style.color: red
func Move(g *d2graph.Graph, boardPath []string, key, newKey string, includeDescendants bool) (_ *d2graph.Graph, err error) {
	defer xdefer.Errorf(&err, "failed to move: %#v to %#v", key, newKey)
	return move(g, boardPath, key, newKey, includeDescendants)
}

func move(g *d2graph.Graph, boardPath []string, key, newKey string, includeDescendants bool) (*d2graph.Graph, error) {
	if key == newKey {
		return g, nil
	}

	boardG := g
	baseAST := g.AST

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
		// TODO beter name
		baseAST = boardG.BaseAST
	}

	newKey, _, err := generateUniqueKey(boardG, newKey, nil, nil)
	if err != nil {
		return nil, err
	}

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}

	mk2, err := d2parser.ParseMapKey(newKey)
	if err != nil {
		return nil, err
	}
	edgeTrimCommon(mk)
	edgeTrimCommon(mk2)

	if len(mk.Edges) > 0 && mk.EdgeKey == nil {
		if d2format.Format(mk.Key) != d2format.Format(mk2.Key) {
			// TODO just prevent moving edges at all
			return nil, errors.New("moving across scopes isn't supported for edges")
		}
		obj := g.Root
		if mk.Key != nil {
			var ok bool
			obj, ok = g.Root.HasChild(d2graph.Key(mk.Key))
			if !ok {
				return nil, fmt.Errorf("edge referenced by from does not exist")
			}
		}
		e, ok := obj.HasEdge(mk)
		if !ok {
			return nil, fmt.Errorf("edge referenced by to does not exist")
		}
		_, ok = obj.HasEdge(mk2)
		if ok {
			return nil, fmt.Errorf("to edge already exists")
		}

		for i := len(e.References) - 1; i >= 0; i-- {
			ref := e.References[i]
			ref.MapKey.Edges[ref.MapKeyEdgeIndex].SrcArrow = mk2.Edges[0].SrcArrow
			ref.MapKey.Edges[ref.MapKeyEdgeIndex].DstArrow = mk2.Edges[0].DstArrow
		}
		return recompile(g)
	}

	prevG, err := recompile(boardG)
	if err != nil {
		return nil, err
	}

	ak := d2graph.Key(mk.Key)
	ak2 := d2graph.Key(mk2.Key)

	isCrossScope := strings.Join(ak[:len(ak)-1], ".") != strings.Join(ak2[:len(ak2)-1], ".")

	if isCrossScope && !includeDescendants {
		boardG, err = renameConflictsToParent(boardG, mk.Key)
		if err != nil {
			return nil, err
		}
	}

	obj, ok := boardG.Root.HasChild(ak)
	if !ok {
		return nil, fmt.Errorf("key referenced by from does not exist")
	}

	if len(boardPath) > 0 {
		writeableRefs := GetWriteableRefs(obj, baseAST)
		if len(writeableRefs) != len(obj.References) {
			return nil, OutsideScopeError{}
		}
	}

	toParent := boardG.Root
	if isCrossScope && len(ak2) > 1 {
		toParent, ok = boardG.Root.HasChild(ak2[:len(ak2)-1])
		if !ok {
			return nil, fmt.Errorf("key referenced by to parent does not exist")
		}
	}

	// Cross-scope move:
	// 1. Ensure parent node exists as a Key
	// 2. Ensure parent node Key has a map to accept moved node
	// 3. Rename
	// 4. Update all Key references
	// 5. Update all Edge references

	// 1. Ensure parent node exists as a Key
	// The toParent may only exist as an implicit, edge-created node
	//
	// For example, d.e here
	// a.b.c -> d.e.f
	// MOVE(a.b, d.e.q)
	// We can't open d.e as a map, so we need to create a new key

	// If the key targeted exists only as implicit edge creation, e.g. the b in `a.b -> ...`,
	// then we'll be able to move it anywhere by changing that key, e.g. `_._.c.x -> ...`
	// Otherwise, we'll need to make sure the parent exists as a map to move into
	needsLandingMap := false
	if isCrossScope {
		for _, ref := range obj.References {
			if !ref.InEdge() {
				needsLandingMap = true
				break
			}
			if ref.KeyPathIndex != len(ref.Key.Path)-1 {
				needsLandingMap = true
				break
			}
		}
	}
	if isCrossScope && len(ak2) > 1 && needsLandingMap {
		parentExistsAsKey := false
		for _, ref := range toParent.References {
			if len(ref.MapKey.Edges) == 0 {
				parentExistsAsKey = true
				break
			}
		}
		if !parentExistsAsKey {
			// Choose the most nested edge as cursor for this new node
			var mostNestedRef d2graph.Reference
			for _, ref := range toParent.References {
				if mostNestedRef == (d2graph.Reference{}) || len(ref.ScopeObj.AbsIDArray()) > len(mostNestedRef.ScopeObj.AbsIDArray()) {
					mostNestedRef = ref
				}
			}
			detachedMK := &d2ast.Key{
				Key: cloneKey(mostNestedRef.MapKey.Key),
			}
			detachedMK.Key.Path = mostNestedRef.Key.Path[:mostNestedRef.KeyPathIndex+1]
			detachedMK.Range = d2ast.MakeRange(",1:0:0-1:0:0")
			mostNestedRef.Scope.InsertAfter(mostNestedRef.MapKey, detachedMK)

			mostNestedRef.ScopeObj.AppendReferences(d2graph.Key(detachedMK.Key), d2graph.Reference{
				Key:    detachedMK.Key,
				MapKey: detachedMK,
				Scope:  mostNestedRef.Scope,
			}, mostNestedRef.ScopeObj)
		}
	}

	println(d2format.Format(g.AST))
	// 2. Ensure parent node Key has a map to accept moved node.
	// This map will be what MOVE will append the new key to
	toScope := boardG.AST
	if isCrossScope && len(ak2) > 1 && needsLandingMap {
		mostNestedParentRefs := getMostNestedRefs(toParent)
		mapExists := false
		for _, ref := range mostNestedParentRefs {
			if ref.KeyPathIndex == len(ref.Key.Path)-1 && ref.MapKey.Value.Map != nil {
				toScope = ref.MapKey.Value.Map
				mapExists = true
				break
			}
		}

		if !mapExists {
			toScope = &d2ast.Map{
				Range: d2ast.MakeRange(",1:0:0-1:0:0"),
			}

			ref := mostNestedParentRefs[len(mostNestedParentRefs)-1]
			// Parent node key exists as part of a flat key, need to split up
			if ref.KeyPathIndex < len(ref.Key.Path)-1 {
				detachedMK := &d2ast.Key{
					Key: cloneKey(ref.MapKey.Key),
				}
				detachedMK.Value = ref.MapKey.Value
				detachedMK.Key.Path = ref.Key.Path[ref.KeyPathIndex+1:]
				detachedMK.Range = d2ast.MakeRange(",1:0:0-1:0:0")

				ref.Key.Path = ref.Key.Path[:ref.KeyPathIndex+1]
				appendUniqueMapKey(toScope, detachedMK)
			} else {
				ref.MapKey.Primary = ref.MapKey.Value.ScalarBox()
			}
			ref.MapKey.Value = d2ast.MakeValueBox(toScope)
		}
	}
	mostNestedRefs := getMostNestedRefs(obj)
	for _, ref := range obj.References {
		isExplicit := ref.KeyPathIndex == len(trimReservedSuffix(ref.Key.Path))-1 && ref.MapKey.Value.Map == nil

		// 3. Rename
		if ak[len(ak)-1] != ak2[len(ak2)-1] {
			ref.Key.Path[ref.KeyPathIndex] = d2ast.MakeValueBox(d2ast.RawString(
				mk2.Key.Path[len(mk2.Key.Path)-1].Unbox().ScalarString(),
				true,
			)).StringBox()
		}

		// 4. Update all Key references
		if len(ref.MapKey.Edges) != 0 {
			continue
		}

		firstNonUnderscoreIndex := 0
		ida := d2graph.Key(ref.Key)
		for i, id := range ida {
			if id != "_" {
				firstNonUnderscoreIndex = i
				break
			}
		}
		resolvedObj, resolvedIDA, err := d2graph.ResolveUnderscoreKey(ida, ref.ScopeObj)
		if err != nil {
			return nil, err
		}
		if resolvedObj != obj {
			ida = resolvedIDA
		}
		// e.g. "a.b.shape: circle"
		_, endsWithReserved := d2ast.ReservedKeywords[ida[len(ida)-1]]
		ida = go2.Filter(ida, func(x string) bool {
			_, ok := d2ast.ReservedKeywords[x]
			return !ok
		})

		// There are 3 cases of what we want to do with Key references in cross scope
		// 1. Transplant. Remove from its current scope, plop it into new scope
		// -- The ref key is the key being moved
		// 2. Split. One node remains, while another gets added to new scope
		// -- The ref key is a flat map with more than just the key being moved
		// 3. Extend.
		// -- The key is moving from its current scope into a more nested scope
		// 4. Slice.
		// -- The key is moving from its current scope out to a less nested scope
		if isCrossScope {
			if (!includeDescendants && len(ida) == 1) || (includeDescendants && ref.KeyPathIndex == firstNonUnderscoreIndex) {
				// 1. Transplant
				absKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
				if err != nil {
					absKey = &d2ast.KeyPath{}
				}
				absKey.Path = append(absKey.Path, ref.Key.Path...)
				if !includeDescendants {
					hoistRefChildren(boardG, absKey, ref)
				}
				deleteFromMap(ref.Scope, ref.MapKey)
				detachedMK := &d2ast.Key{Primary: ref.MapKey.Primary, Key: cloneKey(ref.MapKey.Key)}
				detachedMK.Key.Path = go2.Filter(detachedMK.Key.Path, func(x *d2ast.StringBox) bool {
					return x.Unbox().ScalarString() != "_"
				})
				detachedMK.Value = ref.MapKey.Value
				if ref.MapKey != nil && ref.MapKey.Value.Map != nil {
					// Without including descendants, just copy over the reserved
					if !includeDescendants {
						detachedMK.Value.Map = &d2ast.Map{
							Range: ref.MapKey.Value.Map.Range,
						}
						for _, n := range ref.MapKey.Value.Map.Nodes {
							if n.MapKey == nil {
								continue
							}
							if n.MapKey.Key != nil {
								_, ok := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
								if ok {
									detachedMK.Value.Map.Nodes = append(detachedMK.Value.Map.Nodes, n)
								}
							}
						}
						if len(detachedMK.Value.Map.Nodes) == 0 {
							detachedMK.Value.Map = nil
						}
					} else {
						// Usually copy everything as is when including descendants
						// The exception is underscored keys, which need to be updated
						for _, n := range ref.MapKey.Value.Map.Nodes {
							if n.MapKey == nil {
								continue
							}
							if n.MapKey.Key != nil {
								if n.MapKey.Key.Path[0].Unbox().ScalarString() == "_" {
									resolvedParent, resolvedScopeKey, err := d2graph.ResolveUnderscoreKey(d2graph.Key(n.MapKey.Key), obj)
									if err != nil {
										return nil, err
									}

									newPath, err := pathFromScopeKey(boardG, &d2ast.Key{Key: d2ast.MakeKeyPath(append(resolvedParent.AbsIDArray(), resolvedScopeKey...))}, ak2)
									if err != nil {
										return nil, err
									}
									n.MapKey.Key.Path = newPath
								}
							}
							for _, e := range n.MapKey.Edges {
								if e.Src.Path[0].Unbox().ScalarString() == "_" {
									resolvedParent, resolvedScopeKey, err := d2graph.ResolveUnderscoreKey(d2graph.Key(e.Src), obj)
									if err != nil {
										return nil, err
									}

									newPath, err := pathFromScopeKey(boardG, &d2ast.Key{Key: d2ast.MakeKeyPath(append(resolvedParent.AbsIDArray(), resolvedScopeKey...))}, ak2)
									if err != nil {
										return nil, err
									}
									e.Src.Path = newPath
								}
								if e.Dst.Path[0].Unbox().ScalarString() == "_" {
									resolvedParent, resolvedScopeKey, err := d2graph.ResolveUnderscoreKey(d2graph.Key(e.Dst), obj)
									if err != nil {
										return nil, err
									}

									newPath, err := pathFromScopeKey(boardG, &d2ast.Key{Key: d2ast.MakeKeyPath(append(resolvedParent.AbsIDArray(), resolvedScopeKey...))}, ak2)
									if err != nil {
										return nil, err
									}
									e.Dst.Path = newPath
								}
							}
						}
					}
				}
				appendUniqueMapKey(toScope, detachedMK)
			} else if len(ida) > 1 && (endsWithReserved || !isExplicit || go2.Contains(mostNestedRefs, ref)) {
				// 2. Split
				detachedMK := &d2ast.Key{Key: cloneKey(ref.MapKey.Key)}
				if includeDescendants {
					detachedMK.Key.Path = append([]*d2ast.StringBox{}, ref.Key.Path[ref.KeyPathIndex:]...)
				} else {
					detachedMK.Key.Path = []*d2ast.StringBox{ref.Key.Path[ref.KeyPathIndex]}
				}
				if includeDescendants {
					detachedMK.Value = ref.MapKey.Value
					ref.MapKey.Value = d2ast.ValueBox{}
				} else if ref.KeyPathIndex == len(filterReservedPath(ref.Key.Path))-1 {
					withReserved, withoutReserved := filterReserved(ref.MapKey.Value)
					detachedMK.Value = withReserved
					ref.MapKey.Value = withoutReserved
					detachedMK.Key.Path = append([]*d2ast.StringBox{}, ref.Key.Path[ref.KeyPathIndex:]...)
					ref.Key.Path = ref.Key.Path[:ref.KeyPathIndex+1]
				}
				if includeDescendants {
					ref.Key.Path = ref.Key.Path[:ref.KeyPathIndex]
				} else {
					ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex], ref.Key.Path[ref.KeyPathIndex+1:]...)
				}
				appendUniqueMapKey(toScope, detachedMK)
			} else if len(getCommonPath(ak, ak2)) > 0 {
				// 3. Extend
				// This case does not make sense for includeDescendants
				newKeyPath := ref.Key.Path[:ref.KeyPathIndex]
				newKeyPath = append(newKeyPath, mk2.Key.Path[len(getCommonPath(ak, ak2)):]...)
				ref.Key.Path = append(newKeyPath, ref.Key.Path[ref.KeyPathIndex+1:]...)
			} else {
				// 4. Slice
				scopePath := ref.ScopeObj.AbsIDArray()
				if len(getCommonPath(scopePath, ak2)) != len(scopePath) {
					deleteFromMap(ref.Scope, ref.MapKey)
				} else {
					ref.Key.Path = ref.Key.Path[ref.KeyPathIndex:]
					exists := false
					for _, n := range toScope.Nodes {
						if n.MapKey != nil && n.MapKey != ref.MapKey && n.MapKey.D2OracleEquals(ref.MapKey) {
							exists = true
						}
					}
					if exists {
						deleteFromMap(ref.Scope, ref.MapKey)
					}
				}
			}
		}
	}
	var refEdges []*d2ast.Edge
	for _, ref := range obj.References {
		if ref.InEdge() {
			refEdges = append(refEdges, ref.MapKey.Edges[ref.MapKeyEdgeIndex])
		}
	}
	for i := 0; i < len(obj.References); i++ {
		if !isCrossScope {
			break
		}

		ref := obj.References[i]
		// 5. Update all Edge references
		if len(ref.MapKey.Edges) == 0 {
			continue
		}

		if i > 0 && ref.Key == obj.References[i-1].Key {
			continue
		}

		firstNonUnderscoreIndex := 0
		ida := d2graph.Key(ref.Key)
		for i, id := range ida {
			if id != "_" {
				firstNonUnderscoreIndex = i
				break
			}
		}

		if ref.KeyPathIndex != len(ref.Key.Path)-1 {
			// When moving a node out of an edge, e.g. the `b` out of `a.b.c -> ...`,
			// The edge needs to continue targeting the same thing (c)
			// Split
			detachedMK := &d2ast.Key{
				Key: cloneKey(ref.Key),
			}
			oldPath, err := pathFromScopeObj(boardG, mk, ref.ScopeObj)
			if err != nil {
				return nil, err
			}
			newPath, err := pathFromScopeObj(boardG, mk2, ref.ScopeObj)
			if err != nil {
				return nil, err
			}
			if includeDescendants {
				// When including descendants, the only thing that gets dropped, if any, is the uncommon leading path of new key and key
				// E.g. when moving `a.b` to `x.b` with edge ref key of `a.b.c`, then changing it to `x.b.c` will drop `a`
				diff := len(oldPath) - len(newPath)
				// Only need to check uncommon path if the lengths are the same
				if diff == 0 {
					diff = len(getUncommonPath(d2graph.Key(&d2ast.KeyPath{Path: oldPath}), d2graph.Key(&d2ast.KeyPath{Path: newPath})))
				}
				// If the old key is longer than the new key, we already know all the diff would be dropped
				if diff > 0 && ref.KeyPathIndex != firstNonUnderscoreIndex {
					detachedMK.Key.Path = append([]*d2ast.StringBox{}, ref.Key.Path[ref.KeyPathIndex-diff:ref.KeyPathIndex]...)
					appendUniqueMapKey(ref.Scope, detachedMK)
				}
			} else {
				detachedMK.Key.Path = []*d2ast.StringBox{ref.Key.Path[ref.KeyPathIndex]}
				appendUniqueMapKey(toScope, detachedMK)
			}

			if includeDescendants {
				ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex-len(oldPath)+1], append(newPath, ref.Key.Path[ref.KeyPathIndex+1:]...)...)
			} else {
				ref.Key.Path = append(ref.Key.Path[:ref.KeyPathIndex], ref.Key.Path[ref.KeyPathIndex+1:]...)
			}
		} else {
			// When moving a node connected to an edge, we have to ensure parents continue to exist
			// e.g. the `c` out of `a.b.c -> ...`
			// `a.b` needs to exist
			newPath, err := pathFromScopeObj(boardG, mk2, ref.ScopeObj)
			if err != nil {
				return nil, err
			}
			if len(go2.Filter(ref.Key.Path, func(x *d2ast.StringBox) bool { return x.Unbox().ScalarString() != "_" })) > 1 {
				detachedK := cloneKey(ref.Key)
				detachedK.Path = detachedK.Path[:len(detachedK.Path)-1]
				ensureNode(boardG, refEdges, ref.ScopeObj, ref.Scope, ref.MapKey, detachedK, false)
			}

			if includeDescendants {
				ref.Key.Path = append(newPath, ref.Key.Path[go2.Min(len(ref.Key.Path), ref.KeyPathIndex+len(newPath)):]...)
			} else {
				ref.Key.Path = newPath
			}
		}
	}

	if err := updateNear(prevG, boardG, &key, &newKey, includeDescendants); err != nil {
		return nil, err
	}

	if len(boardPath) > 0 {
		replaced := ReplaceBoardNode(g.AST, baseAST, boardPath)
		if !replaced {
			return nil, fmt.Errorf("board %v AST not found", boardPath)
		}
		return recompile(g)
	}

	return recompile(boardG)
}

// filterReserved takes a Value and splits it into 2
// 1. Value with reserved keywords
// 2. Without reserved keywords
// Maintains structure, so if reserved keywords were part of map, the output will keep them in a map
func filterReserved(value d2ast.ValueBox) (with, without d2ast.ValueBox) {
	with, without = d2ast.MakeValueBox(value.Unbox()), d2ast.ValueBox{}

	if value.Map != nil {
		var forWith []d2ast.MapNodeBox
		var forWithout []d2ast.MapNodeBox

		// assume comments are above what they describe
		// going down the map line by line, we batch here as we encounter, and flush to either forWith or forWithout, whichever hits first
		var commentBatch []d2ast.MapNodeBox
		flushComments := func(to *[]d2ast.MapNodeBox) {
			*to = append(*to, commentBatch...)
			commentBatch = nil
		}

		for _, n := range value.Map.Nodes {
			if n.MapKey == nil {
				if n.Comment != nil || n.BlockComment != nil {
					commentBatch = append(commentBatch, n)
				}
				continue
			}
			if n.MapKey.Key == nil || (len(n.MapKey.Key.Path) > 1) {
				flushComments(&forWithout)
				forWithout = append(forWithout, n)
				continue
			}
			_, ok := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
			if !ok {
				flushComments(&forWithout)
				forWithout = append(forWithout, n)
				continue
			}
			flushComments(&forWith)
			forWith = append(forWith, n)
		}

		if len(forWith) > 0 {
			if with.Map == nil {
				with.Map = &d2ast.Map{
					Range: d2ast.MakeRange(",1:0:0-1:0:0"),
				}
			}
			with.Map.Nodes = forWith
		} else {
			with.Map = nil
		}
		if len(forWithout) > 0 {
			if without.Map == nil {
				without.Map = &d2ast.Map{
					Range: value.Map.Range,
				}
			}
			without.Map.Nodes = forWithout
		} else {
			without.Map = nil
		}
	}

	return
}

// updateNear updates all the Near fields
// prevG is the graph before the update (i.e. deletion, rename, move)
func updateNear(prevG, g *d2graph.Graph, from, to *string, includeDescendants bool) error {
	mk, _ := d2parser.ParseMapKey(*from)
	if len(mk.Edges) > 0 {
		return nil
	}
	if mk.Key == nil {
		return nil
	}
	if len(mk.Key.Path) == 0 {
		return nil
	}

	// TODO get rid of repetition

	// Update all the `near` keys that are one level nested
	// x: {
	//   near: z
	// }
	for _, obj := range g.Objects {
		if obj.Map == nil {
			continue
		}
		for _, n := range obj.Map.Nodes {
			if n.MapKey == nil {
				continue
			}
			if n.MapKey.Key == nil {
				continue
			}
			if len(n.MapKey.Key.Path) == 0 {
				continue
			}
			if len(n.MapKey.Key.Path) > 1 {
				if n.MapKey.Key.Path[len(n.MapKey.Key.Path)-2].Unbox().ScalarString() == "label" ||
					n.MapKey.Key.Path[len(n.MapKey.Key.Path)-2].Unbox().ScalarString() == "icon" {
					continue
				}
			}
			if n.MapKey.Key.Path[len(n.MapKey.Key.Path)-1].Unbox().ScalarString() == "near" {
				k := n.MapKey.Value.ScalarBox().Unbox().ScalarString()
				if _, ok := d2ast.NearConstants[k]; ok {
					continue
				}
				if strings.EqualFold(k, *from) && to == nil {
					deleteFromMap(obj.Map, n.MapKey)
				} else {
					valueMK, err := d2parser.ParseMapKey(k)
					if err != nil {
						return err
					}
					tmpG, _ := recompile(prevG)
					appendMapKey(tmpG.AST, valueMK)
					if to == nil {
						deltas, err := DeleteIDDeltas(tmpG, nil, *from)
						if err != nil {
							return err
						}
						if v, ok := deltas[k]; ok {
							n.MapKey.Value = d2ast.MakeValueBox(d2ast.RawString(v, false))
						}
					} else {
						deltas, err := MoveIDDeltas(tmpG, *from, *to, includeDescendants)
						if err != nil {
							return err
						}
						if v, ok := deltas[k]; ok {
							n.MapKey.Value = d2ast.MakeValueBox(d2ast.RawString(v, false))
						}
					}
				}
			}
		}
	}

	// Update all the `near` keys that are flat (x.near: z)
	for _, obj := range g.Objects {
		for _, ref := range obj.References {
			if ref.MapKey == nil {
				continue
			}
			if ref.MapKey.Key == nil {
				continue
			}
			if len(ref.MapKey.Key.Path) == 0 {
				continue
			}
			if ref.MapKey.Key.Path[len(ref.MapKey.Key.Path)-1].Unbox().ScalarString() == "near" {
				k := ref.MapKey.Value.ScalarBox().Unbox().ScalarString()
				if strings.EqualFold(k, *from) && to == nil {
					deleteFromMap(obj.Map, ref.MapKey)
				} else {
					valueMK, err := d2parser.ParseMapKey(k)
					if err != nil {
						return err
					}
					tmpG, _ := recompile(prevG)
					appendMapKey(tmpG.AST, valueMK)
					if to == nil {
						deltas, err := DeleteIDDeltas(tmpG, nil, *from)
						if err != nil {
							return err
						}
						if v, ok := deltas[k]; ok {
							ref.MapKey.Value = d2ast.MakeValueBox(d2ast.RawString(v, false))
						}
					} else {
						deltas, err := MoveIDDeltas(tmpG, *from, *to, includeDescendants)
						if err != nil {
							return err
						}
						if v, ok := deltas[k]; ok {
							ref.MapKey.Value = d2ast.MakeValueBox(d2ast.RawString(v, false))
						}
					}
				}
			}
		}
	}

	return nil
}

func deleteFromMap(m *d2ast.Map, mk *d2ast.Key) bool {
	for i, n := range m.Nodes {
		if n.MapKey == mk {
			m.Nodes = append(m.Nodes[:i], m.Nodes[i+1:]...)
			return true
		}
	}
	return false
}

func ReparentIDDelta(g *d2graph.Graph, boardPath []string, key, parentKey string) (string, error) {
	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return "", err
	}

	boardG := g

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return "", fmt.Errorf("board %v not found", boardPath)
		}
	}

	obj, ok := boardG.Root.HasChild(d2graph.Key(mk.Key))
	if !ok {
		return "", errors.New("not found")
	}

	parent := boardG.Root
	if parentKey != "" {
		mk2, err := d2parser.ParseMapKey(parentKey)
		if err != nil {
			return "", err
		}
		parent, ok = boardG.Root.HasChild(d2graph.Key(mk2.Key))
		if !ok {
			return "", errors.New("not found")
		}
	}

	prevParent := obj.Parent
	obj.Parent = parent
	id := obj.AbsID()
	obj.Parent = prevParent
	return id, nil
}

func ReconnectEdgeIDDeltas(g *d2graph.Graph, boardPath []string, edgeKey string, srcKey, dstKey *string) (deltas map[string]string, err error) {
	defer xdefer.Errorf(&err, "failed to get deltas for reconnect edge %#v", edgeKey)
	deltas = make(map[string]string)
	// Reconnection: nothing is created or destroyed, the edge just gets a new ID
	// For deltas, it's indices that change:
	// - old sibling edges may decrement index
	// -- happens when the edge is not the last edge index
	// - new sibling edges may increment index
	// -- happens when the edge is not the last edge index
	// - new edge of course always needs an entry

	// The change happens at the first ref, since that is what changes index
	mk, err := d2parser.ParseMapKey(edgeKey)
	if err != nil {
		return nil, err
	}

	if len(mk.Edges) == 0 {
		return nil, errors.New("edgeKey must be an edge")
	}

	if mk.EdgeIndex == nil {
		return nil, errors.New("edgeKey must refer to an existing edge")
	}

	edgeTrimCommon(mk)

	boardG := g

	if len(boardPath) > 0 {
		// When compiling a nested board, we can read from boardG but only write to baseBoardG
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
	}

	obj := boardG.Root
	if mk.Key != nil {
		var ok bool
		obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return nil, errors.New("edge not found")
		}
	}
	edge, ok := obj.HasEdge(mk)
	if !ok {
		return nil, errors.New("edge not found")
	}

	if srcKey != nil {
		if edge.Src.AbsID() == *srcKey {
			srcKey = nil
		}
	}

	if dstKey != nil {
		if edge.Dst.AbsID() == *dstKey {
			dstKey = nil
		}
	}

	if srcKey == nil && dstKey == nil {
		return nil, nil
	}

	newSrc := edge.Src
	newDst := edge.Dst
	var src *d2graph.Object
	var dst *d2graph.Object
	if srcKey != nil {
		srcmk, err := d2parser.ParseMapKey(*srcKey)
		if err != nil {
			return nil, err
		}
		src, ok = boardG.Root.HasChild(d2graph.Key(srcmk.Key))
		if !ok {
			return nil, errors.New("newSrc not found")
		}
		newSrc = src
	}
	if dstKey != nil {
		dstmk, err := d2parser.ParseMapKey(*dstKey)
		if err != nil {
			return nil, err
		}
		dst, ok = boardG.Root.HasChild(d2graph.Key(dstmk.Key))
		if !ok {
			return nil, errors.New("newDst not found")
		}
		newDst = dst
	}

	// The first ref is always the definition
	firstRef := edge.References[0]
	line := firstRef.MapKey.Range.Start.Line
	newIndex := 0

	// For the edge's own delta, it just needs to know how many edges came before it with the same src and dst
	for _, otherEdge := range boardG.Edges {
		if otherEdge.Src == newSrc && otherEdge.Dst == newDst {
			firstRef := otherEdge.References[0]
			if firstRef.MapKey.Range.Start.Line <= line {
				newIndex++
			}
		}
		if otherEdge.Src == edge.Src && otherEdge.Dst == edge.Dst && otherEdge.Index > edge.Index {
			before := otherEdge.AbsID()
			otherEdge.Index--
			after := otherEdge.AbsID()
			deltas[before] = after
			otherEdge.Index++
		}
	}

	for _, otherEdge := range g.Edges {
		if otherEdge.Src == newSrc && otherEdge.Dst == newDst {
			if otherEdge.Index >= newIndex {
				before := otherEdge.AbsID()
				otherEdge.Index++
				after := otherEdge.AbsID()
				deltas[before] = after
				otherEdge.Index--
			}
		}
	}

	newEdge := &d2graph.Edge{
		Src:      newSrc,
		Dst:      newDst,
		SrcArrow: edge.SrcArrow,
		DstArrow: edge.DstArrow,
		Index:    newIndex,
	}

	deltas[edge.AbsID()] = newEdge.AbsID()

	return deltas, nil
}

// generateUniqueKey generates a unique key by appending a number after `prefix` such that it doesn't conflict with any IDs in `g`
// If `ignored` is not nil, a conflict with the ignored object is allowed. An example use case is to generate a unique ID for a child being
// hoisted out of its container, and you know the container is going to be deleted.
//
// If `included` is not nil, the generated key must also not conflict with a key in `included`, on top of not conflicting with any IDs in `g`.
// This is for when an operation needs to generate multiple unique keys in one go, like deleting a container and giving new IDs to all children
func generateUniqueKey(g *d2graph.Graph, prefix string, ignored *d2graph.Object, included []string) (key string, edge bool, _ error) {
	mk, err := d2parser.ParseMapKey(prefix)
	if err != nil {
		return "", false, err
	}

	if len(mk.Edges) > 1 {
		return "", false, errors.New("cannot generate unique key for edge chain")
	}

	if len(mk.Edges) == 1 {
		if mk.EdgeIndex == nil || mk.EdgeIndex.Int == nil {
			mk.EdgeIndex = &d2ast.EdgeIndex{
				Int: go2.Pointer(0),
			}
		}

		edgeTrimCommon(mk)
		obj := g.Root
		if mk.Key != nil {
			var ok bool
			obj, ok = g.Root.HasChild(d2graph.Key(mk.Key))
			if !ok {
				return d2format.Format(mk), true, nil
			}
		}
		for {
			_, ok := obj.HasEdge(mk)
			if !ok {
				return d2format.Format(mk), true, nil
			}
			mk.EdgeIndex.Int = go2.Pointer(*mk.EdgeIndex.Int + 1)
		}
	}

	// If a key is not provided, we generate one.
	if mk.Key == nil {
		mk.Key = &d2ast.KeyPath{
			Path: []*d2ast.StringBox{d2ast.MakeValueBox(d2ast.RawString(xrand.Base64(16), true)).StringBox()},
		}
	} else if obj, ok := g.Root.HasChild(d2graph.Key(mk.Key)); ok && obj != ignored {
		// The key may already have an index, e.g. "x 2"
		spaced := strings.Split(prefix, " ")
		if len(spaced) > 1 {
			if _, err := strconv.Atoi(spaced[len(spaced)-1]); err == nil {
				withoutIndex := strings.Join(spaced[:len(spaced)-1], " ")
				mk, err = d2parser.ParseMapKey(withoutIndex)
				if err != nil {
					return "", false, err
				}
			}
		}
	}

	k2 := cloneKey(mk.Key)
	i := 0
	for {
		conflictsWithIncluded := false
		for _, s := range included {
			if d2format.Format(k2) == s {
				conflictsWithIncluded = true
				break
			}
		}
		if !conflictsWithIncluded {
			obj, ok := g.Root.HasChild(d2graph.Key(k2))
			if !ok || obj == ignored {
				return d2format.Format(k2), false, nil
			}
		}

		rr := fmt.Sprintf("%s %d", mk.Key.Path[len(mk.Key.Path)-1].Unbox().ScalarString(), i+2)
		k2.Path[len(k2.Path)-1] = d2ast.MakeValueBox(d2ast.RawString(rr, true)).StringBox()
		i++
	}
}

func cloneKey(k *d2ast.KeyPath) *d2ast.KeyPath {
	if k == nil {
		return &d2ast.KeyPath{}
	}
	tmp := *k
	k2 := &tmp
	k2.Path = nil
	for _, p := range k.Path {
		k2.Path = append(k2.Path, d2ast.MakeValueBox(p.Unbox().Copy()).StringBox())
	}
	return k2
}

func getCommonPath(a, b []string) []string {
	var out []string
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			out = a[:i+1]
		}
	}
	return out
}

func getUncommonPath(a, b []string) []string {
	var out []string
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			out = a[:i+1]
		}
	}
	return out
}

func edgeTrimCommon(mk *d2ast.Key) {
	if len(mk.Edges) != 1 {
		return
	}
	e := mk.Edges[0]
	for len(e.Src.Path) > 1 && len(e.Dst.Path) > 1 {
		if !strings.EqualFold(e.Src.Path[0].Unbox().ScalarString(), e.Dst.Path[0].Unbox().ScalarString()) {
			return
		}
		if mk.Key == nil {
			mk.Key = &d2ast.KeyPath{}
		}
		mk.Key.Path = append(mk.Key.Path, e.Src.Path[0])
		e.Src.Path = e.Src.Path[1:]
		e.Dst.Path = e.Dst.Path[1:]
	}
}

func MoveIDDeltas(g *d2graph.Graph, key, newKey string, includeDescendants bool) (deltas map[string]string, err error) {
	defer xdefer.Errorf(&err, "failed to get deltas for move from %#v to %#v", key, newKey)
	deltas = make(map[string]string)

	if key == newKey {
		return deltas, nil
	}

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}

	newKey, _, err = generateUniqueKey(g, newKey, nil, nil)
	if err != nil {
		return nil, err
	}

	mk2, err := d2parser.ParseMapKey(newKey)
	if err != nil {
		return nil, err
	}

	ak := d2graph.Key(mk.Key)
	ak2 := d2graph.Key(mk2.Key)
	isCrossScope := strings.Join(ak[:len(ak)-1], ".") != strings.Join(ak2[:len(ak2)-1], ".")

	edgeTrimCommon(mk)
	obj := g.Root

	// Conflict IDs are when a container is moved and the children conflict with something in parent
	conflictNewIDs := make(map[*d2graph.Object]string)
	conflictOldIDs := make(map[*d2graph.Object]string)
	var newIDs []string
	if mk.Key != nil {
		var ok bool
		obj, ok = g.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return nil, nil
		}

		ignored := obj
		for _, ch := range obj.ChildrenArray {
			if ch.ID == obj.ID {
				ignored = nil
				break
			}
		}

		if !includeDescendants {
			for _, ch := range obj.ChildrenArray {
				chMK, err := d2parser.ParseMapKey(ch.AbsID())
				if err != nil {
					return nil, err
				}
				ida := d2graph.Key(chMK.Key)
				if ida[len(ida)-1] == ida[len(ida)-2] {
					continue
				}

				hoistedAbsID := ch.ID
				if obj.Parent != g.Root {
					hoistedAbsID = obj.Parent.AbsID() + "." + ch.ID
				}
				hoistedMK, err := d2parser.ParseMapKey(hoistedAbsID)
				if err != nil {
					return nil, err
				}

				conflictsWithNewID := false
				for _, id := range newIDs {
					if id == d2format.Format(hoistedMK.Key) {
						conflictsWithNewID = true
						break
					}
				}

				if _, ok := g.Root.HasChild(d2graph.Key(hoistedMK.Key)); ok || conflictsWithNewID {
					newKey, _, err := generateUniqueKey(g, hoistedAbsID, ignored, newIDs)
					if err != nil {
						return nil, err
					}
					newMK, err := d2parser.ParseMapKey(newKey)
					if err != nil {
						return nil, err
					}
					newAK := d2graph.Key(newMK.Key)
					conflictOldIDs[ch] = ch.ID
					conflictNewIDs[ch] = newAK[len(newAK)-1]
					newIDs = append(newIDs, d2format.Format(newMK.Key))
				} else {
					newIDs = append(newIDs, d2format.Format(hoistedMK.Key))
				}
			}
		}
	}

	if len(mk.Edges) > 1 {
		return nil, nil
	}
	if len(mk.Edges) == 1 {
		if len(mk.Edges) == 0 {
			return nil, errors.New("cannot rename edge to node")
		}
		if len(mk.Edges) > 1 {
			return nil, errors.New("cannot rename edge to edge chain")
		}

		e, ok := obj.HasEdge(mk)
		if !ok {
			return nil, nil
		}
		beforeID := e.AbsID()
		tmp := *e
		e2 := &tmp
		e2.SrcArrow = mk2.Edges[0].SrcArrow == "<"
		e2.DstArrow = mk2.Edges[0].DstArrow == ">"
		deltas[beforeID] = e2.AbsID()
		return deltas, nil
	}

	beforeObjID := obj.ID

	toParent := g.Root
	if len(ak2) > 1 {
		var ok bool
		toParent, ok = g.Root.HasChild(ak2[:len(ak2)-1])
		if !ok {
			return nil, errors.New("to parent not found")
		}
	}
	id := ak2[len(ak2)-1]

	tmpRenames := func() func() {
		if isCrossScope && !includeDescendants {
			for _, ch := range obj.ChildrenArray {
				ch.Parent = obj.Parent
			}
		}

		prevParent := obj.Parent
		obj.Parent = toParent
		obj.ID = id

		for k, v := range conflictNewIDs {
			k.ID = v
		}
		return func() {
			for k, v := range conflictOldIDs {
				k.ID = v
			}
			obj.ID = beforeObjID
			obj.Parent = prevParent

			if isCrossScope && !includeDescendants {
				for _, ch := range obj.ChildrenArray {
					ch.Parent = obj
				}
			}
		}
	}

	appendNodeDelta := func(ch *d2graph.Object) {
		beforeID := ch.AbsID()
		revert := tmpRenames()
		deltas[beforeID] = ch.AbsID()
		revert()
	}

	appendEdgeDelta := func(ch *d2graph.Object) {
		for _, e := range obj.Graph.Edges {
			if e.Src == ch || e.Dst == ch {
				beforeID := e.AbsID()
				revert := tmpRenames()
				deltas[beforeID] = e.AbsID()
				revert()
			}
		}
	}

	var recurse func(ch *d2graph.Object)
	recurse = func(ch *d2graph.Object) {
		for _, ch := range ch.ChildrenArray {
			appendNodeDelta(ch)
			appendEdgeDelta(ch)
			recurse(ch)
		}
	}
	appendNodeDelta(obj)
	appendEdgeDelta(obj)
	recurse(obj)
	return deltas, nil
}

func DeleteIDDeltas(g *d2graph.Graph, boardPath []string, key string) (deltas map[string]string, err error) {
	defer xdefer.Errorf(&err, "failed to get deltas for deletion of %#v", key)
	deltas = make(map[string]string)

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}

	edgeTrimCommon(mk)

	boardG := g
	if len(boardPath) > 0 {
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
	}

	obj := boardG.Root
	conflictNewIDs := make(map[*d2graph.Object]string)
	conflictOldIDs := make(map[*d2graph.Object]string)
	var newIDs []string
	if mk.Key != nil {
		ida := d2graph.Key(mk.Key)
		// Deleting a reserved field cannot possibly have any deltas
		if _, ok := d2ast.ReservedKeywords[ida[len(ida)-1]]; ok {
			return nil, nil
		}

		var ok bool
		obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return nil, nil
		}

		ignored := obj
		for _, ch := range obj.ChildrenArray {
			if ch.ID == obj.ID {
				ignored = nil
				break
			}
		}

		for _, ch := range obj.ChildrenArray {
			// Record siblings as the unique key generated should not conflict with any siblings either
			var siblingsToBeHoisted []string
			for _, ch2 := range obj.ChildrenArray {
				if ch2 != ch {
					chMK, err := d2parser.ParseMapKey(ch2.AbsID())
					if err != nil {
						return nil, err
					}
					ida := d2graph.Key(chMK.Key)
					if ida[len(ida)-1] == ida[len(ida)-2] {
						continue
					}
					hoistedAbsID := ch2.ID
					if obj.Parent != boardG.Root {
						hoistedAbsID = obj.Parent.AbsID() + "." + ch2.ID
					}
					siblingsToBeHoisted = append(siblingsToBeHoisted, hoistedAbsID)
				}
			}
			chMK, err := d2parser.ParseMapKey(ch.AbsID())
			if err != nil {
				return nil, err
			}
			ida := d2graph.Key(chMK.Key)
			if ida[len(ida)-1] == ida[len(ida)-2] {
				continue
			}
			hoistedAbsID := ch.ID
			if obj.Parent != boardG.Root {
				hoistedAbsID = obj.Parent.AbsID() + "." + ch.ID
			}
			hoistedMK, err := d2parser.ParseMapKey(hoistedAbsID)
			if err != nil {
				return nil, err
			}

			conflictsWithNewID := false
			for _, id := range newIDs {
				if id == d2format.Format(hoistedMK.Key) {
					conflictsWithNewID = true
					break
				}
			}

			if conflictingObj, ok := boardG.Root.HasChild(d2graph.Key(hoistedMK.Key)); (ok && conflictingObj != obj) || conflictsWithNewID {
				newKey, _, err := generateUniqueKey(boardG, hoistedAbsID, ignored, append(newIDs, siblingsToBeHoisted...))
				if err != nil {
					return nil, err
				}
				newMK, err := d2parser.ParseMapKey(newKey)
				if err != nil {
					return nil, err
				}
				newAK := d2graph.Key(newMK.Key)
				conflictOldIDs[ch] = ch.ID
				conflictNewIDs[ch] = newAK[len(newAK)-1]
				newIDs = append(newIDs, d2format.Format(newMK.Key))
			} else {
				newIDs = append(newIDs, d2format.Format(hoistedMK.Key))
			}
		}
	}
	if len(mk.Edges) > 1 {
		return nil, nil
	}
	if len(mk.Edges) == 1 {
		// Anything deleted in an edge key cannot affect deltas
		if mk.EdgeKey != nil {
			return nil, nil
		}
		e, ok := obj.HasEdge(mk)
		if !ok {
			return nil, nil
		}
		ea, ok := obj.FindEdges(mk)
		if !ok {
			return nil, nil
		}
		for _, e2 := range ea {
			if e2.Index > e.Index {
				beforeID := e2.AbsID()
				e2.Index--
				deltas[beforeID] = e2.AbsID()
				e2.Index++
			}
		}
		return deltas, nil
	}

	for _, ch := range obj.ChildrenArray {
		tmpRenames := func() func() {
			prevIDs := make(map[*d2graph.Object]string)
			for _, ch := range obj.ChildrenArray {
				prevIDs[ch] = ch.ID
				ch.Parent = obj.Parent
			}
			for k, v := range conflictNewIDs {
				k.ID = v
			}

			return func() {
				for k, v := range conflictOldIDs {
					k.ID = v
				}
				for _, ch := range obj.ChildrenArray {
					ch.Parent = obj
					ch.ID = prevIDs[ch]
				}
			}
		}

		appendNodeDelta := func(ch2 *d2graph.Object) {
			beforeAbsID := ch2.AbsID()
			revert := tmpRenames()
			deltas[beforeAbsID] = ch2.AbsID()
			revert()

		}
		appendEdgeDelta := func(ch2 *d2graph.Object) {
			for _, e := range obj.Graph.Edges {
				if e.Src == ch2 || e.Dst == ch2 {
					beforeAbsID := e.AbsID()
					revert := tmpRenames()
					deltas[beforeAbsID] = e.AbsID()
					revert()

				}
			}
		}

		var recurse func(ch2 *d2graph.Object)
		recurse = func(ch2 *d2graph.Object) {
			for _, ch2 := range ch2.ChildrenArray {
				appendNodeDelta(ch2)
				appendEdgeDelta(ch2)
				recurse(ch2)
			}
		}

		appendNodeDelta(ch)
		appendEdgeDelta(ch)
		recurse(ch)
	}
	return deltas, nil
}

func RenameIDDeltas(g *d2graph.Graph, boardPath []string, key, newName string) (deltas map[string]string, err error) {
	defer xdefer.Errorf(&err, "failed to get deltas for renaming of %#v to %#v", key, newName)
	deltas = make(map[string]string)

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}

	boardG := g
	if len(boardPath) > 0 {
		boardG = GetBoardGraph(g, boardPath)
		if boardG == nil {
			return nil, fmt.Errorf("board %v not found", boardPath)
		}
	}

	edgeTrimCommon(mk)
	obj := boardG.Root
	if mk.Key != nil {
		var ok bool
		obj, ok = boardG.Root.HasChild(d2graph.Key(mk.Key))
		if !ok {
			return nil, nil
		}
	}
	if len(mk.Edges) > 1 {
		return nil, nil
	}
	if len(mk.Edges) == 1 {
		mk2, err := d2parser.ParseMapKey(newName)
		if err != nil {
			return nil, err
		}
		if len(mk.Edges) == 0 {
			return nil, errors.New("cannot rename edge to node")
		}
		if len(mk.Edges) > 1 {
			return nil, errors.New("cannot rename edge to edge chain")
		}

		e, ok := obj.HasEdge(mk)
		if !ok {
			return nil, nil
		}
		beforeID := e.AbsID()
		tmp := *e
		e2 := &tmp
		e2.SrcArrow = mk2.Edges[0].SrcArrow == "<"
		e2.DstArrow = mk2.Edges[0].DstArrow == ">"
		deltas[beforeID] = e2.AbsID()
		return deltas, nil
	}

	if mk.Key.Path[len(mk.Key.Path)-1].Unbox().ScalarString() == newName {
		return deltas, nil
	}

	mk.Key.Path[len(mk.Key.Path)-1].Unbox().SetString(newName)
	uniqueKeyStr, _, err := generateUniqueKey(boardG, strings.Join(d2graph.Key(mk.Key), "."), obj, nil)
	if err != nil {
		return nil, err
	}
	uniqueKey, err := d2parser.ParseKey(uniqueKeyStr)
	if err != nil {
		return nil, err
	}
	newNameKey := uniqueKey.Path[len(uniqueKey.Path)-1].Unbox().ScalarString()
	newNameKey = d2format.Format(d2ast.RawString(newNameKey, true))

	beforeObjID := obj.ID

	appendNodeDelta := func(ch *d2graph.Object) {
		if obj.ID != newNameKey {
			beforeID := ch.AbsID()
			obj.ID = newNameKey
			deltas[beforeID] = ch.AbsID()
			obj.ID = beforeObjID
		}
	}

	appendEdgeDelta := func(ch *d2graph.Object) {
		for _, e := range obj.Graph.Edges {
			if e.Src == ch || e.Dst == ch {
				if obj.ID != newNameKey {
					beforeID := e.AbsID()
					obj.ID = newNameKey
					deltas[beforeID] = e.AbsID()
					obj.ID = beforeObjID
				}
			}
		}
	}

	var recurse func(ch *d2graph.Object)
	recurse = func(ch *d2graph.Object) {
		for _, ch := range ch.ChildrenArray {
			appendNodeDelta(ch)
			appendEdgeDelta(ch)
			recurse(ch)
		}
	}
	appendNodeDelta(obj)
	appendEdgeDelta(obj)
	recurse(obj)
	return deltas, nil
}

func hasSpace(tag string) bool {
	for _, r := range tag {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func getMostNestedRefs(obj *d2graph.Object) []d2graph.Reference {
	var most d2graph.Reference
	for _, ref := range obj.References {
		if len(ref.MapKey.Edges) == 0 {
			most = ref
			break
		}
	}
	for _, ref := range obj.References {
		if len(ref.MapKey.Edges) != 0 {
			continue
		}

		scopeKey, err := d2parser.ParseKey(ref.ScopeObj.AbsID())
		if err != nil {
			scopeKey = &d2ast.KeyPath{}
		}
		mostKey, err := d2parser.ParseKey(most.ScopeObj.AbsID())
		if err != nil {
			mostKey = &d2ast.KeyPath{}
		}
		_, resolvedScopeKey, err := d2graph.ResolveUnderscoreKey(d2graph.Key(scopeKey), ref.ScopeObj)
		if err != nil {
			continue
		}
		_, resolvedMostKey, err := d2graph.ResolveUnderscoreKey(d2graph.Key(mostKey), ref.ScopeObj)
		if err != nil {
			continue
		}
		if len(resolvedScopeKey) > len(resolvedMostKey) {
			most = ref
		}
	}

	var out []d2graph.Reference
	for _, ref := range obj.References {
		if len(ref.MapKey.Edges) != 0 {
			continue
		}
		if ref.ScopeObj.AbsID() == most.ScopeObj.AbsID() {
			out = append(out, ref)
		}
	}

	return out
}

func filterReservedPath(path []*d2ast.StringBox) (filtered []*d2ast.StringBox) {
	for _, box := range path {
		if _, ok := d2ast.ReservedKeywords[strings.ToLower(box.Unbox().ScalarString())]; ok {
			return
		}
		filtered = append(filtered, box)
	}
	return
}
