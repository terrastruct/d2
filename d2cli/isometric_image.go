package d2cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/xmain"
)

func isometricImageOptions(ext exportExtension, scale float64) (d2isometricimg.Options, error) {
	o := d2isometricimg.Options{Format: d2isometricimg.PNG, Width: 1600, Height: 1000}
	if ext == SVG || ext == isometricSVG {
		o.Format = d2isometricimg.SVG
	}
	if ext == GIF || ext == isometricGIF {
		o.Format = d2isometricimg.GIF
		o.Width, o.Height = 1000, 625
	}
	if scale == -1 {
		scale = 1
	}
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 || scale > float64(d2isometricimg.MaxDimension)/float64(o.Width) {
		return o, fmt.Errorf("--isometric requires a finite positive --scale within image limits")
	}
	o.Width = int(math.Round(float64(o.Width) * scale))
	o.Height = int(math.Round(float64(o.Height) * scale))
	if o.Width < 64 || o.Height < 64 || o.Width > d2isometricimg.MaxDimension || o.Height > d2isometricimg.MaxDimension || int64(o.Width)*int64(o.Height) > d2isometricimg.MaxPixels {
		return o, fmt.Errorf("--isometric dimensions must be at least 64 pixels per side and within image limits")
	}
	if o.Format == d2isometricimg.GIF && int64(o.Width)*int64(o.Height)*(d2isometricimg.FrameCount+4) > d2isometricimg.MaxAnimationPixels {
		return o, fmt.Errorf("--isometric GIF aggregate pixels exceed limit; reduce --scale")
	}
	return o, nil
}

// SVG scale changes document dimensions rather than allocating a larger bitmap.
// Its geometry canvas stays fixed, including for very small or large display sizes.
func isometricImageQualityOptions(ext exportExtension, scale float64) (d2isometricimg.Options, error) {
	if ext == SVG || ext == isometricSVG {
		if scale != -1 && (math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0) {
			return d2isometricimg.Options{}, fmt.Errorf("isometric SVG scale must be finite and positive")
		}
		scale = 1
	}
	return isometricImageOptions(ext, scale)
}

func renderIsometricImage(ctx context.Context, ms *xmain.State, diagram *d2target.Diagram, render d2svg.RenderOpts, input, output string, ext exportExtension) ([]byte, bool, error) {
	o, err := configuredIsometricImageOptions(ms, render, input, ext)
	if err != nil {
		return nil, false, err
	}
	var data []byte
	if ext == isometricSVG {
		// The shared SVG adapter applies Scale to the document dimensions. Keep
		// its native quality canvas independent from that display scaling.
		o.Width, o.Height = 1600, 1000
		data, err = d2svg.RenderIsometric(ctx, diagram, &render, &o)
	} else {
		data, err = d2isometricimg.Render(ctx, diagram, &o)
	}
	if err != nil {
		return nil, false, err
	}
	return writeIsometricImage(ctx, ms, output, data)
}

func configuredIsometricImageOptions(ms *xmain.State, render d2svg.RenderOpts, input string, ext exportExtension) (d2isometricimg.Options, error) {
	if ext == isometricSVG && render.DarkThemeID != nil {
		return d2isometricimg.Options{}, fmt.Errorf("isometric SVG does not yet support --dark-theme; render a separate SVG with --theme instead")
	}
	scale := 1.0
	if render.Scale != nil {
		scale = *render.Scale
	}
	o, err := isometricImageQualityOptions(ext, scale)
	if err != nil {
		return o, err
	}
	o.FitContent = true
	o.Render.ThemeID = render.ThemeID
	o.Render.ThemeOverrides = render.ThemeOverrides
	o.Fonts, err = newFontFallbackOptions(1)
	if err != nil {
		return o, err
	}
	o.Assets, err = sceneAssetOptions(input, ms.Env.Getenv("IMG_CACHE") == "1")
	if err != nil {
		return o, err
	}
	return o, nil
}

func writeIsometricImage(ctx context.Context, ms *xmain.State, output string, data []byte) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if output == "-" {
		written, err := writeStdout(ms.Stdout, data)
		return data, written, err
	}
	if err := atomicIsometricImage(ctx, output, data); err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Follow the same depth-first root/layers/scenarios/steps order and directory
