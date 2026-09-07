package d2isometricimg

import (
	"fmt"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// Presentation copies can lift a nested plate within the clearance below the
// common component and route planes. Sequence groups keep their timeline's
// original background plane rather than interpreting node elevation as relief.
func hierarchySurfaceY(board d2isometric.Board) float64 {
	y := hierarchyBaseSurfaceY(board)
	if board.Kind != "sequence-group" {
		y += board.Position.Y
	}
	return y
}

func hierarchyBaseSurfaceY(board d2isometric.Board) float64 {
	return .028 + float64(max(0, min(128, board.Level)))*.0001
}

const hierarchyTerraceCeiling = .066 // Leaves start at .07; routes remain at .08.

const hierarchyRelief = .34

// Organizational panels stay shallow, while volume-bearing source shapes
// retain enough height to read as cylinders and prisms in the same camera.
func hierarchyNodeRelief(node d2isometric.Node) float64 {
	if nativeStructuredNode(node) || nativeMarkdownCard(node) {
		return 1 // Row rails and paper cards already use their selected relief.
	}
	if nativeSolidNode(node) {
		return .60
	}
	if nativeReliefSymbol(node) {
		// The selected symbol depth is already a relief, measured from its
		// source footprint. Do not compress it a second time in hierarchy.
		return 1
	}
	return hierarchyRelief
}

func hierarchyRenderNodes(nodes []d2isometric.Node, supports ...map[string]float64) []d2isometric.Node {
	out := append([]d2isometric.Node(nil), nodes...)
	for i := range out {
		if out[i].Container {
			continue
		}
		floor := out[i].Position.Y - out[i].Size.Y/2
		height := out[i].Size.Y
		if nativeSolidNode(out[i]) {
			// These copies are only header obstacles. Record the actual body
			// height before applying relief, including authored 3D modifiers.
			height = nativeSolidHeight(out[i])
		} else if nativeReliefSymbol(out[i]) || nativeStructuredNode(out[i]) || nativeMarkdownCard(out[i]) {
			height = nativeCanonicalHeight(out[i], 0)
		}
		out[i].Size.Y = height * hierarchyNodeRelief(out[i])
		out[i].Position.Y = floor + out[i].Size.Y/2
		if len(supports) > 0 {
			drop := supports[0][out[i].BoardID]
			out[i].Size.Y -= drop
			out[i].Position.Y += drop / 2
		}
	}
	return out
}

// Source layouts reserve label space in two dimensions. Low relief preserves
// those clearances in projection while retaining every footprint and texture.
func (b *meshBuilder) hierarchyNode(node d2isometric.Node, tint string) {
	start := len(b.triangles)
	firstLink := len(b.links)
	relief := hierarchyNodeRelief(node)
	priorSupport := b.nodeSupportDrop
	b.nodeSupportDrop = b.hierarchySupports[node.BoardID] / relief
	b.node(node, tint, relief)
	b.nodeSupportDrop = priorSupport
	floor := node.Position.Y - node.Size.Y/2
	for i := start; i < len(b.triangles); i++ {
		for j := range b.triangles[i].V {
			v := &b.triangles[i].V[j]
			v.Position.Y = floor + (v.Position.Y-floor)*relief
			v.Normal = nunit(nv(v.Normal.X, v.Normal.Y/relief, v.Normal.Z))
		}
	}
	beforeInk := len(b.triangles)
	if !nativeStructuredNode(node) && !nativeMarkdownCard(node) {
		b.classicInkEdges(node, b.triangles[start:])
	}
	if node.Metadata.Original.Animated && len(b.animatedNodes) > 0 {
		last := &b.animatedNodes[len(b.animatedNodes)-1]
		if last.first == start && last.last == beforeInk {
			last.last = len(b.triangles)
		}
	}
	for i := firstLink; i < len(b.links); i++ {
		for j := range b.links[i].points {
			p := &b.links[i].points[j]
			p.Y = floor + (p.Y-floor)*relief
		}
	}
	b.markPaintOwner(start)
}

func hierarchySpacer(node d2isometric.Node) bool {
	return strings.TrimSpace(node.Label) == "" && node.Icon == "" &&
		nativePaint(node.Fill, "#edf1f7").A == 0 &&
		(node.StrokeWidth == 0 || nativePaint(node.Stroke, "#849ebc").A == 0)
}

// Blank white/transparent grid wrappers carry layout, not a physical boundary.
// Their source nodes and links remain in the scene; only decorative paint is
// omitted. Named or visibly colored regions are never removed by this rule.
func hierarchyLayoutWrapper(owner *d2isometric.Node) bool {
	if owner == nil || strings.TrimSpace(owner.Label) != "" || owner.Icon != "" {
		return false
	}
	fill, stroke := nativePaint(owner.Fill, "white"), nativePaint(owner.Stroke, "#849ebc")
	quietFill := fill.A == 0 || fill.R >= 245 && fill.G >= 245 && fill.B >= 245
	quietStroke := owner.StrokeWidth == 0 || stroke.A == 0 || stroke == fill
	return quietFill && quietStroke
}

func hierarchyPresentationBoards(boards []d2isometric.Board, nodes map[string]*d2isometric.Node) []d2isometric.Board {
	out := append([]d2isometric.Board(nil), boards...)
	index := make(map[string]d2isometric.Board, len(boards))
	for _, board := range boards {
		index[board.ID] = board
	}
	for i := range out {
		board := &out[i]
		if hierarchyLayoutWrapper(nodes[board.SourceID]) {
			board.Kind, board.Label = "ungrouped", ""
			continue
		}
		if board.Kind != "group" {
			continue
		}
		parent := board.ParentID
		for steps := 0; parent != "" && steps < len(boards); steps++ {
			p, ok := index[parent]
			if !ok || !hierarchyLayoutWrapper(nodes[p.SourceID]) {
				break
			}
			parent = p.ParentID
		}
		if parent == "" {
			board.Kind = "platform"
		}
	}
	hierarchyTerraces(out, nodes)
	return out
}

// Thickness and elevation belong to presentation copies only. An invisible
// wrapper, dashed scope or transparent region cannot add another physical tier.
func hierarchyTerraces(boards []d2isometric.Board, nodes map[string]*d2isometric.Node) {
	index := make(map[string]int, len(boards))
	for i := range boards {
		index[boards[i].ID] = i
		boards[i].Position.Y = 0
	}
	// Sequence annotations have independent source scopes and timeline planes.
	// Keep their ancestor backgrounds at the original elevation too, avoiding
	// a new opaque plate hiding a sequence's independently emitted group wash.
	sequenceBranch := make(map[string]bool)
	for _, board := range boards {
		if owner := nodes[board.SourceID]; owner != nil && owner.SequenceRole == "container" {
			for id, steps := board.ID, 0; id != "" && steps < len(boards); steps++ {
				if sequenceBranch[id] {
					break
				}
				sequenceBranch[id] = true
				i, ok := index[id]
				if !ok {
					break
				}
				id = boards[i].ParentID
			}
		}
	}
	state := make([]uint8, len(boards))
	inSequence := make([]bool, len(boards))
	var place func(int)
	place = func(i int) {
		if state[i] != 0 {
			return
		}
		state[i] = 1
		board := &boards[i]
		base := hierarchyBaseSurfaceY(*board)
		parentY := .028
		if parent, ok := index[board.ParentID]; ok && state[parent] != 1 {
			place(parent)
			parentY = hierarchySurfaceY(boards[parent])
			inSequence[i] = inSequence[parent]
		}
		owner := nodes[board.SourceID]
		if owner != nil && owner.SequenceRole == "container" {
			inSequence[i] = true
		}
		if board.Kind == "ungrouped" {
			// Carry the nearest visible cap through layout-only wrappers.
			board.Position.Y = max(0, parentY-base)
		}
		if board.Kind == "group" {
			board.Size.Y = 0
			if !sequenceBranch[board.ID] && !inSequence[i] {
				top := max(base, parentY)
				if owner != nil && owner.SequenceRole == "" && owner.Opacity > 0 && owner.StrokeDash == 0 && nativePaint(owner.Fill, "transparent").A > 0 {
					// Spend most clearance on the first tier, then progressively
					// smaller steps. Even deep source nesting stays below leaves.
					top += max(0, hierarchyTerraceCeiling-top) * (.030 / .038)
					board.Size.Y = max(0, top-parentY)
				}
				board.Position.Y = max(0, top-base)
			}
		}
		state[i] = 2
	}
	for i := range boards {
		place(i)
	}
	hierarchyStrongTerraces(boards, nodes, sequenceBranch, inSequence)
}

func hierarchyPhysicalPlate(board d2isometric.Board, owner *d2isometric.Node, fillAlpha uint8) bool {
	if owner == nil || fillAlpha == 0 || owner.StrokeDash > 0 {
		return false
	}
	return board.Kind == "platform" || board.Kind == "terrace" || board.Kind == "group" && board.Position.Y > 0 && owner.SequenceRole == ""
}

func hierarchyCasingFloor(boards []d2isometric.Board, nodes map[string]*d2isometric.Node) float64 {
	floor := 0.
	for _, board := range boards {
		owner := nodes[board.SourceID]
		if (board.Kind == "terrace" || board.Kind == "group" && board.Position.Y > 0) && owner != nil && owner.Opacity > 0 && hierarchyPhysicalPlate(board, owner, nativePaint(owner.Fill, "transparent").A) {
			floor = max(floor, hierarchySurfaceY(board)+.0005)
		}
	}
	return floor
}

// Header contrast follows the actually visible parent surface, including the
// translucent washes between it and this label, rather than the raw fill hue.
func hierarchyHeaderInk(board d2isometric.Board, boards map[string]d2isometric.Board, nodes map[string]*d2isometric.Node, tints map[string]string, background string) string {
	chain := []d2isometric.Board{board}
	for p := board.ParentID; p != "" && len(chain) <= len(boards); {
		parent, ok := boards[p]
		if !ok {
			break
		}
		chain = append(chain, parent)
		p = parent.ParentID
	}
	r, g, blue := 245., 247., 251.
	ground := nativePaint(background, "#f5f7fb")
	a := float64(ground.A) / 255
	r, g, blue = float64(ground.R)*a+r*(1-a), float64(ground.G)*a+g*(1-a), float64(ground.B)*a+blue*(1-a)
	for i := len(chain) - 1; i >= 0; i-- {
		item := chain[i]
		owner := nodes[item.SourceID]
		if owner == nil || item.Kind == "ungrouped" {
			continue
		}
		fill := nativePaint(tints[item.ID], "#edf1f7")
		a := float64(fill.A) / 255 * owner.Opacity
		if !hierarchyPhysicalPlate(item, owner, fill.A) {
			a *= .22
		}
		r, g, blue = float64(fill.R)*a+r*(1-a), float64(fill.G)*a+g*(1-a), float64(fill.B)*a+blue*(1-a)
	}
	return readableSurfaceInk(fmt.Sprintf("#%02x%02x%02x", uint8(r), uint8(g), uint8(blue)), 1)
}

func hierarchyBoardTint(owner *d2isometric.Node, fallback string) string {
	// Source containers already have their theme colors resolved. Their large
	// background surfaces must retain that paint, including transparent fills;
	// a level accent can otherwise turn a pale source container into a dark slab.
	if owner != nil && owner.Fill != "" {
		return owner.Fill
	}
	return fallback
}

// A source container encloses its actual descendants. Organizational regions
// have a fine printed boundary and wash, while outer systems retain a platform.
func (b *meshBuilder) hierarchyBoard(board d2isometric.Board, owner *d2isometric.Node, tint string, opacity float64) {
	if board.Kind == "ungrouped" || owner == nil || opacity <= 0 || board.Size.X <= 0 || board.Size.Z <= 0 {
		return
	}
	first := len(b.triangles)
	fill := nativePaint(tint, "#edf1f7")
	platform := hierarchyPhysicalPlate(board, owner, fill.A)
	if !platform && board.Kind != "sequence-group" {
		fill.A = uint8(math.Round(float64(fill.A) * .22))
	}
	faceOwner := *owner
	faceOwner.Size = board.Size
	source := nativeFaceSource(faceOwner, richColor(fill))
	profiles, err := nativeShapeProfiles(source)
	if err != nil {
		b.err = err
		return
	}
	tex, face := b.nativeFace(source)
	if b.err != nil {
		return
	}
	sx, sz := board.Size.X/float64(source.Width), board.Size.Z/float64(source.Height)
	scale := b.scale
	if scale <= 0 {
		scale = .01
	}
	y := hierarchySurfaceY(board)
	depth := 0.
	if platform {
		depth = .13
		if board.Kind == "group" || board.Kind == "terrace" {
			depth = board.Size.Y
		}
	}
	if owner.Metadata.Original.ThreeDee {
		depth += d2target.THREE_DEE_OFFSET * scale
	}
	side := nativeMaterial(tint, .62, .12, opacity)
	side.Color = tintDark(side.Color, .85)
	for copyIndex := 1; copyIndex >= 0; copyIndex-- {
		if copyIndex == 1 && !owner.Metadata.Original.Multiple {
			continue
		}
		dx, dz := float64(copyIndex)*d2target.MULTIPLE_OFFSET*scale, -float64(copyIndex)*d2target.MULTIPLE_OFFSET*scale
		top := y - float64(copyIndex)*.00001
		if depth > 0 {
			for _, profile := range profiles {
				world := make([]Vec, len(profile))
				for i, p := range profile {
					world[i] = nv(board.Position.X-board.Size.X/2+p.X*sx+dx, top, board.Position.Z-board.Size.Z/2+p.Z*sz+dz)
				}
				b.extrudedProfile(world, top-depth, nil, side)
			}
		}
		paint := face
		paint.center = nv(board.Position.X-board.Size.X/2+face.center.X*sx+dx, top, board.Position.Z-board.Size.Z/2+face.center.Z*sz+dz)
		paint.width, paint.depth = face.width*sx, face.depth*sz
		start := len(b.triangles)
		b.surfaceTexture(tex, paint, opacity)
		for i := start; i < len(b.triangles); i++ {
			b.triangles[i].NoDepthWrite = false
			b.triangles[i].Material.Unlit = !platform
			b.triangles[i].CastShadow = platform || owner.Metadata.Original.Shadow
			if board.Kind == "sequence-group" {
				b.triangles[i].Material.Multiply = true
				b.triangles[i].NoDepthWrite = true
			}
		}
	}
	nativePhysicalShadows(b.triangles[first:], owner.Metadata.Original.Shadow)
	if owner.Metadata.Original.Animated && len(b.triangles) > first {
		b.animatedNodes = append(b.animatedNodes, nativeAnimatedNode{first: first, last: len(b.triangles)})
	}
}

func (b *meshBuilder) hierarchyRect(center Vec, width, depth, y float64, mat *Material) {
	a, c := nv(center.X-width/2, y, center.Z-depth/2), nv(center.X+width/2, y, center.Z-depth/2)
	d, e := nv(center.X+width/2, y, center.Z+depth/2), nv(center.X-width/2, y, center.Z+depth/2)
	b.flat(a, e, d, mat, false)
	b.flat(a, d, c, mat, false)
}
