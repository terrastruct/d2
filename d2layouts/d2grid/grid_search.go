package d2grid

// Row measurements preserve the original left-to-right floating-point addition
// order. Subtracting prefix sums could change ties between candidate layouts.
type gridRowMeasurement struct {
	start, end    int
	size, withGap float64
}

type gridRowMeasurements struct {
	sizes []float64
	gap   float64
	cache []gridRowMeasurement
}

func newGridRowMeasurements(sizes []float64, gap float64) *gridRowMeasurements {
	// Bound the cache independently of diagram size; collisions only cause a
	// measurement to be recomputed. Small diagrams fit without collisions.
	const maxEntries = 32 * 1024
	entries := 1
	for len(sizes) > 0 && entries < maxEntries && entries/len(sizes) < len(sizes) {
		entries *= 2
	}
	return &gridRowMeasurements{sizes: sizes, gap: gap, cache: make([]gridRowMeasurement, entries)}
}

func (m *gridRowMeasurements) get(start, end int) gridRowMeasurement {
	slot := (uint(start)*uint(len(m.sizes)) + uint(end)) & uint(len(m.cache)-1)
	cached := &m.cache[slot]
	if cached.start == start && cached.end == end {
		return *cached
	}
	size, withGap := 0., 0.
	for _, v := range m.sizes[start:end] {
		size += v
		withGap += v + m.gap
	}
	if start < end {
		withGap -= m.gap
	}
	*cached = gridRowMeasurement{start: start, end: end, size: size, withGap: withGap}
	return *cached
}

// The division and row checks run in their original order, including repeated
// checks: their attempt/skip counts affect when the bounded search terminates.
// The callback borrows cuts only until it returns.
type iterDivision func(division []int) (done bool)
type checkCut func(start, end int, starting bool) (ok bool)

func iterDivisions(nObjects, nCuts int, f iterDivision, check checkCut) {
	if nObjects < 2 || nCuts == 0 {
		return
	}
	cuts := make([]int, nCuts)
	var visit func(end, remaining int) bool
	visit = func(end, remaining int) bool {
		for index := end - 1; index >= remaining; index-- {
			if !check(index, end, false) {
				continue
			}
			cuts[remaining-1] = index - 1
			if remaining > 1 {
				if visit(index, remaining-1) {
					return true
				}
			} else if check(0, index, true) && f(cuts) {
				return true
			}
		}
		return false
	}
	visit(nObjects, nCuts)
}
