package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// Vector surface retention is scoped to one serial export. PNG/GIF rendering
// does not retain scene documents or incur any vector serialization work.
type nativeVectorKey struct{}
type nativeVectorRegistry struct {
	surfaces map[*image.RGBA]*nativeVectorSurface
}
type nativeVectorSurface struct {
	document        *d2scene.Document
	inkCoverage     *nativeVectorSurface
	coverageOpacity float64
	capBackground   *color.NRGBA
}

// A solid cap's source viewport includes its centered stroke. Its retained
// fill therefore stops just inside the physical cap, while geometric rim ink
// follows the cap itself. Fill that opaque substrate before its source paint;
// the cap's mesh clip, material lighting and opacity still apply once outside.
// Keep the registry document and other uses of the source texture unchanged.
func nativeVectorSolidCap(surface *nativeVectorSurface, fill color.NRGBA) *nativeVectorSurface {
	if surface == nil || fill.A != 255 {
		return surface
	}
	copy := *surface
	copy.capBackground = &fill
	return &copy
}

func nativeVectorContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, nativeVectorKey{}, &nativeVectorRegistry{surfaces: make(map[*image.RGBA]*nativeVectorSurface)})
}

func nativeVectorEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(nativeVectorKey{}).(*nativeVectorRegistry)
	return ok
}

func retainNativeVectorSurface(ctx context.Context, texture *image.RGBA, document *d2scene.Document) error {
	if ctx == nil {
		return nil
	}
	registry, ok := ctx.Value(nativeVectorKey{}).(*nativeVectorRegistry)
	if !ok || texture == nil || document == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(registry.surfaces) >= 100000 {
		return fmt.Errorf("native SVG exceeds 100000 retained surfaces")
	}
	view := *document
	if view.LogicalWidth <= 0 || view.LogicalHeight <= 0 {
		view.LogicalWidth, view.LogicalHeight = float64(texture.Bounds().Dx()), float64(texture.Bounds().Dy())
	}
	registry.surfaces[texture] = &nativeVectorSurface{document: &view}
	return nil
}

func nativeVectorForTexture(ctx context.Context, texture *image.RGBA) *nativeVectorSurface {
	if ctx == nil {
		return nil
	}
	if registry, ok := ctx.Value(nativeVectorKey{}).(*nativeVectorRegistry); ok {
		return registry.surfaces[texture]
	}
	return nil
}

// Match an RGBA compositing step performed after a document was rasterized:
// background first, followed by the document, with one object-opacity fade.
func retainStyledNativeVectorSurface(ctx context.Context, texture *image.RGBA, document *d2scene.Document, background *color.NRGBA, opacity float64) error {
	if !nativeVectorEnabled(ctx) {
		return nil
	}
	view := *document
	matrix, err := nativeVectorViewport(&view)
	if err != nil {
		return err
	}
	content := d2scene.NewNode(nil)
	content.Transform = d2scene.Scale(view.LogicalWidth, view.LogicalHeight).Mul(matrix)
	content.Children = []*d2scene.Node{view.Root}
	root := d2scene.NewNode(nil)
	root.Opacity = opacity
	box := d2scene.Box{Width: view.LogicalWidth, Height: view.LogicalHeight}
	if background != nil {
		root.Children = append(root.Children, d2scene.NewNode(d2scene.Rect{Box: box, Fill: d2scene.SolidPaint{Color: *background}}))
	}
	root.Children = append(root.Children, content)
	view.Root, view.ViewBox = root, box
	view.ViewportFit = d2scene.ViewportStretch
	return retainNativeVectorSurface(ctx, texture, &view)
}

func nativeVectorViewport(doc *d2scene.Document) (d2scene.Matrix, error) {
	aspect := d2scene.AspectRatio{Align: d2scene.AlignNone}
	if doc.ViewportFit == d2scene.ViewportMeet {
		aspect.Align = d2scene.AlignXMinYMin
		if doc.ViewportAlign == d2scene.ViewportAlignXMidYMid {
			aspect.Align = d2scene.AlignXMidYMid
		}
	}
	width, height := doc.LogicalWidth, doc.LogicalHeight
	if !captionFinite(width) || !captionFinite(height) || width <= 0 || height <= 0 {
		return d2scene.Matrix{}, fmt.Errorf("native SVG surface has invalid dimensions")
	}
	matrix, err := d2scene.AspectRatioMatrix(doc.ViewBox, d2scene.Box{Width: width, Height: height}, aspect)
	if err != nil {
		return d2scene.Matrix{}, fmt.Errorf("native SVG surface has invalid viewport: %w", err)
	}
	return d2scene.Scale(1/width, 1/height).Mul(matrix), nil
}

// Match the border-label opening without baking its antialiased pixels into
// SVG. Both rectangles live in the source document's coordinate system.
func nativeVectorAperture(surface *nativeVectorSurface, aperture d2scene.Box) {
	if aperture.Width <= 0 || aperture.Height <= 0 {
		return
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{surface.document.Root}
	path := d2scene.Path{FillRule: d2scene.EvenOdd}
	for _, box := range []d2scene.Box{surface.document.ViewBox, aperture} {
		path.Commands = append(path.Commands, d2scene.MoveTo(box.X, box.Y), d2scene.LineTo(box.X+box.Width, box.Y), d2scene.LineTo(box.X+box.Width, box.Y+box.Height), d2scene.LineTo(box.X, box.Y+box.Height), d2scene.ClosePath())
	}
	root.Clip = &d2scene.Clip{Path: path, Transform: d2scene.Identity()}
	surface.document.Root = root
}
