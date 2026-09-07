package d2grid

import (
	"math"

	"github.com/d2lang/d2/d2graph"
)

// Frozen pre-optimization search, used as an exact compatibility oracle.
func (gd *gridDiagram) referenceBestLayout(targetSize float64, columns bool) [][]*d2graph.Object {
	var nCuts int
	if columns {
		nCuts = gd.columns - 1
	} else {
		nCuts = gd.rows - 1
	}
	if nCuts == 0 {
		return GenLayout(gd.objects, nil)
	}

	var bestLayout [][]*d2graph.Object
	bestDist := math.MaxFloat64
	fastIsBest := false
	// try fast layout algorithm as a baseline
	if fastLayout := gd.fastLayout(targetSize, nCuts, columns); fastLayout != nil {
		dist := getDistToTarget(fastLayout, targetSize, float64(gd.horizontalGap), float64(gd.verticalGap), columns)
		if dist == 0 {
			return fastLayout
		}
		bestDist = dist
		bestLayout = fastLayout
		fastIsBest = true
	}

	var gap float64
	if columns {
		gap = float64(gd.verticalGap)
	} else {
		gap = float64(gd.horizontalGap)
	}
	getSize := func(o *d2graph.Object) float64 {
		if columns {
			return o.Height
		} else {
			return o.Width
		}
	}

	sizes := []float64{}
	for _, obj := range gd.objects {
		size := getSize(obj)
		sizes = append(sizes, size)
	}
	sd := stddev(sizes)

	skipCount := 0
	count := 0
	// quickly eliminate bad row groupings
	startingCache := make(map[int]bool)
	// Note: we want a low threshold to explore good options within attemptLimit,
	// but the best option may require a few rows that are far from the target size.
	okThreshold := STARTING_THRESHOLD
	rowOk := func(row []*d2graph.Object, starting bool) (ok bool) {
		if starting {
			// we can cache results from starting positions since they repeat and don't change
			// with starting=true it will always be the 1st N objects based on len(row)
			if ok, has := startingCache[len(row)]; has {
				return ok
			}
			defer func() {
				// cache result before returning
				startingCache[len(row)] = ok
			}()
		}

		rowSize := 0.
		for _, obj := range row {
			rowSize += getSize(obj)
		}
		if len(row) > 1 {
			rowSize += gap * float64(len(row)-1)
			// if multiple nodes are too big, it isn't ok. but a single node can't shrink so only check here
			if rowSize > okThreshold*targetSize {
				skipCount++
				// there may even be too many to skip
				return skipCount >= SKIP_LIMIT
			}
		}
		// row is too small to be good overall
		if rowSize < targetSize/okThreshold {
			skipCount++
			return skipCount >= SKIP_LIMIT
		}
		return true
	}

	// get all options for where to place these cuts, preferring later cuts over earlier cuts
	// with 5 objects and 2 cuts we have these options:
	// .       A   B   C │ D │ E     <- these cuts would produce: ┌A─┐ ┌B─┐ ┌C─┐
	// .       A   B │ C   D │ E                                  └──┘ └──┘ └──┘
	// .       A │ B   C   D │ E                                  ┌D───────────┐
	// .       A   B │ C │ D   E                                  └────────────┘
	// .       A │ B   C │ D   E                                  ┌E───────────┐
	// .       A │ B │ C   D   E                                  └────────────┘
	// of these divisions, find the layout with rows closest to the targetSize
	tryDivision := func(division []int) bool {
		layout := GenLayout(gd.objects, division)
		dist := getDistToTarget(layout, targetSize, float64(gd.horizontalGap), float64(gd.verticalGap), columns)
		if dist < bestDist {
			bestLayout = layout
			bestDist = dist
			fastIsBest = false
		} else if fastIsBest && dist == bestDist {
			// prefer ordered search solution to fast layout solution
			bestLayout = layout
			fastIsBest = false
		}
		count++
		// with few objects we can try all options to get best result but this won't scale, so only try up to 100k options
		return count >= ATTEMPT_LIMIT || skipCount >= SKIP_LIMIT
	}

	// try number of different okThresholds depending on std deviation of sizes
	thresholdAttempts := int(math.Ceil(sd))
	if thresholdAttempts < MIN_THRESHOLD_ATTEMPTS {
		thresholdAttempts = MIN_THRESHOLD_ATTEMPTS
	} else if thresholdAttempts > MAX_THRESHOLD_ATTEMPTS {
		thresholdAttempts = MAX_THRESHOLD_ATTEMPTS
	}
	for i := 0; i < thresholdAttempts || bestLayout == nil; i++ {
		count = 0.
		skipCount = 0.
		referenceIterDivisions(gd.objects, nCuts, tryDivision, rowOk)
		okThreshold += THRESHOLD_STEP_SIZE
		startingCache = make(map[int]bool)
		if skipCount == 0 {
			// threshold isn't skipping anything so increasing it won't help
			break
		}
		// okThreshold isn't high enough yet, we skipped every option so don't count it
		if count == 0 && thresholdAttempts < MAX_THRESHOLD_ATTEMPTS {
			thresholdAttempts++
		}
	}

	return bestLayout
}

func referenceIterDivisions(objects []*d2graph.Object, nCuts int, f iterDivision, check func([]*d2graph.Object, bool) bool) {
	if len(objects) < 2 || nCuts == 0 {
		return
	}
	done := false
	// we go in this order to prefer extra objects in starting rows rather than later ones
	lastObj := len(objects) - 1
	// with objects=[A, B, C, D, E]; nCuts=2
	// d:depth; i:index; n:nCuts;
	// ┌────┬───┬───┬─────────────────────┬────────────┐
	// │ d  │ i │ n │ objects             │ cuts       │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ 0  │ 4 │ 2 │ [A   B   C   D | E] │            │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 3 │ 1 │ [A   B   C | D]     │ + | E]     │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 2 │ 1 │ [A   B | C   D]     │ + | E]     │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 1 │ 1 │ [A | B   C   D]     │ + | E]     │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ 0  │ 3 │ 2 │ [A   B   C | D   E] │            │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 2 │ 1 │ [A   B | C]         │ + | D E]   │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 1 │ 1 │ [A | B   C]         │ + | D E]   │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ 0  │ 2 │ 2 │ [A   B | C   D   E] │            │
	// ├────┼───┼───┼─────────────────────┼────────────┤
	// │ └1 │ 1 │ 1 │ [A | B]             │ + | C D E] │
	// └────┴───┴───┴─────────────────────┴────────────┘
	for index := lastObj; index >= nCuts; index-- {
		if !check(objects[index:], false) {
			// optimization: if current cut gives a bad grouping, don't recurse
			continue
		}
		if nCuts > 1 {
			referenceIterDivisions(objects[:index], nCuts-1, func(inner []int) bool {
				done = f(append(inner, index-1))
				return done
			}, check)
		} else {
			if !check(objects[:index], true) {
				// e.g. [A   B   C | D] if [A,B,C] is bad, skip it
				continue
			}
			done = f([]int{index - 1})
		}
		if done {
			return
		}
	}
}
