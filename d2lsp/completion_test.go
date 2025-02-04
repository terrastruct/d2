package d2lsp

import (
	"testing"
)

func TestGetCompletionItems(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		line    int
		column  int
		want    []CompletionItem
		wantErr bool
	}{
		{
			name:   "style dot suggestions",
			text:   "a.style.",
			line:   0,
			column: 8,
			want:   getStyleCompletions(),
		},
		{
			name: "style map suggestions",
			text: `a: {
  style.
}
`,
			line:   1,
			column: 8,
			want:   getStyleCompletions(),
		},
		{
			name: "classes shapes",
			text: `classes: {
  goal: {
    shape:
  }
}
`,
			line:   2,
			column: 10,
			want:   getShapeCompletions(),
		},
		{
			name: "nested style map suggestions",
			text: `a: {
	style: {
    3d:
  }
}
`,
			line:   2,
			column: 7,
			want:   getBooleanCompletions(),
		},
		{
			name: "3d style map suggestions",
			text: `a.style: {
  3d:
}
`,
			line:   1,
			column: 5,
			want:   getBooleanCompletions(),
		},
		{
			name: "fill pattern style map suggestions",
			text: `a.style: {
  fill-pattern:
}
`,
			line:   1,
			column: 15,
			want:   getFillPatternCompletions(),
		},
		{
			name: "opacity style map suggestions",
			text: `a.style: {
  opacity:
}
`,
			line:   1,
			column: 10,
			want:   getValueCompletions("opacity"),
		},
		{
			name:   "width dot",
			text:   `a.width:`,
			line:   0,
			column: 8,
			want:   getValueCompletions("width"),
		},
		{
			name: "layer shape",
			text: `a

layers: {
  hey: {
    go: {
      shape:
    }
  }
}
`,
			line:   5,
			column: 12,
			want:   getShapeCompletions(),
		},
		{
			name:   "stroke width value",
			text:   `a.style.stroke-width: 1`,
			line:   0,
			column: 23,
			want:   nil,
		},
		{
			name: "no style suggestions",
			text: `a.style:
`,
			line:   0,
			column: 8,
			want:   nil,
		},
		{
			name:   "style property suggestions",
			text:   "a -> b: { style. }",
			line:   0,
			column: 16,
			want:   getStyleCompletions(),
		},
		{
			name:   "style.opacity value hint",
			text:   "a -> b: { style.opacity: }",
			line:   0,
			column: 24,
			want:   getValueCompletions("opacity"),
		},
		{
			name:   "fill pattern completions",
			text:   "a -> b: { style.fill-pattern: }",
			line:   0,
			column: 29,
			want:   getFillPatternCompletions(),
		},
		{
			name:   "text transform completions",
			text:   "a -> b: { style.text-transform: }",
			line:   0,
			column: 31,
			want:   getTextTransformCompletions(),
		},
		{
			name:   "boolean property completions",
			text:   "a -> b: { style.shadow: }",
			line:   0,
			column: 23,
			want:   getBooleanCompletions(),
		},
		{
			name:   "near position completions",
			text:   "a -> b: { label.near: }",
			line:   0,
			column: 21,
			want:   getNearCompletions(),
		},
		{
			name:   "direction completions",
			text:   "a -> b: { direction: }",
			line:   0,
			column: 20,
			want:   getDirectionCompletions(),
		},
		{
			name:   "icon url completions",
			text:   "a -> b: { icon: }",
			line:   0,
			column: 15,
			want:   getIconCompletions(),
		},
		{
			name:   "icon dot url completions",
			text:   "a.icon:",
			line:   0,
			column: 7,
			want:   getIconCompletions(),
		},
		{
			name:   "icon near completions",
			text:   "a -> b: { icon.near: }",
			line:   0,
			column: 20,
			want:   getNearCompletions(),
		},
		{
			name: "icon map",
			text: `a.icon: {
  # here
}`,
			line:   1,
			column: 2,
			want:   nil,
		},
		{
			name:   "icon flat dot",
			text:   `a.icon.`,
			line:   0,
			column: 7,
			want:   getLabelCompletions(),
		},
		{
			name:   "label flat dot",
			text:   `a.label.`,
			line:   0,
			column: 8,
			want:   getLabelCompletions(),
		},
		{
			name:   "arrowhead completions - dot syntax",
			text:   "a -> b: { source-arrowhead. }",
			line:   0,
			column: 27,
			want:   getArrowheadCompletions(),
		},
		{
			name:   "arrowhead completions - colon syntax",
			text:   "a -> b: { source-arrowhead: }",
			line:   0,
			column: 27,
			want:   nil,
		},
		{
			name: "arrowhead completions - map syntax",
			text: `a -> b: {
  source-arrowhead: {
    # here
  }
}`,
			line:   2,
			column: 4,
			want:   getArrowheadCompletions(),
		},
		{
			name:   "arrowhead shape completions - flat dot syntax",
			text:   "(a -> b)[0].source-arrowhead.shape:",
			line:   0,
			column: 35,
			want:   getArrowheadShapeCompletions(),
		},
		{
			name:   "arrowhead shape completions - dot syntax",
			text:   "a -> b: { source-arrowhead.shape: }",
			line:   0,
			column: 33,
			want:   getArrowheadShapeCompletions(),
		},
		{
			name:   "arrowhead shape completions - map syntax",
			text:   "a -> b: { source-arrowhead: { shape: } }",
			line:   0,
			column: 36,
			want:   getArrowheadShapeCompletions(),
		},
		{
			name:   "width value hint",
			text:   "a -> b: { width: }",
			line:   0,
			column: 16,
			want:   getValueCompletions("width"),
		},
		{
			name:   "height value hint",
			text:   "a -> b: { height: }",
			line:   0,
			column: 17,
			want:   getValueCompletions("height"),
		},
		{
			name:   "tooltip markdown template",
			text:   "a -> b: { tooltip: }",
			line:   0,
			column: 18,
			want:   getTooltipCompletions(),
		},
		{
			name:   "tooltip dot markdown template",
			text:   "a.tooltip:",
			line:   0,
			column: 10,
			want:   getTooltipCompletions(),
		},
		{
			name:   "shape dot suggestions",
			text:   "a.shape:",
			line:   0,
			column: 8,
			want:   getShapeCompletions(),
		},
		{
			name:   "shape suggestions",
			text:   "a -> b: { shape: }",
			line:   0,
			column: 16,
			want:   getShapeCompletions(),
		},
		{
			name: "shape 2 suggestions",
			text: `a: {
  shape:
}`,
			line:   1,
			column: 8,
			want:   getShapeCompletions(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCompletionItems(tt.text, tt.line, tt.column)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCompletionItems() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("GetCompletionItems() got %d completions, want %d", len(got), len(tt.want))
				return
			}

			// Create maps for easy comparison
			gotMap := make(map[string]CompletionItem)
			wantMap := make(map[string]CompletionItem)
			for _, item := range got {
				gotMap[item.Label] = item
			}
			for _, item := range tt.want {
				wantMap[item.Label] = item
			}

			// Check that each completion exists and has correct properties
			for label, wantItem := range wantMap {
				gotItem, exists := gotMap[label]
				if !exists {
					t.Errorf("missing completion for %q", label)
					continue
				}
				if gotItem.Kind != wantItem.Kind {
					t.Errorf("completion %q Kind = %v, want %v", label, gotItem.Kind, wantItem.Kind)
				}
				if gotItem.Detail != wantItem.Detail {
					t.Errorf("completion %q Detail = %v, want %v", label, gotItem.Detail, wantItem.Detail)
				}
				if gotItem.InsertText != wantItem.InsertText {
					t.Errorf("completion %q InsertText = %v, want %v", label, gotItem.InsertText, wantItem.InsertText)
				}
			}
		})
	}
}

