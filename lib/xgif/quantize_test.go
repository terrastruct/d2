package xgif

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestQuantizeImageMatchesGIFConversion(t *testing.T) {
	tests := []struct {
		image  image.Image
		opaque bool
	}{
		{image: image.NewNRGBA(image.Rect(0, 0, 1, 1))},
		{image: image.NewNRGBA(image.Rect(5, 7, 11, 13))},
		{image: image.NewRGBA(image.Rect(-3, -2, 14, 9))},
		{image: image.NewNRGBA(image.Rect(0, 0, 128, 96))},
		{image: image.NewNRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103))},
		{image: image.NewRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103))},
		{image: image.NewNRGBA(image.Rect(5, 7, 133, 103)), opaque: true},
		{image: image.NewRGBA(image.Rect(-5, -7, 123, 89)), opaque: true},
		{image: image.NewNRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)), opaque: true},
		{image: image.NewRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)), opaque: true},
	}
	state := uint32(42)
	for _, test := range tests {
		img := test.image
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				state = state*1664525 + 1013904223
				value := color.NRGBA{
					R: uint8(state >> 24),
					G: uint8(state >> 16),
					B: uint8(state >> 8),
					A: uint8(state),
				}
				if test.opaque {
					value.A = 255
				}
				switch target := img.(type) {
				case *image.NRGBA:
					target.SetNRGBA(x, y, value)
				case *image.RGBA:
					target.Set(x, y, value)
				}
			}
		}
	}

	for index, test := range tests {
		img := test.image
		want, err := quantizeImageThroughGIF(context.Background(), img)
		if err != nil {
			t.Fatalf("case %d GIF conversion: %v", index, err)
		}
		got, err := QuantizeImage(context.Background(), img)
		if err != nil {
			t.Fatalf("case %d direct conversion: %v", index, err)
		}
		if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("case %d indexed pixels differ: bounds %v/%v, stride %d/%d, pixels equal %v", index, got.Bounds(), want.Bounds(), got.Stride, want.Stride, bytes.Equal(got.Pix, want.Pix))
		}
		if !reflect.DeepEqual(got.Palette, want.Palette) {
			t.Fatalf("case %d palettes differ:\n got: %#v\nwant: %#v", index, got.Palette, want.Palette)
		}

		gotGIF, err := animatePalettedImages(context.Background(), []*image.Paletted{got}, 1000)
		if err != nil {
			t.Fatalf("case %d encode direct conversion: %v", index, err)
		}
		wantGIF, err := animatePalettedImages(context.Background(), []*image.Paletted{want}, 1000)
		if err != nil {
			t.Fatalf("case %d encode GIF conversion: %v", index, err)
		}
		if !bytes.Equal(gotGIF, wantGIF) {
			t.Fatalf("case %d encoded GIF bytes differ", index)
		}
	}
}

