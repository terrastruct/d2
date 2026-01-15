package d2svg

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

func TestEscapeClassNames(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "no special characters",
			input:    []string{"foo", "bar", "baz"},
			expected: []string{"foo", "bar", "baz"},
		},
		{
			name:     "with double quotes",
			input:    []string{"test label: \"Hello World\""},
			expected: []string{"test label: &#34;Hello World&#34;"},
		},
		{
			name:     "with single quotes",
			input:    []string{"test's value"},
			expected: []string{"test&#39;s value"},
		},
		{
			name:     "with angle brackets",
			input:    []string{"<script>", "foo&bar"},
			expected: []string{"&lt;script&gt;", "foo&amp;bar"},
		},
		{
			name:     "mixed classes",
			input:    []string{"normal", "has \"quotes\"", "also-normal"},
			expected: []string{"normal", "has &#34;quotes&#34;", "also-normal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeClassNames(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, expected %d", len(result), len(tt.expected))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("at index %d: got %q, expected %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
