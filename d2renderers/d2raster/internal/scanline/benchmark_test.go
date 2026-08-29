package scanline

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"
)

var benchmarkVectorDestination any

var benchmarkRGBASource = color.NRGBA{
	R: 0x40,
	G: 0x80,
	B: 0xc0,
	A: 0xff,
}

type benchmarkVectorCommand struct {
	kind uint8
	x1   float32
	y1   float32
	x2   float32
	y2   float32
	x3   float32
	y3   float32
}

const (
	benchmarkMoveTo uint8 = iota
	benchmarkLineTo
	benchmarkCubeTo
	benchmarkClosePath
)

type benchmarkVectorWorkload struct {
	name     string
	width    int
	height   int
	commands []benchmarkVectorCommand
}

// BenchmarkRasterizerWorkloads measures complete path submission and drawing.
// Fresh includes destination and rasterizer allocation. ReuseReset mirrors the
// d2raster scratch path: the destination remains live while Reset clears and
// reuses the rasterizer backing store for each primitive.
func BenchmarkRasterizerWorkloads(b *testing.B) {
	workloads := benchmarkVectorWorkloads()
	destinations := []string{"RGBA", "Alpha"}

	for _, workload := range workloads {
		workload := workload
		for _, destination := range destinations {
			destination := destination
			b.Run(workload.name+"/"+destination, func(b *testing.B) {
				b.Run("Fresh", func(b *testing.B) {
					benchmarkFreshRasterizer(b, workload, destination)
				})
				b.Run("ReuseReset", func(b *testing.B) {
					benchmarkReusedRasterizer(b, workload, destination)
				})
			})
		}
	}
}

func benchmarkFreshRasterizer(b *testing.B, workload benchmarkVectorWorkload, destination string) {
	b.Helper()
	b.ReportAllocs()
	bounds := image.Rect(0, 0, workload.width, workload.height)
	var dst any

	b.ResetTimer()
	b.ReportMetric(float64(workload.width*workload.height), "pixels/op")
	for range b.N {
		rasterizer := NewRasterizer(workload.width, workload.height)
		dst = newBenchmarkDestination(destination, bounds)
		replayBenchmarkVectorCommands(rasterizer, workload.commands)
		drawBenchmarkDestination(rasterizer, dst, destination)
	}
	benchmarkVectorDestination = dst
}

func benchmarkReusedRasterizer(b *testing.B, workload benchmarkVectorWorkload, destination string) {
	b.Helper()
	b.ReportAllocs()
	bounds := image.Rect(0, 0, workload.width, workload.height)
	rasterizer := NewRasterizer(workload.width, workload.height)
	dst := newBenchmarkDestination(destination, bounds)

	// Warm both the destination-specific Draw path and any lazily allocated
	// accumulation buffer before measuring Reset-backed reuse.
	replayBenchmarkVectorCommands(rasterizer, workload.commands)
	drawBenchmarkDestination(rasterizer, dst, destination)

	b.ResetTimer()
	b.ReportMetric(float64(workload.width*workload.height), "pixels/op")
	for range b.N {
		rasterizer.Reset(workload.width, workload.height)
		replayBenchmarkVectorCommands(rasterizer, workload.commands)
		drawBenchmarkDestination(rasterizer, dst, destination)
	}
	benchmarkVectorDestination = dst
}

func newBenchmarkDestination(kind string, bounds image.Rectangle) any {
	switch kind {
	case "RGBA":
		return image.NewRGBA(bounds)
	case "Alpha":
		return image.NewAlpha(bounds)
	default:
		panic("unknown benchmark destination: " + kind)
	}
}

func drawBenchmarkDestination(rasterizer *Rasterizer, destination any, kind string) {
	budget := NewWorkBudget(math.MaxInt64)
	var err error
	switch kind {
	case "RGBA":
		err = rasterizer.DrawRGBA(context.Background(), &budget, destination.(*image.RGBA), benchmarkRGBASource)
	case "Alpha":
		err = rasterizer.WriteAlpha(context.Background(), &budget, destination.(*image.Alpha))
	default:
		panic("unknown benchmark destination: " + kind)
	}
	if err != nil {
		panic(err)
	}
}

func replayBenchmarkVectorCommands(rasterizer *Rasterizer, commands []benchmarkVectorCommand) {
	for _, command := range commands {
		switch command.kind {
		case benchmarkMoveTo:
			rasterizer.MoveTo(command.x1, command.y1)
		case benchmarkLineTo:
			rasterizer.LineTo(command.x1, command.y1)
		case benchmarkCubeTo:
			rasterizer.CubeTo(command.x1, command.y1, command.x2, command.y2, command.x3, command.y3)
		case benchmarkClosePath:
			rasterizer.ClosePath()
		default:
			panic("unknown benchmark scanline command")
		}
	}
}

