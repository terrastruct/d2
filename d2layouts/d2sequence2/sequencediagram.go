// Package sequencediagram implements the opt-in sequence-diagram layout.
// It deliberately keeps participant placement, the source-ordered timeline,
// activation extents, and frame geometry as separate passes.
package sequencediagram

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2format"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const (
	padding              = 24.
	framePadding         = 16.
	defaultHorizontalGap = 60.
	defaultVerticalGap   = 48.
	spanWidth            = 12.
	spanPadding          = 12.
)

type item struct {
	object       *d2graph.Object
	edge         *d2graph.Edge
	pos          sourcePosition
	above, below float64
	loopHeight   float64
	y            float64
}

type sourcePosition struct{ line, column, edge, sourceLine, sourceColumn int }

func (a sourcePosition) less(b sourcePosition) bool {
	if a.line != b.line {
		return a.line < b.line
	}
	if a.column != b.column {
		return a.column < b.column
	}
	if a.sourceLine != b.sourceLine {
		return a.sourceLine < b.sourceLine
	}
	if a.sourceColumn != b.sourceColumn {
		return a.sourceColumn < b.sourceColumn
	}
	return a.edge < b.edge
}
func objectPosition(o *d2graph.Object) sourcePosition {
	p := sourcePosition{line: math.MaxInt, column: math.MaxInt, edge: math.MaxInt}
	for _, r := range o.References {
		if r.MapKey == nil || (r.Key != nil && r.Key.HasGlob()) {
			continue
		}
		position := r.MapKey.Range.Start
		// Endpoint references share their enclosing message's map key. Use the
		// referenced path component so client -> server introduces client first,
		// even when group normalization changed the children encounter order.
		if r.Key != nil && r.KeyPathIndex >= 0 && r.KeyPathIndex < len(r.Key.Path) {
			position = r.Key.Path[r.KeyPathIndex].Unbox().GetRange().Start
		}
		q := sourcePosition{line: position.Line, column: position.Column, edge: r.MapKeyEdgeIndex}
		if q.less(p) {
			p = q
		}
	}
	if o.SequenceImportPosition != nil {
		p.sourceLine, p.sourceColumn = p.line, p.column
		p.line, p.column = o.SequenceImportPosition.Line, o.SequenceImportPosition.Column
	}
	return p
}
func edgePosition(e *d2graph.Edge) sourcePosition {
	p := sourcePosition{line: math.MaxInt, column: math.MaxInt, edge: math.MaxInt}
	for _, r := range e.References {
		if r.MapKey == nil || (r.Edge != nil && (r.Edge.Src.HasGlob() || r.Edge.Dst.HasGlob())) {
			continue
		}
		q := sourcePosition{line: r.MapKey.Range.Start.Line, column: r.MapKey.Range.Start.Column, edge: r.MapKeyEdgeIndex}
		if q.less(p) {
			p = q
		}
	}
	if e.SequenceImportPosition != nil {
		p.sourceLine, p.sourceColumn = p.line, p.column
		p.line, p.column = e.SequenceImportPosition.Line, e.SequenceImportPosition.Column
	}
	return p
}

type diagram struct {
	graph                                      *d2graph.Graph
	actors, spans, events, groups, actorGroups []*d2graph.Object
	owner                                      map[*d2graph.Object]*d2graph.Object
	rank                                       map[*d2graph.Object]int
	depth                                      map[*d2graph.Object]int
	timeline                                   []*item
	emptySpans                                 map[*d2graph.Object]float64
	frameTop, frameBottom                      map[*d2graph.Object]float64
	hgap, vgap                                 float64
}

