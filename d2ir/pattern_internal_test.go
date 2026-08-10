package d2ir

import "testing"

func TestMatchPatternAnchors(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		pattern []string
		want    bool
	}{
		{name: "exact", text: "animal", pattern: []string{"animal"}, want: true},
		{name: "exact rejects trailing", text: "animals", pattern: []string{"animal"}, want: false},
		{name: "prefix", text: "animal", pattern: []string{"an", "*"}, want: true},
		{name: "suffix", text: "animal", pattern: []string{"*", "l"}, want: true},
		{name: "repeated suffix", text: "lal", pattern: []string{"*", "l"}, want: true},
		{name: "suffix rejects trailing", text: "jingle", pattern: []string{"*", "l"}, want: false},
		{name: "prefix suffix repeated", text: "torr", pattern: []string{"t", "*", "r"}, want: true},
		{name: "prefix suffix rejects trailing", text: "tinkerbell", pattern: []string{"t", "*", "r"}, want: false},
		{name: "multiple wildcards", text: "abcbxc", pattern: []string{"a", "*", "b", "*", "c"}, want: true},
		{name: "case insensitive suffix", text: "Animal", pattern: []string{"*", "AL"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchPattern(tt.text, tt.pattern); got != tt.want {
				t.Fatalf("matchPattern(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
			}
		})
	}
}
