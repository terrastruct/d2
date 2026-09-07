# D2 isometric renderer

The isometric renderer projects compiled D2 diagrams into static SVGs and PNGs,
animated GIFs, PDF documents and PPTX decks. It runs inside D2 using a pure-Go
renderer. Export requires no browser, JavaScript runtime, GPU, CGO, or external
font files. Local and embedded artwork works offline; remote icon URLs use the
CLI's existing bounded image resolver.

```sh
d2 --isometric --layout elk system.d2 system.svg
d2 --isometric --layout elk system.d2 system.png
d2 --isometric --layout elk system.d2 system.gif
d2 --isometric --layout elk system.d2 system.pdf
d2 --isometric --layout elk system.d2 system.pptx
d2 --isometric --scale 2 --layout elk system.d2 system.png
```

## Configuration

Like sketch, isometric can be selected through a command-line flag, environment
variable, source configuration, Go render options, or D2.js compile/render options:

```sh
D2_ISOMETRIC=true d2 system.d2 system.svg
d2 --isometric=false system.d2 system.svg
```

```d2
vars: {
  d2-config: {
    isometric: true
  }
}
a -> b
```

Explicit flags override environment values, which override source configuration.
Explicit `false` disables a source-enabled mode. Isometric defaults to SVG and
uses the compiled positions, sizes, containers and route anchors. Sketch and
isometric are mutually exclusive presentations. Ordinary rendering is unchanged
when isometric is disabled.

Go callers can pass `Isometric: go2.Pointer(true)` in `d2svg.RenderOpts` to
`d2lib.Compile` and `d2svg.Render`. D2.js accepts `{isometric: true}` wherever it
accepts `{sketch: true}`, including source-file-map compilation. Pass the returned
`renderOptions` into `render` to retain source configuration. The renderer also
runs in WebAssembly.

## Source geometry and presentation

The renderer uses one fixed orthographic camera: 15 degrees of azimuth and
approximately 49.178 degrees of elevation. This gentle skew gives surface labels
a shallow reading angle while retaining visible sidewalls. Component footprints
preserve the compiled D2 width and height at a shared scale of 0.01 world units
per layout pixel. The renderer does not run a second layout, infer groups, or
reposition components. Positive dimensions keep their exact scaled values; zero
dimensions use a one-source-pixel footprint.

Authored containers become supporting platforms. Filled nested groups become
stepped plates with vertical walls and source colors. Each independent hierarchy
lowers its ancestors beneath the existing component tops and connection plane.
The first step is 20 source pixels deep, with smaller subsequent steps and a
bounded total descent. Component walls extend down to their supporting plate;
table and class rows retain their heights above an extended backing. Dashed and
transparent regions retain flat boundaries. Invisible layout wrappers introduce
no visible hardware or additional steps. Every independent root casts onto its
own local ground, so deeper nesting in one system does not lengthen another
system's contact shadows.

Components use classic isometric ink: matte faces, sharp rims and consistent
strokes along visible structural edges and silhouettes. Shared corners use sharp
joins with bounded bevels at acute tips. Hidden edges and triangulation lines are
omitted. Source-contour bodies retain relief within their original footprint.
Curved wall normals are smoothed while sharp and concave corners stay distinct.
Cylinders have upright curved walls and flat caps; queues are horizontal barrels;
circles, ovals and hexagons are solid prisms. Patterns, borders and multiple copies
decorate these volumes. Pages have a physical inclined fold. Clouds, people and
C4 people use horizontal reliefs of their original D2 silhouettes. Cloud depth is
24% of the smaller footprint dimension; person depth is 19%.

D2 outlines, border colors, widths, dashes, radii, multiple copies, double borders,
patterns, opacity and authored shadows are retained. The existing `style.3d`
property continues to affect shape depth. Default surfaces use a muted palette;
authored colors and theme overrides remain authoritative. Solid components and
outer platforms cast restrained contact shadows, with authored shadows
strengthening that treatment. Framing includes projected ground shadows.

