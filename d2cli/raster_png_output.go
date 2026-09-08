package d2cli

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/d2lang/util-go/xmain"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

const pngBandHeight = 256

func pngFrameOptions() d2raster.FrameOptions {
	options := rasterFrameOptions(2, 0)
	// PNG retains one strip instead of a complete frame. Keep the dimension
	// and aggregate work limits, without imposing the full-canvas storage cap.
	options.MaxPixels = int64(rasterMaxDimension) * int64(rasterMaxDimension)
	return options
}

func renderPNGToWriter(ctx context.Context, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts, encoder *rasterPNGEncoder, output io.Writer) error {
	document, err := buildScene(ctx, inputPath, cacheImages, diagram, opts)
	if err != nil {
		return err
	}
	var localEncoder rasterPNGEncoder
	if encoder == nil {
		encoder = &localEncoder
		defer localEncoder.close()
	}
	return encodePNGBands(ctx, output, document, &encoder.bands)
}

func encodePNGBands(ctx context.Context, output io.Writer, document *d2scene.Document, encoder *rasterPNGBandEncoder) error {
	options := pngFrameOptions()
	started := false
	defer encoder.close()
	err := d2raster.RenderBands(ctx, document, options, pngBandHeight, func(band *image.NRGBA) error {
		if !started {
			// RenderBands validates the complete dimensions and resource budget
			// before this callback. Its opaque background guarantees RGB output.
			height := int(math.Ceil(document.LogicalHeight * options.Scale))
			if err := encoder.start(ctx, output, band.Bounds().Dx(), height, true); err != nil {
				return err
			}
			started = true
		}
		return encoder.append(band)
	})
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("d2raster: PNG renderer produced no rows")
	}
	return encoder.finish()
}

// writePNGWithStatus stages file output for atomic replacement, with Write's
// copy fallback when rename is unavailable. The renderer and encoder release
// each strip immediately. A failed render never replaces an existing
// destination. Stdout is streamed directly and can contain a prefix
// when its writer fails or the command is canceled after encoding begins.
func writePNGWithStatus(ctx context.Context, ms *xmain.State, path string, render func(io.Writer) error) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if path == "-" {
		output := &pngStatusWriter{output: ms.Stdout}
		if err := render(output); err != nil {
			return output.written, err
		}
		return output.written, runFinalizer(ctx, ms.Stdout.Close)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, err
	}
	output, err := os.CreateTemp(directory, "tmp-"+filepath.Base(path)+"-")
	if err != nil {
		// A writable destination can live in a directory that does not allow
		// creating siblings (for example a special output device). Spool to
		// the system temporary directory before using the non-atomic fallback.
		output, err = os.CreateTemp("", "d2-png-")
		if err != nil {
			return false, err
		}
	}
	defer os.Remove(output.Name())
	defer output.Close()
	if err := render(output); err != nil {
		return false, err
	}
	if err := output.Close(); err != nil {
		return false, err
	}
	return runStatusFinalizer(ctx, func() (bool, error) {
		if err := os.Rename(output.Name(), path); err != nil {
			// Match Write's fallback for destinations that cannot be replaced
			// atomically, without buffering the compressed PNG in memory.
			return copyPNGWithStatus(ctx, output.Name(), path)
		}
		return true, nil
	})
}

func copyPNGWithStatus(ctx context.Context, sourcePath, destinationPath string) (bool, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return false, err
	}
	defer source.Close()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	writer := &rasterPNGContextWriter{ctx: ctx, output: destination}
	_, copyErr := io.Copy(writer, source)
	return true, errors.Join(copyErr, destination.Close())
}

type pngStatusWriter struct {
	output  io.Writer
	written bool
}

func (w *pngStatusWriter) Write(p []byte) (int, error) {
	n, err := w.output.Write(p)
	w.written = w.written || n > 0
	if n != len(p) {
		err = errors.Join(err, io.ErrShortWrite)
	}
	return n, err
}
