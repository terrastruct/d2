package layoutgraph

import (
	"math"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

func TestIsotropicSpacingMatchesDirectionalPath(t *testing.T) {
	random := rand.New(rand.NewSource(518))
	special := []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1), 10, -10}
	for range 10_000 {
		from, to := NewNode(1, 10, 10), NewNode(2, 10, 10)
		from.TopLeft, to.TopLeft = &geo.Point{}, &geo.Point{X: random.Float64()*100 - 50, Y: random.Float64()*100 - 50}
		from.Width, from.Height = special[random.Intn(len(special))], special[random.Intn(len(special))]
		to.Width, to.Height = special[random.Intn(len(special))], special[random.Intn(len(special))]
		point := &geo.Point{X: special[random.Intn(len(special))], Y: special[random.Intn(len(special))]}
		if random.Intn(2) == 0 {
			gap := random.Intn(200)
			from.Edges = []*Edge{{From: from, To: to, MinWidth: gap, MinHeight: gap}}
		}
		got, err := from.deltaToGuarded(to, point, nil)
		if err != nil {
			t.Fatal(err)
		}
		// A retained zero-valued loop offset forces the full orientation path, but
		// adds only NodeGap, already a lower bound of both required gaps. Thus this
		// is an independent execution of the same directional contract.
		from.LoopOffsets = map[geo.Orientation]float64{geo.NONE: 0}
		want, err := from.deltaToGuarded(to, point, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("spacing %d, want %d; point=%+v from=%+v to=%+v", got, want, point, from.Box, to.Box)
		}
	}
}
