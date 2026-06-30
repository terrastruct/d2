package d2parser

import (
	"fmt"

	"oss.terrastruct.com/d2/lib/ast"
	"oss.terrastruct.com/d2/lib/lexer"
	"oss.terrastruct.com/d2/lib/parser"
)

func Parse(s string) (*ast.Board, error) {
	l := lexer.NewLexer(s)
	p := parser.NewParser()
	board, err := p.Parse(l)
	if err!= nil {
		return nil, fmt.Errorf("failed to parse D2: %v", err)
	}
	return board, nil
}
