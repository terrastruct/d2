package d2isometricimg

import "github.com/d2lang/d2/d2renderers/d2isometric"

// Move a caster only along the viewing ray. Its screen location is unchanged,
// but its shadow on the common receiver now has exactly the projection of its
// shadow on the root's local receiver. One ground atlas can cover every root;
// physical surface shadows continue to use the original unmodified geometry.
func rasterGroundTriangles(ts []Triangle, ground float64, view Vec) []Triangle {
	changed := false
	for _, t := range ts {
		if t.ShadowGround != nil && *t.ShadowGround != ground {
			changed = true
			break
		}
	}
	if !changed {
		return ts
	}
	out := append([]Triangle(nil), ts...)
	for i := range out {
		t := &out[i]
		if t.ShadowGround == nil || *t.ShadowGround == ground {
			continue
		}
		delta := nmul(view, (ground-*t.ShadowGround)/view.Y)
		for j := range t.V {
			t.V[j].Position = nadd(t.V[j].Position, delta)
		}
	}
	return out
}

type hierarchyShadowSpan struct {
	first, last int
	board       string
}

// Domain identity follows visible root containment, including layout-only
// wrappers. It is independent of any unrelated root's depth or shape count.
func hierarchyShadowDomains(boards []d2isometric.Board) map[string]string {
	index := make(map[string]int, len(boards))
	for i, b := range boards {
		index[b.ID] = i
	}
	domains := make(map[string]string, len(boards))
	state := make([]uint8, len(boards))
	var find func(int) string
	find = func(i int) string {
		board := boards[i]
		if state[i] != 0 {
			if domain := domains[board.ID]; domain != "" {
				return domain
			}
			return board.ID
		}
		state[i] = 1
		domain := board.ID
		if parent, ok := index[board.ParentID]; ok && board.Kind != "platform" {
			domain = find(parent)
		}
		domains[board.ID], state[i] = domain, 2
		return domain
	}
	for i := range boards {
		find(i)
	}
	return domains
}

func hierarchyShadowReceivers(ts []Triangle, boards []d2isometric.Board, spans []hierarchyShadowSpan) {
	domains := hierarchyShadowDomains(boards)
	receivers := make(map[string]*float64)
	for _, span := range spans {
		domain, ok := domains[span.board]
		if !ok {
			continue
		}
		receiver := receivers[domain]
		if receiver == nil {
			receiver = new(float64)
			receivers[domain] = receiver
		}
		for _, triangle := range ts[span.first:span.last] {
			for _, vertex := range triangle.V {
				*receiver = min(*receiver, vertex.Position.Y)
			}
		}
	}
	for _, receiver := range receivers {
		*receiver += .08
	}
	for _, span := range spans {
		receiver := receivers[domains[span.board]]
		if receiver == nil {
			continue
		}
		for i := span.first; i < span.last; i++ {
			ts[i].ShadowGround = receiver
		}
	}
}
