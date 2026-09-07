package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

type Vec = d2isometric.Vec3

// Vertex UV coordinates use a top-left texture origin. A zero normal uses
// the triangle's geometric normal; supplied normals are smoothly interpolated.
type Vertex struct {
	Position, Normal Vec
	U, V             float64
}

type Material struct {
	Color                color.NRGBA
	Roughness, Metalness float64
	Emissive             color.NRGBA
	Texture              image.Image
	Vector               *nativeVectorSurface
	Unlit                bool
	// Multiply composites this surface into the existing sRGB color while
	// retaining source-over alpha. Group regions normally also use NoDepthWrite.
	Multiply bool
	// SVG contours paint over their own substrate without cutting a second
	// antialiased boundary into it. Other objects still see opaque linework.
	svgContour bool
	// Physical solid caps and unwrapped walls have a filled source background;
	// their texture's antialiased perimeter is not a geometric hole.
	svgSolidTexture bool
}

type Triangle struct {
	V          [3]Vertex
	Material   *Material
	CastShadow bool
	// Nil uses the material opacity; animation may fade its shadow separately
	// from the visible surface. The pointed value is immutable for a frame.
	ShadowOpacity *float64
	// Filled physical caps use source fill alpha rather than the printed
	// texture's antialiased outline coverage. Nil preserves texture holes.
	// ShadowOpacity still controls the strength of this physical shadow.
	ShadowFillAlpha *uint8
	// Hierarchy roots have independent implicit ground receivers. Nil keeps
	// the legacy scene-wide receiver. The pointed value is immutable.
	ShadowGround *float64
	DepthBias    float64
	NoDepthWrite bool
	// OpacityGroup fades a complete native object after its own opaque
	// surfaces and printed cells have resolved their mutual occlusion.
	OpacityGroup *nativeOpacityGroup
	// PaintOwner keeps a native object's substrate and decals together when
	// they must be ordered around a separate partial-opacity object.
	PaintOwner *nativePaintOwner
	// SVG uses source-contour caps to resolve texture coverage geometrically.
	// These caps neither paint nor participate in the physical ink silhouette.
	svgCoverageOnly bool
}

const (
	rasterMaxTriangles     = 1_000_000
	rasterMaxSamples       = 16_000_000
	rasterMaxTexturePixels = 64_000_000
	rasterMaxWork          = 500_000_000
	rasterCoordinateLimit  = 1e9
	rasterAA               = 2
	rasterStripRows        = 64
)

type rasterCamera struct {
	right, up, direction                 Vec
	centerX, centerY, centerDepth, scale float64
	width, height                        int
}

type rasterShadow struct {
	camera  rasterCamera
	depth   []float32
	opacity []uint8
	bias    float64
}

// Raster owns the static color/depth buffers and a directional shadow map.
// It is immutable after construction. Independent Frame calls may run in
// parallel; callers should bound that concurrency to their memory budget.
type Raster struct {
	width, height, aa int
	camera            rasterCamera
	shadow            rasterShadow
	groundShadow      rasterShadow
	pixels            *image.RGBA
	depth             []float32
	output            *image.RGBA
	static            *rasterDrawPlan // large images replay only touched strips
	background        color.NRGBA
	ground            float64
	groundBounds      image.Rectangle // finite receiver footprint of actual shadows
}

type rasterWork struct {
	ctx         context.Context
	remaining   int64
	groupPixels []uint8
	groupDepth  []float32
}

func (w *rasterWork) charge(n int64) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if n < 0 || n > w.remaining {
		return fmt.Errorf("native isometric raster work exceeds %d sample operations", rasterMaxWork)
	}
	w.remaining -= n
	return nil
}

func rasterContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Native stills use a more open orthographic view: top faces are less
// foreshortened and their reading axis sits closer to horizontal.
// Gentle skew: 15 degrees of azimuth, retaining the 49.178-degree elevation.
// The fixed unit vector keeps all projection consumers on the same camera.
func nativeViewDirection() Vec {
	return Vec{X: 0.16919264451761212, Y: 0.7567450038061342, Z: 0.6314355456066684}
}

// NewRaster fits the occupied vertices with a fixed orthographic camera.
// It uses 2x supersampling at every supported size. Large surfaces render in
// bounded strips rather than dropping coverage quality or allocating all samples.
// Background is final opaque sRGB, unaffected by material lighting.
func NewRaster(ctx context.Context, width, height int, triangles []Triangle, background color.NRGBA) (*Raster, error) {
	deferred := false
	for _, t := range triangles {
		deferred = deferred || t.OpacityGroup != nil
	}
	if width > 0 && height > 0 && width <= 4096 && height <= 4096 && int64(width)*int64(height) <= 12_000_000 && (deferred || int64(width)*int64(height)*rasterAA*rasterAA > rasterMaxSamples) {
		// The public API accepts caller-owned inputs. Deferred strip replay must
		// keep a snapshot even if that caller later changes its mesh or texture.
		var err error
		triangles, err = rasterSnapshot(rasterContext(ctx), triangles)
		if err != nil {
			return nil, err
		}
	}
	return newRaster(ctx, width, height, triangles, background, nil, nil)
}

