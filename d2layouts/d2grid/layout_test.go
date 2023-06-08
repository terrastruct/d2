package d2grid_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2grid"
)

func TestGenLayout(t *testing.T) {
	objects := []*d2graph.Object{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
		{ID: "4"},
		{ID: "5"},
		{ID: "6"},
		{ID: "7"},
		{ID: "8"},
	}
	var cutIndices []int
	var layout [][]*d2graph.Object
	cutIndices = []int{0}
	layout = d2grid.GenLayout(objects, cutIndices)
	fmt.Printf("layout %v\n", len(layout))
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 2 rows from 1 cut")
	assert.Equalf(t, 1, len(layout[0]), "expected first row to be 1 object")
	assert.Equalf(t, 7, len(layout[1]), "expected second row to be 7 objects")
	assert.Equalf(t, objects[0].ID, layout[0][0].ID, "expected first object to be 1")

	cutIndices = []int{6}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 2 rows from 1 cut")
	assert.Equalf(t, 7, len(layout[0]), "expected first row to be 7 objects")
	assert.Equalf(t, 1, len(layout[1]), "expected second row to be 1 object")

	cutIndices = []int{0, 6}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 3 rows from 2 cuts")
	assert.Equalf(t, 1, len(layout[0]), "expected first row to be 1 objects")
	assert.Equalf(t, 6, len(layout[1]), "expected second row to be 6 objects")
	assert.Equalf(t, 1, len(layout[2]), "expected second row to be 1 object")

	cutIndices = []int{1, 5}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 3 rows from 2 cuts")
	assert.Equalf(t, 2, len(layout[0]), "expected first row to be 2 objects")
	assert.Equalf(t, 4, len(layout[1]), "expected second row to be 6 objects")
	assert.Equalf(t, 2, len(layout[2]), "expected second row to be 2 object")

	cutIndices = []int{5}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 2 rows from 1 cut")
	assert.Equalf(t, 6, len(layout[0]), "expected first row to be 6 objects")
	assert.Equalf(t, 2, len(layout[1]), "expected second row to be 2 object")

	cutIndices = []int{1}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 2 rows from 1 cut")
	assert.Equalf(t, 2, len(layout[0]), "expected first row to be 2 object")
	assert.Equalf(t, 6, len(layout[1]), "expected second row to be 6 objects")

	cutIndices = []int{0, 1, 2, 3, 4, 5, 6}
	layout = d2grid.GenLayout(objects, cutIndices)
	assert.Equalf(t, len(cutIndices)+1, len(layout), "expected 3 rows from 2 cuts")
	for i := range layout {
		assert.Equalf(t, 1, len(layout[i]), "expected row %d to be 1 object", i)
	}
}
