package d2grid

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	CONTAINER_PADDING = 60
	DEFAULT_GAP       = 40
)

// Layout runs the grid layout on containers with rows/columns
// Note: children are not allowed edges or descendants
//
// 1. Traverse graph from root, skip objects with no rows/columns
// 2. Construct a grid with the container children
// 3. Remove the children from the main graph
// 4. Run grid layout
// 5. Set the resulting dimensions to the main graph shape
// 6. Run core layouts (without grid children)
// 7. Put grid children back in correct location
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) d2graph.LayoutGraph {
	return func(ctx context.Context, g *d2graph.Graph) error {
		gridDiagrams, objectOrder, err := withoutGridDiagrams(ctx, g, layout)
		if err != nil {
			return err
		}

		if g.Root.IsGridDiagram() && len(g.Root.ChildrenArray) != 0 {
			g.Root.TopLeft = geo.NewPoint(0, 0)
		} else if err := layout(ctx, g); err != nil {
			return err
		}

		cleanup(g, gridDiagrams, objectOrder)
		return nil
	}
}

func withoutGridDiagrams(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) (gridDiagrams map[string]*gridDiagram, objectOrder map[string]int, err error) {
	toRemove := make(map[*d2graph.Object]struct{})
	gridDiagrams = make(map[string]*gridDiagram)

	objectOrder = make(map[string]int)
	for i, obj := range g.Objects {
		objectOrder[obj.AbsID()] = i
	}

	var processGrid func(obj *d2graph.Object) error
	processGrid = func(obj *d2graph.Object) error {
		for _, child := range obj.ChildrenArray {
			if child.IsGridDiagram() {
				if err := processGrid(child); err != nil {
					return err
				}
			} else if len(child.ChildrenArray) > 0 {
				tempGraph := g.ExtractAsNestedGraph(child)
				// emulating setting font size with layout
				fontSize := child.Text().FontSize
				if child.Style.FontSize == nil {
					child.Style.FontSize = &d2graph.Scalar{}
				}
				child.Style.FontSize.Value = strconv.Itoa(fontSize)
				if err := layout(ctx, tempGraph); err != nil {
					return err
				}
				g.InjectNestedGraph(tempGraph, obj)

				sort.SliceStable(g.Objects, func(i, j int) bool {
					return objectOrder[g.Objects[i].AbsID()] < objectOrder[g.Objects[j].AbsID()]
				})
				sort.SliceStable(child.ChildrenArray, func(i, j int) bool {
					return objectOrder[child.ChildrenArray[i].AbsID()] < objectOrder[child.ChildrenArray[j].AbsID()]
				})
				sort.SliceStable(obj.ChildrenArray, func(i, j int) bool {
					return objectOrder[obj.ChildrenArray[i].AbsID()] < objectOrder[obj.ChildrenArray[j].AbsID()]
				})

				for _, o := range tempGraph.Objects {
					toRemove[o] = struct{}{}
				}
			}
		}

		gd, err := layoutGrid(g, obj)
		if err != nil {
			return err
		}
		obj.Children = make(map[string]*d2graph.Object)
		obj.ChildrenArray = nil

		if obj.Box != nil {
			// CONTAINER_PADDING is default, but use gap value if set
			horizontalPadding, verticalPadding := CONTAINER_PADDING, CONTAINER_PADDING
			if obj.GridGap != nil || obj.HorizontalGap != nil {
				horizontalPadding = gd.horizontalGap
			}
			if obj.GridGap != nil || obj.VerticalGap != nil {
				verticalPadding = gd.verticalGap
			}

			// size shape according to grid
			obj.SizeToContent(gd.width, gd.height, float64(2*horizontalPadding), float64(2*verticalPadding))

			// compute where the grid should be placed inside shape
			s := obj.ToShape()
			innerBox := s.GetInnerBox()
			if innerBox.TopLeft.X != 0 || innerBox.TopLeft.Y != 0 {
				gd.shift(innerBox.TopLeft.X, innerBox.TopLeft.Y)
			}

			// compute how much space the label and icon occupy
			var occupiedWidth, occupiedHeight float64
			if obj.Icon != nil {
				iconSpace := float64(d2target.MAX_ICON_SIZE + 2*label.PADDING)
				occupiedWidth = iconSpace
				occupiedHeight = iconSpace
			}

			var dx, dy float64
			if obj.LabelDimensions.Height != 0 {
				occupiedHeight = math.Max(
					occupiedHeight,
					float64(obj.LabelDimensions.Height)+2*label.PADDING,
				)
			}
			if obj.LabelDimensions.Width != 0 {
				// . ├────┤───────├────┤
				// .  icon  label  icon
				// with an icon in top left we need 2x the space to fit the label in the center
				occupiedWidth *= 2
				occupiedWidth += float64(obj.LabelDimensions.Width) + 2*label.PADDING
				if occupiedWidth > obj.Width {
					dx = (occupiedWidth - obj.Width) / 2
					obj.Width = occupiedWidth
				}
			}

			// also check for grid cells with outside top labels or icons
			// the first grid object is at the top (and always exists)
			topY := gd.objects[0].TopLeft.Y
			highestOutside := topY
			for _, o := range gd.objects {
				// we only want to compute label positions for objects at the top of the grid
				if o.TopLeft.Y > topY {
					if gd.rowDirected {
						// if the grid is rowDirected (row1, row2, etc) we can stop after finishing the first row
						break
					} else {
						// otherwise we continue until the next column
						continue
					}
				}
				if o.LabelPosition != nil {
					labelPosition := label.Position(*o.LabelPosition)
					if labelPosition.IsOutside() {
						labelTL := o.GetLabelTopLeft()
						if labelTL.Y < highestOutside {
							highestOutside = labelTL.Y
						}
					}
				}
				if o.IconPosition != nil {
					switch label.Position(*o.IconPosition) {
					case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
						iconSpace := float64(d2target.MAX_ICON_SIZE + label.PADDING)
						if topY-iconSpace < highestOutside {
							highestOutside = topY - iconSpace
						}
					}
				}
			}
			if highestOutside < topY {
				occupiedHeight += topY - highestOutside + 2*label.PADDING
			}
			if occupiedHeight > float64(verticalPadding) {
				// if the label doesn't fit within the padding, we need to add more
				dy = occupiedHeight - float64(verticalPadding)
				obj.Height += dy
			}

			// we need to center children if we have to expand to fit the container label
			if dx != 0 || dy != 0 {
				gd.shift(dx, dy)
			}
		}

		if obj.HasLabel() {
			obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		}
		if obj.Icon != nil {
			obj.IconPosition = go2.Pointer(string(label.InsideTopLeft))
		}
		gridDiagrams[obj.AbsID()] = gd

		for _, o := range gd.objects {
			toRemove[o] = struct{}{}
		}
		return nil
	}

	if len(g.Objects) > 0 {
		queue := make([]*d2graph.Object, 1, len(g.Objects))
		queue[0] = g.Root
		for len(queue) > 0 {
			obj := queue[0]
			queue = queue[1:]
			if len(obj.ChildrenArray) == 0 {
				continue
			}
			if !obj.IsGridDiagram() {
				queue = append(queue, obj.ChildrenArray...)
				continue
			}

			if err := processGrid(obj); err != nil {
				return nil, nil, err
			}
		}
	}

	layoutObjects := make([]*d2graph.Object, 0, len(toRemove))
	for _, obj := range g.Objects {
		if _, exists := toRemove[obj]; !exists {
			layoutObjects = append(layoutObjects, obj)
		}
	}
	g.Objects = layoutObjects

	return gridDiagrams, objectOrder, nil
}