// Layout places a single sequence diagram after graph text dimensions have been
// measured. The caller is responsible for extracting nested sequence diagrams.
func Layout(ctx context.Context, g *d2graph.Graph) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d := &diagram{graph: g, owner: make(map[*d2graph.Object]*d2graph.Object), rank: make(map[*d2graph.Object]int), depth: make(map[*d2graph.Object]int), emptySpans: make(map[*d2graph.Object]float64), frameTop: make(map[*d2graph.Object]float64), frameBottom: make(map[*d2graph.Object]float64)}
	d.hgap = gap(g.Root.HorizontalGap, gap(g.Root.GridGap, defaultHorizontalGap))
	d.vgap = gap(g.Root.VerticalGap, gap(g.Root.GridGap, defaultVerticalGap))
	if err := d.collect(g.Root, nil, 0); err != nil {
		return err
	}
	for i, a := range d.actors {
		d.rank[a] = i
	}
	if len(d.actors) == 0 {
		return fmt.Errorf("no actors declared in sequence diagram")
	}
	if err := d.buildTimeline(); err != nil {
		return err
	}
	d.placeActors()
	if err := ctx.Err(); err != nil {
		return err
	}
	d.placeTimeline()
	d.placeSpans()
	d.routeEndpoints()
	d.placeMessageGroups()
	d.finishActors()
	d.placeActorGroups()
	d.fitRoot()
	return nil
}

func gap(s *d2graph.Scalar, fallback float64) float64 {
	if s == nil {
		return fallback
	}
	n, err := strconv.ParseFloat(s.Value, 64)
	if err != nil || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) {
		return fallback
	}
	return n
}
func setPosition(p **string, v label.Position) {
	if *p == nil {
		s := v.String()
		*p = &s
	}
}