func TestFewOpaqueColorFastPathMatchesGIFConversion(t *testing.T) {
	for _, colorCount := range []int{1, 2, 3, 4, 15, 16, 17, 127, 255, 256} {
		bounds := image.Rect(5, 7, 134, 104)
		for _, rgba := range []bool{false, true} {
			var img image.Image
			if rgba {
				img = image.NewRGBA(image.Rect(0, 0, 140, 110)).SubImage(bounds)
			} else {
				img = image.NewNRGBA(image.Rect(0, 0, 140, 110)).SubImage(bounds)
			}
			colors := make([]color.NRGBA, colorCount)
			state := uint32(42)
			seen := make(map[[3]uint8]bool, colorCount)
			for index := range colors {
				for {
					state = state*1664525 + 1013904223
					channels := [3]uint8{uint8(state >> 24), uint8(state >> 16), uint8(state >> 8)}
					if !seen[channels] {
						seen[channels] = true
						colors[index] = color.NRGBA{R: channels[0], G: channels[1], B: channels[2], A: 255}
						break
					}
				}
			}
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					value := colors[(x*31+y*17)%len(colors)]
					switch target := img.(type) {
					case *image.RGBA:
						target.Set(x, y, value)
					case *image.NRGBA:
						target.SetNRGBA(x, y, value)
					}
				}
			}
			var sourcePixels []byte
			switch source := img.(type) {
			case *image.RGBA:
				sourcePixels = source.Pix
			case *image.NRGBA:
				sourcePixels = source.Pix
			}
			before := bytes.Clone(sourcePixels)
			want, err := quantizeImageThroughGIF(context.Background(), img)
			if err != nil {
				t.Fatal(err)
			}
			got, err := QuantizeImage(context.Background(), img)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
				t.Fatalf("%T with %d colors direct conversion differs", img, colorCount)
			}
			gotGIF, err := animatePalettedImages(context.Background(), []*image.Paletted{got}, 1000)
			if err != nil {
				t.Fatal(err)
			}
			wantGIF, err := animatePalettedImages(context.Background(), []*image.Paletted{want}, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotGIF, wantGIF) {
				t.Fatalf("%T with %d colors encoded GIF bytes differ", img, colorCount)
			}
			if !bytes.Equal(sourcePixels, before) {
				t.Fatalf("%T with %d colors modified its source", img, colorCount)
			}
		}
	}
}

func TestFewOpaqueColorFastPathMatchesSmallCollisionCases(t *testing.T) {
	state := uint32(42)
	for caseIndex := 1; caseIndex <= 128; caseIndex++ {
		width := caseIndex%31 + 1
		height := caseIndex%13 + 1
		bounds := image.Rect(3, 5, 3+width, 5+height)
		backingBounds := image.Rect(0, 0, bounds.Max.X+2, bounds.Max.Y+2)
		var img image.Image
		if caseIndex&1 == 0 {
			img = image.NewRGBA(backingBounds).SubImage(bounds)
		} else {
			img = image.NewNRGBA(backingBounds).SubImage(bounds)
		}
		colorCount := min(width*height, caseIndex%17+1)
		colors := make([]color.NRGBA, colorCount)
		for index := range colors {
			// Low bits repeat frequently after the quantizer's modulo operation,
			// exercising its quadratic collision sequence.
			state = state*1664525 + 1013904223
			colors[index] = color.NRGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(index * width * height * 2), A: 255}
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				value := colors[(x*7+y*13+caseIndex)%len(colors)]
				switch target := img.(type) {
				case *image.RGBA:
					target.Set(x, y, value)
				case *image.NRGBA:
					target.SetNRGBA(x, y, value)
				}
			}
		}
		want, err := quantizeImageThroughGIF(context.Background(), img)
		if err != nil {
			t.Fatal(err)
		}
		got, err := QuantizeImage(context.Background(), img)
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
			t.Fatalf("case %d %T with %d colors direct conversion differs", caseIndex, img, colorCount)
		}
	}
}

func TestFewOpaqueColorFastPathObservesCancellationOnReturn(t *testing.T) {
	ctx := &cancelOnSecondCheckContext{}
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for index := 3; index < len(img.Pix); index += 4 {
		img.Pix[index] = 255
	}
	if _, err := QuantizeImage(ctx, img); !errors.Is(err, context.Canceled) {
		t.Fatalf("QuantizeImage cancellation = %v, want context.Canceled", err)
	}
}

func TestFewOpaqueColorCensusDetectsLateTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for pixel := 0; pixel < len(img.Pix)/4; pixel++ {
		offset := pixel * 4
		img.Pix[offset] = uint8(pixel)
		img.Pix[offset+1] = uint8(pixel >> 8)
		img.Pix[offset+2] = uint8(pixel >> 16)
		img.Pix[offset+3] = 255
	}
	img.Pix[len(img.Pix)-1] = 127
	if paletted, opaque := quantizeFewOpaqueColors(img); paletted != nil || opaque {
		t.Fatalf("late-transparency census = (paletted=%v, opaque=%v), want (false, false)", paletted != nil, opaque)
	}
	want, err := quantizeImageThroughGIF(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	got, err := QuantizeImage(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
		t.Fatal("late-transparency direct conversion differs")
	}
}

