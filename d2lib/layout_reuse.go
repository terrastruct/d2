package d2lib

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

// layoutInputSignature is an exact, length-delimited encoding of the graph
// fields consumed by D2's layout pipeline. It is captured after theme and text
// dimensions have been applied, but before layout mutates geometry.
//
// Deliberately omitted fields are render-only metadata (colors, opacity,
// tooltips, links, classes, and AST references). Text styling is represented by
// the resulting label dimensions. The two visual modifiers that affect routing
// bounds, 3d and multiple, are included explicitly.
type layoutInputSignature string

type layoutSignatureBuilder struct {
	bytes.Buffer
	scratch [8]byte
}

func newLayoutInputSignature(g *d2graph.Graph, engine string) (layoutInputSignature, bool, error) {
	if !supportsLayoutReuse(g, engine) {
		return "", false, nil
	}

	b := &layoutSignatureBuilder{}
	b.writeString(engine)
	b.writeInt(g.RootLevel)
	if err := b.writeObject(g.Root); err != nil {
		return "", false, err
	}
	b.writeInt(len(g.Objects))
	for _, obj := range g.Objects {
		if err := b.writeObject(obj); err != nil {
			return "", false, err
		}
	}
	b.writeInt(len(g.Edges))
	for _, edge := range g.Edges {
		if err := b.writeEdge(edge); err != nil {
			return "", false, err
		}
	}
	return layoutInputSignature(b.String()), true, nil
}

// Sequence, grid, and constant-near layouts can synthesize or temporarily
// restructure graph topology. Geometry-only replay is intentionally disabled
// for those graph types until their generated topology can be snapshotted too.
func supportsLayoutReuse(g *d2graph.Graph, engine string) bool {
	// The built-in Dagre and ELK adapters consume the geometry inputs encoded
	// below. Arbitrary layout plugins receive the whole graph and may choose to
	// make render-only styling affect geometry, so they remain opt-out by
	// default until the layout API exposes an explicit reuse capability.
	if engine != "dagre" && engine != "elk" {
		return false
	}
	objects := make([]*d2graph.Object, 0, len(g.Objects)+1)
	objects = append(objects, g.Root)
	objects = append(objects, g.Objects...)
	for _, obj := range objects {
		if obj.IsSequenceDiagram() || obj.IsGridDiagram() || obj.IsConstantNear() {
			return false
		}
	}
	return true
}

func (b *layoutSignatureBuilder) writeObject(obj *d2graph.Object) error {
	b.writeString(obj.AbsID())
	if obj.Parent == nil {
		b.writeOptionalString(nil)
	} else {
		parentID := obj.Parent.AbsID()
		b.writeOptionalString(&parentID)
	}
	b.writeInt(len(obj.ChildrenArray))
	for _, child := range obj.ChildrenArray {
		b.writeString(child.AbsID())
	}
	b.writeInt(len(obj.Children))
	b.writeBool(obj.IsContainer())
	b.writeBox(obj.Box)
	b.writeOptionalString(obj.LabelPosition)
	b.writeOptionalString(obj.IconPosition)
	b.writeOptionalFloat(obj.ContentAspectRatio)
	b.writeBool(obj.HasLabel())
	b.writeBool(obj.HasIcon())
	b.writeBool(obj.Icon != nil)
	b.writeBool(obj.Is3D())
	b.writeBool(obj.IsMultiple())

	if err := b.writeJSON(obj.Class); err != nil {
		return err
	}
	if err := b.writeJSON(obj.SQLTable); err != nil {
		return err
	}
	b.writeAttributes(&obj.Attributes)
	return nil
}

func (b *layoutSignatureBuilder) writeEdge(edge *d2graph.Edge) error {
	b.writeString(edge.AbsID())
	b.writeString(edge.Src.AbsID())
	b.writeString(edge.Dst.AbsID())
	b.writeInt(edge.Index)
	b.writeOptionalInt(edge.SrcTableColumnIndex)
	b.writeOptionalInt(edge.DstTableColumnIndex)
	b.writeBool(edge.SrcArrow)
	b.writeBool(edge.DstArrow)
	b.writeBool(edge.IsCurve)
	b.writePoints(edge.Route)
	b.writeOptionalString(edge.LabelPosition)
	b.writeOptionalFloat(edge.LabelPercentage)
	b.writeAttributes(&edge.Attributes)

	b.writeBool(edge.SrcArrowhead != nil)
	if edge.SrcArrowhead != nil {
		b.writeAttributes(edge.SrcArrowhead)
	}
	b.writeBool(edge.DstArrowhead != nil)
	if edge.DstArrowhead != nil {
		b.writeAttributes(edge.DstArrowhead)
	}
	return nil
}