func (d *diagram) collect(parent, actor *d2graph.Object, depth int) error {
	children := append([]*d2graph.Object(nil), parent.ChildrenArray...)
	sort.SliceStable(children, func(i, j int) bool { return objectPosition(children[i]).less(objectPosition(children[j])) })
	for _, o := range children {
		if o.Box == nil {
			o.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 60)
		}
		if o.TopLeft == nil {
			o.TopLeft = geo.NewPoint(0, 0)
		}
		if o.IsSequenceDiagram() {
			return fmt.Errorf("actors in sequence diagrams cannot themselves be sequence diagrams: %s", o.AbsID())
		}
		switch o.Shape.Value {
		case d2target.ShapeSequenceDiagramActorGroup:
			d.actorGroups = append(d.actorGroups, o)
			setPosition(&o.LabelPosition, label.InsideTopCenter)
			if err := d.collect(o, nil, 0); err != nil {
				return err
			}
		case d2target.ShapeSequenceDiagramEdgeGroup:
			d.groups = append(d.groups, o)
			setPosition(&o.LabelPosition, label.InsideTopLeft)
			if err := d.collect(o, actor, depth); err != nil {
				return err
			}
		default:
			if actor == nil {
				d.actors = append(d.actors, o)
				d.owner[o] = o
				if o.WidthAttr == nil && o.Width < 100 {
					switch o.Shape.Value {
					case d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapePerson, d2target.ShapeSquare:
						if o.Width > 0 {
							o.Height *= 100 / o.Width
						}
					}
					o.Width = 100
				}
				d.actorAppearance(o)
				if err := d.collect(o, o, 0); err != nil {
					return err
				}
			} else {
				d.owner[o] = actor
				if o.Shape.Value == "" {
					d.spans = append(d.spans, o)
					d.depth[o] = depth
					if o.HasLabel() {
						setPosition(&o.LabelPosition, label.InsideTopLeft)
					}
					if err := d.collect(o, actor, depth+1); err != nil {
						return err
					}
				} else {
					if o.Shape.Value == d2target.ShapeSequenceDiagramActor {
						// A repeat has its own identity and source position, but the actor's
						// appearance. Its children and references still belong to this object.
						importPosition := o.SequenceImportPosition
						o.Attributes = actor.Attributes
						o.SequenceImportPosition = importPosition
						o.Shape.Value = d2target.ShapeSequenceDiagramActor
						o.Box = geo.NewBox(geo.NewPoint(0, 0), actor.Width, actor.Height)
						o.LabelPosition = actor.LabelPosition
						o.IconPosition = actor.IconPosition
						o.Class = actor.Class
						o.SQLTable = actor.SQLTable
						o.ContentAspectRatio = actor.ContentAspectRatio
					} else {
						setPosition(&o.LabelPosition, label.InsideMiddleCenter)
					}
					d.events = append(d.events, o)
					if err := d.collect(o, actor, depth); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (d *diagram) actorAppearance(o *d2graph.Object) {
	if o.Icon != nil && o.Shape.Value != d2target.ShapeImage {
		setPosition(&o.LabelPosition, label.OutsideTopCenter)
		setPosition(&o.IconPosition, label.InsideMiddleCenter)
	} else if o.HasOutsideBottomLabel() {
		setPosition(&o.LabelPosition, label.OutsideBottomCenter)
	} else {
		setPosition(&o.LabelPosition, label.InsideMiddleCenter)
	}
}

func (d *diagram) buildTimeline() error {
	used := make(map[*d2graph.Object]bool)
	for _, e := range d.graph.Edges {
		if d2sequence.IsLifelineEnd(e.Dst) {
			continue
		}
		if d.owner[e.Src] == nil || d.owner[e.Dst] == nil {
			return fmt.Errorf("sequence message %s must connect actors or spans", e.AbsID())
		}
		for _, o := range []*d2graph.Object{e.Src, e.Dst} {
			if d.owner[o] != o {
				if _, ok := d.depth[o]; !ok {
					return fmt.Errorf("sequence messages cannot connect notes or events: %s", o.AbsID())
				}
			}
			for p := o; p != nil && p != d.owner[o]; p = p.Parent {
				used[p] = true
			}
		}
		h := float64(e.LabelDimensions.Height) / 2
		it := &item{edge: e, pos: edgePosition(e), above: h, below: h}
		if d.owner[e.Src] == d.owner[e.Dst] {
			it.loopHeight = math.Max(32, float64(e.LabelDimensions.Height)+12)
			it.below = it.loopHeight + h
		}
		d.timeline = append(d.timeline, it)
	}
	for _, o := range d.events {
		margin, _ := o.Spacing()
		d.timeline = append(d.timeline, &item{object: o, pos: objectPosition(o), above: o.Height/2 + margin.Top, below: o.Height/2 + margin.Bottom})
		for p := o.Parent; p != nil && p != d.owner[o]; p = p.Parent {
			used[p] = true
		}
	}
	for _, s := range d.spans {
		if !used[s] {
			d.timeline = append(d.timeline, &item{object: s, pos: objectPosition(s), above: spanPadding, below: spanPadding})
		}
	}
	// Empty message frames are timeline events too; they reserve space at
	// their declaration rather than floating over a neighboring note or message.
	for _, g := range d.groups {
		hasContent := false
		for _, it := range d.timeline {
			if d.inGroup(it, g) {
				hasContent = true
				break
			}
		}
		if !hasContent {
			d.timeline = append(d.timeline, &item{object: g, pos: objectPosition(g), below: math.Max(40, float64(g.LabelDimensions.Height)+framePadding*2)})
		}
	}
	// Stable sorting uses the graph's deterministic encounter order for generated
	// or imported references that have no distinct source position.
	sort.SliceStable(d.timeline, func(i, j int) bool { return d.timeline[i].pos.less(d.timeline[j].pos) })
	d.reserveSpanExtents()
	return nil
}

// Account for activation borders and labels before scheduling. Growing a
// nested activation after routing must not move it into a participant header.
func (d *diagram) reserveSpanExtents() {
	first, last := make(map[*d2graph.Object]int), make(map[*d2graph.Object]int)
	for _, s := range d.spans {
		for i, it := range d.timeline {
			inside := it.object != nil && it.object.IsDescendantOf(s)
			if it.edge != nil {
				inside = it.edge.Src.IsDescendantOf(s) || it.edge.Dst.IsDescendantOf(s)
			}
			if inside {
				if _, ok := first[s]; !ok {
					first[s] = i
				}
				last[s] = i
			}
		}
	}
	above, below := make([]float64, len(d.timeline)), make([]float64, len(d.timeline))
	for _, s := range d.spans {
		start, ok := first[s]
		if !ok {
			continue
		}
		end := last[s]
		top, bottom := 0., 0.
		for p := s; p != nil && p != d.owner[s]; p = p.Parent {
			if _, span := d.depth[p]; !span {
				continue
			}
			t, b := spanPadding, spanPadding
			if p.HasLabel() && p.LabelPosition != nil {
				switch label.FromString(*p.LabelPosition) {
				case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight, label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
					t += float64(p.LabelDimensions.Height) + label.PADDING*2
				case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight, label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
					b += float64(p.LabelDimensions.Height) + label.PADDING*2
				}
			}
			if first[p] == start {
				top += t
			}
			if last[p] == end {
				bottom += b
			}
		}
		above[start] = math.Max(above[start], top)
		below[end] = math.Max(below[end], bottom)
	}
	for i, it := range d.timeline {
		it.above += above[i]
		it.below += below[i]
	}
}

func (d *diagram) actorGroupDepth(a *d2graph.Object) float64 {
	v := 0.
	for p := a.Parent; p != nil && p != d.graph.Root; p = p.Parent {
		if p.Shape.Value == d2target.ShapeSequenceDiagramActorGroup {
			v += framePadding + float64(p.LabelDimensions.Height) + label.PADDING*2
		}
	}
	return v
}

func (d *diagram) placeActors() {
	n := len(d.actors)
	left, right := make([]float64, n), make([]float64, n)
	headerHeight := 0.
	for i, a := range d.actors {
		margin, _ := a.Spacing()
		left[i] = a.Width/2 + margin.Left
		right[i] = a.Width/2 + margin.Right
		if a.LabelPosition != nil && (label.FromString(*a.LabelPosition) == label.OutsideTopCenter || label.FromString(*a.LabelPosition) == label.OutsideBottomCenter) {
			left[i] = math.Max(left[i], float64(a.LabelDimensions.Width)/2)
			right[i] = math.Max(right[i], float64(a.LabelDimensions.Width)/2)
		}
		headerHeight = math.Max(headerHeight, a.Height+margin.Top+margin.Bottom+d.actorGroupDepth(a))
	}
	for _, o := range d.events {
		i := d.rank[d.owner[o]]
		m, _ := o.Spacing()
		left[i] = math.Max(left[i], o.Width/2+m.Left)
		right[i] = math.Max(right[i], o.Width/2+m.Right)
	}
	for _, s := range d.spans {
		i := d.rank[d.owner[s]]
		w := spanWidth + float64(d.depth[s])*spanWidth/2
		left[i] = math.Max(left[i], w)
		right[i] = math.Max(right[i], w)
		if s.HasLabel() {
			left[i] = math.Max(left[i], float64(s.LabelDimensions.Width)+w)
			right[i] = math.Max(right[i], float64(s.LabelDimensions.Width)+w)
		}
	}
	for _, it := range d.timeline {
		if it.edge == nil {
			continue
		}
		e := it.edge
		if d.owner[e.Src] == d.owner[e.Dst] {
			i := d.rank[d.owner[e.Src]]
			offset := 0.
			for _, o := range []*d2graph.Object{e.Src, e.Dst} {
				if depth, ok := d.depth[o]; ok {
					offset = math.Max(offset, float64(depth)*spanWidth/2+activationWidth(o)/2)
				}
			}
			right[i] = math.Max(right[i], offset+selfTravel(e)+float64(e.LabelDimensions.Width)/2+spanWidth)
		}
	}
	// Reserve the sides of all enclosing participant frames. This also
	// handles a single participant whose group's label is wider than its lane.
	for i, a := range d.actors {
		for p := a.Parent; p != nil && p != d.graph.Root; p = p.Parent {
			if !containsObject(d.actorGroups, p) {
				continue
			}
			left[i] += framePadding
			right[i] += framePadding
			left[i] = math.Max(left[i], float64(p.LabelDimensions.Width)/2+framePadding)
			right[i] = math.Max(right[i], float64(p.LabelDimensions.Width)/2+framePadding)
		}
	}
	steps := make([]float64, n-1)
	for i := range steps {
		steps[i] = right[i] + left[i+1] + d.hgap
	}
	for _, it := range d.timeline {
		if it.edge == nil {
			continue
		}
		e := it.edge
		lo, hi := d.rank[d.owner[e.Src]], d.rank[d.owner[e.Dst]]
		if lo > hi {
			lo, hi = hi, lo
		}
		if lo == hi {
			continue
		}
		want := float64(e.LabelDimensions.Width) + padding*2
		have := 0.
		for i := lo; i < hi; i++ {
			have += steps[i]
		}
		if want > have {
			for i := lo; i < hi; i++ {
				steps[i] += (want - have) / float64(hi-lo)
			}
		}
	}
	x := left[0]
	titleHeight := 0.
	if d.graph.Root.HasLabel() {
		titleHeight = float64(d.graph.Root.LabelDimensions.Height) + padding
	}
	for i, a := range d.actors {
		margin, _ := a.Spacing()
		a.TopLeft = geo.NewPoint(x-a.Width/2, titleHeight+headerHeight-a.Height-margin.Bottom)
		a.ZIndex = 3
		if i < len(steps) {
			x += steps[i]
		}
	}
}

func selfTravel(e *d2graph.Edge) float64 {
	return math.Max(60, float64(e.LabelDimensions.Width)/2+padding)
}
func (d *diagram) inGroup(it *item, g *d2graph.Object) bool {
	if it.edge != nil {
		return it.edge.ContainedBy(g)
	}
	return it.object != g && (it.object.IsDescendantOf(g) || it.object.ContainedBy(g))
}

func (d *diagram) placeTimeline() {
	first, last := make(map[*d2graph.Object]int), make(map[*d2graph.Object]int)
	for _, g := range d.groups {
		first[g] = -1
		for i, it := range d.timeline {
			if d.inGroup(it, g) {
				if first[g] < 0 {
					first[g] = i
				}
				last[g] = i
			}
		}
	}
	headerBottom := 0.
	for _, a := range d.actors {
		b := objectBounds(a)
		headerBottom = math.Max(headerBottom, b.maxY)
	}
	end, bottom := headerBottom, headerBottom
	nextGap := d.vgap
	explicit := false
	for i, it := range d.timeline {
		y := end + nextGap
		if !explicit {
			y = math.Max(y, bottom+d.vgap+it.above)
		}
		opening := append([]*d2graph.Object(nil), d.groups...)
		sort.SliceStable(opening, func(i, j int) bool { return opening[i].Level() < opening[j].Level() })
		for _, g := range opening {
			if first[g] != i {
				continue
			}
			top := y - it.above
			d.frameTop[g] = top
			y += framePadding + float64(g.LabelDimensions.Height) + label.PADDING*2
		}
		it.y = y
		if it.edge != nil {
			e := it.edge
			sx, dx := d.owner[e.Src].Center().X, d.owner[e.Dst].Center().X
			if d.owner[e.Src] == d.owner[e.Dst] {
				by := y + it.loopHeight
				e.Route = []*geo.Point{geo.NewPoint(sx, y), geo.NewPoint(sx+selfTravel(e), y), geo.NewPoint(sx+selfTravel(e), by), geo.NewPoint(dx, by)}
				end = by
			} else {
				e.Route = []*geo.Point{geo.NewPoint(sx, y), geo.NewPoint(dx, y)}
				end = y
			}
			e.IsCurve = false
			e.ZIndex = 5
			setPosition(&e.LabelPosition, label.InsideMiddleCenter)
		} else {
			o := it.object
			if containsObject(d.groups, o) {
				d.frameTop[o] = y
				d.frameBottom[o] = y + it.below
			} else if _, isSpan := d.depth[o]; isSpan {
				d.emptySpans[o] = y
			} else {
				o.TopLeft = geo.NewPoint(d.owner[o].Center().X-o.Width/2, y-o.Height/2)
				o.ZIndex = 6
			}
			end = y
			if containsObject(d.groups, o) {
				end += it.below
			}
		}
		bottom = math.Max(bottom, y+it.below)
		for _, g := range opening {
			if first[g] >= 0 && last[g] == i {
				d.frameBottom[g] = bottom + framePadding
				bottom += framePadding
			}
		}
		explicit = it.edge != nil && it.edge.VerticalGap != nil
		nextGap = d.vgap
		if explicit {
			nextGap = gap(it.edge.VerticalGap, d.vgap)
		}
	}
	// An empty group still has a finite, visible frame at its textual position.
	for _, g := range d.groups {
		if _, placed := d.frameTop[g]; placed {
			continue
		}
		if first[g] >= 0 {
			continue
		}
		y := headerBottom + d.vgap
		p := objectPosition(g)
		for _, it := range d.timeline {
			if it.pos.less(p) {
				y = math.Max(y, it.y+it.below+framePadding)
			}
		}
		d.frameTop[g] = y
		d.frameBottom[g] = y + math.Max(40, float64(g.LabelDimensions.Height)+framePadding*2)
	}
}

func (d *diagram) placeSpans() {
	spans := append([]*d2graph.Object(nil), d.spans...)
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Level() > spans[j].Level() })
	for _, s := range spans {
		minY, maxY := math.Inf(1), math.Inf(-1)
		for _, it := range d.timeline {
			if it.edge == nil {
				continue
			}
			e := it.edge
			if e.Src.IsDescendantOf(s) {
				minY = math.Min(minY, e.Route[0].Y)
				maxY = math.Max(maxY, e.Route[0].Y)
			}
			if e.Dst.IsDescendantOf(s) {
				y := e.Route[len(e.Route)-1].Y
				minY = math.Min(minY, y)
				maxY = math.Max(maxY, y)
			}
		}
		for _, child := range s.ChildrenArray {
			if child.Box != nil && !containsObject(d.groups, child) {
				b := objectBounds(child)
				minY = math.Min(minY, b.minY)
				maxY = math.Max(maxY, b.maxY)
			}
		}
		if y, ok := d.emptySpans[s]; ok {
			minY = math.Min(minY, y)
			maxY = math.Max(maxY, y)
		}
		if math.IsInf(minY, 1) {
			minY = d.owner[s].TopLeft.Y + d.owner[s].Height + d.vgap
			maxY = minY
		}
		minY -= spanPadding
		maxY += spanPadding
		if s.HasLabel() && s.LabelPosition != nil {
			p := label.FromString(*s.LabelPosition)
			switch p {
			case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
				minY -= float64(s.LabelDimensions.Height) + label.PADDING*2
			case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
				maxY += float64(s.LabelDimensions.Height) + label.PADDING*2
			}
		}
		h := math.Max(32, maxY-minY)
		w := activationWidth(s)
		if s.HeightAttr != nil {
			h = math.Max(h, gap(s.HeightAttr, h))
		}
		s.Box = geo.NewBox(geo.NewPoint(d.owner[s].Center().X+float64(d.depth[s])*spanWidth/2-w/2, minY), w, h)
		s.ZIndex = 4
	}
}

func activationWidth(s *d2graph.Object) float64 {
	w := spanWidth
	if s.HasLabel() && s.LabelPosition != nil && !label.FromString(*s.LabelPosition).IsOutside() {
		w = math.Max(w, float64(s.LabelDimensions.Width)+label.PADDING*2)
	}
	if s.WidthAttr != nil {
		w = gap(s.WidthAttr, w)
	}
	return w
}

func (d *diagram) routeEndpoints() {
	for _, it := range d.timeline {
		if it.edge == nil {
			continue
		}
		e := it.edge
		for i, o := range []*d2graph.Object{e.Src, e.Dst} {
			if _, span := d.depth[o]; !span {
				continue
			}
			other := e.Dst
			point := e.Route[0]
			if i == 1 {
				other = e.Src
				point = e.Route[len(e.Route)-1]
			}
			right := d.rank[d.owner[other]] >= d.rank[d.owner[o]]
			point.X = o.TopLeft.X
			if right {
				point.X += o.Width
			}
		}
		if d.owner[e.Src] == d.owner[e.Dst] {
			x := math.Max(e.Route[0].X, e.Route[len(e.Route)-1].X) + selfTravel(e)
			e.Route[1].X = x
			e.Route[2].X = x
		}
	}
}

type bounds struct{ minX, minY, maxX, maxY float64 }

func emptyBounds() bounds { return bounds{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)} }
func (b *bounds) point(x, y float64) {
	b.minX = math.Min(b.minX, x)
	b.minY = math.Min(b.minY, y)
	b.maxX = math.Max(b.maxX, x)
	b.maxY = math.Max(b.maxY, y)
}
func (b *bounds) include(other bounds) {
	b.point(other.minX, other.minY)
	b.point(other.maxX, other.maxY)
}
func (b bounds) box() *geo.Box {
	return geo.NewBox(geo.NewPoint(b.minX, b.minY), b.maxX-b.minX, b.maxY-b.minY)
}
func objectBounds(o *d2graph.Object) bounds {
	b := bounds{o.TopLeft.X, o.TopLeft.Y, o.TopLeft.X + o.Width, o.TopLeft.Y + o.Height}
	margin, _ := o.Spacing()
	b.minX -= margin.Left
	b.minY -= margin.Top
	b.maxX += margin.Right
	b.maxY += margin.Bottom
	if o.HasLabel() && o.LabelPosition != nil {
		p := label.FromString(*o.LabelPosition).GetPointOnBox(o.Box, label.PADDING, float64(o.LabelDimensions.Width), float64(o.LabelDimensions.Height))
		b.point(p.X, p.Y)
		b.point(p.X+float64(o.LabelDimensions.Width), p.Y+float64(o.LabelDimensions.Height))
	}
	return b
}
func edgeBounds(e *d2graph.Edge) bounds {
	b := emptyBounds()
	for _, p := range e.Route {
		b.point(p.X, p.Y)
	}
	if e.Label.Value != "" && e.LabelPosition != nil {
		p, _ := label.FromString(*e.LabelPosition).GetPointOnRoute(e.Route, 2, .5, float64(e.LabelDimensions.Width), float64(e.LabelDimensions.Height))
		b.point(p.X, p.Y)
		b.point(p.X+float64(e.LabelDimensions.Width), p.Y+float64(e.LabelDimensions.Height))
	}
	return b
}

func (d *diagram) placeMessageGroups() {
	groups := append([]*d2graph.Object(nil), d.groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Level() > groups[j].Level() })
	for _, g := range groups {
		b := emptyBounds()
		for _, it := range d.timeline {
			if d.inGroup(it, g) {
				if it.edge != nil {
					b.include(edgeBounds(it.edge))
				} else {
					b.include(objectBounds(it.object))
				}
			}
		}
		for _, child := range g.ChildrenArray {
			if child.Shape.Value == d2target.ShapeSequenceDiagramEdgeGroup || containsObject(d.groups, child) {
				b.include(objectBounds(child))
			}
		}
		if math.IsInf(b.minX, 1) {
			b.minX = d.actors[0].Center().X - framePadding
			b.maxX = d.actors[len(d.actors)-1].Center().X + framePadding
		}
		b.minX -= framePadding
		b.maxX += framePadding
		b.minY = d.frameTop[g]
		b.maxY = math.Max(b.maxY+framePadding, d.frameBottom[g])
		if b.maxX-b.minX < float64(g.LabelDimensions.Width)+framePadding*2 {
			b.maxX = b.minX + float64(g.LabelDimensions.Width) + framePadding*2
		}
		g.Box = b.box()
		g.ZIndex = 2
	}
}
func containsObject(list []*d2graph.Object, o *d2graph.Object) bool {
	for _, v := range list {
		if o == v {
			return true
		}
	}
	return false
}

