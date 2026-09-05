package d2svg

import (
	"encoding/xml"
	"io"
	"strings"

	"github.com/d2lang/d2/lib/color"
)

// pruneThemeCSS specializes a complete ThemeCSS stylesheet to the rendered SVG
// fragments. ThemeCSS itself remains available with its complete rule set.
func pruneThemeCSS(stylesheet string, sources ...string) string {
	return pruneThemeCSSWithClasses(stylesheet, nil, sources...)
}

// The font scan can supply body classes so only the small background fragments
// need a second XML pass. The map belongs to the current Render call.
func pruneThemeCSSWithClasses(stylesheet string, used map[string]bool, sources ...string) string {
	if stylesheet == "" {
		return stylesheet
	}
	// appendix.Append adds this separator after Render, without an inline color.
	// Its text color uses the separate .appendix rule, which is never pruned.
	if used == nil {
		used = make(map[string]bool)
	}
	used["stroke-B2"] = true
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

	return pruneThemeRules(stylesheet, used)
}

// Scan the simple palette selectors emitted by ThemeCSS, leaving all other
// rules intact. This recognizes the same grammar as the original regexp,
// including its ASCII whitespace and class names, without repeatedly matching
// the complete stylesheet and then matching each rule again.
func pruneThemeRules(stylesheet string, used map[string]bool) string {
	var out strings.Builder
	copied := 0
	for scan := 0; scan < len(stylesheet); {
		rel := strings.IndexByte(stylesheet[scan:], '{')
		if rel < 0 {
			break
		}
		open := scan + rel
		scan = open + 1
		start, class, ok := themeRuleSelector(stylesheet, open)
		if !ok {
			continue
		}
		end := strings.IndexAny(stylesheet[scan:], "{}")
		if end < 0 {
			break
		}
		end += scan
		if stylesheet[end] != '}' {
			continue
		}
		scan = end + 1
		if !isPaletteClass(class) || used[class] {
			continue
		}
		for start > copied && themeRuleWhitespace(stylesheet[start-1]) {
			start--
		}
		if out.Cap() == 0 {
			out.Grow(len(stylesheet))
		}
		out.WriteString(stylesheet[copied:start])
		copied = scan
	}
	if copied == 0 {
		return stylesheet
	}
	out.WriteString(stylesheet[copied:])
	return out.String()
}

func themeRuleSelector(stylesheet string, open int) (start int, class string, ok bool) {
	start = open
	for start > 0 && themeRuleWord(stylesheet[start-1]) {
		start--
	}
	if start == 0 || stylesheet[start-1] != '.' {
		return 0, "", false
	}
	class = stylesheet[start:open]
	value := ""
	if strings.HasPrefix(class, "sketch-overlay-") {
		value = class[len("sketch-overlay-"):]
		start--
	} else {
		for _, prefix := range []string{"fill-", "stroke-", "background-color-", "color-"} {
			if strings.HasPrefix(class, prefix) {
				value = class[len(prefix):]
				break
			}
		}
		// The scoped form has exactly one space between its two classes.
		if start < 3 || stylesheet[start-2] != ' ' {
			return 0, "", false
		}
		end := start - 2
		start = end
		for start > 0 && themeRuleWord(stylesheet[start-1]) {
			start--
		}
		if start == end || start == 0 || stylesheet[start-1] != '.' {
			return 0, "", false
		}
		start--
	}
	if value == "" {
		return 0, "", false
	}
	for i := range value {
		if c := value[i]; !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return 0, "", false
		}
	}
	return start, class, true
}

func themeRuleWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func themeRuleWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isPaletteClass(class string) bool {
	for _, prefix := range []string{"fill-", "stroke-", "background-color-", "color-", "sketch-overlay-"} {
		if value, ok := strings.CutPrefix(class, prefix); ok {
			return color.IsThemeColor(value)
		}
	}
	return false
}