// naming as D2's static exports. Folder-only boards contribute no blank image.
func renderIsometricImages(ctx context.Context, ms *xmain.State, diagram *d2target.Diagram, render d2svg.RenderOpts, input, output string, ext exportExtension, intervalMS int64, sourceRootID string) ([]byte, bool, error) {
	boards, err := collectGIFBoards(ctx, diagram, d2isometricimg.MaxTimelineBoards)
	if err != nil {
		return nil, false, err
	}
	if len(boards) == 0 {
		return nil, false, fmt.Errorf("isometric export has no drawable boards")
	}
	if intervalMS < 0 || intervalMS > 600000 || intervalMS > 0 && ext != isometricGIF {
		return nil, false, fmt.Errorf("--isometric --animate-interval requires GIF and an interval from 1 to 600000 milliseconds")
	}
	hasSubboards := len(diagram.Layers)+len(diagram.Scenarios)+len(diagram.Steps) > 0
	if ext == isometricGIF && (hasSubboards || len(boards) > 1 || intervalMS > 0) {
		if intervalMS == 0 {
			intervalMS = 1000
		}
		o, err := configuredIsometricImageOptions(ms, render, input, ext)
		if err != nil {
			return nil, false, err
		}
		data, err := d2isometricimg.RenderTimeline(ctx, boards, &o, time.Duration(intervalMS)*time.Millisecond)
		if err != nil {
			return nil, false, err
		}
		return writeIsometricImage(ctx, ms, output, data)
	}
	if output == "-" {
		if len(boards) != 1 {
			format := "PNG"
			if ext == isometricSVG {
				format = "SVG"
			}
			return nil, false, fmt.Errorf("isometric %s stdout requires one board; use --target or an output filename for multiple boards", format)
		}
		board := boards[0]
		if ext == isometricSVG {
			board, err = relinkIsometricSVGBoard(board, "root", sourceRootID, map[string]string{"root": output})
			if err != nil {
				return nil, false, err
			}
		}
		return renderIsometricImage(ctx, ms, board, render, input, output, ext)
	}
	// A selected leaf retains the requested filename, just as single-board isometric
	// exports did before traversal support was added.
	if !hasSubboards {
		if ext == isometricSVG {
			diagram, err = relinkIsometricSVGBoard(diagram, "root", sourceRootID, map[string]string{"root": output})
			if err != nil {
				return nil, false, err
			}
		}
		return renderIsometricImage(ctx, ms, diagram, render, input, output, ext)
	}
	paths, err := resolveLinks("root", output, diagram)
	if err != nil {
		return nil, false, err
	}
	type boardOutput struct {
		board     *d2target.Diagram
		key, path string
	}
	var outputs []boardOutput
	usedPaths := make(map[string]bool)
	base := strings.TrimSuffix(output, filepath.Ext(output))
	var walk func(*d2target.Diagram, string) error
	walk = func(board *d2target.Diagram, key string) error {
		if !board.IsFolderOnly {
			path, exists := paths[key]
			if !exists || usedPaths[path] {
				return fmt.Errorf("isometric board output path is missing or duplicated: %s", key)
			}
			rel, err := filepath.Rel(base, path)
			if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("isometric board name escapes the output directory: %s", key)
			}
			usedPaths[path] = true
			outputs = append(outputs, boardOutput{board: board, key: key, path: path})
		}
		for i, children := range [][]*d2target.Diagram{board.Layers, board.Scenarios, board.Steps} {
			kind := []string{"layers", "scenarios", "steps"}[i]
			for _, child := range children {
				if err := walk(child, strings.Join([]string{key, kind, child.Name}, ".")); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(diagram, "root"); err != nil {
		return nil, false, err
	}
	var written bool
	var last []byte
	for _, item := range outputs {
		board := item.board
		if ext == isometricSVG {
			board, err = relinkIsometricSVGBoard(board, item.key, sourceRootID, paths)
			if err != nil {
				return nil, written, err
			}
		}
		data, didWrite, err := renderIsometricImage(ctx, ms, board, render, input, item.path, ext)
		written = written || didWrite
		if err != nil {
			return nil, written, fmt.Errorf("isometric board %s: %w", item.key, err)
		}
		last = data
	}
	return last, written, nil
}

// Rebase source board links onto the static export paths without changing the
// compiled diagram shared with other exports. Selected subtrees retain their
// original source IDs while output traversal starts at root.
func relinkIsometricSVGBoard(diagram *d2target.Diagram, key, sourceRootID string, paths map[string]string) (*d2target.Diagram, error) {
	if sourceRootID == "" {
		sourceRootID = "root"
	}
	board := *diagram
	board.Shapes = append([]d2target.Shape(nil), diagram.Shapes...)
	board.Connections = append([]d2target.Connection(nil), diagram.Connections...)
	rebase := func(link string) (string, error) {
		target := link
		if sourceRootID != "root" && (link == sourceRootID || strings.HasPrefix(link, sourceRootID+".")) {
			target = "root" + strings.TrimPrefix(link, sourceRootID)
		}
		output, exists := paths[target]
		if !exists {
			return link, nil
		}
		rel, err := filepath.Rel(filepath.Dir(paths[key]), output)
		return filepath.ToSlash(rel), err
	}
	for i := range board.Shapes {
		var err error
		board.Shapes[i].Link, err = rebase(board.Shapes[i].Link)
		if err != nil {
			return nil, err
		}
	}
	for i := range board.Connections {
		var err error
		board.Connections[i].Link, err = rebase(board.Connections[i].Link)
		if err != nil {
			return nil, err
		}
	}
	return &board, nil
}

// Never fall back to truncating the previous output after an atomic-write
// failure. Encoding and cancellation checks complete before the rename.
func atomicIsometricImage(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".d2-isometric-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	mode := os.FileMode(0600)
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode().IsRegular() {
		mode = info.Mode().Perm()
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