func TestOpaqueRasterPixelsChecksEveryAlphaLane(t *testing.T) {
	for pixelCount := 0; pixelCount <= 73; pixelCount++ {
		pixels := make([]byte, pixelCount*4)
		for index := range pixels {
			pixels[index] = uint8(index*37 + pixelCount*11)
		}
		for offset := 3; offset < len(pixels); offset += 4 {
			pixels[offset] = 255
		}
		if !opaqueRasterPixels(pixels) {
			t.Fatalf("%d opaque pixels reported transparency", pixelCount)
		}
		for pixel := 0; pixel < pixelCount; pixel++ {
			pixels[pixel*4+3] = uint8(pixel)
			if opaqueRasterPixels(pixels) {
				t.Fatalf("%d-pixel input missed transparency at pixel %d", pixelCount, pixel)
			}
			pixels[pixel*4+3] = 255
		}
	}
}

type cancelOnSecondCheckContext struct {
	checks int
}

func (*cancelOnSecondCheckContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*cancelOnSecondCheckContext) Done() <-chan struct{}       { return nil }
func (*cancelOnSecondCheckContext) Value(any) any               { return nil }
func (ctx *cancelOnSecondCheckContext) Err() error {
	ctx.checks++
	if ctx.checks >= 2 {
		return context.Canceled
	}
	return nil
}

func TestPaletteTreeMatchesLinearNearestColor(t *testing.T) {
	state := uint32(42)
	for _, paletteLength := range []int{1, 2, 3, 16, 127, 255, 256} {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			state = state*1664525 + 1013904223
			palette[index] = color.RGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: 255}
		}
		if paletteLength > 2 {
			palette[paletteLength-1] = palette[0]
		}
		tree := newPaletteTree(palette, 3)
		tree.prepareSeeds()
		for sample := 0; sample < 20_000; sample++ {
			state = state*1664525 + 1013904223
			r := int32(state & 0xffff)
			state = state*1664525 + 1013904223
			g := int32(state & 0xffff)
			state = state*1664525 + 1013904223
			b := int32(state & 0xffff)

			wantIndex := 0
			wantDistance := ^uint32(0)
			for index, c := range palette {
				pr, pg, pb, _ := c.RGBA()
				distance := squaredColorDifference(r, int32(pr)) +
					squaredColorDifference(g, int32(pg)) +
					squaredColorDifference(b, int32(pb))
				if distance < wantDistance {
					wantIndex = index
					wantDistance = distance
				}
			}
			if got := int(tree.nearest(r, g, b)); got != wantIndex {
				t.Fatalf("palette %d sample %d nearest index = %d, want %d", paletteLength, sample, got, wantIndex)
			}
		}
	}
}

func TestPaletteTreeMatchesLinearNearestRGBA(t *testing.T) {
	state := uint32(42)
	for _, paletteLength := range []int{1, 2, 3, 16, 127, 255, 256} {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			state = state*1664525 + 1013904223
			palette[index] = color.RGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: uint8(state)}
		}
		if paletteLength > 2 {
			palette[paletteLength-1] = palette[0]
		}
		tree := newPaletteTree(palette, 4)
		for sample := 0; sample < 20_000; sample++ {
			state = state*1664525 + 1013904223
			r := int32(state & 0xffff)
			state = state*1664525 + 1013904223
			g := int32(state & 0xffff)
			state = state*1664525 + 1013904223
			b := int32(state & 0xffff)
			state = state*1664525 + 1013904223
			a := int32(state & 0xffff)

			wantIndex := 0
			wantDistance := ^uint32(0)
			for index, c := range palette {
				pr, pg, pb, pa := c.RGBA()
				distance := squaredColorDifference(r, int32(pr)) +
					squaredColorDifference(g, int32(pg)) +
					squaredColorDifference(b, int32(pb)) +
					squaredColorDifference(a, int32(pa))
				if distance < wantDistance {
					wantIndex = index
					wantDistance = distance
				}
			}
			if got := int(tree.nearestRGBA(r, g, b, a)); got != wantIndex {
				t.Fatalf("palette %d sample %d nearest index = %d, want %d", paletteLength, sample, got, wantIndex)
			}
		}
	}
}