func benchmarkVectorWorkloads() []benchmarkVectorWorkload {
	return []benchmarkVectorWorkload{
		{
			name:   "SmallRectangle",
			width:  64,
			height: 64,
			commands: []benchmarkVectorCommand{
				{kind: benchmarkMoveTo, x1: 8.25, y1: 9.5},
				{kind: benchmarkLineTo, x1: 55.75, y1: 9.5},
				{kind: benchmarkLineTo, x1: 55.75, y1: 53.25},
				{kind: benchmarkLineTo, x1: 8.25, y1: 53.25},
				{kind: benchmarkClosePath},
			},
		},
		{
			name:   "SmallPolygon",
			width:  64,
			height: 64,
			commands: []benchmarkVectorCommand{
				{kind: benchmarkMoveTo, x1: 32, y1: 4},
				{kind: benchmarkLineTo, x1: 39, y1: 23},
				{kind: benchmarkLineTo, x1: 60, y1: 24},
				{kind: benchmarkLineTo, x1: 43, y1: 37},
				{kind: benchmarkLineTo, x1: 49, y1: 58},
				{kind: benchmarkLineTo, x1: 32, y1: 46},
				{kind: benchmarkLineTo, x1: 15, y1: 58},
				{kind: benchmarkLineTo, x1: 21, y1: 37},
				{kind: benchmarkLineTo, x1: 4, y1: 24},
				{kind: benchmarkLineTo, x1: 25, y1: 23},
				{kind: benchmarkClosePath},
			},
		},
		{
			name:     "OverlappingStrokeQuads256",
			width:    512,
			height:   512,
			commands: benchmarkOverlappingStrokeQuads(256),
		},
		{
			name:     "OverlappingCircles256",
			width:    512,
			height:   512,
			commands: benchmarkOverlappingCircles(256),
		},
		{
			name:     "LargeComplexPath1024Cubics",
			width:    1024,
			height:   1024,
			commands: benchmarkLargeComplexPath(1024),
		},
	}
}

func benchmarkOverlappingStrokeQuads(count int) []benchmarkVectorCommand {
	commands := make([]benchmarkVectorCommand, 0, count*5)
	for i := 0; i < count; i++ {
		offset := float32(i%64-32) * 2.5
		width := float32(2 + i%5)
		commands = append(commands,
			benchmarkVectorCommand{kind: benchmarkMoveTo, x1: 20, y1: 256 + offset},
			benchmarkVectorCommand{kind: benchmarkLineTo, x1: 492, y1: 256 - offset},
			benchmarkVectorCommand{kind: benchmarkLineTo, x1: 492, y1: 256 - offset + width},
			benchmarkVectorCommand{kind: benchmarkLineTo, x1: 20, y1: 256 + offset + width},
			benchmarkVectorCommand{kind: benchmarkClosePath},
		)
	}
	return commands
}

func benchmarkOverlappingCircles(count int) []benchmarkVectorCommand {
	const circleControl = float32(0.5522847498307936)
	commands := make([]benchmarkVectorCommand, 0, count*6)
	for i := 0; i < count; i++ {
		centerX := float32(226 + (i%16)*4)
		centerY := float32(226 + (i/16)*4)
		radius := float32(10 + i%7)
		control := radius * circleControl
		commands = append(commands,
			benchmarkVectorCommand{kind: benchmarkMoveTo, x1: centerX + radius, y1: centerY},
			benchmarkVectorCommand{kind: benchmarkCubeTo, x1: centerX + radius, y1: centerY + control, x2: centerX + control, y2: centerY + radius, x3: centerX, y3: centerY + radius},
			benchmarkVectorCommand{kind: benchmarkCubeTo, x1: centerX - control, y1: centerY + radius, x2: centerX - radius, y2: centerY + control, x3: centerX - radius, y3: centerY},
			benchmarkVectorCommand{kind: benchmarkCubeTo, x1: centerX - radius, y1: centerY - control, x2: centerX - control, y2: centerY - radius, x3: centerX, y3: centerY - radius},
			benchmarkVectorCommand{kind: benchmarkCubeTo, x1: centerX + control, y1: centerY - radius, x2: centerX + radius, y2: centerY - control, x3: centerX + radius, y3: centerY},
			benchmarkVectorCommand{kind: benchmarkClosePath},
		)
	}
	return commands
}

func benchmarkLargeComplexPath(segments int) []benchmarkVectorCommand {
	const (
		center = 512
		base   = 330
	)
	commands := make([]benchmarkVectorCommand, 0, segments+2)
	point := func(theta float64) (float32, float32) {
		radius := base + 92*math.Sin(17*theta) + 36*math.Sin(43*theta)
		return float32(center + radius*math.Cos(theta)), float32(center + radius*math.Sin(theta))
	}

	x, y := point(0)
	commands = append(commands, benchmarkVectorCommand{kind: benchmarkMoveTo, x1: x, y1: y})
	step := 2 * math.Pi / float64(segments)
	for i := 1; i <= segments; i++ {
		theta := float64(i) * step
		control1X, control1Y := point(theta - step*0.72)
		control2X, control2Y := point(theta - step*0.28)
		x, y = point(theta)
		commands = append(commands, benchmarkVectorCommand{
			kind: benchmarkCubeTo,
			x1:   control1X,
			y1:   control1Y,
			x2:   control2X,
			y2:   control2Y,
			x3:   x,
			y3:   y,
		})
	}
	commands = append(commands, benchmarkVectorCommand{kind: benchmarkClosePath})
	return commands
}
