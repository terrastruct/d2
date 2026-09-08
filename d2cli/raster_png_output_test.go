package d2cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/util-go/go2"
	"github.com/d2lang/util-go/xmain"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	d2png "github.com/d2lang/d2/lib/png"
)

func TestPNGStreamMatchesCompleteFrame(t *testing.T) {
	var encoder rasterPNGEncoder
	defer encoder.close()
	for _, shadow := range []bool{false, true} {
		for _, scale := range []float64{0.5, 1, 1.25} {
			diagram := simpleRasterDiagramWithSize(230, 590)
			diagram.Shapes[0].Shadow = shadow
			diagram.Shapes[0].BorderRadius = 13
			diagram.Shapes[0].Stroke = "#234567"
			diagram.Shapes[0].StrokeWidth = 3
			diagram.Shapes[0].Opacity = 0.73
			opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(11)), Scale: &scale}
			want, err := renderPNGWithEncoder(context.Background(), "-", false, diagram, opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			want, err = d2png.AddExif(want)
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := renderPNGToWriter(context.Background(), "-", false, diagram, opts, &encoder, &got); err != nil {
				t.Fatalf("shadow=%v scale=%v: %v", shadow, scale, err)
			}
			// Identical encoded bytes also verify PNG filtering across strip
			// boundaries and the position and contents of the EXIF chunk.
			if !bytes.Equal(got.Bytes(), want) {
				t.Logf("shadow=%v scale=%v", shadow, scale)
				assertPNGOutputPixels(t, got.Bytes(), want)
				t.Fatalf("shadow=%v scale=%v: PNG bytes differ despite equal pixels", shadow, scale)
			}
		}
	}
}

func TestPNGStreamDiagramFixtures(t *testing.T) {
	fixtures := []string{
		"txtar/gradient", "txtar/sketch-cross-arrowhead",
		"txtar/sequence-diagram-note-md", "regression/sql_table_overflow",
		"regression/md_font_weight", "themes/terminal",
		"real_world/queue_workers", "real_world/mocha_soc",
		"real_world/spyre_encoder", "real_world/ross_overview",
	}
	for _, fixture := range fixtures {
		for _, layout := range []string{"dagre", "elk"} {
			t.Run(fixture+"/"+layout, func(t *testing.T) {
				path := filepath.Join("..", "e2etests", "testdata", fixture, layout, "board.exp.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var diagram d2target.Diagram
				if err := json.Unmarshal(data, &diagram); err != nil {
					t.Fatal(err)
				}
				opts := d2svg.RenderOpts{}
				if config := diagram.Config; config != nil {
					opts.Pad, opts.ThemeID, opts.ThemeOverrides = config.Pad, config.ThemeID, config.ThemeOverrides
					opts.Center, opts.Sketch = config.Center, config.Sketch
				}
				want, err := renderPNGWithEncoder(context.Background(), "-", false, &diagram, opts, nil)
				if err != nil {
					t.Fatal(err)
				}
				want, err = d2png.AddExif(want)
				if err != nil {
					t.Fatal(err)
				}
				var got bytes.Buffer
				if err := renderPNGToWriter(context.Background(), "-", false, &diagram, opts, nil, &got); err != nil {
					t.Fatal(err)
				}
				assertPNGOutputPixels(t, got.Bytes(), want)
			})
		}
	}
}

func assertPNGOutputPixels(t *testing.T, got, want []byte) {
	t.Helper()
	// The common path is byte-identical and needs no decoded frame allocation.
	// If compression changes, still require every decoded pixel to match.
	if bytes.Equal(got, want) {
		return
	}
	gotImage, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	wantImage, err := png.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	if gotImage.Bounds() != wantImage.Bounds() {
		t.Fatalf("image bounds %v, want %v", gotImage.Bounds(), wantImage.Bounds())
	}
	for y := 0; y < wantImage.Bounds().Dy(); y++ {
		for x := 0; x < wantImage.Bounds().Dx(); x++ {
			gr, gg, gb, ga := gotImage.At(x, y).RGBA()
			wr, wg, wb, wa := wantImage.At(x, y).RGBA()
			if gr != wr || gg != wg || gb != wb || ga != wa {
				t.Fatalf("different pixel (%d,%d): %v, want %v", x, y, gotImage.At(x, y), wantImage.At(x, y))
			}
		}
	}
}

func TestPNGStreamAdmitsAreaBeyondCompleteFrameLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes a 67 megapixel image")
	}
	// A sparse scene just over the old area limit must no longer require a
	// complete 256 MiB canvas. Decode only its header to keep this test bounded.
	document := d2scene.NewDocument(d2scene.Box{Width: 4097, Height: 4096}, d2scene.NewNode(nil))
	var output pngHeaderCapture
	var encoder rasterPNGBandEncoder
	if err := encodePNGBands(context.Background(), &output, document, &encoder); err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(output.header))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 8194 || config.Height != 8192 {
		t.Fatalf("dimensions %dx%d, want 8194x8192", config.Width, config.Height)
	}
	if int64(config.Width)*int64(config.Height) <= rasterMaxPixels {
		t.Fatal("test did not cross the old area limit")
	}
}