func TestPaletteTreeTraversalStackCapacity(t *testing.T) {
	// A depth-first traversal that pushes the far child before the near child
	// retains at most one far node per ancestor, so its pending-node bound is
	// the tree depth. Exercise every supported non-linear palette size and both
	// tree dimensionalities to lock that bound below the fixed stack capacity.
	for paletteLength := 17; paletteLength <= 256; paletteLength++ {
		palette := make(color.Palette, paletteLength)
		for index := range palette {
			palette[index] = color.RGBA{
				R: uint8(index),
				G: uint8(index * 17),
				B: uint8(index * 31),
				A: uint8(index * 47),
			}
		}
		for _, dimensions := range []int{3, 4} {
			tree := newPaletteTree(palette, dimensions)
			depth := paletteTreeDepth(tree, tree.root)
			pendingBound := paletteTreePendingBound(tree, tree.root)
			if depth > 9 {
				t.Fatalf("palette length %d dimensions %d tree depth = %d, want <= 9", paletteLength, dimensions, depth)
			}
			if pendingBound > 9 {
				t.Fatalf("palette length %d dimensions %d pending bound = %d, want <= 9", paletteLength, dimensions, pendingBound)
			}
			if pendingBound > paletteTraversalStackCapacity {
				t.Fatalf("palette length %d dimensions %d pending bound %d exceeds stack capacity %d", paletteLength, dimensions, pendingBound, paletteTraversalStackCapacity)
			}
		}
	}
}

func paletteTreeDepth(tree paletteTree, nodeIndex int16) int {
	if nodeIndex < 0 {
		return 0
	}
	node := tree.nodes[nodeIndex]
	return 1 + max(paletteTreeDepth(tree, node.left), paletteTreeDepth(tree, node.right))
}

func paletteTreePendingBound(tree paletteTree, nodeIndex int16) int {
	if nodeIndex < 0 {
		return 0
	}
	node := tree.nodes[nodeIndex]
	left := paletteTreePendingBound(tree, node.left)
	right := paletteTreePendingBound(tree, node.right)
	if left == 0 {
		return max(1, right)
	}
	if right == 0 {
		return max(1, left)
	}
	// Both child nodes are pushed together. While the near subtree is
	// traversed, the far child remains pending; either child can be near.
	return max(2, 1+max(left, right))
}

func TestDitherOpaqueRasterMatchesFloydSteinberg(t *testing.T) {
	palette := make(color.Palette, 256)
	state := uint32(42)
	for index := range palette {
		state = state*1664525 + 1013904223
		palette[index] = color.RGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: 255}
	}
	tests := []image.Image{
		image.NewNRGBA(image.Rect(-5, -7, 123, 89)),
		image.NewRGBA(image.Rect(5, 7, 133, 103)),
		image.NewNRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)),
		image.NewRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)),
	}
	for _, img := range tests {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				state = state*1664525 + 1013904223
				value := color.NRGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: 255}
				switch target := img.(type) {
				case *image.NRGBA:
					target.SetNRGBA(x, y, value)
				case *image.RGBA:
					target.Set(x, y, value)
				}
			}
		}
		want := image.NewPaletted(bounds, palette)
		draw.FloydSteinberg.Draw(want, bounds, img, bounds.Min)
		got := image.NewPaletted(bounds, palette)
		ditherOpaqueRaster(got, img)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%T bounds %v indexed pixels differ", img, bounds)
		}
	}
}

