package d2isometricimg

import "github.com/d2lang/d2/d2renderers/d2isometric"

// Sequence time remains the compiled Z coordinate. Only the material depth
// changes: groups are regions below the timeline, activations are inked rails,
// and notes are folded pages above messages. None of these semantic children
// turn an actor header into a container or enlarge its source footprint.
func (b *meshBuilder) sequenceNode(node d2isometric.Node, tint string) {
	switch node.SequenceRole {
	case "group":
		firstAnimation := len(b.animatedNodes)
		owner := nativeClassicNode(node)
		board := d2isometric.Board{
			ID: node.ID, SourceID: node.ID, Kind: "sequence-group",
			Level:    node.Metadata.Original.Level,
			Position: node.Position, Size: node.Size,
		}
		// The compiler's multiply wash preserves overlapping source scopes
		// without an opaque slab hiding activations or lifelines.
		b.hierarchyBoard(board, &owner, node.Fill, node.Opacity)
		source := nativeFaceSource(owner, owner.Fill)
		b.canonicalNodeContent(owner, source, owner.Fill, .29)
		if len(b.animatedNodes) > firstAnimation {
			b.animatedNodes[firstAnimation].last = len(b.triangles)
		}
	case "span":
		// Nested caps remain below the common message plane: a message may
		// intentionally cross another actor's activation. Tiny offsets retain
		// the source nesting order without hiding those interior crossings.
		top := .28 + float64(max(0, min(128, node.Metadata.Original.Level)))*.00001
		node.Size.Y = (top - .10) / 1.15
		// An authored 3D modifier extends the body downward, keeping its cap
		// below messages rather than obscuring a later source paint layer.
		node.Position.Y = top - nativeCanonicalHeight(node, b.scale) + node.Size.Y/2
		b.node(node, tint)
	case "note":
		node.Position.Y = .16 + node.Size.Y/2
		b.hierarchyNode(node, tint)
	default:
		b.hierarchyNode(node, tint)
	}
}
