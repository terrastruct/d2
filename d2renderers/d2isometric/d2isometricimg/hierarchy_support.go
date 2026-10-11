package d2isometricimg

import (
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

const hierarchyMaxDescent = .60

// Spend the most visible relief on the first source tier, with progressively
// smaller steps and a fixed bound even for very deep source nesting.
func hierarchyTierHeight(depth int) float64 {
	return hierarchyMaxDescent * (1 - math.Pow(2./3, float64(max(0, depth))))
}

// Each independent visible root gets its own support plan. Deep containers
// never stretch an unrelated shallow diagram. Sequence-containing roots keep
// their existing background planes and semantic group washes.
func hierarchyStrongTerraces(boards []d2isometric.Board, nodes map[string]*d2isometric.Node, sequenceBranch map[string]bool, inSequence []bool) {
	index := make(map[string]int, len(boards))
	for i, board := range boards {
		index[board.ID] = i
	}
	depth, root := make([]int, len(boards)), make([]int, len(boards))
	physical, blocked := make([]bool, len(boards)), make([]bool, len(boards))
	state := make([]uint8, len(boards))
	var visit func(int)
	visit = func(i int) {
		if state[i] != 0 {
			return
		}
		state[i], root[i] = 1, i
		board := boards[i]
		if p, ok := index[board.ParentID]; ok && state[p] != 1 && board.Kind != "platform" {
			visit(p)
			depth[i], root[i] = depth[p], root[p]
		}
		owner := nodes[board.SourceID]
		physical[i] = board.Kind == "group" && owner != nil && owner.SequenceRole == "" && owner.Opacity > 0 && owner.StrokeDash == 0 && nativePaint(owner.Fill, "transparent").A > 0
		if physical[i] {
			depth[i]++
		}
		state[i] = 2
	}
	for i := range boards {
		visit(i)
		if sequenceBranch[boards[i].ID] || inSequence[i] {
			blocked[root[i]] = true
		}
	}
	deepest := make([]int, len(boards))
	for i := range boards {
		deepest[root[i]] = max(deepest[root[i]], depth[i])
	}
	for i := range boards {
		r := root[i]
		if blocked[r] || deepest[r] == 0 {
			continue
		}
		boards[i].Position.Y = hierarchyTierHeight(depth[i]) - hierarchyTierHeight(deepest[r])
		if depth[i] == deepest[r] {
			boards[i].Position.Y = 0
		}
		if physical[i] {
			// A presentation-only kind keeps source paint physical even when
			// floating-point saturation makes a very deep final step zero.
			boards[i].Kind = "terrace"
		}
	}
	for i := range boards {
		if boards[i].Kind != "terrace" {
			continue
		}
		bottom := hierarchyBaseSurfaceY(boards[i]) + hierarchyTierHeight(depth[i]-1) - hierarchyTierHeight(deepest[root[i]])
		if p, ok := index[boards[i].ParentID]; ok {
			bottom = hierarchySurfaceY(boards[p])
		}
		boards[i].Size.Y = max(0, hierarchySurfaceY(boards[i])-bottom)
	}
}

func hierarchySupportOffsets(boards []d2isometric.Board) map[string]float64 {
	var offsets map[string]float64
	for _, board := range boards {
		if board.Position.Y < 0 {
			if offsets == nil {
				offsets = make(map[string]float64)
			}
			offsets[board.ID] = board.Position.Y
		}
	}
	return offsets
}

// Solid contact queries use the same cap-anchored extension as construction.
// The body is taller beneath its original top; labels and route Y stay fixed.
type hierarchyExtension struct{ top, scale float64 }

func hierarchyNodeExtension(n d2isometric.Node, drop float64) hierarchyExtension {
	floor := n.Position.Y - n.Size.Y/2
	height := nativeCanonicalHeight(n, 0)
	if nativeSolidNode(n) {
		height = nativeSolidHeight(n)
	}
	height *= hierarchyNodeRelief(n)
	return hierarchyExtension{floor + height, 1 - min(0, drop)/max(.001, height)}
}

func (e hierarchyExtension) inverseY(y float64) float64 {
	if y >= e.top || e.scale <= 1 {
		return y
	}
	return e.top + (y-e.top)/e.scale
}

func (e hierarchyExtension) pointY(y float64) float64 {
	if y >= e.top || e.scale <= 1 {
		return y
	}
	return e.top + (y-e.top)*e.scale
}
