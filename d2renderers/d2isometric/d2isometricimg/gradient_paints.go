package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/mazznoer/csscolorparser"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/scenepaint"
	libcolor "github.com/d2lang/d2/lib/color"
)

const nativeGradientTextureSize = 256
const maxNativeGradientPixels = 8 << 20

func nativePaintDocument(paint d2scene.Paint) *d2scene.Document {
	box := d2scene.Box{Width: nativeGradientTextureSize, Height: nativeGradientTextureSize}
	return d2scene.NewDocument(box, d2scene.NewNode(d2scene.Rect{Box: box, Fill: paint}))
}

func (b *meshBuilder) gradientTexture(raw string) *image.RGBA {
	if b.err != nil {
		return nil
	}
	if texture := b.gradientTextures[raw]; texture != nil {
		return texture
	}
	if len(b.gradientTextures) >= maxNativeGradientPixels/(nativeGradientTextureSize*nativeGradientTextureSize) {
		b.err = fmt.Errorf("isometric gradients exceed %d texture pixels", maxNativeGradientPixels)
		return nil
	}
	paint, err := scenepaint.Gradient(raw)
	if err != nil {
		b.err = err
		return nil
	}
	texture, err := rasterNativeSurfaceDocument(b.ctx, nativePaintDocument(paint), nativeGradientTextureSize, nativeGradientTextureSize)
	if err != nil {
		b.err = err
		return nil
	}
	if b.gradientTextures == nil {
		b.gradientTextures = make(map[string]*image.RGBA)
	}
	b.gradientTextures[raw] = texture
	return texture
}

// A geometric stroke is one paint operation even when tessellation splits it
// into many ribbons. Map its complete visible footprint into one gradient,
// retaining the same mapping across dashes, joins and arrowheads. Projecting
// onto a common plane also gives upright side ink a nondegenerate UV mapping.
func (b *meshBuilder) prepareGradientMaterials() {
	type extent struct{ x0, z0, x1, z1 float64 }
	bounds := make(map[*Material]extent)
	view := nativeCameraAxes().direction
	point := func(p Vec) Vec { return nsub(p, nmul(view, p.Y/view.Y)) }
	for i, t := range b.triangles {
		if i%1024 == 0 {
			if err := b.ctx.Err(); err != nil {
				b.err = err
				return
			}
		}
		m := t.Material
		if m == nil || m.gradient == "" || m.Texture != nil {
			continue
		}
		e, ok := bounds[m]
		if !ok {
			e = extent{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		}
		for _, v := range t.V {
			p := point(v.Position)
			e.x0 = min(e.x0, p.X)
			e.z0 = min(e.z0, p.Z)
			e.x1 = max(e.x1, p.X)
			e.z1 = max(e.z1, p.Z)
		}
		bounds[m] = e
	}
	for m := range bounds {
		texture := b.gradientTexture(m.gradient)
		if b.err != nil {
			return
		}
		m.Texture, m.Vector = texture, nativeVectorForTexture(b.ctx, texture)
	}
	for i := range b.triangles {
		if i%1024 == 0 {
			if err := b.ctx.Err(); err != nil {
				b.err = err
				return
			}
		}
		t := &b.triangles[i]
		e, ok := bounds[t.Material]
		if !ok {
			continue
		}
		for j := range t.V {
			p := point(t.V[j].Position)
			t.V[j].U = (p.X - e.x0) / max(1e-9, e.x1-e.x0)
			t.V[j].V = (p.Z - e.z0) / max(1e-9, e.z1-e.z0)
		}
	}
}

type nativeGradientBackgroundKey struct{}

func nativeGradientBackgroundContext(ctx context.Context, texture *image.RGBA) context.Context {
	if texture == nil {
		return ctx
	}
	return context.WithValue(ctx, nativeGradientBackgroundKey{}, texture)
}

// Keep the ordinary opaque presentation background beneath translucent stops.
func nativeGradientBackground(texture *image.RGBA, u, v float64, fallback color.NRGBA) color.NRGBA {
	r, g, b, a := rasterTexture(texture, u, v)
	return color.NRGBA{uint8(math.Round((r*a + float64(fallback.R)/255*(1-a)) * 255)), uint8(math.Round((g*a + float64(fallback.G)/255*(1-a)) * 255)), uint8(math.Round((b*a + float64(fallback.B)/255*(1-a)) * 255)), 255}
}

func nativeLabelBackgroundPaint(style labelTextStyle) d2scene.Paint {
	if style.BackgroundPaint != nil {
		return style.BackgroundPaint
	}
	if style.Background != nil {
		return d2scene.SolidPaint{Color: *style.Background}
	}
	return nil
}

func nativeLabelBackground(ctx context.Context, style labelTextStyle, width, height int) (image.Image, error) {
	if style.BackgroundPaint == nil {
		if style.Background != nil {
			return image.NewUniform(*style.Background), nil
		}
		return nil, nil
	}
	box := d2scene.Box{Width: float64(width), Height: float64(height)}
	doc := d2scene.NewDocument(box, d2scene.NewNode(d2scene.Rect{Box: box, Fill: style.BackgroundPaint}))
	return rasterNativeSurfaceDocument(ctx, doc, width, height)
}

// Scalar decisions (contrast, fully transparent bodies, opaque rims) need a
// representative color, while the actual geometry retains the complete paint.
func nativeGradientSummary(raw string) (color.NRGBA, bool) {
	paint, err := scenepaint.Gradient(raw)
	if err != nil {
		return color.NRGBA{}, false
	}
	var stops []d2scene.GradientStop
	switch p := paint.(type) {
	case d2scene.LinearGradient:
		stops = p.Stops
	case d2scene.RadialGradient:
		stops = p.Stops
	}
	if len(stops) == 0 {
		return color.NRGBA{}, false
	}
	var sums [4]float64
	opaque, clear := true, true
	for _, s := range stops {
		c := s.Color
		sums[0] += float64(c.R)
		sums[1] += float64(c.G)
		sums[2] += float64(c.B)
		sums[3] += float64(c.A)
		opaque = opaque && c.A == 255
		clear = clear && c.A == 0
	}
	for i := range sums {
		sums[i] /= float64(len(stops))
	}
	alpha := uint8(math.Round(sums[3]))
	if !opaque && alpha == 255 {
		alpha = 254
	}
	if !clear && alpha == 0 {
		alpha = 1
	}
	return color.NRGBA{uint8(math.Round(sums[0])), uint8(math.Round(sums[1])), uint8(math.Round(sums[2])), alpha}, true
}

// Organizational container washes preserve gradient stops and geometry while
// applying the same presentation opacity already used for solid source fills.
func nativeGradientOpacity(raw string, opacity float64) (string, error) {
	gradient, err := libcolor.ParseGradient(raw)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(gradient.ColorStops)+1)
	if gradient.Direction != "" {
		parts = append(parts, gradient.Direction)
	}
	for _, stop := range gradient.ColorStops {
		parsed, err := csscolorparser.Parse(stop.Color)
		if err != nil {
			return "", err
		}
		r, g, b, a := parsed.RGBA255()
		value := richColor(color.NRGBA{r, g, b, uint8(math.Round(float64(a) * opacity))})
		if stop.Position != "" {
			value += " " + stop.Position
		}
		parts = append(parts, value)
	}
	return gradient.Type + "-gradient(" + strings.Join(parts, ",") + ")", nil
}