func TestDitherTranslucentRasterMatchesFloydSteinberg(t *testing.T) {
	palette := make(color.Palette, 256)
	state := uint32(42)
	for index := range palette {
		state = state*1664525 + 1013904223
		palette[index] = color.RGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: uint8(state)}
	}
	tests := []image.Image{
		image.NewNRGBA(image.Rect(-5, -7, 123, 89)),
		image.NewRGBA(image.Rect(5, 7, 133, 103)),
		image.NewNRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)),
		image.NewRGBA(image.Rect(0, 0, 140, 110)).SubImage(image.Rect(5, 7, 133, 103)),
	}
	for _, img := range tests {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				state = state*1664525 + 1013904223
				value := color.NRGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: uint8(state)}
				switch target := img.(type) {
				case *image.NRGBA:
					target.SetNRGBA(x, y, value)
				case *image.RGBA:
					target.Set(x, y, value)
				}
			}
		}
		want := image.NewPaletted(bounds, palette)
		draw.FloydSteinberg.Draw(want, bounds, img, bounds.Min)
		got := image.NewPaletted(bounds, palette)
		ditherTranslucentRaster(got, img)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%T bounds %v indexed pixels differ", img, bounds)
		}
	}
}

func TestQuantizeImageDoesNotModifyRaster(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		img := image.NewNRGBA(image.Rect(5, 7, 133, 103))
		state := uint32(42)
		for index := 0; index < len(img.Pix); index += 4 {
			state = state*1664525 + 1013904223
			img.Pix[index] = uint8(state >> 24)
			img.Pix[index+1] = uint8(state >> 16)
			img.Pix[index+2] = uint8(state >> 8)
			img.Pix[index+3] = uint8(state)
			if opaque {
				img.Pix[index+3] = 255
			}
		}
		before := append([]byte(nil), img.Pix...)
		if _, err := QuantizeImage(context.Background(), img); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(img.Pix, before) {
			t.Fatalf("opaque %v source pixels were modified", opaque)
		}
	}
}

