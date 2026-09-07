package nodeshape

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"
)

func TestSnapPointPercentages(t *testing.T) {
	table := shapeTable{numColumns: 4}

	snapPointsPercentages := table.SnapPointPercentages()
	assert.Equal(t, 3, len(snapPointsPercentages[0])) // 3 snap points above
	assert.Equal(t, 4, len(snapPointsPercentages[1])) // 4 (NumRows) snap points on the left
	assert.Equal(t, 3, len(snapPointsPercentages[2])) // 3 snap points below
	assert.Equal(t, 4, len(snapPointsPercentages[3])) // 4 (NumRows) snap points on the right

	// left
	assert.Equal(t, 0.3, snapPointsPercentages[1][0].YPercentage)
	assert.Equal(t, 0.5, snapPointsPercentages[1][1].YPercentage)
	assert.Equal(t, 0.7, snapPointsPercentages[1][2].YPercentage)
	assert.Equal(t, 0.9, snapPointsPercentages[1][3].YPercentage)

	// right
	assert.Equal(t, 0.3, snapPointsPercentages[3][0].YPercentage)
	assert.Equal(t, 0.5, snapPointsPercentages[3][1].YPercentage)
	assert.Equal(t, 0.7, snapPointsPercentages[3][2].YPercentage)
	assert.Equal(t, 0.9, snapPointsPercentages[3][3].YPercentage)
}

func TestSnapPointPercentagesWithoutColumns(t *testing.T) {
	table := shapeTable{numColumns: 0}

	snapPointsPercentages := table.SnapPointPercentages()
	assert.Equal(t, 3, len(snapPointsPercentages[0]))
	assert.Equal(t, 1, len(snapPointsPercentages[1]))
	assert.Equal(t, 3, len(snapPointsPercentages[2]))
	assert.Equal(t, 1, len(snapPointsPercentages[3]))

	// left
	assert.Equal(t, 0.5, snapPointsPercentages[1][0].YPercentage)
	assert.Equal(t, 0., snapPointsPercentages[1][0].XPercentage)

	// right
	assert.Equal(t, 0.5, snapPointsPercentages[3][0].YPercentage)
	assert.Equal(t, 1.0, snapPointsPercentages[3][0].XPercentage)
}

func TestCenterPortIndices(t *testing.T) {
	table := shapeTable{numColumns: 4}

	indices := table.CenterPortIndices()
	assert.Equal(t, 0, len(indices))

	assert.Equal(t, -1, table.CenterPortIndex(geo.Top))
	assert.Equal(t, -1, table.CenterPortIndex(geo.Left))
	assert.Equal(t, -1, table.CenterPortIndex(geo.Bottom))
	assert.Equal(t, -1, table.CenterPortIndex(geo.Right))
}

func TestCenterPortIndicesWithoutColumns(t *testing.T) {
	table := shapeTable{numColumns: 0}

	indices := table.CenterPortIndices()
	assert.Equal(t, 1, indices[0]) // top
	assert.Equal(t, 3, indices[1]) // left
	assert.Equal(t, 5, indices[2]) // bottom
	assert.Equal(t, 7, indices[3]) // right

	assert.Equal(t, 1, table.CenterPortIndex(geo.Top))
	assert.Equal(t, 3, table.CenterPortIndex(geo.Left))
	assert.Equal(t, 5, table.CenterPortIndex(geo.Bottom))
	assert.Equal(t, 7, table.CenterPortIndex(geo.Right))
}

func TestMirroredPortIndices(t *testing.T) {
	table := shapeTable{numColumns: 4}

	assert.Equal(t, 0, len(table.MirroredPortIndices()))
}

func TestMirroredPortIndicesWithoutColumns(t *testing.T) {
	table := shapeTable{numColumns: 0}

	mirrored := table.MirroredPortIndices()
	assert.Equal(t, 5, mirrored[1])
	assert.Equal(t, 1, mirrored[5])
	assert.Equal(t, 3, mirrored[7])
	assert.Equal(t, 7, mirrored[3])
}

func TestPortIndices(t *testing.T) {
	table := shapeTable{numColumns: 5}

	assert.Equal(t, []int{0, 1, 2}, table.PortIndices(geo.Top))
	assert.Equal(t, []int{3, 4, 5, 6, 7}, table.PortIndices(geo.Left))
	assert.Equal(t, []int{8, 9, 10}, table.PortIndices(geo.Bottom))
	assert.Equal(t, []int{11, 12, 13, 14, 15}, table.PortIndices(geo.Right))

	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7}, table.PortIndices(geo.TopLeft))
	assert.Equal(t, []int{0, 1, 2, 11, 12, 13, 14, 15}, table.PortIndices(geo.TopRight))
	assert.Equal(t, []int{8, 9, 10, 3, 4, 5, 6, 7}, table.PortIndices(geo.BottomLeft))
	assert.Equal(t, []int{8, 9, 10, 11, 12, 13, 14, 15}, table.PortIndices(geo.BottomRight))
}

func TestPortIndicesWithoutColumns(t *testing.T) {
	table := shapeTable{numColumns: 0}

	assert.Equal(t, []int{0, 1, 2}, table.PortIndices(geo.Top))
	assert.Equal(t, []int{3}, table.PortIndices(geo.Left)) // header
	assert.Equal(t, []int{4, 5, 6}, table.PortIndices(geo.Bottom))
	assert.Equal(t, []int{7}, table.PortIndices(geo.Right)) // header
}
