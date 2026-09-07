package d2cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/xmain"
)

// Watch displays the selected board as native SVG while keeping the requested
// file export in its own format. A folder uses its first drawable descendant.
func renderIsometricPreview(ctx context.Context, ms *xmain.State, diagram *d2target.Diagram, render d2svg.RenderOpts, input, sourceRootID string, roots ...*d2target.Diagram) ([]byte, error) {
	// The preview is one static board even when the file is an animated GIF.
	// File-format validation already handled incompatible SVG theme options;
	// previews of raster/paged output use that output's single selected theme.
	render.MasterID = ""
	render.DarkThemeID, render.DarkThemeOverrides = nil, nil
	boards, err := collectGIFBoards(ctx, diagram, d2isometricimg.MaxTimelineBoards)
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 {
		return nil, fmt.Errorf("isometric preview has no drawable boards")
	}
	root := diagram
	if len(roots) > 0 && roots[0] != nil {
		root, sourceRootID = roots[0], "root"
	}
	// Validate the full tree before traversing its links, including when the
	// preview itself is a selected leaf in a larger document.
	if _, err := collectGIFBoards(ctx, root, d2isometricimg.MaxTimelineBoards); err != nil {
		return nil, err
	}
	paths := make(map[string]string)
	var walk func(*d2target.Diagram, string, []string)
	walk = func(board *d2target.Diagram, key string, parts []string) {
		path := "/index.svg"
		if len(parts) > 0 {
			path = "/" + strings.Join(parts, "/") + ".svg"
		}
		paths[key] = (&url.URL{Path: path}).EscapedPath()
		for i, children := range [][]*d2target.Diagram{board.Layers, board.Scenarios, board.Steps} {
			kind := []string{"layers", "scenarios", "steps"}[i]
			for _, child := range children {
				childParts := append(append([]string(nil), parts...), kind, child.Name)
				walk(child, key+"."+kind+"."+child.Name, childParts)
			}
		}
	}
	if sourceRootID == "" {
		sourceRootID = "root"
	}
	walk(root, sourceRootID, nil)
	board := *boards[0]
	board.Layers, board.Scenarios, board.Steps = nil, nil, nil
	board.Shapes = append([]d2target.Shape(nil), board.Shapes...)
	board.Connections = append([]d2target.Connection(nil), board.Connections...)
	relink := func(link string) string {
		if path, ok := paths[link]; ok {
			return path
		}
		return link
	}
	board.Root.Link = relink(board.Root.Link)
	for i := range board.Shapes {
		board.Shapes[i].Link = relink(board.Shapes[i].Link)
	}
	for i := range board.Connections {
		board.Connections[i].Link = relink(board.Connections[i].Link)
	}
	o, err := configuredIsometricImageOptions(ms, render, input, isometricSVG)
	if err != nil {
		return nil, err
	}
	// SVG scaling belongs to its document viewport, not the quality canvas.
	o.Width, o.Height = 1600, 1000
	return d2svg.RenderIsometric(ctx, &board, &render, &o)
}
