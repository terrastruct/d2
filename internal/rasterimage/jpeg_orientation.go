package rasterimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
)

const (
	jpegMarkerPrefix = 0xff
	jpegMarkerSOI    = 0xd8
	jpegMarkerEOI    = 0xd9
	jpegMarkerSOS    = 0xda
	jpegMarkerAPP1   = 0xe1
	exifOrientation  = 0x0112
)

// jpegEXIFOrientation reads only marker and TIFF metadata preceding the first
// entropy-coded scan. Invalid or unsupported EXIF is ignored, matching image
// viewers that render the JPEG normally when its optional metadata is broken.
func jpegEXIFOrientation(ctx context.Context, data []byte) (uint8, error) {
	if len(data) < 2 || data[0] != jpegMarkerPrefix || data[1] != jpegMarkerSOI {
		return 1, nil
	}
	for offset := 2; offset < len(data); {
		if err := contextError(ctx); err != nil {
			return 1, err
		}
		if data[offset] != jpegMarkerPrefix {
			return 1, nil
		}
		for offset < len(data) && data[offset] == jpegMarkerPrefix {
			offset++
		}
		if offset >= len(data) {
			return 1, nil
		}
		marker := data[offset]
		offset++
		switch {
		case marker == jpegMarkerSOS || marker == jpegMarkerEOI:
			return 1, nil
		case marker == jpegMarkerSOI || marker == 0x01 || marker >= 0xd0 && marker <= 0xd7:
			continue
		}
		if len(data)-offset < 2 {
			return 1, nil
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if length < 2 || length > len(data)-offset {
			return 1, nil
		}
		payload := data[offset+2 : offset+length]
		if marker == jpegMarkerAPP1 {
			if orientation, ok := parseEXIFOrientation(ctx, payload); ok {
				return orientation, nil
			}
			if err := contextError(ctx); err != nil {
				return 1, err
			}
		}
		offset += length
	}
	return 1, nil
}

func parseEXIFOrientation(ctx context.Context, payload []byte) (uint8, bool) {
	if len(payload) < 14 || !bytes.Equal(payload[:6], []byte("Exif\x00\x00")) {
		return 0, false
	}
	tiff := payload[6:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0, false
	}
	ifdOffset := uint64(order.Uint32(tiff[4:8]))
	if ifdOffset > uint64(len(tiff)) || uint64(len(tiff))-ifdOffset < 2 {
		return 0, false
	}
	ifd := tiff[int(ifdOffset):]
	entries := uint64(order.Uint16(ifd[:2]))
	if entries > (uint64(len(ifd))-2)/12 {
		return 0, false
	}
	for index := uint64(0); index < entries; index++ {
		if index&255 == 0 {
			if contextError(ctx) != nil {
				return 0, false
			}
		}
		entry := ifd[2+index*12 : 2+(index+1)*12]
		if order.Uint16(entry[:2]) != exifOrientation {
			continue
		}
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			return 0, false
		}
		orientation := order.Uint16(entry[8:10])
		if orientation < 1 || orientation > 8 {
			return 0, false
		}
		return uint8(orientation), true
	}
	return 0, false
}

func orientedConfig(config image.Config, orientation uint8) image.Config {
	if orientation >= 5 && orientation <= 8 {
		config.Width, config.Height = config.Height, config.Width
	}
	return config
}

func orientImage(source image.Image, orientation uint8) image.Image {
	if orientation <= 1 || orientation > 8 {
		return source
	}
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	destinationWidth, destinationHeight := width, height
	if orientation >= 5 {
		destinationWidth, destinationHeight = height, width
	}
	return orientedImage{
		Image: source, orientation: orientation,
		bounds: image.Rect(0, 0, destinationWidth, destinationHeight),
	}
}

type orientedImage struct {
	image.Image
	orientation uint8
	bounds      image.Rectangle
}

func (img orientedImage) Bounds() image.Rectangle { return img.bounds }

func (img orientedImage) At(x, y int) color.Color {
	point := image.Pt(x, y)
	if !point.In(img.bounds) {
		return color.NRGBA{}
	}
	x -= img.bounds.Min.X
	y -= img.bounds.Min.Y
	width, height := img.Image.Bounds().Dx(), img.Image.Bounds().Dy()
	var sourceX, sourceY int
	switch img.orientation {
	case 2:
		sourceX, sourceY = width-1-x, y
	case 3:
		sourceX, sourceY = width-1-x, height-1-y
	case 4:
		sourceX, sourceY = x, height-1-y
	case 5:
		sourceX, sourceY = y, x
	case 6:
		sourceX, sourceY = y, height-1-x
	case 7:
		sourceX, sourceY = width-1-y, height-1-x
	case 8:
		sourceX, sourceY = width-1-y, x
	default:
		sourceX, sourceY = x, y
	}
	minimum := img.Image.Bounds().Min
	return img.Image.At(minimum.X+sourceX, minimum.Y+sourceY)
}