func layoutGrid(g *d2graph.Graph, obj *d2graph.Object) (*gridDiagram, error) {
	gd := newGridDiagram(obj)

	if gd.rows != 0 && gd.columns != 0 {
		gd.layoutEvenly(g, obj)
	} else {
		gd.layoutDynamic(g, obj)
	}

	// position labels and icons
	for _, o := range gd.objects {
		if o.Icon != nil {
			// don't overwrite position if nested graph layout positioned label/icon
			if o.LabelPosition == nil {
				o.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			}
			if o.IconPosition == nil {
				o.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
			}
		} else {
			if o.LabelPosition == nil {
				o.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			}
		}
	}

	return gd, nil
}

func (gd *gridDiagram) layoutEvenly(g *d2graph.Graph, obj *d2graph.Object) {
	// layout objects in a grid with these 2 properties:
	// all objects in the same row should have the same height
	// all objects in the same column should have the same width

	getObject := func(rowIndex, columnIndex int) *d2graph.Object {
		var index int
		if gd.rowDirected {
			index = rowIndex*gd.columns + columnIndex
		} else {
			index = columnIndex*gd.rows + rowIndex
		}
		if index < len(gd.objects) {
			return gd.objects[index]
		}
		return nil
	}

	rowHeights := make([]float64, 0, gd.rows)
	colWidths := make([]float64, 0, gd.columns)
	for i := 0; i < gd.rows; i++ {
		rowHeight := 0.
		for j := 0; j < gd.columns; j++ {
			o := getObject(i, j)
			if o == nil {
				break
			}
			rowHeight = math.Max(rowHeight, o.Height)
		}
		rowHeights = append(rowHeights, rowHeight)
	}
	for j := 0; j < gd.columns; j++ {
		columnWidth := 0.
		for i := 0; i < gd.rows; i++ {
			o := getObject(i, j)
			if o == nil {
				break
			}
			columnWidth = math.Max(columnWidth, o.Width)
		}
		colWidths = append(colWidths, columnWidth)
	}

	horizontalGap := float64(gd.horizontalGap)
	verticalGap := float64(gd.verticalGap)

	cursor := geo.NewPoint(0, 0)
	if gd.rowDirected {
		for i := 0; i < gd.rows; i++ {
			for j := 0; j < gd.columns; j++ {
				o := getObject(i, j)
				if o == nil {
					break
				}
				o.Width = colWidths[j]
				o.Height = rowHeights[i]
				o.MoveWithDescendantsTo(cursor.X, cursor.Y)
				cursor.X += o.Width + horizontalGap
			}
			cursor.X = 0
			cursor.Y += rowHeights[i] + verticalGap
		}
	} else {
		for j := 0; j < gd.columns; j++ {
			for i := 0; i < gd.rows; i++ {
				o := getObject(i, j)
				if o == nil {
					break
				}
				o.Width = colWidths[j]
				o.Height = rowHeights[i]
				o.MoveWithDescendantsTo(cursor.X, cursor.Y)
				cursor.Y += o.Height + verticalGap
			}
			cursor.X += colWidths[j] + horizontalGap
			cursor.Y = 0
		}
	}

	var totalWidth, totalHeight float64
	for _, w := range colWidths {
		totalWidth += w + horizontalGap
	}
	for _, h := range rowHeights {
		totalHeight += h + verticalGap
	}
	totalWidth -= horizontalGap
	totalHeight -= verticalGap
	gd.width = totalWidth
	gd.height = totalHeight
}

