package d2svgimport

import (
	"context"
	"strings"
)

// stylesheetRule is deliberately smaller than CSS. Every rule has exactly
// one simple class selector, so source order is the only specificity tie that
// needs to be represented here.
type stylesheetRule struct {
	class        string
	declarations map[string]string
}

func (i *svgImporter) compileStylesheets(root *svgElement) error {
	stack := []*svgElement{root}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		element := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if element.name == "style" {
			if err := i.validateStyleElement(element); err != nil {
				return err
			}
			rules, err := i.parseStylesheet(string(element.text))
			if err != nil {
				return err
			}
			i.stylesheetRules = append(i.stylesheetRules, rules...)
		}
		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, element.children[index])
		}
	}

	return nil
}

func (i *svgImporter) validateStyleElement(element *svgElement) error {
	for _, name := range element.attrOrder {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		if name != "type" {
			return i.errorf("element <style> has unsupported attribute %q", displayXMLName(name))
		}
	}
	if len(element.children) != 0 {
		return i.errorf("element <style> cannot contain child elements")
	}
	if raw, ok := element.attrs["type"]; ok {
		value, err := trimSVGSpace(i.ctx, raw)
		if err != nil {
			return err
		}
		if !equalASCIIEqualFold(value, "text/css") {
			return i.errorf("element <style> has unsupported type")
		}
	}
	return nil
}

func (i *svgImporter) parseStylesheet(input string) ([]stylesheetRule, error) {
	for index := 0; index < len(input); index++ {
		if index&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return nil, err
			}
		}
		if index+1 < len(input) && (input[index:index+2] == "/*" || input[index:index+2] == "*/") {
			return nil, i.errorf("CSS comments in <style> are unsupported")
		}
	}

	var rules []stylesheetRule
	offset := 0
	for {
		var err error
		offset, err = skipStylesheetSpace(i.ctx, input, offset)
		if err != nil {
			return nil, err
		}
		if offset == len(input) {
			return rules, nil
		}
		if input[offset] == '}' {
			return nil, i.errorf("malformed stylesheet: unexpected closing brace")
		}

		selectorStart := offset
		for offset < len(input) && input[offset] != '{' {
			if offset&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if input[offset] == '}' {
				return nil, i.errorf("malformed stylesheet selector")
			}
			offset++
		}
		if offset == len(input) {
			return nil, i.errorf("malformed stylesheet: missing declaration block")
		}
		selector, err := trimSVGSpace(i.ctx, input[selectorStart:offset])
		if err != nil {
			return nil, err
		}
		class, err := i.simpleClassSelector(selector)
		if err != nil {
			return nil, err
		}
		offset++
		declarationStart := offset
		for offset < len(input) && input[offset] != '}' {
			if offset&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if input[offset] == '{' {
				return nil, i.errorf("malformed stylesheet declaration block")
			}
			offset++
		}
		if offset == len(input) {
			return nil, i.errorf("malformed stylesheet: unterminated declaration block")
		}
		declarations, err := i.stylesheetDeclarations(input[declarationStart:offset])
		if err != nil {
			return nil, err
		}
		rules = append(rules, stylesheetRule{class: class, declarations: declarations})
		offset++
	}
}

func skipStylesheetSpace(ctx context.Context, input string, offset int) (int, error) {
	for offset < len(input) && isSVGListSpace(input[offset]) {
		offset++
		if offset&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
	}
	return offset, ctx.Err()
}

func (i *svgImporter) simpleClassSelector(selector string) (string, error) {
	if len(selector) < 2 || selector[0] != '.' {
		if strings.HasPrefix(selector, "@") {
			return "", i.errorf("CSS at-rules in <style> are unsupported")
		}
		return "", i.errorf("unsupported stylesheet selector")
	}
	class := selector[1:]
	for index := 0; index < len(class); index++ {
		if index&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return "", err
			}
		}
		character := class[index]
		first := index == 0
		if first {
			if !isASCIIAlpha(character) && character != '_' {
				return "", i.errorf("unsupported stylesheet selector")
			}
			continue
		}
		if !isASCIIAlpha(character) && (character < '0' || character > '9') && character != '_' && character != '-' {
			return "", i.errorf("unsupported stylesheet selector")
		}
	}
	if err := i.ctx.Err(); err != nil {
		return "", err
	}
	return class, nil
}

func (i *svgImporter) stylesheetDeclarations(input string) (map[string]string, error) {
	declarations := make(map[string]string)
	for start := 0; start <= len(input); {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		end := start
		for end < len(input) && input[end] != ';' {
			end++
			if end&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
		}
		raw, err := trimSVGSpace(i.ctx, input[start:end])
		if err != nil {
			return nil, err
		}
		if end == len(input) {
			start = len(input) + 1
		} else {
			start = end + 1
		}
		if raw == "" {
			continue
		}

		colon := -1
		for index := 0; index < len(raw); index++ {
			if index&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if raw[index] == ':' {
				colon = index
				break
			}
		}
		if colon <= 0 {
			return nil, i.errorf("malformed stylesheet declaration")
		}
		property, err := trimSVGSpace(i.ctx, raw[:colon])
		if err != nil {
			return nil, err
		}
		value, err := trimSVGSpace(i.ctx, raw[colon+1:])
		if err != nil {
			return nil, err
		}
		if len(property) > 64 {
			return nil, i.errorf("malformed stylesheet declaration")
		}
		property = strings.ToLower(property)
		if !validCSSPropertyName(property) || value == "" {
			return nil, i.errorf("malformed stylesheet declaration")
		}
		if _, supported := presentationProperties[property]; !supported {
			return nil, i.errorf("unsupported stylesheet property %q", property)
		}
		if property == "transform" {
			return nil, i.errorf("unsupported stylesheet transform; use the transform presentation attribute")
		}
		for index := 0; index < len(value); index++ {
			if index&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if value[index] == '!' {
				return nil, i.errorf("unsupported !important in <style>")
			}
		}
		declarations[property] = value
	}
	return declarations, i.ctx.Err()
}

func (i *svgImporter) classTokens(element *svgElement) ([]string, map[string]struct{}, error) {
	raw, ok := element.attrs["class"]
	if !ok {
		return nil, nil, nil
	}
	classes := make([]string, 0, 1)
	seen := make(map[string]struct{})
	start := -1
	for offset := 0; offset <= len(raw); offset++ {
		if offset&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return nil, nil, err
			}
		}
		separator := offset == len(raw) || isSVGListSpace(raw[offset])
		if !separator {
			if start < 0 {
				start = offset
			}
			continue
		}
		if start < 0 {
			continue
		}
		class := raw[start:offset]
		if _, duplicate := seen[class]; !duplicate {
			seen[class] = struct{}{}
			classes = append(classes, class)
		}
		start = -1
	}
	return classes, seen, i.ctx.Err()
}

func elementHasClass(element *svgElement, class string) bool {
	_, ok := element.classSet[class]
	return ok
}
