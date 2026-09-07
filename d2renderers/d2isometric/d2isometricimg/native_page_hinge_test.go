package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func pageHingeTestKey(a, c Vec) classicInkPair {
	x, y := classicInkKeyOf(a), classicInkKeyOf(c)
	if classicInkLess(y, x) {
		x, y = y, x
	}
	return classicInkPair{x, y}
}

// The fold must attach to the rounded source boundary itself. A separate
// straight triangle slightly above that boundary creates a second diagonal
// outline and an independent miter at the fold's upper corner.
func TestNativePageFoldSharesItsSourceHinge(t *testing.T) {
	for _, scale := range []float64{.006, .03} {
		for _, config := range []struct {
			pattern      string
			multiple     bool
			dash         float64
			sourceHeight int
		}{{}, {pattern: "dots", multiple: true}, {pattern: "lines", dash: 3}, {sourceHeight: 32}} {
			t.Run(fmt.Sprintf("scale=%g/pattern=%s/multiple=%t/height=%d", scale, config.pattern, config.multiple, config.sourceHeight), func(t *testing.T) {
				n := fidelityNode(d2target.ShapePage)
				if config.sourceHeight > 0 {
					n.Metadata.Original.Height = config.sourceHeight
					n.Size.Z = float64(config.sourceHeight) * .01
				}
				n.Size, n.Position = nmul(n.Size, scale/.01), nmul(n.Position, scale/.01)
				n.StrokeWidth, n.StrokeDash = 6, config.dash
				n.Metadata.Original.FillPattern = config.pattern
				n.Metadata.Original.Multiple = config.multiple
				ctx := nativeVectorContext(context.Background())
				b := &meshBuilder{ctx: ctx, scale: scale, options: nativeSceneOptions{vector: true}}
				b.canonicalNode(n, "")
				if b.err != nil {
					t.Fatal(b.err)
				}
				s := nativeFaceSource(n, n.Fill)
				outer := shape.GetPathCommands(shape.NewPage(geo.NewBox(geo.NewPoint(0, 0), float64(s.Width), float64(s.Height))))[0]
				// The top horizontal segment and second corner curve end at
				// the two attachment points. Read the actual rounded source
				// commands, independently of the renderer's contour traversal.
				a, c := outer[1].End, outer[4].End
				if a.Y != 0 || c.X != float64(s.Width) || a.X >= c.X || c.Y <= 0 || c.Y >= float64(s.Height) {
					t.Fatal("fixture no longer describes the source page corner")
				}
				profiles, err := nativeShapeProfiles(s)
				if err != nil {
					t.Fatal(err)
				}
				height := nativeCanonicalHeight(n, scale)
				top := n.Position.Y - n.Size.Y/2 + height
				copies := 1
				if config.multiple {
					copies++
				}
				segments, err := classicInkSegments(ctx, n, b.triangles)
				if err != nil {
					t.Fatal(err)
				}
				ink := make(map[classicInkPair]int)
				for _, segment := range segments {
					ink[pageHingeTestKey(segment.a, segment.c)]++
				}
				for copyIndex := 0; copyIndex < copies; copyIndex++ {
					dx := float64(copyIndex) * d2target.MULTIPLE_OFFSET * scale
					dz := -dx
					y := top - float64(copyIndex)*math.Min(.045, height*.25)
					perimeter := make(map[classicInkPair]bool)
					for _, profile := range profiles {
						world := make([]Vec, len(profile))
						for i, p := range profile {
							world[i] = nv(n.Position.X-n.Size.X/2+dx+p.X*n.Size.X/float64(s.Width), y,
								n.Position.Z-n.Size.Z/2+dz+p.Z*n.Size.Z/float64(s.Height))
						}
						for i, p := range world {
							perimeter[pageHingeTestKey(p, world[(i+1)%len(world)])] = true
						}
					}
					hinges := make(map[classicInkPair]int)
					hingeVertices := make(map[classicInkKey]int)
					wall := make(map[classicInkPair]int)
					for _, tri := range b.triangles {
						if !tri.CastShadow || tri.Material == nil || tri.NoDepthWrite {
							continue
						}
						var anchors []Vec
						above, below := false, false
						for _, v := range tri.V {
							if math.Abs(v.Position.Y-y) < 1e-9 {
								anchors = append(anchors, v.Position)
							}
							above = above || v.Position.Y > y+.01
							below = below || v.Position.Y < y-.01
						}
						if len(anchors) == 2 && below && tri.Material.Texture == nil {
							wall[pageHingeTestKey(anchors[0], anchors[1])]++
						}
						if !above || tri.Material.Texture == nil || tri.V[0].Normal.Y < .1 {
							continue
						}
						// Copies are physically separate: only a face attached at
						// this copy's exact height belongs to its folded corner.
						if len(anchors) != 2 {
							if copies == 1 {
								t.Fatal("raised page fold is detached from the source cap")
							}
							continue
						}
						key := pageHingeTestKey(anchors[0], anchors[1])
						if !perimeter[key] {
							t.Fatal("fold hinge cuts across the rounded source perimeter")
						}
						hinges[key]++
						for _, anchor := range anchors {
							hingeVertices[classicInkKeyOf(anchor)]++
						}
						for _, vertex := range tri.V {
							if math.Abs(vertex.Position.X-n.Position.X-dx) > n.Size.X/2+1e-9 ||
								math.Abs(vertex.Position.Z-n.Position.Z-dz) > n.Size.Z/2+1e-9 {
								t.Fatal("fold extends beyond its source copy's footprint")
							}
						}
						texture, ok := tri.Material.Texture.(*image.RGBA)
						if !ok || tri.Material.Vector == nil || tri.Material.Vector != nativeVectorForTexture(ctx, texture) {
							t.Fatal("fold lost its retained source vector paint")
						}
						if config.pattern != "" {
							fragment, err := nativeSurfaceSVG(ctx, tri.Material.Vector, "fold")
							if err != nil {
								t.Fatal(err)
							}
							if !strings.Contains(fragment, "<pattern") || strings.Contains(fragment, "<image") {
								t.Fatal("fold flattened or dropped the authored pattern")
							}
						}
					}
					if len(hinges) < 3 {
						t.Fatalf("copy %d did not follow the rounded hinge: %d segments", copyIndex, len(hinges))
					}
					// A partial prefix of the rounded hinge cannot substitute
					// for attachment at both ends, including on short pages.
					for _, source := range []geo.Point{a, c} {
						endpoint := nv(n.Position.X-n.Size.X/2+dx+source.X*n.Size.X/float64(s.Width), y,
							n.Position.Z-n.Size.Z/2+dz+source.Y*n.Size.Z/float64(s.Height))
						if count := hingeVertices[classicInkKeyOf(endpoint)]; count != 1 {
							t.Fatalf("source hinge endpoint %v must attach once, got %d", endpoint, count)
						}
					}
					for key, count := range hinges {
						if count != 1 || wall[key] != 1 {
							t.Fatalf("hinge must join one fold face and one body wall: fold=%d wall=%d", count, wall[key])
						}
						if config.dash == 0 && ink[key] != 1 {
							t.Fatalf("hinge must have one structural outline, got %d", ink[key])
						}
					}
				}
			})
		}
	}
}