func (b *layoutSignatureBuilder) writeAttributes(attrs *d2graph.Attributes) {
	b.writeBool(attrs.Label.Value != "")
	b.writeInt(attrs.LabelDimensions.Width)
	b.writeInt(attrs.LabelDimensions.Height)
	b.writeString(attrs.Shape.Value)
	b.writeString(attrs.Direction.Value)
	b.writeStrings(attrs.Constraint)
	b.writeString(attrs.Language)
	b.writeOptionalScalar(attrs.WidthAttr)
	b.writeOptionalScalar(attrs.HeightAttr)
	b.writeOptionalScalar(attrs.Top)
	b.writeOptionalScalar(attrs.Left)
	b.writeOptionalScalar(attrs.GridRows)
	b.writeOptionalScalar(attrs.GridColumns)
	b.writeOptionalScalar(attrs.GridGap)
	b.writeOptionalScalar(attrs.VerticalGap)
	b.writeOptionalScalar(attrs.HorizontalGap)
	b.writeOptionalScalar(attrs.LabelPosition)
	b.writeOptionalScalar(attrs.IconPosition)

	if attrs.NearKey == nil {
		b.writeBool(false)
	} else {
		b.writeBool(true)
		b.writeStrings(d2graph.Key(attrs.NearKey))
	}
}

func (b *layoutSignatureBuilder) writeJSON(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b.writeBytes(encoded)
	return nil
}

func (b *layoutSignatureBuilder) writeBytes(value []byte) {
	b.writeUint64(uint64(len(value)))
	_, _ = b.Write(value)
}

func (b *layoutSignatureBuilder) writeString(value string) {
	b.writeBytes([]byte(value))
}

func (b *layoutSignatureBuilder) writeStrings(values []string) {
	b.writeInt(len(values))
	for _, value := range values {
		b.writeString(value)
	}
}

func (b *layoutSignatureBuilder) writeBool(value bool) {
	if value {
		_ = b.WriteByte(1)
	} else {
		_ = b.WriteByte(0)
	}
}

func (b *layoutSignatureBuilder) writeInt(value int) {
	b.writeUint64(uint64(value))
}

func (b *layoutSignatureBuilder) writeUint64(value uint64) {
	binary.LittleEndian.PutUint64(b.scratch[:], value)
	_, _ = b.Write(b.scratch[:])
}

func (b *layoutSignatureBuilder) writeFloat(value float64) {
	b.writeUint64(math.Float64bits(value))
}

func (b *layoutSignatureBuilder) writeOptionalFloat(value *float64) {
	b.writeBool(value != nil)
	if value != nil {
		b.writeFloat(*value)
	}
}

func (b *layoutSignatureBuilder) writeOptionalInt(value *int) {
	b.writeBool(value != nil)
	if value != nil {
		b.writeInt(*value)
	}
}

func (b *layoutSignatureBuilder) writeOptionalString(value *string) {
	b.writeBool(value != nil)
	if value != nil {
		b.writeString(*value)
	}
}

func (b *layoutSignatureBuilder) writeOptionalScalar(value *d2graph.Scalar) {
	b.writeBool(value != nil)
	if value != nil {
		b.writeString(value.Value)
	}
}

func (b *layoutSignatureBuilder) writeBox(box *geo.Box) {
	b.writeBool(box != nil)
	if box == nil {
		return
	}
	b.writePoint(box.TopLeft)
	b.writeFloat(box.Width)
	b.writeFloat(box.Height)
}

func (b *layoutSignatureBuilder) writePoint(point *geo.Point) {
	b.writeBool(point != nil)
	if point != nil {
		b.writeFloat(point.X)
		b.writeFloat(point.Y)
	}
}