func newRaster(ctx context.Context, width, height int, triangles []Triangle, background color.NRGBA, camera, shadowCamera *rasterCamera, groundCamera ...*rasterCamera) (*Raster, error) {
	ctx = rasterContext(ctx)
	w := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	if err := w.charge(0); err != nil {
		return nil, err
	}
	if width < 1 || height < 1 || width > 4096 || height > 4096 || int64(width)*int64(height) > 12_000_000 {
		return nil, fmt.Errorf("native isometric raster dimensions exceed limits")
	}
	if err := rasterValidate(ctx, triangles); err != nil {
		return nil, err
	}
	r := &Raster{width: width, height: height, aa: rasterAA, background: background, ground: rasterShadowGround(triangles)}
	r.camera = rasterFit(triangles, nativeViewDirection(), width*r.aa, height*r.aa, 1.08)
	if camera != nil {
		if camera.width < 1 || camera.height < 1 {
			return nil, fmt.Errorf("native raster camera requires positive dimensions")
		}
		r.camera = cameraAtResolution(*camera, width*r.aa, height*r.aa)
	}
	var err error
	if shadowCamera == nil {
		r.shadow, err = rasterBuildShadow(&w, triangles)
	} else {
		r.shadow, err = rasterBuildShadow(&w, triangles, *shadowCamera)
	}
	if err != nil {
		return nil, err
	}
	r.groundShadow = r.shadow
	groundTriangles := rasterGroundTriangles(triangles, r.ground, r.camera.direction)
	if len(triangles) > 0 && &groundTriangles[0] != &triangles[0] {
		if err := w.charge(int64(len(triangles))); err != nil {
			return nil, err
		}
		if len(groundCamera) > 0 && groundCamera[0] != nil {
			r.groundShadow, err = rasterBuildShadow(&w, groundTriangles, *groundCamera[0])
		} else {
			r.groundShadow, err = rasterBuildShadow(&w, groundTriangles)
		}
		if err != nil {
			return nil, err
		}
	}
	if len(r.groundShadow.depth) > 0 {
		var extent projectedExtent
		extent.preparedGroundShadows(groundTriangles, nil, 0, r.ground, r.groundShadow.camera)
		if extent.valid {
			x0 := float64(r.camera.width)/2 + (extent.minX-r.camera.centerX)*r.camera.scale
			x1 := float64(r.camera.width)/2 + (extent.maxX-r.camera.centerX)*r.camera.scale
			y0 := float64(r.camera.height)/2 - (extent.maxY-r.camera.centerY)*r.camera.scale
			y1 := float64(r.camera.height)/2 - (extent.minY-r.camera.centerY)*r.camera.scale
			r.groundBounds = image.Rect(int(max(0, min(float64(r.camera.width), math.Floor(x0)))), int(max(0, min(float64(r.camera.height), math.Floor(y0)))), int(max(0, min(float64(r.camera.width), math.Ceil(x1)))), int(max(0, min(float64(r.camera.height), math.Ceil(y1)))))
		}
	}
	r.output = image.NewRGBA(image.Rect(0, 0, width, height))
	plan := r.rasterPrepare(triangles)
	if int64(r.camera.width)*int64(r.camera.height) > rasterMaxSamples || len(plan.groups) > 0 {
		r.static = &plan
		if err := r.renderStrips(&w, r.output, nil); err != nil {
			return nil, err
		}
		return r, ctx.Err()
	}
	r.pixels = image.NewRGBA(image.Rect(0, 0, r.camera.width, r.camera.height))
	r.depth = make([]float32, r.camera.width*r.camera.height)
	for i := range r.depth {
		r.depth[i] = float32(math.Inf(-1))
	}
	if err = r.paintGround(&w, r.pixels, background); err != nil {
		return nil, err
	}
	if err = r.drawPrepared(&w, r.pixels, r.depth, plan, nil); err != nil {
		return nil, err
	}
	if err = w.charge(int64(width) * int64(height) * int64(r.aa*r.aa)); err != nil {
		return nil, err
	}
	for y := 0; y < height; y++ {
		if y%32 == 0 {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 0; x < width; x++ {
			r.downsamplePixel(r.output, r.pixels, x, y)
		}
	}
	return r, ctx.Err()
}

// Frame reuses the static image and shadow map. Small images retain sample
// depth; large images replay the same static samples only in affected strips.
// Unchanged pixels remain identical across frames. The result is caller-owned.
func (r *Raster) Frame(ctx context.Context, dynamic []Triangle) (*image.RGBA, error) {
	ctx = rasterContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || r.output == nil {
		return nil, fmt.Errorf("native raster is uninitialized")
	}
	if err := rasterValidate(ctx, dynamic); err != nil {
		return nil, err
	}
	out := image.NewRGBA(r.output.Rect)
	copy(out.Pix, r.output.Pix)
	if len(dynamic) == 0 {
		return out, ctx.Err()
	}
	if r.static != nil {
		w := rasterWork{ctx: ctx, remaining: rasterMaxWork}
		plan := r.rasterPrepare(dynamic)
		if err := r.renderStrips(&w, out, &plan); err != nil {
			return nil, err
		}
		return out, ctx.Err()
	}
	pixels := image.NewRGBA(r.pixels.Rect)
	copy(pixels.Pix, r.pixels.Pix)
	depth := append([]float32(nil), r.depth...)
	dirty := make([]bool, r.width*r.height)
	w := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	if err := r.draw(&w, pixels, depth, dynamic, dirty); err != nil {
		return nil, err
	}
	for y := 0; y < r.height; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 0; x < r.width; x++ {
			if dirty[y*r.width+x] {
				r.downsamplePixel(out, pixels, x, y)
			}
		}
	}
	return out, ctx.Err()
}

