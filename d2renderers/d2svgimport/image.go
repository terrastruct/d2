package d2svgimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/internal/rasterimage"
)

const embeddedRasterAssetPrefix = "svg-raster:"

func (i *svgImporter) compileEmbeddedImage(element *svgElement) (svgGeometry, error) {
	if len(element.children) != 0 {
		return svgGeometry{}, i.errorf("element <image> cannot contain child elements")
	}
	raw, ok := element.attrs["href"]
	if !ok {
		return svgGeometry{}, i.errorf("element <image> requires one data: href")
	}
	if _, ok := element.attrs["width"]; !ok {
		return svgGeometry{}, i.errorf("element <image> requires explicit width and height")
	}
	if _, ok := element.attrs["height"]; !ok {
		return svgGeometry{}, i.errorf("element <image> requires explicit width and height")
	}

	x, err := i.lengthAttribute(element, "x", 0, false)
	if err != nil {
		return svgGeometry{}, err
	}
	y, err := i.lengthAttribute(element, "y", 0, false)
	if err != nil {
		return svgGeometry{}, err
	}
	width, err := i.lengthAttribute(element, "width", 0, true)
	if err != nil {
		return svgGeometry{}, err
	}
	height, err := i.lengthAttribute(element, "height", 0, true)
	if err != nil {
		return svgGeometry{}, err
	}
	if width <= 0 || height <= 0 {
		return svgGeometry{}, i.errorf("element <image> width and height must be positive")
	}
	if math.IsInf(x+width, 0) || math.IsNaN(x+width) || math.IsInf(y+height, 0) || math.IsNaN(y+height) {
		return svgGeometry{}, i.errorf("element <image> geometry exceeds the finite numeric domain")
	}
	aspect, err := i.parseAspectRatioFor(element.attrs["preserveAspectRatio"], "embedded image")
	if err != nil {
		return svgGeometry{}, err
	}
	assetID, err := i.resolveEmbeddedRaster(raw)
	if err != nil {
		return svgGeometry{}, err
	}
	return svgGeometry{
		kind: geometryImage, box: d2scene.Box{X: x, Y: y, Width: width, Height: height},
		asset: assetID, aspect: aspect,
	}, nil
}

func (i *svgImporter) resolveEmbeddedRaster(raw string) (d2scene.AssetID, error) {
	mimeType, payload, err := i.parseEmbeddedRasterDataURI(raw)
	if err != nil {
		return "", err
	}
	data, err := decodeCanonicalEmbeddedBase64(i.ctx, payload)
	if err != nil {
		if contextErr := i.ctx.Err(); contextErr != nil {
			return "", contextErr
		}
		return "", i.errorf("element <image> has malformed canonical base64 data")
	}
	if err := i.ctx.Err(); err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", i.errorf("element <image> data payload is empty")
	}
	if len(data) > i.limits.MaxBytes {
		return "", i.errorf("embedded raster bytes exceed limit %d", i.limits.MaxBytes)
	}
	format := embeddedRasterFormatForMIME(mimeType)
	if got := embeddedRasterSignature(data); got == "" {
		return "", i.errorf("element <image> data has no supported raster signature")
	} else if got != format {
		return "", i.errorf("element <image> MIME type does not match its raster signature")
	}

	assetID, err := i.embeddedRasterContentID(mimeType, data)
	if err != nil {
		return "", err
	}
	if existing, ok := i.assets[assetID]; ok {
		asset, ok := existing.(d2scene.RasterAsset)
		if !ok || asset.MIMEType != mimeType {
			return "", i.errorf("embedded raster content ID collision")
		}
		equal, err := equalRasterBytes(i.ctx, asset.Data, data)
		if err != nil {
			return "", err
		}
		if !equal {
			return "", i.errorf("embedded raster content ID collision")
		}
		return assetID, nil
	}
	if len(i.ids)+len(i.assets) >= i.limits.MaxResources {
		return "", i.errorf("declared resource count exceeds limit %d", i.limits.MaxResources)
	}
	if len(data) > i.limits.MaxBytes-i.rasterBytes {
		return "", i.errorf("embedded raster bytes exceed limit %d", i.limits.MaxBytes)
	}
	if err := inspectStaticEmbeddedRaster(i.ctx, data, format); err != nil {
		if contextErr := i.ctx.Err(); contextErr != nil {
			return "", contextErr
		}
		return "", i.errorf("element <image> has unsupported or malformed %s data: %v", strings.ToUpper(format), err)
	}

	config, animated, err := rasterimage.Config(i.ctx, data, format)
	if err != nil {
		if contextErr := i.ctx.Err(); contextErr != nil {
			return "", contextErr
		}
		return "", i.errorf("element <image> has malformed %s dimensions: %v", strings.ToUpper(format), err)
	}
	if animated {
		return "", i.errorf("element <image> has unsupported animated %s data", strings.ToUpper(format))
	}
	decodedBytes, err := embeddedRasterDecodedBytes(config)
	if err != nil {
		return "", i.errorf("element <image> has invalid decoded dimensions: %v", err)
	}
	if decodedBytes > int64(i.limits.MaxBytes)-i.rasterDecodedBytes {
		return "", i.errorf("embedded raster decoded bytes exceed limit %d", i.limits.MaxBytes)
	}

	if err := i.ctx.Err(); err != nil {
		return "", err
	}

	i.assets[assetID] = d2scene.RasterAsset{
		MIMEType: mimeType, Data: data, PixelWidth: config.Width, PixelHeight: config.Height, DecodedBytes: decodedBytes,
	}
	i.rasterBytes += len(data)
	i.rasterDecodedBytes += decodedBytes
	return assetID, nil
}