func (b *layoutSignatureBuilder) writePoints(points []*geo.Point) {
	b.writeBool(points != nil)
	if points == nil {
		return
	}
	b.writeInt(len(points))
	for _, point := range points {
		b.writePoint(point)
	}
}

type layoutGeometry struct {
	signature layoutInputSignature
	root      objectGeometry
	objects   map[string]objectGeometry
	edges     map[string]edgeGeometry
}

type objectGeometry struct {
	box           *geo.Box
	labelPosition *string
	iconPosition  *string
}

type edgeGeometry struct {
	route           []*geo.Point
	labelPosition   *string
	labelPercentage *float64
	isCurve         bool
}

func captureLayoutGeometry(g *d2graph.Graph, signature layoutInputSignature) *layoutGeometry {
	geometry := &layoutGeometry{
		signature: signature,
		root:      captureObjectGeometry(g.Root),
		objects:   make(map[string]objectGeometry, len(g.Objects)),
		edges:     make(map[string]edgeGeometry, len(g.Edges)),
	}
	for _, obj := range g.Objects {
		geometry.objects[obj.AbsID()] = captureObjectGeometry(obj)
	}
	for _, edge := range g.Edges {
		geometry.edges[edge.AbsID()] = captureEdgeGeometry(edge)
	}
	return geometry
}

func captureObjectGeometry(obj *d2graph.Object) objectGeometry {
	return objectGeometry{
		box:           copyBox(obj.Box),
		labelPosition: copyString(obj.LabelPosition),
		iconPosition:  copyString(obj.IconPosition),
	}
}

func captureEdgeGeometry(edge *d2graph.Edge) edgeGeometry {
	return edgeGeometry{
		route:           copyPoints(edge.Route),
		labelPosition:   copyString(edge.LabelPosition),
		labelPercentage: copyFloat(edge.LabelPercentage),
		isCurve:         edge.IsCurve,
	}
}

// apply replays only geometry and layout-selected label/icon placement. Every
// structural key must be present; otherwise the caller falls back to layout.
func (geometry *layoutGeometry) apply(g *d2graph.Graph, signature layoutInputSignature) bool {
	if geometry == nil || geometry.signature != signature ||
		len(geometry.objects) != len(g.Objects) || len(geometry.edges) != len(g.Edges) {
		return false
	}
	for _, obj := range g.Objects {
		if _, ok := geometry.objects[obj.AbsID()]; !ok {
			return false
		}
	}
	for _, edge := range g.Edges {
		if _, ok := geometry.edges[edge.AbsID()]; !ok {
			return false
		}
	}

	applyObjectGeometry(g.Root, geometry.root)
	for _, obj := range g.Objects {
		applyObjectGeometry(obj, geometry.objects[obj.AbsID()])
	}
	for _, edge := range g.Edges {
		applyEdgeGeometry(edge, geometry.edges[edge.AbsID()])
	}
	return true
}

func applyObjectGeometry(obj *d2graph.Object, geometry objectGeometry) {
	obj.Box = copyBox(geometry.box)
	obj.LabelPosition = copyString(geometry.labelPosition)
	obj.IconPosition = copyString(geometry.iconPosition)
}

func applyEdgeGeometry(edge *d2graph.Edge, geometry edgeGeometry) {
	edge.Route = copyPoints(geometry.route)
	edge.LabelPosition = copyString(geometry.labelPosition)
	edge.LabelPercentage = copyFloat(geometry.labelPercentage)
	edge.IsCurve = geometry.isCurve
}

func copyBox(box *geo.Box) *geo.Box {
	if box == nil {
		return nil
	}
	return geo.NewBox(copyPoint(box.TopLeft), box.Width, box.Height)
}

func copyPoints(points []*geo.Point) []*geo.Point {
	if points == nil {
		return nil
	}
	result := make([]*geo.Point, len(points))
	for i, point := range points {
		result[i] = copyPoint(point)
	}
	return result
}

func copyPoint(point *geo.Point) *geo.Point {
	if point == nil {
		return nil
	}
	return geo.NewPoint(point.X, point.Y)
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
