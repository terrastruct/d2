package d2cli

import (
	"bytes"
	"context"
	"fmt"

	"github.com/d2lang/d2/d2plugin"
	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

// Native geometry is built from the compiled diagram. A legacy plugin's edits
// to flat SVG cannot be silently discarded, even when the output is native SVG.
// Match the ordinary native raster path: no-op postprocessors remain compatible.
func validateIsometricPostProcessor(ctx context.Context, plugin d2plugin.Plugin, diagram *d2target.Diagram, opts d2svg.RenderOpts) error {
	processor, ok := plugin.(d2plugin.PostProcessor)
	if !ok {
		return nil
	}
	boards, err := collectGIFBoards(ctx, diagram, d2isometricimg.MaxTimelineBoards)
	if err != nil {
		return err
	}
	disabled := false
	opts.Isometric = &disabled
	for _, board := range boards {
		if err := ctx.Err(); err != nil {
			return err
		}
		source, err := d2svg.Render(board, &opts)
		if err != nil {
			return err
		}
		processed, err := processor.PostProcess(ctx, bytes.Clone(source))
		if err != nil {
			return fmt.Errorf("isometric postprocessor validation: %w", err)
		}
		if !bytes.Equal(source, processed) {
			return fmt.Errorf("isometric export cannot apply flat SVG changes made by the layout plugin postprocessor; disable that postprocessor or use ordinary SVG")
		}
	}
	return nil
}