func decodeCanonicalEmbeddedBase64(ctx context.Context, payload string) ([]byte, error) {
	padding := 0
	if strings.HasSuffix(payload, "=") {
		padding++
	}
	if strings.HasSuffix(payload, "==") {
		padding++
	}
	decodedLen := base64.StdEncoding.DecodedLen(len(payload)) - padding
	if decodedLen < 0 {
		return nil, base64.CorruptInputError(0)
	}
	data := make([]byte, decodedLen)
	const encodedChunkBytes = 32 << 10 // divisible by one four-byte base64 quantum
	written := 0
	for offset := 0; offset < len(payload); offset += encodedChunkBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + encodedChunkBytes
		if end > len(payload) {
			end = len(payload)
		}
		count, err := base64.StdEncoding.Strict().Decode(data[written:], []byte(payload[offset:end]))
		if err != nil {
			return nil, err
		}
		written += count
	}
	if written != len(data) {
		return nil, base64.CorruptInputError(len(payload))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (i *svgImporter) parseEmbeddedRasterDataURI(raw string) (mimeType, payload string, err error) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", i.errorf("element <image> href must be one embedded data: raster; external, file, network, blob, and local references are forbidden")
	}
	remainder := raw[len("data:"):]
	const maxMetadataBytes = 64
	comma := -1
	for index := 0; index < len(remainder) && index <= maxMetadataBytes; index++ {
		if index&31 == 0 {
			if err := i.ctx.Err(); err != nil {
				return "", "", err
			}
		}
		if remainder[index] == ',' {
			comma = index
			break
		}
	}
	if comma < 0 {
		if len(remainder) > maxMetadataBytes {
			return "", "", i.errorf("element <image> data URI metadata exceeds limit %d", maxMetadataBytes)
		}
		return "", "", i.errorf("element <image> data URI is missing its comma separator")
	}
	metadata, payload := remainder[:comma], remainder[comma+1:]
	parts := strings.Split(metadata, ";")
	if len(parts) != 2 || parts[1] != "base64" {
		return "", "", i.errorf("element <image> data URI must contain one MIME type followed by one final base64 marker")
	}
	switch parts[0] {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		mimeType = parts[0]
	default:
		return "", "", i.errorf("element <image> has unsupported data URI MIME type; expected image/png, image/jpeg, image/gif, or image/webp")
	}
	if payload == "" {
		return "", "", i.errorf("element <image> data payload is empty")
	}
	if len(payload)%4 != 0 {
		return "", "", i.errorf("element <image> has malformed canonical base64 data")
	}
	paddingStart := -1
	for index := 0; index < len(payload); index++ {
		if index&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return "", "", err
			}
		}
		value := payload[index]
		if value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '+' || value == '/' {
			continue
		}
		if value == '=' && index >= len(payload)-2 {
			if paddingStart < 0 {
				paddingStart = index
			}
			continue
		}
		return "", "", i.errorf("element <image> has malformed canonical base64 data")
	}
	if paddingStart >= 0 {
		padding := len(payload) - paddingStart
		if padding > 2 || padding == 2 && payload[len(payload)-1] != '=' {
			return "", "", i.errorf("element <image> has malformed canonical base64 data")
		}
	}
	return mimeType, payload, i.ctx.Err()
}

