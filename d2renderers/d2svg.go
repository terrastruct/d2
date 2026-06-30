package d2renderers

import (
	"fmt"

	"oss.terrastruct.com/d2/lib/ast"
	"oss.terrastruct.com/d2/lib/renderer"
)

func RenderSVG(board *ast.Board) (string, error) {
	r := renderer.NewRenderer(board)
	svg, err := r.RenderSVG()
	if err!= nil {
		return "", fmt.Errorf("failed to render SVG: %v", err)
	}
	return svg, nil
}
