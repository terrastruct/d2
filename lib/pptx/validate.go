package pptx

import "github.com/d2lang/d2/internal/testutil"

// Validate checks that pptxContent contains the structure emitted by D2's PPTX
// exporter for nSlides.
//
// Deprecated: Validate is a test-only helper specific to D2's output and will
// be removed after one compatibility release. Downstream tests should validate
// the ZIP and OOXML properties they rely on directly.
func Validate(pptxContent []byte, nSlides int) error {
	return testutil.ValidatePPTX(pptxContent, PPTX_TEMPLATE, nSlides)
}
