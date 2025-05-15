package textmeasure

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func ReplaceSubstitutionsMarkdown(mdText string, variables map[string]string) string {
	source := []byte(mdText)
	reader := text.NewReader(source)
	doc := markdownRenderer.Parser().Parse(reader)

	type substitution struct {
		start  int
		stop   int
		newVal string
	}
	var substitutions []substitution

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if isCodeNode(n) {
			return ast.WalkSkipChildren, nil
		}

		if textNode, ok := n.(*ast.Text); ok {
			segment := textNode.Segment
			originalText := string(segment.Value(source))
			newText := replaceVariables(originalText, variables)

			if originalText != newText {
				substitutions = append(substitutions, substitution{
					start:  segment.Start,
					stop:   segment.Stop,
					newVal: newText,
				})
			}
		}
		return ast.WalkContinue, nil
	})

	if len(substitutions) == 0 {
		return mdText
	}

	sort.Slice(substitutions, func(i, j int) bool {
		return substitutions[i].start > substitutions[j].start
	})

	result := string(source)
	for _, sub := range substitutions {
		result = result[:sub.start] + sub.newVal + result[sub.stop:]
	}

	return result
}

func isCodeNode(n ast.Node) bool {
	switch n.Kind() {
	case ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindCodeSpan:
		return true
	}
	return false
}

func replaceVariables(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}
