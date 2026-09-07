package d2talalayout

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/routing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

type translation struct {
	objectIDs        map[*d2graph.Object]layoutgraph.EntityID
	edgeIDs          map[*d2graph.Edge]layoutgraph.EntityID
	edgeDestinations map[*d2graph.Edge]*d2graph.Object
}

const firstD2SpillEntityID layoutgraph.EntityID = 1 << 32

func d2FNV32(id string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return hash.Sum32()
}

type d2EntityIdentity[T comparable] struct {
	entity T
	absID  string
}

type hashedD2EntityIdentity[T comparable] struct {
	d2EntityIdentity[T]
	hash uint32
}

// allocateD2EntityIDs preserves the unsigned FNV-1a ID for every nonzero
// singleton bucket. Only colliding or root-reserved zero hashes receive
// deterministic spill IDs above the uint32 range. Consequently, every bounded
// set of distinct D2 identities has a total, architecture-independent mapping
// without perturbing ordinary graph ordering.
func allocateD2EntityIDs[T comparable](
	ctx context.Context,
	kind string,
	identities []d2EntityIdentity[T],
) (map[T]layoutgraph.EntityID, error) {
	if ctx == nil {
		return nil, fmt.Errorf("allocate D2 %s IDs requires a context", kind)
	}
	hashed := make([]hashedD2EntityIdentity[T], len(identities))
	bucketSizes := make(map[uint32]int, len(identities))
	seenAbsIDs := make(map[string]struct{}, len(identities))
	for i, identity := range identities {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if _, duplicate := seenAbsIDs[identity.absID]; duplicate {
			return nil, fmt.Errorf("D2 %s ID %q is repeated", kind, identity.absID)
		}
		seenAbsIDs[identity.absID] = struct{}{}
		hash := d2FNV32(identity.absID)
		hashed[i] = hashedD2EntityIdentity[T]{d2EntityIdentity: identity, hash: hash}
		bucketSizes[hash]++
	}

	allocated := make(map[T]layoutgraph.EntityID, len(identities))
	ambiguous := make([]hashedD2EntityIdentity[T], 0)
	for i, identity := range hashed {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if identity.hash != 0 && bucketSizes[identity.hash] == 1 {
			allocated[identity.entity] = layoutgraph.EntityID(identity.hash)
			continue
		}
		ambiguous = append(ambiguous, identity)
	}
	slices.SortFunc(ambiguous, func(a, b hashedD2EntityIdentity[T]) int {
		if order := cmp.Compare(a.hash, b.hash); order != 0 {
			return order
		}
		return cmp.Compare(a.absID, b.absID)
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i, identity := range ambiguous {
		allocated[identity.entity] = firstD2SpillEntityID + layoutgraph.EntityID(i)
	}
	return allocated, nil
}

func d2TALAEdgeAbsID(edge *d2graph.Edge) string {
	if !isSupportedLifelineEdge(edge) {
		return edge.AbsID()
	}
	effectiveEdge := *edge
	effectiveEdge.Dst = edge.Src
	return effectiveEdge.AbsID()
}

func translateGraph(ctx context.Context, g *d2graph.Graph, talaGraph *layoutgraph.Graph, loadExistingLayout bool) (translation, error) {
	bindings := translation{
		objectIDs:        make(map[*d2graph.Object]layoutgraph.EntityID),
		edgeIDs:          make(map[*d2graph.Edge]layoutgraph.EntityID),
		edgeDestinations: make(map[*d2graph.Edge]*d2graph.Object),
	}
	if err := ctx.Err(); err != nil {
		return bindings, err
	}
	if g == nil || g.Root == nil {
		return bindings, fmt.Errorf("tala requires a D2 graph with a root object")
	}
	if talaGraph == nil {
		return bindings, fmt.Errorf("tala requires a destination graph")
	}
	visits, objects, err := collectD2ObjectVisits(ctx, g.Root)
	if err != nil {
		return bindings, err
	}
	if err := validateD2GraphStructure(g, visits, objects); err != nil {
		return bindings, err
	}
	if err := validateD2Objects(ctx, visits, loadExistingLayout); err != nil {
		return bindings, err
	}
	if err := validateD2Edges(ctx, g.Edges, objects); err != nil {
		return bindings, err
	}
	objectIdentities := make([]d2EntityIdentity[*d2graph.Object], len(visits))
	for i, visit := range visits {
		objectIdentities[i] = d2EntityIdentity[*d2graph.Object]{
			entity: visit.object,
			absID:  visit.object.AbsID(),
		}
	}
	objectIDs, err := allocateD2EntityIDs(ctx, "object", objectIdentities)
	if err != nil {
		return bindings, err
	}
	edgeIdentities := make([]d2EntityIdentity[*d2graph.Edge], len(g.Edges))
	edgeAbsIDs := make(map[*d2graph.Edge]string, len(g.Edges))
	for i, edge := range g.Edges {
		absID := d2TALAEdgeAbsID(edge)
		edgeAbsIDs[edge] = absID
		edgeIdentities[i] = d2EntityIdentity[*d2graph.Edge]{entity: edge, absID: absID}
	}
	edgeIDs, err := allocateD2EntityIDs(ctx, "edge", edgeIdentities)
	if err != nil {
		return bindings, err
	}

	toNode := make(map[*d2graph.Object]*layoutgraph.Node)
	if g.Root.Direction.Value != "" {
		talaGraph.Directions[nil] = parseDirection(g.Root.Direction.Value)
	}
	if g.Root.Shape.Value == d2target.ShapeHierarchy {
		talaGraph.IsRootHierarchy = true
	}

	for _, visit := range visits {
		if err := ctx.Err(); err != nil {
			return bindings, err
		}
		obj, parent := visit.object, visit.parent
		intID := objectIDs[obj]
		bindings.objectIDs[obj] = intID
		n := layoutgraph.NewNode(intID, obj.Width, obj.Height)
		if obj.Top != nil && obj.Left != nil {
			top, _ := parseD2IntegerAttribute(obj.ID, "top", obj.Top, -maxResultCoordinate, maxResultCoordinate)
			left, _ := parseD2IntegerAttribute(obj.ID, "left", obj.Left, -maxResultCoordinate, maxResultCoordinate)
			n.FixedTopLeft = geo.NewPoint(float64(left), float64(top))
		}
		toNode[obj] = n
		if obj.WidthAttr != nil {
			desiredWidth, _ := parseD2IntegerAttribute(obj.ID, "width", obj.WidthAttr, 1, maxResultCoordinate)
			n.DesiredWidth = new(float64(desiredWidth))
		}
		if obj.HeightAttr != nil {
			desiredHeight, _ := parseD2IntegerAttribute(obj.ID, "height", obj.HeightAttr, 1, maxResultCoordinate)
			n.DesiredHeight = new(float64(desiredHeight))
		}

		if loadExistingLayout {
			if obj.TopLeft != nil {
				n.TopLeft = obj.TopLeft.Copy()
			}
		}

		if d2target.IsShape(obj.Shape.Value) {
			dslShape := strings.ToLower(obj.Shape.Value)
			if dslShape == d2target.ShapeHierarchy {
				n.ForceHierarchy = true
			}
			if shapeType, ok := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]; ok {
				n.SetShape(shapeType)
			}
			if obj.SQLTable != nil {
				n.SetNumColumns(len(obj.SQLTable.Columns))
			}
		}
		if obj.Icon != nil {
			n.InitIcon()
			if obj.IconPosition != nil {
				n.Icon.Position = label.FromString(*obj.IconPosition)
			}
		}
		if obj.Direction.Value != "" {
			talaGraph.Directions[n] = parseDirection(obj.Direction.Value)
		}

		n.FontSize = new(obj.Text().FontSize)
		if obj.Class != nil || obj.SQLTable != nil {
			*n.FontSize -= d2target.HeaderFontAdd
		}

		if obj.HasLabel() {
			n.Label = &layoutgraph.Label{
				Text:   obj.Label.Value,
				Width:  float64(obj.LabelDimensions.Width),
				Height: float64(obj.LabelDimensions.Height),
			}
			if obj.LabelPosition != nil {
				n.Label.Position = label.FromString(*obj.LabelPosition)
			}
		}

		if obj.Is3D() {
			n.Is3D = true
		}
		if obj.IsMultiple() {
			n.IsMultiple = true
		}

		if obj.Style.Opacity != nil {
			f, err := strconv.ParseFloat(obj.Style.Opacity.Value, 64)
			if err == nil && f == 0 {
				n.IsInvisible = true
			}
		}
		if !n.IsInvisible {
			noFill := false
			noStroke := false
			noLabel := false
			noIcon := false

			if obj.Style.Fill != nil && strings.EqualFold(obj.Style.Fill.Value, "transparent") {
				noFill = true
			}
			if obj.Style.StrokeWidth != nil {
				f, err := strconv.Atoi(obj.Style.StrokeWidth.Value)
				if err == nil && f == 0 {
					noStroke = true
				}
			}
			if obj.Style.Stroke != nil && strings.EqualFold(obj.Style.Stroke.Value, "transparent") {
				noStroke = true
			}
			if obj.Label.Value == "" {
				noLabel = true
			}
			if obj.Icon == nil {
				noIcon = true
			}

			if noFill && noStroke && noLabel && noIcon {
				n.IsInvisible = true
			}
		}

		n.D2ID = new(obj.AbsID())

		talaGraph.AddNewNodeToContainer(toNode[parent], n)
	}

	for _, visit := range visits {
		if err := ctx.Err(); err != nil {
			return bindings, err
		}
		obj := visit.object
		if obj.NearKey != nil {
			if len(obj.NearKey.Path) > maxInputTreeDepth {
				return bindings, fmt.Errorf("D2 object %q near key depth exceeds limit %d", obj.AbsID(), maxInputTreeDepth)
			}
			for partIndex, part := range obj.NearKey.Path {
				if part == nil || part.Unbox() == nil {
					return bindings, fmt.Errorf("D2 object %q has an invalid near key part at index %d", obj.AbsID(), partIndex)
				}
			}
			key := d2graph.Key(obj.NearKey)
			if len(key) == 0 {
				return bindings, fmt.Errorf("D2 object %q has an empty near key", obj.AbsID())
			}
			near, found, err := findD2Object(ctx, g.Root, key)
			if err != nil {
				return bindings, err
			}
			if found {
				nearNode, translated := toNode[near]
				if !translated {
					return bindings, fmt.Errorf("D2 object %q has a near key outside the object tree", obj.AbsID())
				}
				toNode[obj].AddNear(nearNode)
				continue
			}
			if len(key) != 1 {
				return bindings, fmt.Errorf("D2 object %q has a near key outside the object tree", obj.AbsID())
			}
			if _, isConst := d2ast.NearConstants[key[0]]; !isConst {
				return bindings, fmt.Errorf("D2 object %q has a near key outside the object tree", obj.AbsID())
			}
		}
	}

	for _, e := range g.Edges {
		if err := ctx.Err(); err != nil {
			return bindings, err
		}
		if e == nil || e.Src == nil || e.Dst == nil {
			return bindings, fmt.Errorf("tala cannot translate an edge with a nil endpoint")
		}

		// if connection is a <- b create it as src:b dst:a
		var talaEdge *layoutgraph.Edge
		edgeID := edgeAbsIDs[e]
		edgeSrc := e.Src
		edgeDst := e.Dst
		srcColumnIndex := e.SrcTableColumnIndex
		dstColumnIndex := e.DstTableColumnIndex
		if e.SrcArrow && !e.DstArrow {
			edgeSrc, edgeDst = edgeDst, edgeSrc
			talaEdge = talaGraph.Connect(toNode[edgeSrc], toNode[edgeDst])
			talaEdge.SourceArrowhead = layoutgraph.Arrowhead(d2target.NoArrowhead)
			if ah := e.DstArrowhead; ah != nil {
				if ah.Label.Value != "" {
					talaEdge.SourceArrowheadLabel = &layoutgraph.Label{
						Text:   ah.Label.Value,
						Width:  float64(ah.LabelDimensions.Width),
						Height: float64(ah.LabelDimensions.Height),
					}
				}
			}
			if ah := e.SrcArrowhead; ah != nil {
				var filled *bool
				if ah.Style.Filled != nil {
					v, _ := strconv.ParseBool(ah.Style.Filled.Value)
					filled = new(v)
				}
				talaEdge.TargetArrowhead = layoutgraph.Arrowhead(d2target.ToArrowhead(ah.Shape.Value, filled))
				if ah.Label.Value != "" {
					talaEdge.TargetArrowheadLabel = &layoutgraph.Label{
						Text:   ah.Label.Value,
						Width:  float64(ah.LabelDimensions.Width),
						Height: float64(ah.LabelDimensions.Height),
					}
				}
			} else {
				talaEdge.TargetArrowhead = layoutgraph.Arrowhead(d2target.TriangleArrowhead)
			}
			srcColumnIndex, dstColumnIndex = dstColumnIndex, srcColumnIndex

			if loadExistingLayout {
				for _, v := range slices.Backward(e.Route) {
					talaEdge.Points = append(talaEdge.Points, v.Copy())
				}
			}
		} else {
			// Pretend it's a self-loop, since sequence diagram lifelines aren't connected on both ends
			if isSupportedLifelineEdge(e) {
				edgeDst = edgeSrc
				bindings.edgeDestinations[e] = edgeDst
			}
			talaEdge = talaGraph.Connect(toNode[edgeSrc], toNode[edgeDst])
			if ah := e.SrcArrowhead; ah != nil {
				var filled *bool
				if ah.Style.Filled != nil {
					v, _ := strconv.ParseBool(ah.Style.Filled.Value)
					filled = new(v)
				}
				talaEdge.SourceArrowhead = layoutgraph.Arrowhead(d2target.ToArrowhead(ah.Shape.Value, filled))
				if ah.Label.Value != "" {
					talaEdge.SourceArrowheadLabel = &layoutgraph.Label{
						Text:   ah.Label.Value,
						Width:  float64(ah.LabelDimensions.Width),
						Height: float64(ah.LabelDimensions.Height),
					}
				}
			} else {
				if e.SrcArrow {
					talaEdge.SourceArrowhead = layoutgraph.Arrowhead(d2target.TriangleArrowhead)
				} else {
					talaEdge.SourceArrowhead = layoutgraph.Arrowhead(d2target.NoArrowhead)
				}
			}
			if ah := e.DstArrowhead; ah != nil {
				var filled *bool
				if ah.Style.Filled != nil {
					v, _ := strconv.ParseBool(ah.Style.Filled.Value)
					filled = new(v)
				}
				talaEdge.TargetArrowhead = layoutgraph.Arrowhead(d2target.ToArrowhead(ah.Shape.Value, filled))
				if ah.Label.Value != "" {
					talaEdge.TargetArrowheadLabel = &layoutgraph.Label{
						Text:   ah.Label.Value,
						Width:  float64(ah.LabelDimensions.Width),
						Height: float64(ah.LabelDimensions.Height),
					}
				}
			} else {
				if e.DstArrow {
					talaEdge.TargetArrowhead = layoutgraph.Arrowhead(d2target.TriangleArrowhead)
				} else {
					talaEdge.TargetArrowhead = layoutgraph.Arrowhead(d2target.NoArrowhead)
				}
			}

			if loadExistingLayout {
				for _, p := range e.Route {
					talaEdge.Points = append(talaEdge.Points, p.Copy())
				}
			}
		}

		talaEdge.ID = edgeIDs[e]
		bindings.edgeIDs[e] = talaEdge.ID
		talaEdge.D2ID = new(edgeID)
		talaEdge.Style = cloneStyle(e.Style)
		minWidth := int64(e.LabelDimensions.Width + 2*label.PADDING)
		minHeight := int64(e.LabelDimensions.Height + 2*label.PADDING)
		// TODO update minWidth/minHeight logic
		if l := talaEdge.SourceArrowheadLabel; l != nil {
			max := math.Max(l.Width, l.Height) + 5
			minWidth += int64(max)
			minHeight += int64(max)
		}
		if l := talaEdge.TargetArrowheadLabel; l != nil {
			max := math.Max(l.Width, l.Height) + 5
			minWidth += int64(max)
			minHeight += int64(max)
		}
		if minWidth > maxResultCoordinate || minHeight > maxResultCoordinate {
			return bindings, fmt.Errorf("D2 edge %q minimum dimensions exceed supported limit %d", e.AbsID(), maxResultCoordinate)
		}
		talaEdge.MinWidth = int(minWidth)
		talaEdge.MinHeight = int(minHeight)

		if e.Label.Value != "" {
			talaEdge.Label = &layoutgraph.Label{
				Text:   e.Label.Value,
				Width:  float64(e.LabelDimensions.Width),
				Height: float64(e.LabelDimensions.Height),
			}
			if loadExistingLayout && e.LabelPosition != nil {
				talaEdge.Label.Position = label.FromString(*e.LabelPosition)
				if e.LabelPercentage != nil {
					talaEdge.LabelPercentage = *e.LabelPercentage
				}
				if e.SrcArrow && !e.DstArrow {
					// The route was reversed above. Preserve the label's
					// physical position so rerouting reserves its actual box.
					talaEdge.Label.Position = talaEdge.Label.Position.Mirrored()
					if e.LabelPercentage != nil || talaEdge.Label.Position.IsUnlocked() {
						talaEdge.LabelPercentage = 1 - talaEdge.LabelPercentage
					}
				}
			}
		}

		if edgeSrc.SQLTable != nil && srcColumnIndex != nil {
			talaEdge.FromTableColumnIndex = new(*srcColumnIndex)
		}
		if edgeDst.SQLTable != nil && dstColumnIndex != nil {
			talaEdge.ToTableColumnIndex = new(*dstColumnIndex)
		}
		if e.Style.Opacity != nil {
			f, err := strconv.ParseFloat(e.Style.Opacity.Value, 64)
			if err == nil && f == 0 {
				talaEdge.IsInvisible = true
				talaEdge.MinHeight = 0
				talaEdge.MinWidth = 0
			}
		}
		if e.Style.Stroke != nil && strings.EqualFold(e.Style.Stroke.Value, "transparent") {
			talaEdge.IsInvisible = true
		}
	}

	return bindings, nil
}

func cloneStyle(style d2graph.Style) layoutgraph.EdgeStyle {
	cloneScalar := func(scalar *d2graph.Scalar) *layoutgraph.StyleScalar {
		if scalar == nil {
			return nil
		}
		// MapKey points back into the mutable D2 AST. TALA only consumes the
		// scalar value, so do not retain that provenance in the isolated graph.
		return &layoutgraph.StyleScalar{Value: scalar.Value}
	}
	return layoutgraph.EdgeStyle{
		Opacity:       cloneScalar(style.Opacity),
		Stroke:        cloneScalar(style.Stroke),
		Fill:          cloneScalar(style.Fill),
		FillPattern:   cloneScalar(style.FillPattern),
		StrokeWidth:   cloneScalar(style.StrokeWidth),
		StrokeDash:    cloneScalar(style.StrokeDash),
		BorderRadius:  cloneScalar(style.BorderRadius),
		Shadow:        cloneScalar(style.Shadow),
		ThreeDee:      cloneScalar(style.ThreeDee),
		Multiple:      cloneScalar(style.Multiple),
		Font:          cloneScalar(style.Font),
		FontSize:      cloneScalar(style.FontSize),
		FontColor:     cloneScalar(style.FontColor),
		Animated:      cloneScalar(style.Animated),
		Bold:          cloneScalar(style.Bold),
		Italic:        cloneScalar(style.Italic),
		Underline:     cloneScalar(style.Underline),
		Filled:        cloneScalar(style.Filled),
		DoubleBorder:  cloneScalar(style.DoubleBorder),
		TextTransform: cloneScalar(style.TextTransform),
	}
}

func parseDirection(direction string) geo.Orientation {
	switch direction {
	case "up":
		return geo.Top
	case "down":
		return geo.Bottom
	case "left":
		return geo.Left
	case "right":
		return geo.Right
	default:
		return geo.NONE
	}
}

// RouteEdges reroutes edges in g. It mutates g and must not run concurrently
// against the same graph.
func RouteEdges(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) (err error) {
	defer recoverAsError("edge routing", &err)
	if ctx == nil {
		return fmt.Errorf("TALA edge routing requires a context")
	}
	if err := validateRequestedD2Edges(g, edges); err != nil {
		return err
	}

	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(ctx, g, talaGraph, true)
	if err != nil {
		return err
	}

	talaEdgeByID := make(map[layoutgraph.EntityID]*layoutgraph.Edge, len(talaGraph.Edges))
	for _, edge := range talaGraph.Edges {
		talaEdgeByID[edge.ID] = edge
	}
	talaEdges := make([]*layoutgraph.Edge, 0, len(edges))
	for _, e := range edges {
		if e == nil {
			return fmt.Errorf("cannot route a nil D2 edge")
		}
		talaEdgeID, bound := bindings.edgeIDs[e]
		talaEdge, found := talaEdgeByID[talaEdgeID]
		if !bound || !found {
			return fmt.Errorf("could not find edge %#v in graph", e.AbsID())
		}
		talaEdges = append(talaEdges, talaEdge)
	}

	workCtx := ctx
	cancelWork := func() {}
	if ctx.Done() == nil {
		workCtx, cancelWork = context.WithCancel(ctx)
	}
	defer cancelWork()
	err = routing.RouteEdges(workCtx, talaGraph, talaEdges)
	if err != nil {
		return preferContextError(ctx, "edge routing", err)
	}

	patch, err := buildRoutePatch(workCtx, g, bindings, talaEdges)
	if err != nil {
		return preferContextError(ctx, "edge routing", err)
	}
	return commitRoutePatch(workCtx, patch)
}

func recoverAsError(operation string, err *error) {
	if recover() != nil {
		*err = fmt.Errorf("TALA %s failed due to an internal invariant", operation)
	}
}
