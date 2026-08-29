package svg

import (
	"strings"
	"testing"
)

func TestIsSupportedExternalDoctype(t *testing.T) {
	t.Parallel()
	const (
		svg11Token = `DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"`
		svg11Raw   = `<!` + svg11Token + `>`
		svg10Token = "DOCTYPE\nsvg\tPUBLIC '-//W3C//DTD SVG 1.0//EN'\r\n'http://www.w3.org/TR/2001/REC-SVG-20010904/DTD/svg10.dtd' "
		svg10Raw   = `<!` + svg10Token + `>`
	)
	tests := []struct {
		name                string
		document, directive string
		want                bool
	}{
		{
			name:      "svg-1.1",
			document:  svg11Raw + `<svg/>`,
			directive: svg11Token,
			want:      true,
		},
		{
			name:      "svg-1.0-single-quotes-and-whitespace",
			document:  svg10Raw + `<svg/>`,
			directive: svg10Token,
			want:      true,
		},
		{
			name:      "iso-8859-1-comment-before-doctype",
			document:  "<?xml version=\"1.0\" encoding=\"iso-8859-1\"?>\n<!--caf\xe9-->\n" + svg11Raw + `<svg/>`,
			directive: svg11Token,
			want:      true,
		},
		{name: "bare", document: `<!DOCTYPE svg><svg/>`, directive: `DOCTYPE svg`},
		{name: "system", document: `<!DOCTYPE svg SYSTEM "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: `DOCTYPE svg SYSTEM "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"`},
		{name: "wrong-root", document: `<!DOCTYPE html PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: `DOCTYPE html PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"`},
		{name: "wrong-public-id", document: `<!DOCTYPE svg PUBLIC "-//EXAMPLE//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: `DOCTYPE svg PUBLIC "-//EXAMPLE//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"`},
		{name: "wrong-system-id", document: `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "https://example.invalid/svg11.dtd"><svg/>`, directive: `DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "https://example.invalid/svg11.dtd"`},
		{name: "internal-subset", document: `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [<!ENTITY bomb "boom">]><svg/>`, directive: `DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [<!ENTITY bomb "boom">]`},
		{name: "duplicate-content", document: `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" extra><svg/>`, directive: `DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" extra`},
		{name: "case-sensitive", document: `<!doctype svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: `doctype svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"`},
		{name: "comment-normalization", document: `<!DOCTYPE<!--comment--> svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: svg11Token},
		{name: "doctype-inside-comment", document: `<!--` + svg11Raw + `--><svg/>`, directive: svg11Token},
		{name: "leading-space", document: `<! DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, directive: svg11Token},
		{name: "oversized", document: `<!DOCTYPE ` + strings.Repeat(" ", maxSupportedExternalDoctypeBytes) + `><svg/>`, directive: `DOCTYPE ` + strings.Repeat(" ", maxSupportedExternalDoctypeBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSupportedExternalDoctype([]byte(test.document), []byte(test.directive)); got != test.want {
				t.Fatalf("IsSupportedExternalDoctype(%q, %q) = %v, want %v", test.document, test.directive, got, test.want)
			}
		})
	}
}