// Helper function to compare CompletionItem slices
func equalCompletions(a, b []CompletionItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label ||
			a[i].Kind != b[i].Kind ||
			a[i].Detail != b[i].Detail ||
			a[i].InsertText != b[i].InsertText {
			return false
		}
	}
	return true
}

func TestGetArrowheadShapeCompletions(t *testing.T) {
	got := getArrowheadShapeCompletions()

	expectedLabels := []string{
		"triangle", "arrow", "diamond", "circle",
		"cf-one", "cf-one-required",
		"cf-many", "cf-many-required",
	}

	if len(got) != len(expectedLabels) {
		t.Errorf("getArrowheadShapeCompletions() returned %d items, want %d", len(got), len(expectedLabels))
		return
	}

	for i, label := range expectedLabels {
		if got[i].Label != label {
			t.Errorf("completion[%d].Label = %v, want %v", i, got[i].Label, label)
		}
		if got[i].Kind != ShapeCompletion {
			t.Errorf("completion[%d].Kind = %v, want ShapeCompletion", i, got[i].Kind)
		}
		if got[i].InsertText != label {
			t.Errorf("completion[%d].InsertText = %v, want %v", i, got[i].InsertText, label)
		}
	}
}

func TestGetValueCompletions(t *testing.T) {
	tests := []struct {
		property   string
		wantLabel  string
		wantDetail string
	}{
		{
			property:   "opacity",
			wantLabel:  "(number between 0.0 and 1.0)",
			wantDetail: "e.g. 0.4",
		},
		{
			property:   "stroke-width",
			wantLabel:  "(number between 0 and 15)",
			wantDetail: "e.g. 2",
		},
		{
			property:   "font-size",
			wantLabel:  "(number between 8 and 100)",
			wantDetail: "e.g. 14",
		},
		{
			property:   "width",
			wantLabel:  "(pixels)",
			wantDetail: "e.g. 400",
		},
		{
			property:   "stroke",
			wantLabel:  "(color name or hex code)",
			wantDetail: "e.g. blue, #ff0000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.property, func(t *testing.T) {
			got := getValueCompletions(tt.property)
			if len(got) != 1 {
				t.Fatalf("getValueCompletions(%s) returned %d items, want 1", tt.property, len(got))
			}
			if got[0].Label != tt.wantLabel {
				t.Errorf("completion.Label = %v, want %v", got[0].Label, tt.wantLabel)
			}
			if got[0].Detail != tt.wantDetail {
				t.Errorf("completion.Detail = %v, want %v", got[0].Detail, tt.wantDetail)
			}
			if got[0].InsertText != "" {
				t.Errorf("completion.InsertText = %v, want empty string", got[0].InsertText)
			}
		})
	}
}