func rasterSnapshot(ctx context.Context, ts []Triangle) ([]Triangle, error) {
	if err := rasterValidate(ctx, ts); err != nil {
		return nil, err
	}
	out := append([]Triangle(nil), ts...)
	materials := make(map[*Material]*Material)
	textures := make(map[image.Image]image.Image)
	groups := make(map[*nativeOpacityGroup]*nativeOpacityGroup)
	owners := make(map[*nativePaintOwner]*nativePaintOwner)
	for i := range out {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t := &out[i]
		if t.PaintOwner != nil {
			owner, ok := owners[t.PaintOwner]
			if !ok {
				copy := *t.PaintOwner
				owner = &copy
				owners[t.PaintOwner] = owner
			}
			t.PaintOwner = owner
		}
		if t.OpacityGroup != nil {
			group, ok := groups[t.OpacityGroup]
			if !ok {
				copy := *t.OpacityGroup
				group = &copy
				groups[t.OpacityGroup] = group
			}
			t.OpacityGroup = group
		}
		if t.ShadowOpacity != nil {
			opacity := *t.ShadowOpacity
			t.ShadowOpacity = &opacity
		}
		if t.ShadowFillAlpha != nil {
			alpha := *t.ShadowFillAlpha
			t.ShadowFillAlpha = &alpha
		}
		if t.ShadowGround != nil {
			ground := *t.ShadowGround
			t.ShadowGround = &ground
		}
		if t.Material == nil {
			continue
		}
		m, ok := materials[t.Material]
		if !ok {
			copy := *t.Material
			m = &copy
			materials[t.Material] = m
			if m.Texture != nil {
				texture, ok := textures[m.Texture]
				if !ok {
					bounds := m.Texture.Bounds()
					var dst draw.Image
					switch m.Texture.(type) {
					case *image.RGBA:
						dst = image.NewRGBA(bounds)
					case *image.NRGBA:
						dst = image.NewNRGBA(bounds)
					case *image.Alpha:
						dst = image.NewAlpha(bounds)
					default:
						return nil, fmt.Errorf("unsupported native isometric texture type %T", m.Texture)
					}
					for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
						if y%64 == 0 {
							if err := ctx.Err(); err != nil {
								return nil, err
							}
						}
						draw.Draw(dst, image.Rect(bounds.Min.X, y, bounds.Max.X, y+1), m.Texture, image.Pt(bounds.Min.X, y), draw.Src)
					}
					texture = dst
					textures[m.Texture] = texture
				}
				m.Texture = texture
			}
		}
		t.Material = m
	}
	return out, ctx.Err()
}

// Large images retain final pixels and an immutable native draw plan, not a
// full supersampled color/depth surface. Replay uses global sample coordinates,
// so strip boundaries cannot change coverage, shadow lookup or triangle order.
func (r *Raster) renderStrips(w *rasterWork, out *image.RGBA, dynamic *rasterDrawPlan) error {
	if err := w.charge(0); err != nil {
		return err
	}
	rows := min(rasterStripRows, r.height)
	sampleRows := rows * r.aa
	pixelBuffer := make([]uint8, r.camera.width*sampleRows*4)
	depthBuffer := make([]float32, r.camera.width*sampleRows)
	active := make([]bool, (r.height+rows-1)/rows)
	static := *r.static
	// A cached translucent object cannot be painted underneath later moving
	// geometry. Replay their bounded draw plan together in the touched strips.
	combined := dynamic != nil && (len(static.groups) > 0 || len(dynamic.groups) > 0)
	if combined {
		if len(static.triangles) > rasterMaxTriangles-len(dynamic.triangles) {
			return fmt.Errorf("combined native isometric triangles exceed %d", rasterMaxTriangles)
		}
		if err := w.charge(int64(len(static.triangles) + len(dynamic.triangles))); err != nil {
			return err
		}
		triangles := append(append([]Triangle(nil), static.triangles...), dynamic.triangles...)
		static = r.rasterPrepare(triangles)
	}
	if dynamic != nil {
		for _, triangle := range dynamic.triangles {
			if err := w.charge(1); err != nil {
				return err
			}
			var p [3]rasterPoint
			for j, vertex := range triangle.V {
				p[j] = r.camera.project(vertex.Position)
			}
			x0, y0, x1, y1 := rasterBounds(p, r.camera.width, r.camera.height)
			if x0 >= x1 || y0 >= y1 {
				continue
			}
			for strip := y0 / sampleRows; strip <= (y1-1)/sampleRows; strip++ {
				active[strip] = true
			}
		}
	}
	for y0 := 0; y0 < r.height; y0 += rows {
		if err := w.charge(0); err != nil {
			return err
		}
		if dynamic != nil && !active[y0/rows] {
			continue
		}
		y1 := min(r.height, y0+rows)
		bounds := image.Rect(0, y0*r.aa, r.camera.width, y1*r.aa)
		samples := bounds.Dx() * bounds.Dy()
		pixels := &image.RGBA{Pix: pixelBuffer[:samples*4], Stride: bounds.Dx() * 4, Rect: bounds}
		depth := depthBuffer[:samples]
		for i := range depth {
			depth[i] = float32(math.Inf(-1))
		}
		if err := r.paintGround(w, pixels, r.background); err != nil {
			return err
		}
		if err := r.drawPrepared(w, pixels, depth, static, nil); err != nil {
			return err
		}
		if dynamic != nil && !combined {
			if err := r.drawPrepared(w, pixels, depth, *dynamic, nil); err != nil {
				return err
			}
		}
		if err := w.charge(int64(samples)); err != nil {
			return err
		}
		for y := y0; y < y1; y++ {
			for x := 0; x < r.width; x++ {
				r.downsamplePixel(out, pixels, x, y)
			}
		}
	}
	return w.ctx.Err()
}

