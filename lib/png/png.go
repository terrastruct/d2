package png

import (
	"bytes"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	pngstruct "github.com/dsoprea/go-png-image-structure/v2"

	"github.com/d2lang/d2/lib/version"
)

// SCALE is the static raster device scale used by PNG, PDF, and PPTX export.
const SCALE = 2.

func AddExif(png []byte) ([]byte, error) {
	// https://pkg.go.dev/github.com/dsoprea/go-png-image-structure/v2?utm_source=godoc#example-ChunkSlice.SetExif
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, err
	}

	ti := exif.NewTagIndex()

	ib := exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, exifcommon.TestDefaultByteOrder)

	err = ib.AddStandardWithName("Make", "D2")
	if err != nil {
		return nil, err
	}

	err = ib.AddStandardWithName("Model", version.Version)
	if err != nil {
		return nil, err
	}

	pmp := pngstruct.NewPngMediaParser()
	intfc, err := pmp.ParseBytes(png)
	if err != nil {
		return nil, err
	}
	cs := intfc.(*pngstruct.ChunkSlice)
	err = cs.SetExif(ib)
	if err != nil {
		return nil, err
	}
	b := new(bytes.Buffer)
	err = cs.WriteTo(b)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
