# Raster fixtures

Tests compare decoded pixels exactly and require repeated exports to produce
identical PNG/GIF bytes. A mismatch writes a self-contained HTML report with
expected, actual, overlay, and heatmap images.

The intentionally small fixture set covers distinct regression categories:

- `geometry`: shape bounds, paint order, connection stroke, and arrowhead;
- `markdown`: list bullets, styled spans, link paint, label masking, and text;
- `local-icon`: a deterministic in-tree SVG asset, gradient, and path import;
- `animation`: frame sampling/order/change and both sub-second (310 ms) and
  fractional-second (1050 ms) centisecond schedules;
- `unlabelled-connection-link`: implicit connection-link labels and native
  connection text/link rendering.

From the repository root, regenerate the fixtures with the same arguments used
by the tests:

```sh
go run . --pad=16 e2etests-cli/testdata/raster/geometry.d2 e2etests-cli/testdata/raster/geometry.exp.png
go run . --pad=16 e2etests-cli/testdata/raster/markdown.d2 e2etests-cli/testdata/raster/markdown.exp.png
go run . --pad=16 e2etests-cli/testdata/raster/local-icon.d2 e2etests-cli/testdata/raster/local-icon.exp.png
go run . --pad=16 e2etests-cli/testdata/raster/unlabelled-connection-link.d2 e2etests-cli/testdata/raster/unlabelled-connection-link.exp.png
go run . --pad=16 --animate-interval=310 e2etests-cli/testdata/raster/animation.d2 e2etests-cli/testdata/raster/animation-310.exp.gif
go run . --pad=16 --animate-interval=1050 e2etests-cli/testdata/raster/animation.d2 e2etests-cli/testdata/raster/animation-1050.exp.gif
go test ./e2etests-cli -run 'Test(PNG|GIF)Fixtures'
```

Inspect each rendered fixture before updating only the affected expected file.