func (d *diagram) finishActors() {
	end := 0.
	for _, o := range d.graph.Objects {
		if o == d.graph.Root || containsObject(d.actorGroups, o) {
			continue
		}
		end = math.Max(end, objectBounds(o).maxY)
	}
	for _, it := range d.timeline {
		if it.edge != nil {
			end = math.Max(end, edgeBounds(it.edge).maxY)
		}
	}
	end += math.Max(d.vgap, padding)
	for _, a := range d.actors {
		b := objectBounds(a)
		x := a.Center().X
		lineEnd := end
		if d.graph.Root.Sequence != nil && d.graph.Root.Sequence.Mirror {
			// Structured shapes consume their field children during compilation.
			// Repetitions are graph objects and need a new child map in that case.
			if a.Children == nil {
				a.Children = make(map[string]*d2graph.Object)
			}
			clone := *a
			clone.Parent = a
			clone.Shape.Value = d2target.ShapeSequenceDiagramActor
			clone.Children = make(map[string]*d2graph.Object)
			clone.ChildrenArray = nil
			clone.References = nil
			for i := 0; ; i++ {
				clone.IDVal = "mirror"
				if i > 0 {
					clone.IDVal += strconv.Itoa(i)
				}
				clone.ID = d2format.Format(d2ast.MakeKeyPath([]string{clone.IDVal}))
				if _, exists := a.Children[strings.ToLower(clone.ID)]; !exists {
					break
				}
			}
			clone.Box = geo.NewBox(geo.NewPoint(a.TopLeft.X, end), a.Width, a.Height)
			margin, _ := clone.Spacing()
			clone.TopLeft.Y += margin.Top
			lineEnd = clone.TopLeft.Y - margin.Top
			a.Children[strings.ToLower(clone.ID)] = &clone
			a.ChildrenArray = append(a.ChildrenArray, &clone)
			d.graph.Objects = append(d.graph.Objects, &clone)
		}
		style := d2graph.Style{StrokeDash: &d2graph.Scalar{Value: "6"}, StrokeWidth: &d2graph.Scalar{Value: "2"}}
		if a.Style.Stroke != nil {
			style.Stroke = a.Style.Stroke
		}
		if a.Style.StrokeDash != nil {
			style.StrokeDash = a.Style.StrokeDash
		}
		// Break the lifeline around event bodies and outside labels. Index is
		// part of edge identity, so each visible segment is independently addressable.
		var interruptions []bounds
		for _, o := range d.events {
			if d.owner[o] == a {
				interruptions = append(interruptions, objectBounds(o))
			}
		}
		sort.SliceStable(interruptions, func(i, j int) bool { return interruptions[i].minY < interruptions[j].minY })
		start := b.maxY
		index := 0
		segment := func(stop float64) {
			if stop <= start {
				return
			}
			d.graph.Edges = append(d.graph.Edges, &d2graph.Edge{Index: index, Src: a, Dst: &d2graph.Object{ID: d2sequence.LifelineEndID(a.ID)}, Attributes: d2graph.Attributes{Style: style}, Route: []*geo.Point{geo.NewPoint(x, start), geo.NewPoint(x, stop)}, ZIndex: 1})
			index++
		}
		for _, gap := range interruptions {
			segment(math.Min(lineEnd, gap.minY-label.PADDING))
			start = math.Max(start, gap.maxY+label.PADDING)
		}
		segment(lineEnd)
	}
}

