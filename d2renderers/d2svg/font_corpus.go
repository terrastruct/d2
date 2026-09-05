package d2svg

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"unicode"
)

// fontCorpora contains the characters rendered by each embedded font class.
// Absence means the face is unused. A nil map asks the embedding code to keep
// its legacy source triggers and whole-diagram corpus instead.
type fontCorpora map[string]string

// BaseStylesheet is public and callers may add font-selection rules. Keep the
// original text so those callers retain whole-corpus font subsets.
var fontCorpusBaseStylesheet = BaseStylesheet

// restrictFontCorpora narrows the freshly collected map in place only when its
// characters appeared in the legacy corpus. Rendering can introduce characters
// absent from that corpus (notably code's nonbreaking spaces). Their browser
// fallback can use other legacy glyphs, such as ordinary spaces, so preserve the
// complete legacy subset for that face instead of changing those dependencies.
// Unicode normalization and bidirectional shaping can also use characters from
// other runs or faces. Keep all legacy faces and their corpora for non-ASCII
// input, and for controls whose whitespace aliases are browser-dependent.
func restrictFontCorpora(corpora fontCorpora, corpus string) fontCorpora {
	if corpora == nil {
		return nil
	}
	var allowed [128]bool
	for i := 0; i < len(corpus); i++ {
		c := corpus[i]
		if c >= 128 || c == 127 || c < 32 && c != '\n' {
			return nil
		}
		allowed[c] = true
	}
	for class, rendered := range corpora {
		for _, r := range rendered {
			if r >= 128 || !allowed[r] {
				corpora[class] = corpus
				break
			}
		}
	}
	return corpora
}

func appendFontOnTrigger(buf *bytes.Buffer, source string, triggers []string, corpora fontCorpora, class, corpus string, content func(string) string) {
	if corpora == nil {
		appendOnTriggerLazy(buf, source, triggers, func() string { return content(corpus) })
	} else if rendered, ok := corpora[class]; ok {
		buf.WriteString(content(rendered))
	}
}

// This order is the cascade order of the font-family rules in EmbedFonts.
// In particular, matching text-mono-bold must not also match text-mono.
var embeddedFontClasses = [...]string{
	"text", "text-semibold", "text-bold", "text-italic",
	"text-mono", "text-mono-semibold", "text-mono-bold", "text-mono-italic",
}

// collectFontCorpora is for the body fragment produced by Render, before its
// stylesheets are added. It follows SVG text inheritance, including native
// Markdown runs and syntax-highlighted tspans. It is deliberately not a CSS or
// HTML evaluator: unsupported font selection, referenced/dynamic text, and
// malformed fragments request the existing full-corpus embedding path.
func collectFontCorpora(source string) (fontCorpora, bool) {
	corpora, _, ok := collectFontCorporaAndClasses(source)
	return corpora, ok
}

