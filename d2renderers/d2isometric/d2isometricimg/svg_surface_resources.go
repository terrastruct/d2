package d2isometricimg

import "fmt"

// One retained surface can cover hundreds of mesh triangles and also cast a
// shadow. Serialize its vectors once per export, then reuse the same definition
// through each geometry/lighting transform; paper patterns stay vector artwork.
func (w *nativeSVGWriter) surfaceDefinition(surface *nativeVectorSurface) string {
	if w.err != nil {
		return ""
	}
	if w.surfaces == nil {
		w.surfaces = make(map[*nativeVectorSurface]string)
	}
	if id := w.surfaces[surface]; id != "" {
		return id
	}
	id := fmt.Sprintf("surface-%d", len(w.surfaces))
	fragment, err := nativeSurfaceSVG(w.ctx, surface, id+"-")
	if err != nil {
		w.err = err
		return ""
	}
	w.write(`<defs><g id="%s">%s</g></defs>`, id, fragment)
	if w.err != nil {
		return ""
	}
	w.surfaces[surface] = id
	return id
}