Labels and icons lie flat on their physical surfaces. Their source dimensions,
anchors, line breaks and whitespace are retained. Outside captions clear visible
sidewalls in projection. Container labels can move to a clear area within their
original container when raised components obscure the source anchor. Authored
light ink gets a small contrasting backing when a translucent group wash needs
it. Dense source layouts can still leave too little space for every projected
label.

Markdown headings, lists, emphasis, inline code, syntax-highlighted code blocks,
and LaTeX reuse D2's native content renderer. Standalone Markdown text uses a
raised ivory card within its compiled allocation. Rich content is uniformly
fitted to the available face. SQL tables and classes use separate solid header
and row rails, with a wider seam between class fields and methods. Gutters use
existing row padding; compiled footprints, text anchors and SQL row attachment
coordinates stay unchanged.

SVG and raster icons retain aspect ratio and transparency. Image shapes use their
face for the artwork; container and connection icons share header or caption
space. Outside icon positions are fitted inside the owning surface. CLI custom
fonts and the ordinary bounded font fallback resolver are supported. Library
callers can supply `Options.Fonts` and `Options.Assets`; without an asset resolver,
only embedded data URLs are resolved.

Every connection retains its semantic endpoints, original route, both arrowhead
kinds, labels and metadata. Polylines are preserved and cubic curves are sampled
to a bounded geometric tolerance. Inputs without a usable compiled route receive
a warning and a simple perimeter fallback. Ordinary routes remain flat. Short
contact segments meet curved endpoint walls within the source footprint. Visible
arrowheads can retreat along the same route when a raised endpoint would obscure
them; the wire stops at the displayed marker's stem.

Small, thickness-aware bridges distinguish transverse crossings. Overlapping
straight runs use small parallel lanes when space permits; these taper back to
the original ports and avoid components and headers. Crowded runs, shallow
crossings and crossings too near a corner or port retain their geometry. Wires,
dashes, arrows and animated traffic share the resolved path. Default connections
use stable muted colors, matching caption ink and a narrow paper-colored casing.
Authored colors, opacity, dashes and font colors remain intact. Authored caption
positions and percentages are retained. Captions without a position seek free
space beside the route; on-edge labels use a gap in their own wire without adding
a default background. Explicit label fills are preserved.

Sequence diagrams use individual actors, activation rails and folded notes.
Source group rectangles keep their multiply shading. Compiled positions,
dimensions, messages and caption anchors are preserved, including nested
sequences alongside ordinary shapes. Backgrounds retain source/theme fills.
Lifelines and messages stay planar; their intentional intersections bypass route
separation and bridges. Semantic spans and notes do not create platforms.
Built-in legends, root labels, icons, descriptions and root frames are rendered.

## Export behavior

SVG contains projected vector faces, edges, text outlines and vector artwork.
Source bitmaps and bitmap texture tiles, such as grain, remain embedded images.
The CLI and `d2svg.Render` envelope honor ordinary padding, centering, scale,
XML declaration, salt and version options. SVG scale changes the document
viewport and retains vector detail at arbitrary zoom. SVG face lighting uses the
native shading model and soft shadows use SVG filters. SVG casts shadows onto
the ground plane; PNG also shades raised receivers. Shadow softness and edge
antialiasing can differ slightly between formats.

PNG uses triangle meshes, a depth buffer, directional lighting, soft shadows and
2×2 coverage sampling. Large images use bounded strips to retain the same edge
quality without a full supersampled image in memory. Matte lighting preserves
face colors and reveals sidewalls; outlines, labels and connector ink remain
unlit. Surface textures use output density to choose their resolution without
changing physical text sizes or footprints.

CLI native SVG geometry, PNG, PDF and PPTX images fit within 1600 × 1000 pixels;
GIFs fit within 1000 × 625. The canvas follows the diagram's projected aspect
ratio, with a small margin and at least 64 pixels per side. `--scale` scales these
maximum dimensions for raster and paged exports. Native images are bounded to
4096 pixels per side, 12 million pixels per still image, 100 million aggregate
capture pixels per GIF, 64 MiB of encoded output, and two minutes of rendering.
A failed file export leaves the previous destination intact.