func (gd *gridDiagram) layoutDynamic(g *d2graph.Graph, obj *d2graph.Object) {
	// assume we have the following objects to layout:
	// . ┌A──────────────┐  ┌B──┐  ┌C─────────┐  ┌D────────┐  ┌E────────────────┐
	// . └───────────────┘  │   │  │          │  │         │  │                 │
	// .                    │   │  └──────────┘  │         │  │                 │
	// .                    │   │                │         │  └─────────────────┘
	// .                    └───┘                │         │
	// .                                         └─────────┘
	// Note: if the grid is row dominant, all objects should be the same height (same width if column dominant)
	// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐  ┌D────────┐  ┌E────────────────┐
	// . ├ ─ ─ ─ ─ ─ ─ ─┤  │   │  │          │  │         │  │                 │
	// . │              │  │   │  ├ ─ ─ ─ ─ ─┤  │         │  │                 │
	// . │              │  │   │  │          │  │         │  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤
	// . │              │  ├ ─ ┤  │          │  │         │  │                 │
	// . └──────────────┘  └───┘  └──────────┘  └─────────┘  └─────────────────┘

	horizontalGap := float64(gd.horizontalGap)
	verticalGap := float64(gd.verticalGap)

	// we want to split up the total width across the N rows or columns as evenly as possible
	var totalWidth, totalHeight float64
	for _, o := range gd.objects {
		totalWidth += o.Width
		totalHeight += o.Height
	}
	totalWidth += horizontalGap * float64(len(gd.objects)-gd.rows)
	totalHeight += verticalGap * float64(len(gd.objects)-gd.columns)

	var layout [][]*d2graph.Object
	if gd.rowDirected {
		targetWidth := totalWidth / float64(gd.rows)
		layout = gd.getBestLayout(targetWidth, false)
	} else {
		targetHeight := totalHeight / float64(gd.columns)
		layout = gd.getBestLayout(targetHeight, true)
	}

	cursor := geo.NewPoint(0, 0)
	var maxY, maxX float64
	if gd.rowDirected {
		// measure row widths
		rowWidths := []float64{}
		for _, row := range layout {
			x := 0.
			for _, o := range row {
				x += o.Width + horizontalGap
			}
			rowWidth := x - horizontalGap
			rowWidths = append(rowWidths, rowWidth)
			maxX = math.Max(maxX, rowWidth)
		}

		// TODO if object is a nested grid, consider growing descendants according to the inner grid layout

		// then expand thinnest objects to make each row the same width
		// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐ ┬ maxHeight(A,B,C)
		// . │              │  │   │  │          │ │
		// . │              │  │   │  │          │ │
		// . │              │  │   │  │          │ │
		// . └──────────────┘  └───┘  └──────────┘ ┴
		// . ┌D────────┬────┐  ┌E────────────────┐ ┬ maxHeight(D,E)
		// . │              │  │                 │ │
		// . │         │    │  │                 │ │
		// . │              │  │                 │ │
		// . │         │    │  │                 │ │
		// . └─────────┴────┘  └─────────────────┘ ┴
		for i, row := range layout {
			rowWidth := rowWidths[i]
			if rowWidth == maxX {
				continue
			}
			delta := maxX - rowWidth
			var widest float64
			for _, o := range row {
				widest = math.Max(widest, o.Width)
			}
			diffs := make([]float64, len(row))
			totalDiff := 0.
			for i, o := range row {
				diffs[i] = widest - o.Width
				totalDiff += diffs[i]
			}
			if totalDiff > 0 {
				// expand smaller nodes up to the size of the larger ones with delta
				// percentage diff
				for i := range diffs {
					diffs[i] /= totalDiff
				}
				growth := math.Min(delta, totalDiff)
				// expand smaller objects to fill remaining space
				for i, o := range row {
					o.Width += diffs[i] * growth
				}
			}
			if delta > totalDiff {
				growth := (delta - totalDiff) / float64(len(row))
				for _, o := range row {
					o.Width += growth
				}
			}
		}

		// if we have 2 rows, then each row's objects should have the same height
		// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐ ┬ maxHeight(A,B,C)
		// . ├ ─ ─ ─ ─ ─ ─ ─┤  │   │  │          │ │
		// . │              │  │   │  ├ ─ ─ ─ ─ ─┤ │
		// . │              │  │   │  │          │ │
		// . └──────────────┘  └───┘  └──────────┘ ┴
		// . ┌D────────┐  ┌E────────────────┐ ┬ maxHeight(D,E)
		// . │         │  │                 │ │
		// . │         │  │                 │ │
		// . │         │  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤ │
		// . │         │  │                 │ │
		// . └─────────┘  └─────────────────┘ ┴
		for _, row := range layout {
			rowHeight := 0.
			for _, o := range row {
				o.MoveWithDescendantsTo(cursor.X, cursor.Y)
				cursor.X += o.Width + horizontalGap
				rowHeight = math.Max(rowHeight, o.Height)
			}

			// set all objects in row to the same height
			for _, o := range row {
				o.Height = rowHeight
			}

			// new row
			cursor.X = 0
			cursor.Y += rowHeight + verticalGap
		}
		maxY = cursor.Y - horizontalGap
	} else {
		// measure column heights
		colHeights := []float64{}
		for _, column := range layout {
			y := 0.
			for _, o := range column {
				y += o.Height + verticalGap
			}
			colHeight := y - verticalGap
			colHeights = append(colHeights, colHeight)
			maxY = math.Max(maxY, colHeight)
		}

		// then expand shortest objects to make each column the same height
		// . ├maxWidth(A,B)─┤  ├maxW(C,D)─┤  ├maxWidth(E)──────┤
		// . ┌A─────────────┐  ┌C─────────┐  ┌E────────────────┐
		// . ├ ─ ─ ─ ─ ─ ─  ┤  │          │  │                 │
		// . │              │  └──────────┘  │                 │
		// . └──────────────┘  ┌D─────────┐  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤
		// . ┌B─────────────┐  │          │  │                 │
		// . │              │  │          │  │                 │
		// . │              │  │          │  │                 │
		// . │              │  │          │  │                 │
		// . └──────────────┘  └──────────┘  └─────────────────┘
		for i, column := range layout {
			colHeight := colHeights[i]
			if colHeight == maxY {
				continue
			}
			delta := maxY - colHeight
			var tallest float64
			for _, o := range column {
				tallest = math.Max(tallest, o.Height)
			}
			diffs := make([]float64, len(column))
			totalDiff := 0.
			for i, o := range column {
				diffs[i] = tallest - o.Height
				totalDiff += diffs[i]
			}
			if totalDiff > 0 {
				// expand smaller nodes up to the size of the larger ones with delta
				// percentage diff
				for i := range diffs {
					diffs[i] /= totalDiff
				}
				growth := math.Min(delta, totalDiff)
				// expand smaller objects to fill remaining space
				for i, o := range column {
					o.Height += diffs[i] * growth
				}
			}
			if delta > totalDiff {
				growth := (delta - totalDiff) / float64(len(column))
				for _, o := range column {
					o.Height += growth
				}
			}
		}
		// if we have 3 columns, then each column's objects should have the same width
		// . ├maxWidth(A,B)─┤  ├maxW(C,D)─┤  ├maxWidth(E)──────┤
		// . ┌A─────────────┐  ┌C─────────┐  ┌E────────────────┐
		// . └──────────────┘  │          │  │                 │
		// . ┌B──┬──────────┐  └──────────┘  │                 │
		// . │              │  ┌D────────┬┐  └─────────────────┘
		// . │   │          │  │          │
		// . │              │  │         ││
		// . └───┴──────────┘  │          │
		// .                   │         ││
		// .                   └─────────┴┘
		for _, column := range layout {
			colWidth := 0.
			for _, o := range column {
				o.MoveWithDescendantsTo(cursor.X, cursor.Y)
				cursor.Y += o.Height + verticalGap
				colWidth = math.Max(colWidth, o.Width)
			}
			// set all objects in column to the same width
			for _, o := range column {
				o.Width = colWidth
			}

			// new column
			cursor.Y = 0
			cursor.X += colWidth + horizontalGap
		}
		maxX = cursor.X - horizontalGap
	}
	gd.width = maxX
	gd.height = maxY
}

