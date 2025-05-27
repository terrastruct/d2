package d2ascii

import (
	"testing"

	"oss.terrastruct.com/d2/d2target"
)

func TestSortObjects(t *testing.T) {
	allObjects := []DiagramObject{
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

	expectedOrder := []DiagramObject{
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

	if len(allObjects) != len(expectedOrder) {
		t.Fatal("number of objects changed while sorting")
	}

	for i := 0; i < len(allObjects); i++ {
		if allObjects[i].GetID() != expectedOrder[i].GetID() {
			t.Fatalf("object order differs at index %d, got '%s' expected '%s'", i, allObjects[i].GetID(), expectedOrder[i].GetID())
		}
	}
}

func TestGrid(t *testing.T) {
	grid := NewGrid(5, 3)
	
	// Test initial state
	if grid.Width != 5 || grid.Height != 3 {
		t.Fatalf("Grid dimensions incorrect: got %dx%d, expected 5x3", grid.Width, grid.Height)
	}
	
	// Test setting and getting cells
	grid.SetCell(2, 1, 'X')
	if grid.GetCell(2, 1) != 'X' {
		t.Fatalf("Cell value incorrect: got %c, expected X", grid.GetCell(2, 1))
	}
	
	// Test bounds checking
	grid.SetCell(-1, 0, 'Y') // Should not panic
	grid.SetCell(10, 10, 'Z') // Should not panic
	
	if grid.GetCell(-1, 0) != ' ' {
		t.Fatalf("Out of bounds cell should return space")
	}
}

func TestRender(t *testing.T) {
	// Create a simple diagram with one shape
	diagram := &d2target.Diagram{
		Shapes: []d2target.Shape{
			{
				ID:     "test",
				Type:   d2target.ShapeRectangle,
				Pos:    d2target.Point{X: 10, Y: 10},
				Width:  40,
				Height: 32,
				Text: d2target.Text{
					Label: "Test",
				},
			},
		},
		Connections: []d2target.Connection{},
	}
	
	opts := &RenderOpts{
		Pad: nil, // Use default padding
	}
	
	result, err := Render(diagram, opts)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	
	if len(result) == 0 {
		t.Fatal("Render produced empty result")
	}
	
	// Check that result contains expected characters
	resultStr := string(result)
	if !containsAny(resultStr, []string{"╭", "╮", "╰", "╯", "│", "─"}) {
		t.Fatal("Render result doesn't contain expected box drawing characters")
	}
}

func TestDrawShapes(t *testing.T) {
	grid := NewGrid(20, 10)
	
	// Test rectangle
	shape := d2target.Shape{
		Type:   d2target.ShapeRectangle,
		Text:   d2target.Text{Label: "Rect"},
	}
	drawRectangle(grid, 1, 1, 5, 3, shape)
	
	// Check corners
	if grid.GetCell(1, 1) != '╭' {
		t.Errorf("Top-left corner incorrect: got %c, expected ╭", grid.GetCell(1, 1))
	}
	if grid.GetCell(5, 1) != '╮' {
		t.Errorf("Top-right corner incorrect: got %c, expected ╮", grid.GetCell(5, 1))
	}
	
	// Test circle
	grid2 := NewGrid(20, 10)
	drawCircle(grid2, 5, 2, 6, 4, shape)
	
	// Should have some circle characters
	found := false
	for i := 0; i < grid2.Width; i++ {
		for j := 0; j < grid2.Height; j++ {
			if grid2.GetCell(i, j) == '●' {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("Circle drawing didn't produce expected circle characters")
	}
}

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		for _, r := range s {
			if string(r) == substr {
				return true
			}
		}
	}
	return false
} 