GIF uses the same fixed camera. Animation follows authored `style.animated`;
ordinary connections and lifelines do not acquire traffic. Traffic loops contain
100 frames over 8.33 seconds. Authored shape or surface-panel animation uses 96
frames over 8 seconds, fitting repeating periods to whole cycles. The exact-time
frame API retains authored timing. Static geometry, palette and quantization are
shared across frames. No intermediate PNG files or external processes are used.

PDF and PPTX use D2's existing native document writers with one image per page or
slide. They retain board navigation and projected links, including inline
Markdown links. The diagram is a raster image; use `--scale` to increase its
detail. Existing paged-writer aggregate image and metadata limits apply.

Layers, scenarios and steps export as SVG or PNG using the ordinary directory
structure. GIF plays selected boards in traversal order; `--animate-interval`
sets the interval, defaulting to 1000ms per board. Multi-board GIF measures the
union of boards and animated extents, keeping one camera and canvas. PDF and PPTX
use ordinary page/slide traversal. `--target` selects a board for each supported
format. `--stdout-format=svg -` writes one selected board to stdout.

SVG, PDF and PPTX retain supported projected links and tooltip metadata. Internal
board links target sibling exports. PNG and GIF have no interactive annotations.
Watch uses D2's ordinary file watcher with an SVG preview while writing the
requested format. Source edits can enable or disable isometric without restarting
watch.

## Image API

Library callers render a compiled board directly to encoded SVG or image bytes:

```go
png, err := d2isometricimg.Render(ctx, diagram, &d2isometricimg.Options{
    Format: d2isometricimg.PNG,
    FitContent: true,
})
```

Use `d2isometricimg.SVG` for vector output or `d2isometricimg.GIF` for animation.
The image API retains fixed `Width` and `Height` by default (1600 × 1000 for
SVG/PNG, 1000 × 625 for GIF). `FitContent` treats them as maximum dimensions,
matching CLI framing. `RenderPage` returns PNG data, final dimensions and bounded
annotation regions for document writers. `d2svg.RenderIsometric` also accepts
native options for asset and font resolvers.

`BuildScene` maps one compiled D2 board into an independently owned scene. Y is
elevation; the source X/Y layout lies in world-space X/Z. IDs, container parents,
semantic endpoints and complete original metadata are retained. Construction and
rendering are deterministic and never mutate or share nested source data. Scene
admission bounds inputs to 2,000 nodes, 5,000 edges, 100,000 collection entries and
8 MiB of text. Invalid or nonfinite coordinates, excessive dimensions, duplicate
IDs, nil route points and unknown endpoints without usable routes are rejected.

## Compatibility boundaries

The renderer shares D2's compiler and layout engines, but some presentation
features differ from the flat and sketch renderers:

- Responsive `--dark-theme`, `--force-appendix`, sketch combination and
  multi-board animated SVG are rejected. One selected dark theme works;
  GIF supports authored animation and board progression. SVG is static.
- `tooltip.near` does not paint a positioned callout. SVG retains tooltip hover
  metadata; PNG has no automatic tooltip/link appendix.
- Connection corner rounding uses native presentation radii rather than authored
  connection `style.border-radius`.
- Transparent root backgrounds become opaque paper. Outside icons fit inside
  their faces. API/plugin `ZIndex` does not override physical depth occlusion for
  ordinary shapes. Root title placement is renderer-defined.
- Raster and paged padding retains native framing instead of the ordinary SVG
  `--pad` contract. Artwork stays embedded when `--bundle=false` is supplied.
- D2.js and direct `d2svg.Render` calls require embedded image data URLs. CLI and
  Go callers with a supplied asset resolver can use local or remote artwork.
- Legacy layout plugins that change flat SVG through `PostProcess` fail before
  native export because those edits cannot transfer to native geometry. No-op
  postprocessors work.

## Development

```sh
go test ./d2renderers/d2isometric/... ./d2cli
```

Tests cover source geometry and metadata ownership, containment and quoted IDs,
sequence semantics, generated endpoints, parallel edges and self-loops, paint
provenance, typography, route visibility, shadows, vector surfaces, animation,
resource admission and deterministic output. Isometric SVG snapshots sit beside
regular and sketch SVGs in `e2etests`. The separate `e2etests/png` bundle checks
PNG output.