type pngHeaderCapture struct{ header []byte }

func (w *pngHeaderCapture) Write(p []byte) (int, error) {
	if len(w.header) < 128 {
		w.header = append(w.header, p[:min(len(p), 128-len(w.header))]...)
	}
	return len(p), nil
}

func TestPNGStreamRejectsOversizeBeforeWriting(t *testing.T) {
	document := d2scene.NewDocument(d2scene.Box{Width: rasterMaxDimension, Height: 1}, d2scene.NewNode(nil))
	var output bytes.Buffer
	var encoder rasterPNGBandEncoder
	err := encodePNGBands(context.Background(), &output, document, &encoder)
	if err == nil || !strings.Contains(err.Error(), "frame width") {
		t.Fatalf("error %v, want dimension limit", err)
	}
	if output.Len() != 0 {
		t.Fatal("invalid image wrote PNG header")
	}
}

func TestPNGStreamFilePublication(t *testing.T) {
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "output.png")
		if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
		failure := errors.New("render failed")
		written, err := writePNGWithStatus(context.Background(), &xmain.State{}, path, func(w io.Writer) error {
			if _, err := io.WriteString(w, "replacement"); err != nil {
				return err
			}
			if fail {
				return failure
			}
			return nil
		})
		if fail && (!errors.Is(err, failure) || written) {
			t.Fatalf("failed render: written=%v err=%v", written, err)
		}
		if !fail && (err != nil || !written) {
			t.Fatalf("successful render: written=%v err=%v", written, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := "replacement"
		if fail {
			want = "existing"
		}
		if string(got) != want {
			t.Fatalf("destination %q, want %q", got, want)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatalf("temporary output leaked: %v, %v", entries, err)
		}
	}
}

func TestPNGStreamCanceledBeforePublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.png")
	ctx, cancel := context.WithCancel(context.Background())
	written, err := writePNGWithStatus(ctx, &xmain.State{}, path, func(w io.Writer) error {
		_, err := io.WriteString(w, "encoded")
		cancel()
		return err
	})
	if written || !errors.Is(err, context.Canceled) {
		t.Fatalf("written=%v error=%v", written, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("canceled render published destination: %v", err)
	}
}

func BenchmarkPNGStreamLarge(b *testing.B) {
	document := d2scene.NewDocument(d2scene.Box{Width: 2048, Height: 2048}, d2scene.NewNode(nil))
	var encoder rasterPNGBandEncoder
	b.ReportAllocs()
	b.SetBytes(4096 * 4096 * 4)
	for b.Loop() {
		if err := encodePNGBands(context.Background(), io.Discard, document, &encoder); err != nil {
			b.Fatal(err)
		}
	}
}