func (i *svgImporter) embeddedRasterContentID(mimeType string, data []byte) (d2scene.AssetID, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("d2svgimport:raster\x00"))
	_, _ = hash.Write([]byte(mimeType))
	_, _ = hash.Write([]byte{0})
	const chunkBytes = 32 << 10
	for offset := 0; offset < len(data); offset += chunkBytes {
		if err := i.ctx.Err(); err != nil {
			return "", err
		}
		end := offset + chunkBytes
		if end > len(data) {
			end = len(data)
		}
		_, _ = hash.Write(data[offset:end])
	}
	if err := i.ctx.Err(); err != nil {
		return "", err
	}
	return d2scene.AssetID(fmt.Sprintf("%s%x", embeddedRasterAssetPrefix, hash.Sum(nil))), nil
}

func equalRasterBytes(ctx context.Context, left, right []byte) (bool, error) {
	if len(left) != len(right) {
		return false, nil
	}
	const chunkBytes = 32 << 10
	for offset := 0; offset < len(left); offset += chunkBytes {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		end := offset + chunkBytes
		if end > len(left) {
			end = len(left)
		}
		if !bytes.Equal(left[offset:end], right[offset:end]) {
			return false, nil
		}
	}
	return true, ctx.Err()
}

func embeddedRasterFormatForMIME(mimeType string) string {
	switch mimeType {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpeg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func embeddedRasterSignature(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpeg"
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	default:
		return ""
	}
}

func embeddedRasterDecodedBytes(config image.Config) (int64, error) {
	if config.Width <= 0 || config.Height <= 0 {
		return 0, fmt.Errorf("decoded dimensions %dx%d are not positive", config.Width, config.Height)
	}
	if int64(config.Width) > math.MaxInt64/int64(config.Height) {
		return 0, fmt.Errorf("decoded pixel count overflows int64")
	}
	pixels := int64(config.Width) * int64(config.Height)
	bytesPerPixel := int64(4)
	if config.ColorModel != nil {
		sample := config.ColorModel.Convert(color.RGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
		switch sample.(type) {
		case color.RGBA64, color.NRGBA64, color.Gray16, color.Alpha16:
			bytesPerPixel = 8
		}
	}
	if pixels > math.MaxInt64/bytesPerPixel {
		return 0, fmt.Errorf("decoded byte count overflows int64")
	}
	return pixels * bytesPerPixel, nil
}

func inspectStaticEmbeddedRaster(ctx context.Context, data []byte, format string) error {
	switch format {
	case "png":
		return inspectEmbeddedPNG(ctx, data)
	case "jpeg":
		return inspectEmbeddedJPEG(ctx, data)
	case "gif":
		return inspectEmbeddedGIF(ctx, data)
	case "webp":
		return inspectEmbeddedWebP(ctx, data)
	default:
		return fmt.Errorf("unsupported raster format %q", format)
	}
}

func inspectEmbeddedJPEG(ctx context.Context, data []byte) error {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return fmt.Errorf("JPEG is missing its SOI marker")
	}
	offset := 2
	inScan := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if inScan {
			for offset < len(data) {
				if offset&32767 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if data[offset] != 0xff {
					offset++
					continue
				}
				markerStart := offset
				for offset < len(data) && data[offset] == 0xff {
					if offset&32767 == 0 {
						if err := ctx.Err(); err != nil {
							return err
						}
					}
					offset++
				}
				if offset >= len(data) {
					return fmt.Errorf("JPEG entropy data ends in a partial marker")
				}
				marker := data[offset]
				if marker == 0x00 || marker >= 0xd0 && marker <= 0xd7 {
					offset++
					continue
				}
				offset = markerStart
				inScan = false
				break
			}
			if inScan {
				return fmt.Errorf("JPEG is missing its EOI marker")
			}
		}
		if offset >= len(data) || data[offset] != 0xff {
			return fmt.Errorf("JPEG has data outside an entropy-coded scan")
		}
		for offset < len(data) && data[offset] == 0xff {
			if offset&32767 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			offset++
		}
		if offset >= len(data) {
			return fmt.Errorf("JPEG ends in a partial marker")
		}
		marker := data[offset]
		offset++
		switch {
		case marker == 0xd9:
			if offset != len(data) {
				return fmt.Errorf("JPEG has trailing data after its EOI marker")
			}
			return nil
		case marker == 0xd8:
			return fmt.Errorf("JPEG contains an unexpected nested SOI marker")
		case marker == 0x01 || marker >= 0xd0 && marker <= 0xd7:
			continue
		}
		if len(data)-offset < 2 {
			return fmt.Errorf("JPEG has a truncated marker length")
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if length < 2 || length > len(data)-offset {
			return fmt.Errorf("JPEG has a malformed or truncated marker segment")
		}
		offset += length
		if marker == 0xda {
			inScan = true
		}
	}
}

func inspectEmbeddedPNG(ctx context.Context, data []byte) error {
	offset := 8
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(data)-offset < 12 {
			return fmt.Errorf("truncated PNG chunk")
		}
		length := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkBytes := length + 12
		if chunkBytes > uint64(len(data)-offset) {
			return fmt.Errorf("truncated PNG chunk data")
		}
		chunkType := string(data[offset+4 : offset+8])
		if chunkType == "acTL" || chunkType == "fcTL" || chunkType == "fdAT" {
			return fmt.Errorf("animated PNG is unsupported")
		}
		offset += int(chunkBytes)
		if chunkType == "IEND" {
			if length != 0 || offset != len(data) {
				return fmt.Errorf("PNG has malformed IEND or trailing data")
			}
			return nil
		}
	}
}

