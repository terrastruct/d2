# d2fonts

The SVG renderer embeds fonts directly into the SVG as base64 data. This is to give
deterministic outputs and load without a network call.

To include your own font, e.g. `Helvetica`, you must include the Truetype glyphs:
- `./ttf/Helvetica-Bold.ttf`
- `./ttf/Helvetica-Italic.ttf`
- `./ttf/Helvetica-Regular.ttf`

You must also include an encoded version of these of mimetype `application/font-woff`:
- `./ttf/Helvetica-Bold.txt`
- `./ttf/Helvetica-Italic.txt`
- `./ttf/Helvetica-Regular.txt`

If you include a font to contribute, it must have an open license.