func rasterValidate(ctx context.Context, triangles []Triangle) error {
	if len(triangles) > rasterMaxTriangles {
		return fmt.Errorf("native isometric triangles exceed %d", rasterMaxTriangles)
	}
	textures := make(map[image.Image]bool)
	var texturePixels int64
	for i, t := range triangles {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if !rasterFinite(t.DepthBias) || math.Abs(t.DepthBias) > rasterCoordinateLimit {
			return fmt.Errorf("triangle %d has invalid depth bias", i)
		}
		if t.ShadowOpacity != nil && (!rasterFinite(*t.ShadowOpacity) || *t.ShadowOpacity < 0 || *t.ShadowOpacity > 1) {
			return fmt.Errorf("triangle %d has invalid shadow opacity", i)
		}
		if t.ShadowGround != nil && (!rasterFinite(*t.ShadowGround) || math.Abs(*t.ShadowGround) > rasterCoordinateLimit) {
			return fmt.Errorf("triangle %d has invalid shadow ground", i)
		}
		if t.OpacityGroup != nil && (!rasterFinite(t.OpacityGroup.Opacity) || t.OpacityGroup.Opacity < 0 || t.OpacityGroup.Opacity > 1) {
			return fmt.Errorf("triangle %d has invalid object opacity", i)
		}
		for _, v := range t.V {
			for _, n := range []float64{v.Position.X, v.Position.Y, v.Position.Z, v.Normal.X, v.Normal.Y, v.Normal.Z, v.U, v.V} {
				if !rasterFinite(n) || math.Abs(n) > rasterCoordinateLimit {
					return fmt.Errorf("triangle %d has invalid vertex", i)
				}
			}
		}
		m := t.Material
		if m == nil {
			continue
		}
		if !rasterFinite(m.Roughness) || !rasterFinite(m.Metalness) {
			return fmt.Errorf("triangle %d has nonfinite material", i)
		}
		if m.Texture != nil {
			// Restrict textures to owned standard raster types. Their comparable
			// pointers also make shared-atlas admission independent of triangle count.
			switch texture := m.Texture.(type) {
			case *image.RGBA:
				if texture == nil {
					return fmt.Errorf("nil native isometric texture")
				}
				if err := rasterTextureStorage(texture.Rect, texture.Stride, len(texture.Pix), 4); err != nil {
					return err
				}
			case *image.NRGBA:
				if texture == nil {
					return fmt.Errorf("nil native isometric texture")
				}
				if err := rasterTextureStorage(texture.Rect, texture.Stride, len(texture.Pix), 4); err != nil {
					return err
				}
			case *image.Alpha:
				if texture == nil {
					return fmt.Errorf("nil native isometric texture")
				}
				if err := rasterTextureStorage(texture.Rect, texture.Stride, len(texture.Pix), 1); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported native isometric texture type %T", m.Texture)
			}
			if !textures[m.Texture] {
				b := m.Texture.Bounds()
				if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > 16384 || b.Dy() > 16384 {
					return fmt.Errorf("invalid native isometric texture dimensions")
				}
				texturePixels += int64(b.Dx()) * int64(b.Dy())
				if texturePixels > rasterMaxTexturePixels {
					return fmt.Errorf("native isometric texture pixels exceed limit")
				}
				textures[m.Texture] = true
			}
		}
	}
	return nil
}

func rasterTextureStorage(b image.Rectangle, stride, pixels, channels int) error {
	if b.Min.X < -1e9 || b.Min.Y < -1e9 || b.Max.X > 1e9 || b.Max.Y > 1e9 || b.Max.X <= b.Min.X || b.Max.Y <= b.Min.Y || b.Dx() > 16384 || b.Dy() > 16384 || stride < b.Dx()*channels {
		return fmt.Errorf("invalid native isometric texture storage")
	}
	if int64(b.Dy()-1)*int64(stride)+int64(b.Dx()*channels) > int64(pixels) {
		return fmt.Errorf("short native isometric texture storage")
	}
	return nil
}