func inspectEmbeddedGIF(ctx context.Context, data []byte) error {
	if len(data) < 13 {
		return fmt.Errorf("truncated GIF logical screen descriptor")
	}
	offset := 13
	if data[10]&0x80 != 0 {
		offset += 3 * (1 << ((data[10] & 0x07) + 1))
		if offset > len(data) {
			return fmt.Errorf("truncated GIF global color table")
		}
	}
	frames := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if offset >= len(data) {
			return fmt.Errorf("GIF is missing its trailer")
		}
		marker := data[offset]
		offset++
		switch marker {
		case 0x3b:
			if frames != 1 {
				return fmt.Errorf("animated or malformed GIF has %d frames; exactly one is required", frames)
			}
			if offset != len(data) {
				return fmt.Errorf("GIF has trailing data")
			}
			return nil
		case 0x21:
			if offset >= len(data) {
				return fmt.Errorf("truncated GIF extension")
			}
			offset++
			var err error
			offset, err = skipEmbeddedGIFSubBlocks(ctx, data, offset)
			if err != nil {
				return err
			}
		case 0x2c:
			frames++
			if frames > 1 {
				return fmt.Errorf("animated GIF is unsupported")
			}
			if len(data)-offset < 9 {
				return fmt.Errorf("truncated GIF image descriptor")
			}
			packed := data[offset+8]
			offset += 9
			if packed&0x80 != 0 {
				offset += 3 * (1 << ((packed & 0x07) + 1))
				if offset > len(data) {
					return fmt.Errorf("truncated GIF local color table")
				}
			}
			if offset >= len(data) {
				return fmt.Errorf("GIF is missing its LZW code size")
			}
			offset++
			var err error
			offset, err = skipEmbeddedGIFSubBlocks(ctx, data, offset)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected GIF block marker 0x%02x", marker)
		}
	}
}

func skipEmbeddedGIFSubBlocks(ctx context.Context, data []byte, offset int) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if offset >= len(data) {
			return 0, fmt.Errorf("truncated GIF sub-blocks")
		}
		size := int(data[offset])
		offset++
		if size == 0 {
			return offset, nil
		}
		if size > len(data)-offset {
			return 0, fmt.Errorf("truncated GIF sub-block data")
		}
		offset += size
	}
}

func inspectEmbeddedWebP(ctx context.Context, data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("truncated WebP RIFF header")
	}
	declaredEnd := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declaredEnd != uint64(len(data)) || declaredEnd < 12 {
		return fmt.Errorf("WebP has invalid RIFF length or trailing data")
	}
	imageChunks := 0
	for offset := 12; uint64(offset) < declaredEnd; {
		if err := ctx.Err(); err != nil {
			return err
		}
		if declaredEnd-uint64(offset) < 8 {
			return fmt.Errorf("truncated WebP chunk header")
		}
		length := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		paddedLength := length + length%2
		chunkEnd := uint64(offset) + 8 + paddedLength
		if chunkEnd > declaredEnd {
			return fmt.Errorf("truncated WebP chunk data")
		}
		chunkType := string(data[offset : offset+4])
		if chunkType == "ANIM" || chunkType == "ANMF" {
			return fmt.Errorf("animated WebP is unsupported")
		}
		if chunkType == "VP8X" && length >= 1 && data[offset+8]&0x02 != 0 {
			return fmt.Errorf("animated WebP is unsupported")
		}
		if chunkType == "VP8 " || chunkType == "VP8L" {
			imageChunks++
			if imageChunks > 1 {
				return fmt.Errorf("WebP contains multiple static image bitstreams")
			}
		}
		offset = int(chunkEnd)
	}
	if imageChunks != 1 {
		return fmt.Errorf("WebP must contain exactly one static image bitstream")
	}
	return nil
}