// collectFontCorporaAndClasses also records palette classes during the text
// scan. A nonnil class map means the complete fragment passed the existing
// theme parser's safety checks and can avoid a second XML pass.
func collectFontCorporaAndClasses(source string) (fontCorpora, map[string]bool, bool) {
	if BaseStylesheet != fontCorpusBaseStylesheet {
		return nil, nil, false
	}
	decoder := xml.NewDecoder(io.MultiReader(
		strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">`),
		strings.NewReader(source), strings.NewReader(`</svg>`),
	))
	type frame struct {
		font   int
		text   bool
		ignore bool
	}
	stack := []frame{{font: -1}}
	var corpora [len(embeddedFontClasses)]strings.Builder
	classes := make(map[string]bool)
	classesSafe := true
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, false
		}
		switch token := token.(type) {
		case xml.StartElement:
			if strings.EqualFold(token.Name.Local, "script") {
				classesSafe = false
			}
			if token.Name.Space != "" && token.Name.Space != "http://www.w3.org/2000/svg" {
				return nil, nil, false
			}
			current := stack[len(stack)-1]
			switch token.Name.Local {
			case "style", "script", "foreignObject", "use", "tref", "textPath",
				"animate", "animateMotion", "animateTransform", "set":
				return nil, nil, false
			case "title", "desc", "metadata":
				current.ignore = true
			case "text":
				current.text = true
			case "tspan", "a":
			default:
				if current.text && !current.ignore {
					return nil, nil, false
				}
			}

			// A class on the child overrides an inherited family, even if the
			// parent's class occurs later in the stylesheet. Only classes on
			// the same element compete according to stylesheet order.
			localFont := -1
			classSeen := false
			for _, attr := range token.Attr {
				// Match pruneThemeCSS's guards independently of the stricter
				// font guards, so neither specialization changes the other.
				name := strings.ToLower(attr.Name.Local)
				if strings.HasPrefix(name, "on") || name == "attributename" && strings.EqualFold(strings.TrimSpace(attr.Value), "class") {
					classesSafe = false
				}
				if name == "href" {
					value := strings.TrimSpace(attr.Value)
					if strings.EqualFold(token.Name.Local, "use") && !strings.HasPrefix(value, "#") || strings.HasPrefix(strings.ToLower(value), "javascript:") {
						classesSafe = false
					}
				}
				if strings.HasPrefix(attr.Name.Local, "font-variant") {
					return nil, nil, false
				}
				switch attr.Name.Local {
				case "class":
					if attr.Name.Space != "" || classSeen {
						return nil, nil, false
					}
					classSeen = true
					for _, class := range strings.FieldsFunc(attr.Value, func(r rune) bool {
						// The separate theme parser uses strings.Fields, which
						// also splits Unicode whitespace. Preserve that path
						// for custom classes with different tokenization.
						if r >= 128 && unicode.IsSpace(r) {
							classesSafe = false
						}
						return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
					}) {
						if isPaletteClass(class) {
							classes[class] = true
						}
						for i, fontClass := range embeddedFontClasses {
							if class == fontClass && i > localFont {
								localFont = i
							}
						}
					}
				case "font-family", "text-transform", "direction", "unicode-bidi":
					return nil, nil, false
				case "style":
					if attr.Name.Space != "" || !fontCorpusStyleSafe(attr.Value) {
						return nil, nil, false
					}
				default:
					if strings.HasPrefix(attr.Name.Local, "on") {
						return nil, nil, false
					}
				}
			}
			if localFont >= 0 {
				current.font = localFont
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) == 1 {
				return nil, nil, false
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			current := stack[len(stack)-1]
			if current.text && !current.ignore && len(token) != 0 {
				if current.font < 0 {
					return nil, nil, false
				}
				// Keep whitespace too: code uses nonbreaking spaces, and text
				// layout can preserve ordinary spaces with xml:space.
				corpora[current.font].Write(token)
			}
		case xml.Directive, xml.ProcInst:
			return nil, nil, false
		}
	}
	if len(stack) != 1 {
		return nil, nil, false
	}
	result := make(fontCorpora)
	for i := range corpora {
		if corpora[i].Len() != 0 {
			result[embeddedFontClasses[i]] = corpora[i].String()
		}
	}
	// The legacy regular-face trigger is narrower than CSS class matching.
	// Customized SVG fragments may use another class first, or single quotes;
	// do not introduce a regular-family rule that the old output lacked.
	if _, regular := result["text"]; regular &&
		!strings.Contains(source, `class="text"`) && !strings.Contains(source, `class="text `) &&
		!strings.Contains(source, `class="md"`) && !strings.Contains(source, `class="md `) {
		return nil, nil, false
	}
	if !classesSafe {
		classes = nil
	}
	return result, classes, true
}

// Generated inline styles use simple declarations. Reject CSS syntax that
// could disguise a font-family override or change the rendered characters.
// Font weight/style alone synthesize against the selected family; the D2
// @font-face declarations do not declare separate weight/style descriptors.
func fontCorpusStyleSafe(style string) bool {
	if strings.ContainsAny(style, `\/{}`) {
		return false
	}
	for _, declaration := range strings.Split(style, ";") {
		if strings.TrimSpace(declaration) == "" {
			continue
		}
		property, _, ok := strings.Cut(declaration, ":")
		if !ok {
			return false
		}
		property = strings.ToLower(strings.TrimSpace(property))
		if strings.HasPrefix(property, "font-variant") {
			return false
		}
		switch property {
		case "font", "font-family", "text-transform", "direction", "unicode-bidi", "all", "content":
			return false
		}
	}
	return true
}
