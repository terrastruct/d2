# Target pixel coverage

`target-shapes.exp.png` is the exact raster-renderer composite for every
value in `d2target.Shapes`. `target-arrowheads.exp.png` is the matching
composite for every distinct `d2target.Arrowhead` value; `DefaultArrowhead` is
excluded because it aliases `TriangleArrowhead`.

The executable tests use the listed constants to construct typed targets,
build `d2scene` documents, and render final PNG pixels with `d2raster`. They
also require deterministic repeated PNG bytes and non-background pixels in
every value's region. The shape list is checked directly against
`d2target.Shapes`; the arrowhead list is checked against the configurable and
derived arrowhead values before either composite is rendered.

An exact mismatch writes ignored, persistent `*.got.png` and
`*.got.diff.html` artifacts beside the golden. The HTML report embeds the
expected image, rendered output, overlay, and heatmap. Regenerate only after
reviewing the rendered composites:

```sh
D2_UPDATE_TARGET_PIXEL_GOLDENS=1 GOWORK=off \
  go test ./d2renderers/d2raster \
  -run '^TestTarget(Shape|Arrowhead)Pixels$' -count=1 -v
```

Regenerate only with the command above and record the encoded SHA-256 values.
