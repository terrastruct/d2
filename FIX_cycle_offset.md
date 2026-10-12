# Fix: cycle shape render offset

Fix cycle shape offset by adding stroke width to bounds calculation in render/shape.go:

```go
case "cycle", "circle":
    radius := math.Min(float64(width), float64(height))/2 + float64(strokeWidth)
        cx, cy := float64(width)/2, float64(height)/2
            bounds = image.Rect(int(cx-radius), int(cy-radius), int(cx+radius), int(cy+radius))
            ```

            Closes #1234
