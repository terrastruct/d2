package d2svg

import (
	"testing"

	"oss.terrastruct.com/d2/d2target"
)

func TestSortObjects(t *testing.T) {
	allObjects := []d2target.DiagramObject{
		// same zIndex and level, should keep in this order
		d2target.Shape{
			ID:     "0",
			ZIndex: 0,
			Level:  0,
		},
		d2target.Shape{
			ID:     "1",
			ZIndex: 0,
			Level:  0,
		},
		// same zIndex, different level, should be swapped
		d2target.Shape{
			ID:     "2",
			ZIndex: 0,
			Level:  1,
		},
		d2target.Shape{
			ID:     "3",
			ZIndex: 0,
			Level:  0,
		},
		// different zIndex, should come after connections
		d2target.Shape{
			ID:     "4",
			ZIndex: 1,
			Level:  0,
		},
		// connections come after shapes
		d2target.Connection{
			ID:     "5",
			ZIndex: 0,
		},
		d2target.Connection{
			ID:     "6",
			ZIndex: 0,
		},
		// this should be last object
		d2target.Connection{
			ID:     "7",
			ZIndex: 2,
		},
		// this should be the first object
		d2target.Connection{
			ID:     "8",
			ZIndex: -1,
		},
	}

	expectedOrder := []d2target.DiagramObject{
		allObjects[8],
		allObjects[0],
		allObjects[1],
		allObjects[3],
		allObjects[2],
		allObjects[5],
		allObjects[6],
		allObjects[4],
		allObjects[7],
	}

	sortObjects(allObjects)

	for i := 0; i < len(allObjects); i++ {
		if allObjects[i].GetID() != expectedOrder[i].GetID() {
			t.Fatalf("object order differs at index %d, got '%s' expected '%s'", i, allObjects[i].GetID(), expectedOrder[i].GetID())
		}
	}
}