// generate the best layout of objects aiming for each row to be the targetSize width
// if columns is true, each column aims to have the targetSize height
func (gd *gridDiagram) getBestLayout(targetSize float64, columns bool) [][]*d2graph.Object {
	debug := false
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
		if debug {
			fmt.Printf("fast dist %v dist per row %v\n", dist, dist/(float64(nCuts)+1))
		}
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
	if debug {
		fmt.Printf("sizes (%d): %v\n", len(sizes), sizes)
		fmt.Printf("std dev: %v; targetSize %v\n", sd, targetSize)
	}

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
				if skipCount >= SKIP_LIMIT {
					// there may even be too many to skip
					return true
				}
				return false
			}
		}
		// row is too small to be good overall
		if rowSize < targetSize/okThreshold {
			skipCount++
			if skipCount >= SKIP_LIMIT {
				return true
			}
			return false
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
		iterDivisions(gd.objects, nCuts, tryDivision, rowOk)
		okThreshold += THRESHOLD_STEP_SIZE
		if debug {
			fmt.Printf("count %d, skip count %d, bestDist %v increasing ok threshold to %v\n", count, skipCount, bestDist, okThreshold)
		}
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

	if debug {
		fmt.Printf("best layout: %v\n", layoutString(bestLayout, sizes))
	}
	return bestLayout
}

