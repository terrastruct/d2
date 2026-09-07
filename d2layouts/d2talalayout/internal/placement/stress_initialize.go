package placement

import (
	"context"
	"math"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// initializeByGraphDistance is an experimental, bounded alternative starting
// arrangement. It gives distant parts of the graph distinct space rather than
// minimizing only distances to already placed neighbors. The ordinary orthogonal
// optimizer still owns the final placement.
func initializeByGraphDistance(ctx context.Context, g *layoutgraph.Graph) (bool, error) {
	if err := layoutgraph.Validate(ctx, "GraphDistanceInitialization", g); err != nil {
		return false, err
	}
	n := len(g.Nodes)
	if n < 4 || n > 64 || len(g.Edges) > 256 || g.HasFixedNode() {
		return false, nil
	}
	index := make(map[*layoutgraph.Node]int, n)
	for i, node := range g.Nodes {
		index[node] = i
	}
	d := make([][]float64, n)
	for i := range d {
		d[i] = make([]float64, n)
		for j := range d[i] {
			if i != j {
				d[i][j] = math.Inf(1)
			}
		}
	}
	for _, edge := range g.Edges {
		i, okI := index[edge.From]
		j, okJ := index[edge.To]
		if okI && okJ && i != j {
			d[i][j], d[j][i] = 1, 1
		}
	}
	// Near-only or indirect connectivity falls back to ordinary initialization.
	for k := 0; k < n; k++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				d[i][j] = math.Min(d[i][j], d[i][k]+d[k][j])
			}
		}
	}
	for i := range d {
		for j := range d[i] {
			if math.IsInf(d[i][j], 1) {
				return false, nil
			}
		}
	}
	x, y := make([]float64, n), make([]float64, n)
	for i := range x {
		a := 2 * math.Pi * float64(i) / float64(n)
		x[i], y[i] = math.Sqrt(float64(n))*math.Cos(a), math.Sqrt(float64(n))*math.Sin(a)
	}
	// Pairwise relaxation of sum ((EuclideanDistance - graphDistance) /
	// graphDistance)^2. Deterministic sweeps leave the annealing RNG untouched.
	for sweep := 0; sweep < 48; sweep++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		eta := .7 * math.Pow(.02/.7, float64(sweep)/47)
		for p := 0; p < n; p++ {
			i := (p + sweep) % n
			for q := p + 1; q < n; q++ {
				j := (q + sweep) % n
				dx, dy := x[i]-x[j], y[i]-y[j]
				length := math.Hypot(dx, dy)
				if length < 1e-9 {
					dx, dy, length = 1e-6, 1e-6, math.Sqrt(2)*1e-6
				}
				mu := math.Min(1, eta/(d[i][j]*d[i][j]))
				amount := .5 * mu * (length - d[i][j]) / length
				x[i] -= dx * amount
				y[i] -= dy * amount
				x[j] += dx * amount
				y[j] += dy * amount
			}
		}
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return len(g.Nodes[order[i]].Edges) > len(g.Nodes[order[j]].Edges) })
	occupied := make(map[[2]int]bool, n)
	points := make([]*geo.Point, n)
	for _, i := range order {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		tx, ty := x[i]*2+float64(n), y[i]*2+float64(n)
		cx, cy := int(math.Round(tx)), int(math.Round(ty))
		best := [2]int{cx, cy}
		cost := math.Inf(1)
		// n bounds the radius needed to find an unused integer cell.
		for radius := 0; radius <= n; radius++ {
			for xx := cx - radius; xx <= cx+radius; xx++ {
				for yy := cy - radius; yy <= cy+radius; yy++ {
					if radius > 0 && xx != cx-radius && xx != cx+radius && yy != cy-radius && yy != cy+radius {
						continue
					}
					p := [2]int{xx, yy}
					if occupied[p] {
						continue
					}
					c := math.Pow(float64(xx)-tx, 2) + math.Pow(float64(yy)-ty, 2)
					if c < cost {
						best, cost = p, c
					}
				}
			}
			if cost < math.Inf(1) {
				break
			}
		}
		occupied[best] = true
		points[i] = geo.NewPoint(float64(best[0]), float64(best[1]))
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for i, node := range g.Nodes {
		node.TopLeft = points[i]
	}
	return true, nil
}
