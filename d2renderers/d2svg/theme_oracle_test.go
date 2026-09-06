package d2svg

import (
	"encoding/xml"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

// Match only the palette and sketch rules emitted by ThemeCSS. Other rules,
// including Markdown variables and light/dark code visibility, stay intact.
var legacyThemePaletteRule = regexp.MustCompile(`\s*(?:\.[\w-]+ \.((?:fill|stroke|background-color|color)-[A-Z0-9]+)|\.(sketch-overlay-[A-Z0-9]+))\{[^{}]*\}`)

// Frozen implementation from before the selector scanner. It deliberately
// retains the original regexp and XML checks as an exact-output oracle.
func legacyPruneThemeCSS(stylesheet string, sources ...string) string {
	if stylesheet == "" {
		return stylesheet
	}
	// appendix.Append adds this separator after Render, without an inline color.
	// Its text color uses the separate .appendix rule, which is never pruned.
	used := map[string]bool{"stroke-B2": true}
	for _, source := range sources {
		decoder := xml.NewDecoder(strings.NewReader(source))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				// Preserve the existing output for custom/malformed SVG fragments.
				return stylesheet
			}
			element, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			if strings.EqualFold(element.Name.Local, "script") {
				return stylesheet
			}
			for _, attr := range element.Attr {
				// Custom icon SVG can mutate classes at runtime. Keep all rules
				// when scripts, event handlers, or SMIL make class use dynamic.
				name := strings.ToLower(attr.Name.Local)
				if strings.HasPrefix(name, "on") || name == "attributename" && strings.EqualFold(strings.TrimSpace(attr.Value), "class") {
					return stylesheet
				}
				if name == "href" {
					value := strings.TrimSpace(attr.Value)
					if strings.EqualFold(element.Name.Local, "use") && !strings.HasPrefix(value, "#") || strings.HasPrefix(strings.ToLower(value), "javascript:") {
						return stylesheet
					}
				}
				if attr.Name.Local == "class" {
					for _, class := range strings.Fields(attr.Value) {
						if isPaletteClass(class) {
							used[class] = true
						}
					}
				}
			}
		}
	}

	return legacyThemePaletteRule.ReplaceAllStringFunc(stylesheet, func(rule string) string {
		match := legacyThemePaletteRule.FindStringSubmatch(rule)
		class := match[1]
		if class == "" {
			class = match[2]
		}
		if isPaletteClass(class) && !used[class] {
			return ""
		}
		return rule
	})
}

func TestPruneThemeCSSLegacyOracle(t *testing.T) {
	dark := int64(200)
	for _, theme := range append(d2themescatalog.LightCatalog[:len(d2themescatalog.LightCatalog):len(d2themescatalog.LightCatalog)], d2themescatalog.DarkCatalog...) {
		for _, hash := range []string{"d2-123456789", "d2_UNDER-score", "custom.hash", "odd hash"} {
			full, err := ThemeCSS(hash, &theme.ID, &dark, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{
				`<g/>`,
				`<g class="fill-N1 stroke-B2 color-AB4 sketch-overlay-AA2"/>`,
				`<g class="fill-N1&#9;fill-N2 background-color-N7"/><g class="sketch-overlay-N1"/>`,
				`<g class="fill-N1&#160;stroke-N2"/>`,
				`<g onload="changeClasses()"/>`,
				`<g><set attributeName="class" to="fill-N1"/></g>`,
				`<g`,
			} {
				got, want := pruneThemeCSS(full, source), legacyPruneThemeCSS(full, source)
				if got != want {
					t.Fatalf("theme %d, hash %q, source %q:\ngot %q\nwant %q", theme.ID, hash, source, got, want)
				}
			}
		}
	}
}

func TestPruneThemeRulesLegacyOracle(t *testing.T) {
	fragments := []string{
		".d2-x .fill-N1{fill:red;}", ".d2-x .fill-N2{fill:blue;}",
		".d2-x .stroke-B2{stroke:red;}", ".d2-x .background-color-N7{background-color:white;}",
		".d2-x .color-AA2{color:yellow;}", ".sketch-overlay-N1{fill:url(#streaks-dark);}",
		".d2-x .fill-CUSTOM{fill:red;}", ".d2-x .fill-N10{fill:red;}", ".d2-x .fill-n1{fill:red;}",
		".fill-N1{fill:red;}", ".d2-x  .fill-N1{fill:red;}", ".d2-x\t.fill-N1{fill:red;}",
		".word.- .fill-N1{fill:red;}", "._-09azAZ .fill-N1{fill:red;}", ".é .fill-N1{fill:red;}",
		".fill-N1{.d2-x .fill-N2{fill:blue;}}", ".sketch-overlay-N1{{fill:red;}}", ".x .fill-N1{",
		".d2-x .fill-N1{\x00;\xff}", ".d2-x .fill-N1{content:'}'}", ".d2-x .fill-N1{content:'{'}",
		"@media dark{", "}", "{", ".", "", "\xff", "unrelated", "\n\t\f\r ", "\v", "\u00a0",
	}
	used := map[string]bool{"stroke-B2": true, "fill-N1": true, "sketch-overlay-N2": true}
	oracle := func(stylesheet string) string {
		return legacyThemePaletteRule.ReplaceAllStringFunc(stylesheet, func(rule string) string {
			match := legacyThemePaletteRule.FindStringSubmatch(rule)
			class := match[1]
			if class == "" {
				class = match[2]
			}
			if isPaletteClass(class) && !used[class] {
				return ""
			}
			return rule
		})
	}
	check := func(stylesheet string) {
		t.Helper()
		if got, want := pruneThemeRules(stylesheet, used), oracle(stylesheet); got != want {
			t.Fatalf("stylesheet %q:\ngot %q\nwant %q", stylesheet, got, want)
		}
	}
	for _, a := range fragments {
		check(a)
		for _, b := range fragments {
			check(a + b)
		}
	}
	rng := rand.New(rand.NewSource(53))
	for i := 0; i < 3000; i++ {
		var css strings.Builder
		for j, n := 0, 1+rng.Intn(30); j < n; j++ {
			css.WriteString(fragments[rng.Intn(len(fragments))])
		}
		check(css.String())
	}
}

func BenchmarkPruneThemeCSS(b *testing.B) {
	dark := int64(200)
	full, err := ThemeCSS("d2-123456789", nil, &dark, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	const source = `<g class="fill-N1 stroke-B2"><text class="text fill-N1">hello</text></g><rect class="fill-N7"/><g class="sketch-overlay-AA2"/>`
	for _, tc := range []struct {
		name  string
		prune func(string, ...string) string
	}{{"legacy", legacyPruneThemeCSS}, {"scanner", pruneThemeCSS}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if tc.prune(full, source) == "" {
					b.Fatal("empty output")
				}
			}
		})
	}
}