func TestOpaqueQuantizationWorkspaceMatchesOwnedOutputAndReusesPixels(t *testing.T) {
	for _, fewColors := range []bool{false, true} {
		img := image.NewNRGBA(image.Rect(-7, 11, 121, 107))
		state := uint32(42)
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				state = state*1664525 + 1013904223
				pixel := color.NRGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: 255}
				if fewColors {
					pixel = color.NRGBA{R: uint8(x&1) * 255, G: uint8(y&1) * 255, A: 255}
				}
				img.SetNRGBA(x, y, pixel)
			}
		}
		want, err := QuantizeImage(context.Background(), img)
		if err != nil {
			t.Fatal(err)
		}

		var workspace OpaqueQuantizationWorkspace
		var firstFrame *image.Paletted
		var firstPixel *byte
		for iteration := 0; iteration < 2; iteration++ {
			err := workspace.Quantize(context.Background(), img, func(got *image.Paletted) error {
				if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
					t.Fatalf("fewColors=%v iteration=%d workspace output differs", fewColors, iteration)
				}
				if iteration == 0 {
					firstFrame = got
					firstPixel = &got.Pix[0]
				} else if got != firstFrame || &got.Pix[0] != firstPixel {
					t.Fatalf("fewColors=%v workspace did not reuse its indexed frame", fewColors)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestOpaqueQuantizationWorkspaceMatchesOwnedSequentialEncoding(t *testing.T) {
	const (
		width  = 73
		height = 59
	)
	sources := make([]image.Image, 4)
	for frameIndex := range sources {
		var source image.Image
		if frameIndex&1 == 0 {
			source = image.NewNRGBA(image.Rect(0, 0, width, height))
		} else {
			source = image.NewRGBA(image.Rect(0, 0, width, height))
		}
		state := uint32(42 + frameIndex)
		for y := range height {
			for x := range width {
				state = state*1664525 + 1013904223
				value := color.NRGBA{R: uint8(state >> 24), G: uint8(state >> 16), B: uint8(state >> 8), A: 255}
				if frameIndex&2 == 0 {
					value = color.NRGBA{R: uint8(x&1) * 255, G: uint8(y&1) * 255, B: uint8(frameIndex) * 31, A: 255}
				}
				switch typed := source.(type) {
				case *image.NRGBA:
					typed.SetNRGBA(x, y, value)
				case *image.RGBA:
					typed.Set(x, y, value)
				}
			}
		}
		sources[frameIndex] = source
	}

	owned := make([]*image.Paletted, len(sources))
	for index, source := range sources {
		var err error
		owned[index], err = QuantizeImage(context.Background(), source)
		if err != nil {
			t.Fatalf("owned frame %d: %v", index, err)
		}
	}
	want := encodeOpaquePalettedFramesForTest(t, owned, 1000, 1<<30)

	encoder, err := NewOpaquePalettedAnimationEncoder(context.Background(), width, height, len(sources), 1000, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	var workspace OpaqueQuantizationWorkspace
	for index, source := range sources {
		err := workspace.Quantize(context.Background(), source, func(frame *image.Paletted) error {
			if frame.Bounds() != owned[index].Bounds() || frame.Stride != owned[index].Stride ||
				!bytes.Equal(frame.Pix, owned[index].Pix) || !reflect.DeepEqual(frame.Palette, owned[index].Palette) {
				t.Fatalf("workspace frame %d differs from owned quantization", index)
			}
			return encoder.WriteFrame(frame)
		})
		if err != nil {
			t.Fatalf("workspace frame %d: %v", index, err)
		}
	}
	got, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("workspace animation differs from owned animation: %d versus %d bytes", len(got), len(want))
	}
}

func TestOpaqueQuantizationWorkspaceValidationAndCancellation(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for offset := 3; offset < len(img.Pix); offset += 4 {
		img.Pix[offset] = 255
	}
	consume := func(*image.Paletted) error { return nil }
	var nilWorkspace *OpaqueQuantizationWorkspace
	if err := nilWorkspace.Quantize(context.Background(), img, consume); err == nil {
		t.Fatal("nil opaque quantization workspace was accepted")
	}
	var workspace OpaqueQuantizationWorkspace
	if err := workspace.Quantize(context.Background(), img, nil); err == nil {
		t.Fatal("nil opaque quantization consumer was accepted")
	}
	if err := workspace.Quantize(context.Background(), boundsOnlyImage{bounds: image.Rect(0, 0, 2, 2)}, consume); err == nil {
		t.Fatal("non-raster image was accepted")
	}
	img.Pix[len(img.Pix)-1] = 127
	called := false
	if err := workspace.Quantize(context.Background(), img, func(*image.Paletted) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatalf("translucent workspace input = called %v/error %v, want false/error", called, err)
	}
	img.Pix[len(img.Pix)-1] = 255
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := workspace.Quantize(ctx, img, consume); !errors.Is(err, context.Canceled) {
		t.Fatalf("workspace cancellation = %v, want context.Canceled", err)
	}
	wantErr := errors.New("consume failed")
	if err := workspace.Quantize(context.Background(), img, func(*image.Paletted) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("workspace consumer error = %v, want %v", err, wantErr)
	}
	want, err := QuantizeImage(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Quantize(context.Background(), img, func(got *image.Paletted) error {
		if got.Bounds() != want.Bounds() || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) || !reflect.DeepEqual(got.Palette, want.Palette) {
			t.Fatal("workspace did not recover after input and consumer errors")
		}
		return nil
	}); err != nil {
		t.Fatalf("workspace reuse after errors: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if err := workspace.Quantize(ctx, img, func(*image.Paletted) error {
		cancel()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("workspace post-consumer cancellation = %v, want context.Canceled", err)
	}
}

func BenchmarkQuantizeImage(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	translucent := image.NewNRGBA(img.Bounds())
	fewColors := image.NewNRGBA(img.Bounds())
	colors := [...]color.NRGBA{
		{R: 255, A: 255},
		{G: 255, A: 255},
		{B: 255, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
	}
	state := uint32(42)
	for index := 0; index < len(img.Pix); index += 4 {
		state = state*1664525 + 1013904223
		img.Pix[index] = uint8(state >> 24)
		img.Pix[index+1] = uint8(state >> 16)
		img.Pix[index+2] = uint8(state >> 8)
		img.Pix[index+3] = 255
		copy(translucent.Pix[index:index+4], img.Pix[index:index+4])
		translucent.Pix[index+3] = uint8(state)
		fewColors.SetNRGBA((index/4)%512, (index/4)/512, colors[(index/4)%len(colors)])
	}
	b.Run("Direct", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := QuantizeImage(context.Background(), img); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := QuantizeImage(context.Background(), img); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("GIFConversion", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := quantizeImageThroughGIF(context.Background(), img); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := quantizeImageThroughGIF(context.Background(), img); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("TranslucentDirect", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := QuantizeImage(context.Background(), translucent); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := QuantizeImage(context.Background(), translucent); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("TranslucentGIFConversion", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := quantizeImageThroughGIF(context.Background(), translucent); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := quantizeImageThroughGIF(context.Background(), translucent); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("FewColorsDirect", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := QuantizeImage(context.Background(), fewColors); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := QuantizeImage(context.Background(), fewColors); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("FewColorsGIFConversion", func(b *testing.B) {
		b.ReportAllocs()
		if _, err := quantizeImageThroughGIF(context.Background(), fewColors); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := quantizeImageThroughGIF(context.Background(), fewColors); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkQuantizeImageOpaqueSizes(b *testing.B) {
	for _, size := range []int{17, 24, 32, 48, 64, 96, 128, 192, 256, 384, 512} {
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		state := uint32(42)
		for index := 0; index < len(img.Pix); index += 4 {
			state = state*1664525 + 1013904223
			img.Pix[index] = uint8(state >> 24)
			img.Pix[index+1] = uint8(state >> 16)
			img.Pix[index+2] = uint8(state >> 8)
			img.Pix[index+3] = 255
		}
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := QuantizeImage(context.Background(), img); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var quantizedPixelBenchmarkSink byte

func BenchmarkOpaqueQuantizationWorkspace(b *testing.B) {
	for _, fewColors := range []bool{false, true} {
		name := "HighColor"
		if fewColors {
			name = "FewColors"
		}
		img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
		state := uint32(42)
		for pixel := 0; pixel < len(img.Pix)/4; pixel++ {
			offset := pixel * 4
			state = state*1664525 + 1013904223
			img.Pix[offset] = uint8(state >> 24)
			img.Pix[offset+1] = uint8(state >> 16)
			img.Pix[offset+2] = uint8(state >> 8)
			img.Pix[offset+3] = 255
			if fewColors {
				img.Pix[offset] = uint8(pixel&1) * 255
				img.Pix[offset+1] = uint8(pixel>>9&1) * 255
				img.Pix[offset+2] = 0
			}
		}
		b.Run(name+"/Owned", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				frame, err := QuantizeImage(context.Background(), img)
				if err != nil {
					b.Fatal(err)
				}
				quantizedPixelBenchmarkSink = frame.Pix[len(frame.Pix)/2]
			}
		})
		b.Run(name+"/Workspace", func(b *testing.B) {
			b.ReportAllocs()
			var workspace OpaqueQuantizationWorkspace
			consume := func(frame *image.Paletted) error {
				quantizedPixelBenchmarkSink = frame.Pix[len(frame.Pix)/2]
				return nil
			}
			for range b.N {
				if err := workspace.Quantize(context.Background(), img, consume); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