func sum(values []float64) float64 {
	s := 0.
	for _, v := range values {
		s += v
	}
	return s
}

func avg(values []float64) float64 {
	return sum(values) / float64(len(values))
}

func variance(values []float64) float64 {
	mean := avg(values)
	total := 0.
	for _, value := range values {
		dev := mean - value
		total += dev * dev
	}
	return total / float64(len(values))
}

func stddev(values []float64) float64 {
	return math.Sqrt(variance(values))
}

func (gd *gridDiagram) fastLayout(targetSize float64, nCuts int, columns bool) (layout [][]*d2graph.Object) {
	var gap float64
	if columns {
		gap = float64(gd.verticalGap)
	} else {
		gap = float64(gd.horizontalGap)
	}

	debt := 0.
	fastDivision := make([]int, 0, nCuts)
	rowSize := 0.
	for i := 0; i < len(gd.objects); i++ {
		o := gd.objects[i]
		var size float64
		if columns {
			size = o.Height
		} else {
			size = o.Width
		}
		if rowSize == 0 {
			// if a single object meets the target size, end the row here
			if size > targetSize-debt {
				// cut row with just this object
				fastDivision = append(fastDivision, i)
				// we build up a debt of distance past the target size across rows
				newDebt := size - targetSize
				debt += newDebt
			} else {
				rowSize += size
			}
			continue
		}
		// debt is paid by decreasing threshold to start new row and ending below targetSize
		if rowSize+gap+(size)/2. > targetSize-debt {
			// start a new row before this object since it is mostly past the target size
			// .              size
			// ├...row─┼gap┼───┼───┤
			// ├──targetSize──┤ (debt=0)
			fastDivision = append(fastDivision, i-1)
			newDebt := rowSize - targetSize
			debt += newDebt
			rowSize = size
		} else {
			rowSize += gap + size
		}
	}
	if len(fastDivision) == nCuts {
		layout = GenLayout(gd.objects, fastDivision)
	}

	return layout
}