func (d *diagram) placeActorGroups() {
	groups := append([]*d2graph.Object(nil), d.actorGroups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Level() > groups[j].Level() })
	for _, g := range groups {
		b := emptyBounds()
		for _, o := range d.graph.Objects {
			if o != g && o.IsDescendantOf(g) {
				b.include(objectBounds(o))
			}
		}
		for _, e := range d.graph.Edges {
			if d2sequence.IsLifelineEnd(e.Dst) && e.Src.IsDescendantOf(g) {
				b.include(edgeBounds(e))
			}
		}
		if math.IsInf(b.minX, 1) {
			b = bounds{0, 0, 100, 60}
		}
		b.minX -= framePadding
		b.maxX += framePadding
		b.minY -= framePadding + float64(g.LabelDimensions.Height) + label.PADDING*2
		b.maxY += framePadding
		if b.maxX-b.minX < float64(g.LabelDimensions.Width)+framePadding*2 {
			extra := (float64(g.LabelDimensions.Width) + framePadding*2 - b.maxX + b.minX) / 2
			b.minX -= extra
			b.maxX += extra
		}
		g.Box = b.box()
		g.ZIndex = 0
	}
}

func (d *diagram) fitRoot() {
	b := emptyBounds()
	for _, o := range d.graph.Objects {
		if o != d.graph.Root {
			b.include(objectBounds(o))
		}
	}
	for _, e := range d.graph.Edges {
		b.include(edgeBounds(e))
	}
	title := 0.
	if d.graph.Root.HasLabel() {
		title = float64(d.graph.Root.LabelDimensions.Height) + padding
	}
	dx, dy := padding-b.minX, padding+title-b.minY
	for _, o := range d.graph.Objects {
		if o != d.graph.Root {
			o.TopLeft.X += dx
			o.TopLeft.Y += dy
		}
	}
	for _, e := range d.graph.Edges {
		e.Move(dx, dy)
	}
	w := math.Max(b.maxX-b.minX+padding*2, float64(d.graph.Root.LabelDimensions.Width)+padding*2)
	h := b.maxY - b.minY + padding*2 + title
	d.graph.Root.Box = geo.NewBox(geo.NewPoint(0, 0), w, h)
	setPosition(&d.graph.Root.LabelPosition, label.InsideTopCenter)
}
