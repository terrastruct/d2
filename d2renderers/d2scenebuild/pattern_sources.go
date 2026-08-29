package d2scenebuild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2renderers/internal/patternassets"
)

const (
	paperPatternSourceSHA256    = "98b3c110441c6c02cb685c0a7303e7a36c1faee7fa1f65f6424b48ec51123527"
	paperPatternPathCount       = 1042
	paperPatternCommandCount    = 10926
	paperPatternMaskPathData    = "M75.0871 -0.0878906H-0.0878906V75.0871H75.0871V-0.0878906Z"
	grainPatternPNGSourceSHA256 = "7aec37844fa033cbdb586efaa1955dc6f43d56707c0b1bf91e8b5e32386d8d69"
	grainPatternPNGBytes        = 20845
	grainPatternPixelWidth      = 466
	grainPatternPixelHeight     = 349
	grainPatternDecodedBytes    = int64(grainPatternPixelWidth * grainPatternPixelHeight * 4)
	sketchStreakCommandCount    = 687
)

type paperPatternPath struct {
	color    uint8
	commands []d2scene.PathCommand
}

type paperPatternSource struct {
	paths []paperPatternPath
}

type grainPatternSource struct {
	png          []byte
	pixelWidth   int
	pixelHeight  int
	decodedBytes int64
}

type patternSourceCache[T any] struct {
	mu      sync.Mutex
	loading chan struct{}
	value   T
	ready   bool
}

func (c *patternSourceCache[T]) load(ctx context.Context, parse func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		c.mu.Lock()
		if c.ready {
			value := c.value
			c.mu.Unlock()
			return value, nil
		}
		if loading := c.loading; loading != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-loading:
				continue
			}
		}

		loading := make(chan struct{})
		c.loading = loading
		c.mu.Unlock()
		value, err := parse(ctx)
		c.mu.Lock()
		if err == nil {
			c.value = value
			c.ready = true
		}
		c.loading = nil
		close(loading)
		c.mu.Unlock()
		return value, err
	}
}

var (
	paperPatternCache  patternSourceCache[*paperPatternSource]
	grainPatternCache  patternSourceCache[*grainPatternSource]
	streakPatternCache patternSourceCache[[]d2scene.PathCommand]
)

func sharedPaperPatternSource(ctx context.Context) (*paperPatternSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return paperPatternCache.load(ctx, func(ctx context.Context) (*paperPatternSource, error) {
		return parsePaperPatternSource(ctx, patternassets.PaperSVG())
	})
}

func sharedGrainPatternSource(ctx context.Context) (*grainPatternSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return grainPatternCache.load(ctx, func(ctx context.Context) (*grainPatternSource, error) {
		return parseGrainPatternSource(ctx, patternassets.GrainPNG())
	})
}

func sharedSketchStreakCommands(ctx context.Context) ([]d2scene.PathCommand, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return streakPatternCache.load(ctx, func(ctx context.Context) ([]d2scene.PathCommand, error) {
		return parseSketchStreakSource(ctx, patternassets.StreakPathData())
	})
}

func parsePaperPatternSource(ctx context.Context, source string) (*paperPatternSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sourceStringDigest(source) != paperPatternSourceSHA256 {
		return nil, fmt.Errorf("paper definition digest does not match its asset ID")
	}
	type sourcePath struct {
		data  string
		color uint8
	}
	paths := make([]sourcePath, 0, paperPatternPathCount+1)
	decoder := xml.NewDecoder(strings.NewReader(source))
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse paper XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "path" {
			continue
		}
		if len(paths)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		var data, fill string
		for _, attribute := range start.Attr {
			switch attribute.Name.Local {
			case "d":
				data = attribute.Value
			case "fill":
				fill = attribute.Value
			}
		}
		colorIndex, ok := paperPatternColorIndex(fill)
		if !ok || data == "" {
			return nil, fmt.Errorf("paper path %d has unsupported fill %q or empty data", len(paths), fill)
		}
		paths = append(paths, sourcePath{data: data, color: colorIndex})
	}
	if len(paths) != paperPatternPathCount+1 || paths[0].color != 6 || paths[0].data != paperPatternMaskPathData || paths[1].color != 0 {
		return nil, fmt.Errorf("paper definition topology changed: got %d paths", len(paths))
	}
	paths = paths[1:]

	parsed := &paperPatternSource{paths: make([]paperPatternPath, len(paths))}
	commandCount := 0
	for pathIndex, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		commands, err := d2svgimport.ParsePath(ctx, "paper pattern", path.data, d2svgimport.PathLimits{
			MaxBytes: len(path.data), MaxCommands: paperPatternCommandCount,
		})
		if err != nil {
			return nil, fmt.Errorf("paper path %d: %w", pathIndex, err)
		}
		if len(commands) == 0 || commandCount > paperPatternCommandCount-len(commands) {
			return nil, fmt.Errorf("paper path %d exceeds the command topology", pathIndex)
		}
		parsed.paths[pathIndex] = paperPatternPath{color: path.color, commands: commands}
		commandCount += len(commands)
	}
	if commandCount != paperPatternCommandCount {
		return nil, fmt.Errorf("paper command count %d, want %d", commandCount, paperPatternCommandCount)
	}
	return parsed, nil
}

func paperPatternColorIndex(fill string) (uint8, bool) {
	switch fill {
	case "black":
		return 0, true
	case "#EFEFEF":
		return 1, true
	case "#F5F5F5":
		return 2, true
	case "#F7F7F7":
		return 3, true
	case "#F9F9F9":
		return 4, true
	case "#FCFCFC":
		return 5, true
	case "white":
		return 6, true
	default:
		return 0, false
	}
}

func parseGrainPatternSource(ctx context.Context, pngBytes []byte) (*grainPatternSource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sourceBytesDigest(pngBytes) != grainPatternPNGSourceSHA256 {
		return nil, fmt.Errorf("grain definition digest does not match its asset ID")
	}
	if len(pngBytes) != grainPatternPNGBytes || len(pngBytes) < 24 ||
		!bytes.Equal(pngBytes[:8], []byte("\x89PNG\r\n\x1a\n")) || string(pngBytes[12:16]) != "IHDR" {
		return nil, fmt.Errorf("grain image is not the expected PNG")
	}
	width := int(binary.BigEndian.Uint32(pngBytes[16:20]))
	height := int(binary.BigEndian.Uint32(pngBytes[20:24]))
	if width != grainPatternPixelWidth || height != grainPatternPixelHeight {
		return nil, fmt.Errorf("grain dimensions changed to %dx%d", width, height)
	}
	return &grainPatternSource{
		png: pngBytes, pixelWidth: width, pixelHeight: height,
		decodedBytes: int64(width) * int64(height) * 4,
	}, nil
}

func parseSketchStreakSource(ctx context.Context, pathData string) ([]d2scene.PathCommand, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pathData == "" {
		return nil, fmt.Errorf("sketch streak path is empty")
	}
	commands, err := d2svgimport.ParsePath(ctx, "sketch streak pattern", pathData, d2svgimport.PathLimits{
		MaxBytes: len(pathData), MaxCommands: sketchStreakCommandCount,
	})
	if err != nil {
		return nil, err
	}
	if len(commands) != sketchStreakCommandCount {
		return nil, fmt.Errorf("sketch streak command count %d, want %d", len(commands), sketchStreakCommandCount)
	}
	return commands, nil
}

func sourceStringDigest(source string) string {
	digest := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x", digest)
}

func sourceBytesDigest(source []byte) string {
	digest := sha256.Sum256(source)
	return fmt.Sprintf("%x", digest)
}
