package d2talalayout

import (
	"fmt"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestSeedSelectionAcrossCompletionOrders(t *testing.T) {
	tests := []struct {
		name   string
		scores [3]layoutScore
		want   int
	}{
		{
			name: "near penalties",
			// The label score contributes 1 - 1/(1+overlapScore). Adjacent
			// values here are within the former tolerance, while the first
			// and last are not. Comparing area on those approximate ties
			// made the winner depend on which attempt completed first.
			scores: [3]layoutScore{
				{penalty: 1 - 1.0/115, area: 300},
				{penalty: 1 - 1.0/116, area: 200},
				{penalty: 1 - 1.0/117, area: 100},
			},
			want: 0,
		},
		{
			name: "adjacent floating point penalties",
			scores: [3]layoutScore{
				{penalty: math.Nextafter(1, 2), area: 1},
				{penalty: 1, area: 300},
				{penalty: 2, area: 1},
			},
			want: 1,
		},
		{
			name: "area breaks exact penalty ties",
			scores: [3]layoutScore{
				{penalty: 1, area: 300},
				{penalty: 1, area: 100},
				{penalty: 1, area: 200},
			},
			want: 1,
		},
		{
			name: "later seed breaks exact score ties",
			scores: [3]layoutScore{
				{penalty: 1, area: 100},
				{penalty: 1, area: 100},
				{penalty: 1, area: 100},
			},
			want: 2,
		},
	}
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, order := range orders {
				t.Run(fmt.Sprint(order), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						graphs := [3]*layoutgraph.Graph{layoutgraph.NewGraph(), layoutgraph.NewGraph(), layoutgraph.NewGraph()}
						var delays [3]time.Duration
						for position, index := range order {
							delays[index] = time.Duration(position+1) * time.Millisecond
						}
						best, err := coordinateLocalSeeds(t.Context(), []int64{1, 2, 3}, 3, func(index int, seed int64) localSeedAttempt {
							time.Sleep(delays[index])
							return localSeedAttempt{
								index:  index,
								seed:   seed,
								result: seedResult{graph: graphs[index], score: test.scores[index]},
							}
						})
						if err != nil {
							t.Fatal(err)
						}
						if best.graph != graphs[test.want] {
							t.Fatalf("selected score %+v, want seed %d with score %+v", best.score, test.want+1, test.scores[test.want])
						}
					})
				})
			}
		})
	}
}