func rasterFinite(f float64) bool    { return !math.IsNaN(f) && !math.IsInf(f, 0) }
func rasterAdd(a, b Vec) Vec         { return Vec{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }
func rasterSub(a, b Vec) Vec         { return Vec{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func rasterMul(a Vec, s float64) Vec { return Vec{X: a.X * s, Y: a.Y * s, Z: a.Z * s} }
func rasterDot(a, b Vec) float64     { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func rasterCross(a, b Vec) Vec {
	return Vec{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func rasterUnit(a Vec) Vec {
	l := math.Sqrt(rasterDot(a, a))
	if l < 1e-15 {
		return Vec{Y: 1}
	}
	return rasterMul(a, 1/l)
}
func rasterClamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func rasterFit(ts []Triangle, direction Vec, width, height int, padding float64) rasterCamera {
	direction = rasterUnit(direction)
	right := rasterUnit(rasterCross(Vec{Y: 1}, direction))
	up := rasterUnit(rasterCross(direction, right))
	minX, minY, minZ, maxX, maxY, maxZ := math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)
	for _, t := range ts {
		for _, v := range t.V {
			p := v.Position
			x, y, z := rasterDot(p, right), rasterDot(p, up), rasterDot(p, direction)
			minX = math.Min(minX, x)
			maxX = math.Max(maxX, x)
			minY = math.Min(minY, y)
			maxY = math.Max(maxY, y)
			minZ = math.Min(minZ, z)
			maxZ = math.Max(maxZ, z)
		}
	}
	if math.IsInf(minX, 1) {
		minX, minY, minZ, maxX, maxY, maxZ = -1, -1, -1, 1, 1, 1
	}
	span := math.Max(.02, math.Max(maxY-minY, (maxX-minX)*float64(height)/float64(width))) * padding
	return rasterCamera{right: right, up: up, direction: direction, centerX: (minX + maxX) / 2, centerY: (minY + maxY) / 2, centerDepth: (minZ + maxZ) / 2, scale: float64(height) / span, width: width, height: height}
}

type rasterPoint struct{ x, y, z float64 }

func (c rasterCamera) project(p Vec) rasterPoint {
	return rasterPoint{x: (rasterDot(p, c.right)-c.centerX)*c.scale + float64(c.width)/2, y: float64(c.height)/2 - (rasterDot(p, c.up)-c.centerY)*c.scale, z: rasterDot(p, c.direction) - c.centerDepth}
}
func (c rasterCamera) world(x, y, z float64) Vec {
	return rasterAdd(rasterAdd(rasterMul(c.right, (x-float64(c.width)/2)/c.scale+c.centerX), rasterMul(c.up, (float64(c.height)/2-y)/c.scale+c.centerY)), rasterMul(c.direction, z+c.centerDepth))
}
func rasterOrient(a, b, c rasterPoint) float64 {
	// Evaluate each shared edge from one canonical endpoint. Reversing an
	// edge must produce the exact opposite value, including at subpixel
	// boundaries where two independently rounded expressions can both be
	// negative. The top-left rule then assigns every sample to one face.
	sign := 1.
	if a.x > b.x || a.x == b.x && a.y > b.y {
		a, b = b, a
		sign = -1
	}
	return sign * ((b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x))
}
func rasterTopLeft(a, b rasterPoint) bool { return b.y < a.y || (b.y == a.y && b.x > a.x) }

func rasterBounds(p [3]rasterPoint, width, height int) (int, int, int, int) {
	minX, maxX := math.Min(p[0].x, math.Min(p[1].x, p[2].x)), math.Max(p[0].x, math.Max(p[1].x, p[2].x))
	minY, maxY := math.Min(p[0].y, math.Min(p[1].y, p[2].y)), math.Max(p[0].y, math.Max(p[1].y, p[2].y))
	// Clamp floats before integer conversion; even finite world coordinates
	// can project far outside the canvas after fitting tiny static geometry.
	return int(math.Max(0, math.Min(float64(width), math.Floor(minX)))), int(math.Max(0, math.Min(float64(height), math.Floor(minY)))), int(math.Max(0, math.Min(float64(width), math.Ceil(maxX)))), int(math.Max(0, math.Min(float64(height), math.Ceil(maxY))))
}

// Intersect the three oriented half-planes at the sample row. Long, thin
// diagonal tubes otherwise test almost their entire empty bounding rectangle.
// The one-pixel guard preserves the existing barycentric/top-left decisions
// at floating-point boundaries; this function only reduces candidate work.
func rasterRowSpan(p [3]rasterPoint, y, x0, x1 int) (int, int) {
	left, right := float64(x0)+.5, float64(x1)-.5
	row := float64(y) + .5
	for i, a := range p {
		b := p[(i+1)%3]
		dx, dy := b.x-a.x, b.y-a.y
		if dy == 0 {
			if dx*(row-a.y) < 0 {
				return x0, x0
			}
			continue
		}
		crossing := a.x + dx*((row-a.y)/dy)
		if dy < 0 {
			left = math.Max(left, crossing)
		} else {
			right = math.Min(right, crossing)
		}
	}
	if left > right+1e-9 {
		return x0, x0
	}
	// Clamp before conversion, including triangles projected far off canvas.
	return int(math.Max(float64(x0), math.Min(float64(x1), math.Floor(left)-1))), int(math.Max(float64(x0), math.Min(float64(x1), math.Ceil(right)+1)))
}

const rasterShadowNormalOffset = .025

func rasterShadowDirection() Vec { return Vec{X: -1, Y: 1.8, Z: 1} }

func rasterShadowGround(ts []Triangle) float64 {
	ground := 0.
	for _, t := range ts {
		for _, v := range t.V {
			ground = min(ground, v.Position.Y)
		}
	}
	// Keep the receiver close to the board substrate, so surface traces do
	// not appear suspended above a distant ground plane.
	return ground + .08
}

func rasterBuildShadow(w *rasterWork, ts []Triangle, fixedCamera ...rasterCamera) (rasterShadow, error) {
	casters := false
	for _, t := range ts {
		if t.CastShadow && (t.ShadowOpacity == nil || *t.ShadowOpacity > 0) && (t.Material == nil || t.Material.Color.A > 0) {
			casters = true
			break
		}
	}
	if !casters {
		return rasterShadow{}, nil
	}
	shadow := rasterShadow{camera: rasterFit(ts, rasterShadowDirection(), 2048, 2048, 1.08)}
	if len(fixedCamera) > 0 {
		shadow.camera = fixedCamera[0]
	}
	shadow.depth = make([]float32, 2048*2048)
	shadow.opacity = make([]uint8, 2048*2048)
	for i := range shadow.depth {
		shadow.depth[i] = float32(math.Inf(-1))
	}
	shadow.bias = math.Max(.012, 2/shadow.camera.scale)
	for i, t := range ts {
		if i%512 == 0 {
			if err := w.charge(0); err != nil {
				return shadow, err
			}
		}
		if !t.CastShadow || (t.ShadowOpacity != nil && *t.ShadowOpacity <= 0) || (t.Material != nil && t.Material.Color.A == 0) {
			continue
		}
		var p [3]rasterPoint
		vertices := t.V
		for j, v := range t.V {
			p[j] = shadow.camera.project(v.Position)
		}
		a := rasterOrient(p[0], p[1], p[2])
		if math.Abs(a) < 1e-10 {
			continue
		}
		if a < 0 {
			p[1], p[2] = p[2], p[1]
			vertices[1], vertices[2] = vertices[2], vertices[1]
			a = -a
		}
		x0, y0, x1, y1 := rasterBounds(p, 2048, 2048)
		if err := w.charge(int64(y1-y0) * 3); err != nil {
			return shadow, err
		}
		opacity := uint8(255)
		if t.Material != nil {
			opacity = t.Material.Color.A
		}
		if t.ShadowFillAlpha != nil {
			opacity = *t.ShadowFillAlpha
		}
		if t.ShadowOpacity != nil {
			opacity = uint8(math.Round(float64(opacity) * *t.ShadowOpacity))
		}
		if t.OpacityGroup != nil {
			opacity = uint8(math.Round(float64(opacity) * t.OpacityGroup.Opacity))
		}
		for y := y0; y < y1; y++ {
			if y%32 == 0 {
				if err := w.ctx.Err(); err != nil {
					return shadow, err
				}
			}
			start, end := rasterRowSpan(p, y, x0, x1)
			if err := w.charge(int64(end - start)); err != nil {
				return shadow, err
			}
			for x := start; x < end; x++ {
				q := rasterPoint{x: float64(x) + .5, y: float64(y) + .5}
				u, v, s := rasterOrient(p[1], p[2], q), rasterOrient(p[2], p[0], q), rasterOrient(p[0], p[1], q)
				if u < 0 || v < 0 || s < 0 {
					continue
				}
				z := float32((u*p[0].z + v*p[1].z + s*p[2].z) / a)
				sampleOpacity := opacity
				if t.ShadowFillAlpha == nil && t.Material != nil && t.Material.Texture != nil {
					tx := (u*vertices[0].U + v*vertices[1].U + s*vertices[2].U) / a
					ty := (u*vertices[0].V + v*vertices[1].V + s*vertices[2].V) / a
					_, _, _, alpha := rasterTexture(t.Material.Texture, tx, ty)
					sampleOpacity = uint8(math.Round(float64(opacity) * alpha))
					if sampleOpacity == 0 {
						continue
					}
				}
				at := y*2048 + x
				if z > shadow.depth[at] {
					shadow.depth[at] = z
					shadow.opacity[at] = sampleOpacity
				}
			}
		}
	}
	return shadow, nil
}

func (s *rasterShadow) occlusion(world, normal Vec) float64 {
	if len(s.depth) == 0 {
		return 0
	}
	p := s.camera.project(rasterAdd(world, rasterMul(normal, rasterShadowNormalOffset)))
	if p.x < 1 || p.y < 1 || p.x >= float64(s.camera.width-1) || p.y >= float64(s.camera.height-1) {
		return 0
	}
	x, y := int(p.x), int(p.y)
	sum := 0.0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			at := (y+dy)*s.camera.width + x + dx
			if float64(s.depth[at]) > p.z+s.bias {
				sum += float64(s.opacity[at]) / 255
			}
		}
	}
	return sum / 9
}

func (r *Raster) paintGround(w *rasterWork, pixels *image.RGBA, bg color.NRGBA) error {
	ground := r.ground
	samples := int64(pixels.Rect.Dx()) * int64(pixels.Rect.Dy())
	shadowBounds := pixels.Rect.Intersect(r.groundBounds)
	work := samples + int64(shadowBounds.Dx())*int64(shadowBounds.Dy())*9
	if err := w.charge(work); err != nil {
		return err
	}
	for y := pixels.Rect.Min.Y; y < pixels.Rect.Max.Y; y++ {
		if y%32 == 0 {
			if err := w.ctx.Err(); err != nil {
				return err
			}
		}
		for x := pixels.Rect.Min.X; x < pixels.Rect.Max.X; x++ {
			shadow := 0.
			if x >= shadowBounds.Min.X && x < shadowBounds.Max.X && y >= shadowBounds.Min.Y && y < shadowBounds.Max.Y {
				p := r.camera.world(float64(x)+.5, float64(y)+.5, 0)
				p = rasterAdd(p, rasterMul(r.camera.direction, (ground-p.Y)/r.camera.direction.Y))
				shadow = r.groundShadow.occlusion(p, Vec{Y: 1}) * .11
			}
			at := pixels.PixOffset(x, y)
			pixels.Pix[at] = uint8(math.Round(float64(bg.R)*(1-shadow) + 36*shadow))
			pixels.Pix[at+1] = uint8(math.Round(float64(bg.G)*(1-shadow) + 55*shadow))
			pixels.Pix[at+2] = uint8(math.Round(float64(bg.B)*(1-shadow) + 84*shadow))
			pixels.Pix[at+3] = 255
		}
	}
	return nil
}

type rasterPaint struct {
	base, emission              Vec
	alpha, roughness, metalness float64
	texture                     image.Image
	unlit                       bool
	multiply                    bool
}

func rasterMaterial(m *Material) rasterPaint {
	if m == nil {
		m = &Material{Color: color.NRGBA{R: 240, G: 243, B: 249, A: 255}, Roughness: .35}
	}
	return rasterPaint{base: Vec{X: rasterLinear(float64(m.Color.R) / 255), Y: rasterLinear(float64(m.Color.G) / 255), Z: rasterLinear(float64(m.Color.B) / 255)}, emission: Vec{X: rasterLinear(float64(m.Emissive.R)/255) * float64(m.Emissive.A) / 255, Y: rasterLinear(float64(m.Emissive.G)/255) * float64(m.Emissive.A) / 255, Z: rasterLinear(float64(m.Emissive.B)/255) * float64(m.Emissive.A) / 255}, alpha: float64(m.Color.A) / 255, roughness: math.Max(.05, rasterClamp(m.Roughness)), metalness: rasterClamp(m.Metalness), texture: m.Texture, unlit: m.Unlit, multiply: m.Multiply}
}
func rasterLinear(c float64) float64 {
	if c <= .04045 {
		return c / 12.92
	}
	return math.Pow((c+.055)/1.055, 2.4)
}
func rasterSRGB(c float64) float64 {
	c = rasterClamp(c)
	if c <= .0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - .055
}

type rasterDrawPlan struct {
	triangles []Triangle
	order     []int
	paints    map[*Material]rasterPaint
	groups    map[int]rasterOpacityPlan
}

func (r *Raster) draw(w *rasterWork, pixels *image.RGBA, depth []float32, ts []Triangle, dirty []bool) error {
	return r.drawPrepared(w, pixels, depth, r.rasterPrepare(ts), dirty)
}

func (r *Raster) rasterPrepare(ts []Triangle) rasterDrawPlan {
	for _, t := range ts {
		if t.OpacityGroup != nil {
			return r.rasterPrepareOpacityGroups(ts)
		}
	}
	order := make([]int, len(ts))
	paints := make(map[*Material]rasterPaint)
	for i, t := range ts {
		order[i] = i
		if _, ok := paints[t.Material]; !ok {
			paints[t.Material] = rasterMaterial(t.Material)
		}
	}
	transparent := func(t Triangle) bool {
		return t.NoDepthWrite || paints[t.Material].multiply || paints[t.Material].alpha < .999 || paints[t.Material].texture != nil
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := ts[order[i]], ts[order[j]]
		// Printed icons and labels do not write depth. Draw their substrates
		// first, including textured faces whose transparent margins prevented
		// classification as opaque. Centroid sorting alone can otherwise paint
		// half of a large face over a small decal on that very same surface.
		if a.NoDepthWrite != b.NoDepthWrite {
			return !a.NoDepthWrite
		}
		ta, tb := transparent(a), transparent(b)
		if ta != tb {
			return !ta
		}
		if !ta {
			return false
		}
		za, zb := 0.0, 0.0
		for k := 0; k < 3; k++ {
			za += rasterDot(a.V[k].Position, r.camera.direction)
			zb += rasterDot(b.V[k].Position, r.camera.direction)
		}
		return za < zb
	})
	return rasterDrawPlan{triangles: ts, order: order, paints: paints}
}

func (r *Raster) drawPrepared(w *rasterWork, pixels *image.RGBA, depth []float32, plan rasterDrawPlan, dirty []bool) error {
	ts, paints := plan.triangles, plan.paints
	for _, index := range plan.order {
		if err := w.charge(1); err != nil {
			return err
		}
		if group, ok := plan.groups[index]; ok {
			if err := r.drawOpacityGroup(w, pixels, depth, group, dirty); err != nil {
				return err
			}
			continue
		}
		t := ts[index]
		paint := paints[t.Material]
		if paint.alpha == 0 {
			continue
		}
		// Physical meshes use outward normals. Rendering their rear faces a
		// second time would compound authored translucency. Opaque geometry
		// remains two-sided so winding cannot silently remove a solid face.
		if paint.alpha < .999 || paint.multiply {
			n := rasterAdd(rasterAdd(t.V[0].Normal, t.V[1].Normal), t.V[2].Normal)
			if rasterDot(n, n) > 1e-20 && rasterDot(n, r.camera.direction) <= 0 {
				continue
			}
		}
		var p [3]rasterPoint
		for i, v := range t.V {
			p[i] = r.camera.project(v.Position)
		}
		area := rasterOrient(p[0], p[1], p[2])
		if math.Abs(area) < 1e-10 {
			continue
		}
		if area < 0 {
			p[1], p[2] = p[2], p[1]
			t.V[1], t.V[2] = t.V[2], t.V[1]
			area = -area
		}
		faceNormal := rasterUnit(rasterCross(rasterSub(t.V[1].Position, t.V[0].Position), rasterSub(t.V[2].Position, t.V[0].Position)))
		for i := range t.V {
			if rasterDot(t.V[i].Normal, t.V[i].Normal) < 1e-20 {
				t.V[i].Normal = faceNormal
			}
		}
		x0, y0, x1, y1 := rasterBounds(p, r.camera.width, r.camera.height)
		x0, y0, x1, y1 = max(x0, pixels.Rect.Min.X), max(y0, pixels.Rect.Min.Y), min(x1, pixels.Rect.Max.X), min(y1, pixels.Rect.Max.Y)
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		if err := w.charge(int64(y1-y0) * 3); err != nil {
			return err
		}
		tl0, tl1, tl2 := rasterTopLeft(p[1], p[2]), rasterTopLeft(p[2], p[0]), rasterTopLeft(p[0], p[1])
		for y := y0; y < y1; y++ {
			if y%16 == 0 {
				if err := w.ctx.Err(); err != nil {
					return err
				}
			}
			start, end := rasterRowSpan(p, y, x0, x1)
			if err := w.charge(int64(end - start)); err != nil {
				return err
			}
			for x := start; x < end; x++ {
				q := rasterPoint{x: float64(x) + .5, y: float64(y) + .5}
				a, b, c := rasterOrient(p[1], p[2], q), rasterOrient(p[2], p[0], q), rasterOrient(p[0], p[1], q)
				if a < 0 || b < 0 || c < 0 || (a == 0 && !tl0) || (b == 0 && !tl1) || (c == 0 && !tl2) {
					continue
				}
				a /= area
				b /= area
				c /= area
				z := float32(a*p[0].z + b*p[1].z + c*p[2].z + t.DepthBias)
				at := (y-pixels.Rect.Min.Y)*pixels.Rect.Dx() + x - pixels.Rect.Min.X
				if z < depth[at] {
					continue
				}
				alpha := paint.alpha
				rr, gg, bb := 0.0, 0.0, 0.0
				if paint.texture != nil {
					u, v := a*t.V[0].U+b*t.V[1].U+c*t.V[2].U, a*t.V[0].V+b*t.V[1].V+c*t.V[2].V
					var ta float64
					rr, gg, bb, ta = rasterTexture(paint.texture, u, v)
					alpha *= ta
					if alpha < .015 {
						continue
					}
					// Unlit label atlases are premultiplied during interpolation,
					// then unpremultiplied once before final over compositing.
					if !paint.unlit {
						position := rasterWeighted(t.V[0].Position, t.V[1].Position, t.V[2].Position, a, b, c)
						normal := rasterWeighted(t.V[0].Normal, t.V[1].Normal, t.V[2].Normal, a, b, c)
						textPaint := paint
						textPaint.base = Vec{X: rasterLinear(rr) * paint.base.X, Y: rasterLinear(gg) * paint.base.Y, Z: rasterLinear(bb) * paint.base.Z}
						rr, gg, bb = r.shade(textPaint, position, normal)
					}
				} else {
					position := rasterWeighted(t.V[0].Position, t.V[1].Position, t.V[2].Position, a, b, c)
					normal := rasterWeighted(t.V[0].Normal, t.V[1].Normal, t.V[2].Normal, a, b, c)
					rr, gg, bb = r.shade(paint, position, normal)
				}
				pi := pixels.PixOffset(x, y)
				if paint.multiply {
					rasterMultiplyOver(pixels.Pix[pi:pi+4], rr, gg, bb, alpha)
				} else {
					pixels.Pix[pi] = uint8(math.Round(rasterClamp(rr)*alpha*255 + float64(pixels.Pix[pi])*(1-alpha)))
					pixels.Pix[pi+1] = uint8(math.Round(rasterClamp(gg)*alpha*255 + float64(pixels.Pix[pi+1])*(1-alpha)))
					pixels.Pix[pi+2] = uint8(math.Round(rasterClamp(bb)*alpha*255 + float64(pixels.Pix[pi+2])*(1-alpha)))
				}
				if !t.NoDepthWrite && alpha >= .999 {
					depth[at] = z
				}
				if dirty != nil {
					dirty[(y/r.aa)*r.width+x/r.aa] = true
				}
			}
		}
	}
	return nil
}

// rasterMultiplyOver blends a straight sRGB source into a premultiplied RGBA
// destination. The uncovered backdrop fraction must receive the source color:
// multiplying by zero there would incorrectly turn transparent pixels black.
func rasterMultiplyOver(dst []uint8, red, green, blue, alpha float64) {
	alpha = rasterClamp(alpha)
	backdropAlpha := float64(dst[3]) / 255
	for i, channel := range [3]float64{red, green, blue} {
		source := rasterClamp(channel)
		backdrop := float64(dst[i]) / 255
		out := source*alpha*(1-backdropAlpha) + backdrop*(1-alpha+source*alpha)
		dst[i] = uint8(math.Round(rasterClamp(out) * 255))
	}
	dst[3] = uint8(math.Round((alpha + backdropAlpha*(1-alpha)) * 255))
}

func rasterWeighted(a, b, c Vec, x, y, z float64) Vec {
	return Vec{X: a.X*x + b.X*y + c.X*z, Y: a.Y*x + b.Y*y + c.Y*z, Z: a.Z*x + b.Z*y + c.Z*z}
}

func rasterTexture(texture image.Image, u, v float64) (float64, float64, float64, float64) {
	b := texture.Bounds()
	x := rasterClamp(u)*float64(b.Dx()) - .5
	y := rasterClamp(v)*float64(b.Dy()) - .5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	r, g, bb, a := 0.0, 0.0, 0.0, 0.0
	for dy := 0; dy <= 1; dy++ {
		for dx := 0; dx <= 1; dx++ {
			weight := 1.0
			if dx == 0 {
				weight *= 1 - fx
			} else {
				weight *= fx
			}
			if dy == 0 {
				weight *= 1 - fy
			} else {
				weight *= fy
			}
			sx, sy := max(0, min(b.Dx()-1, x0+dx))+b.Min.X, max(0, min(b.Dy()-1, y0+dy))+b.Min.Y
			cr, cg, cb, ca := texture.At(sx, sy).RGBA()
			r += float64(cr) / 65535 * weight
			g += float64(cg) / 65535 * weight
			bb += float64(cb) / 65535 * weight
			a += float64(ca) / 65535 * weight
		}
	}
	if a <= 1e-10 {
		return 0, 0, 0, 0
	}
	return r / a, g / a, bb / a, a
}

func (r *Raster) downsamplePixel(dst, src *image.RGBA, x, y int) {
	rr, gg, bb := 0, 0, 0
	for dy := 0; dy < r.aa; dy++ {
		for dx := 0; dx < r.aa; dx++ {
			at := src.PixOffset(x*r.aa+dx, y*r.aa+dy)
			rr += int(src.Pix[at])
			gg += int(src.Pix[at+1])
			bb += int(src.Pix[at+2])
		}
	}
	n := r.aa * r.aa
	at := dst.PixOffset(x, y)
	dst.Pix[at] = uint8((rr + n/2) / n)
	dst.Pix[at+1] = uint8((gg + n/2) / n)
	dst.Pix[at+2] = uint8((bb + n/2) / n)
	dst.Pix[at+3] = 255
}