func layoutString(layout [][]*d2graph.Object, sizes []float64) string {
	buf := &bytes.Buffer{}
	i := 0
	fmt.Fprintf(buf, "[\n")
	for _, r := range layout {
		vals := sizes[i : i+len(r)]
		fmt.Fprintf(buf, "%v:\t%v\n", sum(vals), vals)
		i += len(r)
	}
	fmt.Fprintf(buf, "]\n")
	return buf.String()
}

// process current division, return true to stop iterating
type iterDivision func(division []int) (done bool)
type checkCut func(objects []*d2graph.Object, starting bool) (ok bool)

// get all possible divisions of objects by the number of cuts
func iterDivisions(objects []*d2graph.Object, nCuts int, f iterDivision, check checkCut) {
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
			iterDivisions(objects[:index], nCuts-1, func(inner []int) bool {
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

// generate a grid of objects from the given cut indices
// each cut index applies after the object at that index
// e.g. [0 1 2 3 4 5 6 7] with cutIndices [0, 2, 6] => [[0], [1, 2], [3,4,5,6], [7]]
func GenLayout(objects []*d2graph.Object, cutIndices []int) [][]*d2graph.Object {
	layout := make([][]*d2graph.Object, len(cutIndices)+1)
	objIndex := 0
	for i := 0; i <= len(cutIndices); i++ {
		var stop int
		if i < len(cutIndices) {
			stop = cutIndices[i]
		} else {
			stop = len(objects) - 1
		}
		if stop >= objIndex {
			layout[i] = make([]*d2graph.Object, 0, stop-objIndex+1)
		}
		for ; objIndex <= stop; objIndex++ {
			layout[i] = append(layout[i], objects[objIndex])
		}
	}
	return layout
}

func getDistToTarget(layout [][]*d2graph.Object, targetSize float64, horizontalGap, verticalGap float64, columns bool) float64 {
	totalDelta := 0.
	for _, row := range layout {
		rowSize := 0.
		for _, o := range row {
			if columns {
				rowSize += o.Height + verticalGap
			} else {
				rowSize += o.Width + horizontalGap
			}
		}
		if len(row) > 0 {
			if columns {
				rowSize -= verticalGap
			} else {
				rowSize -= horizontalGap
			}
		}
		totalDelta += math.Abs(rowSize - targetSize)
	}
	return totalDelta
}

// cleanup restores the graph after the core layout engine finishes
// - translating the grid to its position placed by the core layout engine
// - restore the children of the grid
// - sorts objects to their original graph order
func cleanup(graph *d2graph.Graph, gridDiagrams map[string]*gridDiagram, objectsOrder map[string]int) {
	defer func() {
		sort.SliceStable(graph.Objects, func(i, j int) bool {
			return objectsOrder[graph.Objects[i].AbsID()] < objectsOrder[graph.Objects[j].AbsID()]
		})
	}()

	var restore func(obj *d2graph.Object)
	restore = func(obj *d2graph.Object) {
		gd, exists := gridDiagrams[obj.AbsID()]
		if !exists {
			return
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))

		horizontalPadding, verticalPadding := CONTAINER_PADDING, CONTAINER_PADDING
		if obj.GridGap != nil || obj.HorizontalGap != nil {
			horizontalPadding = gd.horizontalGap
		}
		if obj.GridGap != nil || obj.VerticalGap != nil {
			verticalPadding = gd.verticalGap
		}

		// shift the grid from (0, 0)
		gd.shift(
			obj.TopLeft.X+float64(horizontalPadding),
			obj.TopLeft.Y+float64(verticalPadding),
		)
		gd.cleanup(obj, graph)

		for _, child := range obj.ChildrenArray {
			restore(child)
		}
	}

	if graph.Root.IsGridDiagram() {
		gd, exists := gridDiagrams[graph.Root.AbsID()]
		if exists {
			gd.cleanup(graph.Root, graph)
		}
	}

	for _, obj := range graph.Objects {
		restore(obj)
	}
}
