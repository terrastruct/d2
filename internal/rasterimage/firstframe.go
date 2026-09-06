// Package rasterimage decodes a deterministic still image from supported
// raster formats. For animated inputs, the returned image is the first
// animation frame composited onto the format's logical canvas.
package rasterimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"

	_ "golang.org/x/image/webp"
)

// Config validates the bounded container structure and reports the logical
// canvas decoded by DecodeFirst. JPEG dimensions include EXIF Orientation.
// Animated is true when the input contains an animation rather than a still.
func Config(ctx context.Context, data []byte, format string) (config image.Config, animated bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := contextError(ctx); err != nil {
		return image.Config{}, false, err
	}
	switch format {
	case "gif":
		animated, err = inspectGIF(ctx, data)
		if err != nil {
			return image.Config{}, false, err
		}
		config, err = gif.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
	case "png":
		plan, planErr := inspectPNG(ctx, data)
		if planErr != nil {
			return image.Config{}, false, planErr
		}
		animated = plan.animated
		config, _, err = image.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
	case "webp":
		plan, planErr := inspectWebP(ctx, data, false)
		if planErr != nil {
			return image.Config{}, false, planErr
		}
		animated = plan.animated
		if !animated {
			config, _, err = image.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
			break
		}
		config = image.Config{ColorModel: color.NRGBAModel, Width: plan.canvas.Dx(), Height: plan.canvas.Dy()}
	case "jpeg":
		orientation, orientationErr := jpegEXIFOrientation(ctx, data)
		if orientationErr != nil {
			return image.Config{}, false, orientationErr
		}
		config, _, err = image.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err == nil {
			config = orientedConfig(config, orientation)
		}
	default:
		config, _, err = image.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
	}
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return image.Config{}, false, contextErr
		}
		return image.Config{}, false, err
	}
	if config.Width <= 0 || config.Height <= 0 {
		return image.Config{}, false, fmt.Errorf("invalid decoded dimensions %dx%d", config.Width, config.Height)
	}
	return config, animated, nil
}

// DecodeFirst returns the first animation frame on the logical canvas and
// applies JPEG EXIF Orientation. It validates container framing but does not
// decode later animation frames.
func DecodeFirst(ctx context.Context, data []byte, format string) (image.Image, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	switch format {
	case "gif":
		if _, err := inspectGIF(ctx, data); err != nil {
			return nil, err
		}
		config, err := gif.DecodeConfig(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		decoded, err := gif.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		canvas := image.Rect(0, 0, config.Width, config.Height)
		if decoded.Bounds() == canvas {
			return decoded, nil
		}
		if decoded.Bounds().Empty() || !decoded.Bounds().In(canvas) {
			return nil, fmt.Errorf("GIF first-frame bounds %v fall outside logical canvas %v", decoded.Bounds(), canvas)
		}
		return logicalImage{Image: decoded, bounds: canvas}, nil
	case "png":
		plan, err := inspectPNG(ctx, data)
		if err != nil {
			return nil, err
		}
		if !plan.animated || plan.pngFrameUsesIDAT {
			decoded, err := png.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
			if err != nil {
				return nil, decodeError(ctx, err)
			}
			return decoded, nil
		}
		firstFrame, err := buildFramePNG(ctx, data, plan)
		if err != nil {
			return nil, err
		}
		decoded, err := png.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(firstFrame)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		return compositeFrame(ctx, plan.canvas, plan.frame, decoded, plan.sourceBlend)
	case "webp":
		plan, err := inspectWebP(ctx, data, true)
		if err != nil {
			return nil, err
		}
		input := data
		if plan.animated {
			input = plan.firstFrame
		}
		decoded, _, err := image.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(input)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		if !plan.animated {
			return decoded, nil
		}
		return compositeFrame(ctx, plan.canvas, plan.frame, decoded, plan.sourceBlend)
	case "jpeg":
		orientation, err := jpegEXIFOrientation(ctx, data)
		if err != nil {
			return nil, err
		}
		decoded, _, err := image.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		return orientImage(decoded, orientation), nil
	default:
		decoded, _, err := image.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err != nil {
			return nil, decodeError(ctx, err)
		}
		return decoded, nil
	}
}

func decodeError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx); contextErr != nil {
		return contextErr
	}
	return err
}

type logicalImage struct {
	image.Image
	bounds image.Rectangle
}

func (img logicalImage) Bounds() image.Rectangle { return img.bounds }
func (img logicalImage) At(x, y int) color.Color {
	if !(image.Point{X: x, Y: y}).In(img.Image.Bounds()) {
		return color.NRGBA{}
	}
	return img.Image.At(x, y)
}

func compositeFrame(ctx context.Context, canvas, frame image.Rectangle, decoded image.Image, sourceBlend bool) (image.Image, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if frame == canvas && decoded.Bounds().Dx() == canvas.Dx() && decoded.Bounds().Dy() == canvas.Dy() {
		return decoded, nil
	}
	result := image.NewNRGBA(canvas)
	op := draw.Over
	if sourceBlend {
		op = draw.Src
	}
	const pixelsPerCancellationCheck = 64 << 10
	rowsPerCancellationCheck := pixelsPerCancellationCheck / max(1, frame.Dx())
	if rowsPerCancellationCheck < 1 {
		rowsPerCancellationCheck = 1
	} else if rowsPerCancellationCheck > 64 {
		rowsPerCancellationCheck = 64
	}
	for y := frame.Min.Y; y < frame.Max.Y; y += rowsPerCancellationCheck {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		endY := y + rowsPerCancellationCheck
		if endY > frame.Max.Y {
			endY = frame.Max.Y
		}
		destination := image.Rect(frame.Min.X, y, frame.Max.X, endY)
		source := decoded.Bounds().Min.Add(image.Pt(0, y-frame.Min.Y))
		draw.Draw(result, destination, decoded, source, op)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func inspectGIF(ctx context.Context, data []byte) (bool, error) {
	if len(data) < 13 {
		return false, errors.New("truncated logical screen descriptor")
	}
	offset := 13
	if data[10]&0x80 != 0 {
		offset += 3 * (1 << ((data[10] & 0x07) + 1))
		if offset > len(data) {
			return false, errors.New("truncated global color table")
		}
	}
	frames := 0
	for {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if offset >= len(data) {
			return false, errors.New("missing GIF trailer")
		}
		marker := data[offset]
		offset++
		switch marker {
		case 0x3b:
			if frames == 0 {
				return false, errors.New("GIF has no image frame")
			}
			return frames > 1, nil
		case 0x21:
			if offset >= len(data) {
				return false, errors.New("truncated GIF extension")
			}
			offset++
			var err error
			offset, err = skipGIFSubBlocks(ctx, data, offset)
			if err != nil {
				return false, err
			}
		case 0x2c:
			frames++
			if len(data)-offset < 9 {
				return false, errors.New("truncated GIF image descriptor")
			}
			packed := data[offset+8]
			offset += 9
			if packed&0x80 != 0 {
				offset += 3 * (1 << ((packed & 0x07) + 1))
				if offset > len(data) {
					return false, errors.New("truncated local color table")
				}
			}
			if offset >= len(data) {
				return false, errors.New("missing GIF LZW code size")
			}
			offset++
			var err error
			offset, err = skipGIFSubBlocks(ctx, data, offset)
			if err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unexpected GIF block marker 0x%02x", marker)
		}
	}
}

func skipGIFSubBlocks(ctx context.Context, data []byte, offset int) (int, error) {
	for {
		if err := contextError(ctx); err != nil {
			return 0, err
		}
		if offset >= len(data) {
			return 0, errors.New("truncated GIF sub-blocks")
		}
		size := int(data[offset])
		offset++
		if size == 0 {
			return offset, nil
		}
		if size > len(data)-offset {
			return 0, errors.New("truncated GIF sub-block data")
		}
		offset += size
	}
}

type animationPlan struct {
	animated         bool
	firstFrameSeen   bool
	canvas           image.Rectangle
	frame            image.Rectangle
	firstFrame       []byte
	sourceBlend      bool
	pngPrefixBytes   int
	pngFrameBytes    int
	pngIHDR          [13]byte
	pngFrameUsesIDAT bool
}

const (
	pngChunkIHDR = uint32('I')<<24 | uint32('H')<<16 | uint32('D')<<8 | uint32('R')
	pngChunkAcTL = uint32('a')<<24 | uint32('c')<<16 | uint32('T')<<8 | uint32('L')
	pngChunkFcTL = uint32('f')<<24 | uint32('c')<<16 | uint32('T')<<8 | uint32('L')
	pngChunkIDAT = uint32('I')<<24 | uint32('D')<<16 | uint32('A')<<8 | uint32('T')
	pngChunkFdAT = uint32('f')<<24 | uint32('d')<<16 | uint32('A')<<8 | uint32('T')
	pngChunkIEND = uint32('I')<<24 | uint32('E')<<16 | uint32('N')<<8 | uint32('D')
)

func inspectPNG(ctx context.Context, data []byte) (animationPlan, error) {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return animationPlan{}, errors.New("invalid PNG signature")
	}
	var plan animationPlan
	var firstFrameHeader []byte
	var declaredFrames uint32
	var actualFrames uint64
	var nextSequence uint64
	sawIHDR := false
	sawIDAT := false
	sawAnimationControl := false
	currentFrame := false
	currentFrameUsesIDAT := false
	currentFrameHasData := false
	collectingFirstFrame := false
	offset := 8
	for {
		if err := contextError(ctx); err != nil {
			return animationPlan{}, err
		}
		if len(data)-offset < 12 {
			return animationPlan{}, errors.New("truncated PNG chunk")
		}
		length := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkBytes := length + 12
		if chunkBytes > uint64(len(data)-offset) {
			return animationPlan{}, errors.New("truncated PNG chunk data")
		}
		chunk := data[offset : offset+int(chunkBytes)]
		chunkType := binary.BigEndian.Uint32(chunk[4:8])
		payload := chunk[8 : 8+int(length)]
		validChecksum, err := validPNGChunkChecksum(ctx, chunk)
		if err != nil {
			return animationPlan{}, err
		}
		if !validChecksum {
			return animationPlan{}, fmt.Errorf("invalid PNG %s checksum", chunk[4:8])
		}
		switch chunkType {
		case pngChunkIHDR:
			if sawIHDR || offset != 8 || len(payload) != 13 {
				return animationPlan{}, errors.New("invalid PNG IHDR")
			}
			sawIHDR = true
			copy(plan.pngIHDR[:], payload)
			width := binary.BigEndian.Uint32(payload[0:4])
			height := binary.BigEndian.Uint32(payload[4:8])
			if width == 0 || height == 0 || uint64(width) > uint64(math.MaxInt) || uint64(height) > uint64(math.MaxInt) {
				return animationPlan{}, errors.New("invalid PNG canvas dimensions")
			}
			plan.canvas = image.Rect(0, 0, int(width), int(height))
		case pngChunkAcTL:
			if !sawIHDR || sawAnimationControl || sawIDAT || len(payload) != 8 || binary.BigEndian.Uint32(payload[:4]) == 0 {
				return animationPlan{}, errors.New("invalid APNG animation control chunk")
			}
			sawAnimationControl = true
			plan.animated = true
			declaredFrames = binary.BigEndian.Uint32(payload[:4])
			if actualFrames > uint64(declaredFrames) {
				return animationPlan{}, errors.New("APNG frame count exceeds acTL declaration")
			}
		case pngChunkFcTL:
			// APNG permits the default image's fcTL on either side of acTL,
			// provided both precede IDAT. No later frame may begin before acTL.
			if !sawIHDR || len(payload) != 26 || (!sawAnimationControl && (sawIDAT || actualFrames != 0)) {
				return animationPlan{}, errors.New("invalid APNG frame control chunk")
			}
			if currentFrame && !currentFrameHasData {
				return animationPlan{}, errors.New("APNG frame has no data")
			}
			if err := consumeAPNGSequence(payload[:4], &nextSequence); err != nil {
				return animationPlan{}, err
			}
			actualFrames++
			if sawAnimationControl && actualFrames > uint64(declaredFrames) {
				return animationPlan{}, errors.New("APNG frame count exceeds acTL declaration")
			}
			frame, sourceBlend, err := apngFrame(payload, plan.canvas)
			if err != nil {
				return animationPlan{}, err
			}
			currentFrame = true
			currentFrameHasData = false
			currentFrameUsesIDAT = actualFrames == 1 && !sawIDAT
			collectingFirstFrame = actualFrames == 1
			if collectingFirstFrame {
				firstFrameHeader = payload
				plan.firstFrameSeen = true
				plan.frame = frame
				plan.sourceBlend = sourceBlend
				plan.pngFrameUsesIDAT = currentFrameUsesIDAT
				if currentFrameUsesIDAT && frame != plan.canvas {
					return animationPlan{}, errors.New("APNG IDAT first frame does not cover its logical canvas")
				}
			}
		case pngChunkIDAT:
			if !sawIHDR {
				return animationPlan{}, errors.New("PNG IDAT precedes IHDR")
			}
			if currentFrame && !sawAnimationControl {
				return animationPlan{}, errors.New("APNG IDAT data precedes animation control")
			}
			if currentFrame && !currentFrameUsesIDAT {
				return animationPlan{}, errors.New("APNG IDAT data follows a non-IDAT frame control")
			}
			sawIDAT = true
			if currentFrameUsesIDAT && len(payload) != 0 {
				currentFrameHasData = true
			}
		case pngChunkFdAT:
			if !sawAnimationControl || !currentFrame {
				return animationPlan{}, errors.New("APNG frame data precedes frame control")
			}
			if len(payload) < 4 || currentFrameUsesIDAT {
				return animationPlan{}, errors.New("invalid APNG frame data chunk")
			}
			if err := consumeAPNGSequence(payload[:4], &nextSequence); err != nil {
				return animationPlan{}, err
			}
			if len(payload) > 4 {
				currentFrameHasData = true
			}
			if collectingFirstFrame {
				plan.pngFrameBytes, err = checkedAddInt(plan.pngFrameBytes, len(payload)-4, "APNG first-frame data bytes")
				if err != nil {
					return animationPlan{}, err
				}
			}
		case pngChunkIEND:
			if len(payload) != 0 {
				return animationPlan{}, errors.New("invalid PNG IEND")
			}
			if !sawIHDR {
				return animationPlan{}, errors.New("PNG is missing IHDR")
			}
			if actualFrames != 0 && !sawAnimationControl {
				return animationPlan{}, errors.New("APNG is missing an animation control chunk")
			}
			if !plan.animated {
				return plan, nil
			}
			if firstFrameHeader == nil {
				return animationPlan{}, errors.New("APNG is missing a first frame control chunk")
			}
			if !currentFrameHasData {
				return animationPlan{}, errors.New("APNG frame has no data")
			}
			if actualFrames != uint64(declaredFrames) {
				return animationPlan{}, fmt.Errorf("APNG contains %d frames, acTL declares %d", actualFrames, declaredFrames)
			}
			if !plan.pngFrameUsesIDAT && plan.pngFrameBytes == 0 {
				return animationPlan{}, errors.New("APNG first frame has no data")
			}
			return plan, nil
		default:
			if !sawIDAT {
				plan.pngPrefixBytes, err = checkedAddInt(plan.pngPrefixBytes, len(chunk), "APNG prefix bytes")
				if err != nil {
					return animationPlan{}, err
				}
			}
		}
		offset += int(chunkBytes)
	}
}

func apngFrame(header []byte, canvas image.Rectangle) (image.Rectangle, bool, error) {
	width := binary.BigEndian.Uint32(header[4:8])
	height := binary.BigEndian.Uint32(header[8:12])
	x := binary.BigEndian.Uint32(header[12:16])
	y := binary.BigEndian.Uint32(header[16:20])
	if width == 0 || height == 0 || uint64(x)+uint64(width) > uint64(canvas.Dx()) || uint64(y)+uint64(height) > uint64(canvas.Dy()) {
		return image.Rectangle{}, false, errors.New("APNG frame falls outside its logical canvas")
	}
	if header[24] > 2 || header[25] > 1 {
		return image.Rectangle{}, false, errors.New("APNG frame has invalid dispose or blend operation")
	}
	return image.Rect(int(x), int(y), int(x+width), int(y+height)), header[25] == 0, nil
}

// buildFramePNG makes a still PNG for an APNG whose default image is not part
// of the animation. inspectPNG is the first pass: it validates the complete
// container and computes the exact amount of prefix and first-frame data. This
// second pass uses one allocation and never retains a slice per input chunk.
func buildFramePNG(ctx context.Context, data []byte, plan animationPlan) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if !plan.animated || !plan.firstFrameSeen || plan.pngFrameUsesIDAT || plan.frame.Empty() {
		return nil, errors.New("invalid APNG first-frame materialization plan")
	}
	total, err := firstFramePNGSize(plan.pngPrefixBytes, plan.pngFrameBytes)
	if err != nil {
		return nil, err
	}
	if plan.pngPrefixBytes > len(data) || plan.pngFrameBytes > len(data) {
		return nil, errors.New("APNG first-frame materialization plan exceeds input")
	}

	result := make([]byte, 0, total)
	result, err = appendContext(ctx, result, []byte("\x89PNG\r\n\x1a\n"))
	if err != nil {
		return nil, err
	}
	frameIHDR := plan.pngIHDR
	binary.BigEndian.PutUint32(frameIHDR[0:4], uint32(plan.frame.Dx()))
	binary.BigEndian.PutUint32(frameIHDR[4:8], uint32(plan.frame.Dy()))
	result = appendPNGChunk(result, "IHDR", frameIHDR[:])

	sawIDAT := false
	collectingFirstFrame := false
	firstFrameStarted := false
	copiedFrameBytes := 0
	var idatChecksum uint32
	offset := 8
	for offset < len(data) {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if len(data)-offset < 12 {
			return nil, errors.New("truncated PNG chunk during first-frame materialization")
		}
		length := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkBytes := length + 12
		if chunkBytes > uint64(len(data)-offset) {
			return nil, errors.New("truncated PNG chunk data during first-frame materialization")
		}
		chunk := data[offset : offset+int(chunkBytes)]
		chunkType := binary.BigEndian.Uint32(chunk[4:8])
		payload := chunk[8 : 8+int(length)]
		switch chunkType {
		case pngChunkIHDR, pngChunkAcTL:
		case pngChunkIDAT:
			sawIDAT = true
		case pngChunkFcTL:
			if !sawIDAT {
				return nil, errors.New("APNG first-frame materialization unexpectedly uses IDAT")
			}
			if firstFrameStarted {
				// inspectPNG already validated the remaining frames. The second
				// pass only needs to materialize the first one.
				offset = len(data)
				continue
			} else {
				firstFrameStarted = true
				collectingFirstFrame = true
				if cap(result)-len(result) < 8 {
					return nil, errors.New("APNG first-frame output size overflow")
				}
				result = binary.BigEndian.AppendUint32(result, uint32(plan.pngFrameBytes))
				result = append(result, "IDAT"...)
				idatChecksum = crc32.Update(0, crc32.IEEETable, []byte("IDAT"))
			}
		case pngChunkFdAT:
			if collectingFirstFrame {
				if len(payload) < 4 {
					return nil, errors.New("invalid APNG frame data during first-frame materialization")
				}
				framePayload := payload[4:]
				copiedFrameBytes, err = checkedAddInt(copiedFrameBytes, len(framePayload), "APNG copied first-frame data bytes")
				if err != nil {
					return nil, err
				}
				if copiedFrameBytes > plan.pngFrameBytes {
					return nil, errors.New("APNG first-frame data exceeds materialization plan")
				}
				result, idatChecksum, err = appendAndChecksumContext(ctx, result, framePayload, idatChecksum)
				if err != nil {
					return nil, err
				}
			}
		case pngChunkIEND:
			offset = len(data)
			continue
		default:
			if !sawIDAT {
				result, err = appendContext(ctx, result, chunk)
				if err != nil {
					return nil, err
				}
			}
		}
		offset += int(chunkBytes)
	}
	if !firstFrameStarted || copiedFrameBytes != plan.pngFrameBytes {
		return nil, fmt.Errorf("APNG copied %d first-frame bytes, expected %d", copiedFrameBytes, plan.pngFrameBytes)
	}
	if cap(result)-len(result) < 4 {
		return nil, errors.New("APNG first-frame output size overflow")
	}
	result = binary.BigEndian.AppendUint32(result, idatChecksum)
	result = appendPNGChunk(result, "IEND", nil)
	if len(result) != total || cap(result) != total {
		return nil, fmt.Errorf("APNG first-frame output is %d bytes, expected %d", len(result), total)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func firstFramePNGSize(prefixBytes, frameBytes int) (int, error) {
	if prefixBytes < 0 || frameBytes < 0 || uint64(frameBytes) > uint64(math.MaxUint32) {
		return 0, errors.New("APNG first-frame output size overflow")
	}
	const fixedBytes = 8 + 25 + 12 + 12 // signature, IHDR, IDAT framing, IEND
	total := uint64(fixedBytes) + uint64(prefixBytes) + uint64(frameBytes)
	if total > uint64(math.MaxInt) {
		return 0, errors.New("APNG first-frame output size overflow")
	}
	return int(total), nil
}

func appendContext(ctx context.Context, destination, source []byte) ([]byte, error) {
	if len(source) > cap(destination)-len(destination) {
		return nil, errors.New("APNG first-frame output size overflow")
	}
	start := len(destination)
	destination = destination[:start+len(source)]
	const copyChunkBytes = 32 << 10
	for copied := 0; copied < len(source); {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		end := copied + copyChunkBytes
		if end > len(source) {
			end = len(source)
		}
		copy(destination[start+copied:start+end], source[copied:end])
		copied = end
	}
	return destination, contextError(ctx)
}

func appendAndChecksumContext(ctx context.Context, destination, source []byte, checksum uint32) ([]byte, uint32, error) {
	if len(source) > cap(destination)-len(destination) {
		return nil, 0, errors.New("APNG first-frame output size overflow")
	}
	start := len(destination)
	destination = destination[:start+len(source)]
	const copyChunkBytes = 32 << 10
	for copied := 0; copied < len(source); {
		if err := contextError(ctx); err != nil {
			return nil, 0, err
		}
		end := copied + copyChunkBytes
		if end > len(source) {
			end = len(source)
		}
		part := source[copied:end]
		copy(destination[start+copied:start+end], part)
		checksum = crc32.Update(checksum, crc32.IEEETable, part)
		copied = end
	}
	if err := contextError(ctx); err != nil {
		return nil, 0, err
	}
	return destination, checksum, nil
}

func validPNGChunkChecksum(ctx context.Context, chunk []byte) (bool, error) {
	if len(chunk) < 12 {
		return false, errors.New("truncated PNG chunk")
	}
	checksum := uint32(0)
	content := chunk[4 : len(chunk)-4]
	const checksumChunkBytes = 32 << 10
	for offset := 0; offset < len(content); {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		end := offset + checksumChunkBytes
		if end > len(content) {
			end = len(content)
		}
		checksum = crc32.Update(checksum, crc32.IEEETable, content[offset:end])
		offset = end
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	return checksum == binary.BigEndian.Uint32(chunk[len(chunk)-4:]), nil
}

func consumeAPNGSequence(encoded []byte, next *uint64) error {
	if len(encoded) != 4 {
		return errors.New("invalid APNG sequence number")
	}
	sequence := uint64(binary.BigEndian.Uint32(encoded))
	if sequence != *next {
		return fmt.Errorf("APNG sequence number is %d, expected %d", sequence, *next)
	}
	(*next)++
	return nil
}

func checkedAddInt(value, addition int, label string) (int, error) {
	if value < 0 || addition < 0 || value > math.MaxInt-addition {
		return 0, fmt.Errorf("%s overflow", label)
	}
	return value + addition, nil
}

func appendPNGChunk(destination []byte, chunkType string, payload []byte) []byte {
	start := len(destination)
	destination = binary.BigEndian.AppendUint32(destination, uint32(len(payload)))
	destination = append(destination, chunkType...)
	destination = append(destination, payload...)
	checksum := crc32.ChecksumIEEE(destination[start+4:])
	return binary.BigEndian.AppendUint32(destination, checksum)
}

func inspectWebP(ctx context.Context, data []byte, materialize bool) (animationPlan, error) {
	if len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return animationPlan{}, errors.New("invalid WebP RIFF header")
	}
	declaredEnd := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declaredEnd != uint64(len(data)) || declaredEnd < 12 {
		return animationPlan{}, errors.New("invalid WebP RIFF length")
	}
	var plan animationPlan
	var sawVP8X, animationFlag bool
	animChunks := 0
	animPayloadLength := -1
	var firstANMF []byte
	sawANMF := false
	imageChunks := 0
	for offset := 12; uint64(offset) < declaredEnd; {
		if err := contextError(ctx); err != nil {
			return animationPlan{}, err
		}
		chunkType, payload, next, err := webPChunk(data, offset, int(declaredEnd))
		if err != nil {
			return animationPlan{}, err
		}
		switch chunkType {
		case "VP8X":
			if sawVP8X || len(payload) != 10 {
				return animationPlan{}, errors.New("invalid WebP VP8X chunk")
			}
			sawVP8X = true
			animationFlag = payload[0]&0x02 != 0
			width := readUint24(payload[4:7]) + 1
			height := readUint24(payload[7:10]) + 1
			if width <= 0 || height <= 0 {
				return animationPlan{}, errors.New("invalid WebP canvas dimensions")
			}
			plan.canvas = image.Rect(0, 0, width, height)
		case "ANIM":
			animChunks++
			if animChunks == 1 {
				animPayloadLength = len(payload)
			}
		case "ANMF":
			if !sawANMF {
				firstANMF = payload
				sawANMF = true
			}
		case "VP8 ", "VP8L":
			imageChunks++
		}
		offset = next
	}
	if animationFlag {
		if !sawVP8X || animChunks != 1 || animPayloadLength != 6 || !sawANMF {
			return animationPlan{}, errors.New("incomplete animated WebP container")
		}
		frame, sourceBlend, encoded, frameErr := extractWebPFrame(ctx, firstANMF, materialize)
		if frameErr != nil {
			return animationPlan{}, frameErr
		}
		plan.animated = true
		plan.firstFrameSeen = true
		plan.frame = frame
		plan.sourceBlend = sourceBlend
		plan.firstFrame = encoded
		if plan.frame.Min.X < 0 || plan.frame.Min.Y < 0 || plan.frame.Max.X > plan.canvas.Dx() || plan.frame.Max.Y > plan.canvas.Dy() {
			return animationPlan{}, errors.New("WebP first frame falls outside its logical canvas")
		}
		return plan, nil
	}
	if imageChunks != 1 {
		return animationPlan{}, fmt.Errorf("expected exactly one static WebP image bitstream, got %d", imageChunks)
	}
	return plan, nil
}

func webPChunk(data []byte, offset, end int) (string, []byte, int, error) {
	if end-offset < 8 {
		return "", nil, 0, errors.New("truncated WebP chunk header")
	}
	length := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
	padded := length + length%2
	chunkEnd := uint64(offset) + 8 + padded
	if chunkEnd > uint64(end) {
		return "", nil, 0, errors.New("truncated WebP chunk data")
	}
	return string(data[offset : offset+4]), data[offset+8 : offset+8+int(length)], int(chunkEnd), nil
}

func extractWebPFrame(ctx context.Context, payload []byte, materialize bool) (image.Rectangle, bool, []byte, error) {
	if len(payload) < 16 {
		return image.Rectangle{}, false, nil, errors.New("truncated WebP ANMF header")
	}
	x := readUint24(payload[0:3]) * 2
	y := readUint24(payload[3:6]) * 2
	width := readUint24(payload[6:9]) + 1
	height := readUint24(payload[9:12]) + 1
	if width <= 0 || height <= 0 || x > math.MaxInt-width || y > math.MaxInt-height {
		return image.Rectangle{}, false, nil, errors.New("invalid WebP first-frame dimensions")
	}
	frame := image.Rect(x, y, x+width, y+height)
	sourceBlend := payload[15]&0x02 != 0
	var alphaChunk, imageChunk []byte
	var imagePayload []byte
	imageType := ""
	for offset := 16; offset < len(payload); {
		if err := contextError(ctx); err != nil {
			return image.Rectangle{}, false, nil, err
		}
		chunkType, chunkPayload, next, err := webPChunk(payload, offset, len(payload))
		if err != nil {
			return image.Rectangle{}, false, nil, err
		}
		switch chunkType {
		case "ALPH":
			if alphaChunk != nil {
				return image.Rectangle{}, false, nil, errors.New("multiple WebP ALPH chunks in first frame")
			}
			if materialize {
				alphaChunk = appendWebPChunk(nil, chunkType, chunkPayload)
			} else {
				alphaChunk = chunkPayload
			}
		case "VP8 ", "VP8L":
			if imageChunk != nil {
				return image.Rectangle{}, false, nil, errors.New("multiple WebP bitstreams in first frame")
			}
			imageType = chunkType
			imagePayload = chunkPayload
			if materialize {
				imageChunk = appendWebPChunk(nil, chunkType, chunkPayload)
			} else {
				imageChunk = chunkPayload
			}
		default:
			// The WebP container permits future or application-specific chunks.
			// webPChunk has already validated and skipped their padded extent.
		}
		offset = next
	}
	if imageChunk == nil || (alphaChunk != nil && imageType != "VP8 ") {
		return image.Rectangle{}, false, nil, errors.New("invalid WebP first-frame bitstream")
	}
	bitstreamWidth, bitstreamHeight, err := webPBitstreamDimensions(imageType, imagePayload)
	if err != nil {
		return image.Rectangle{}, false, nil, err
	}
	if bitstreamWidth != width || bitstreamHeight != height {
		return image.Rectangle{}, false, nil, fmt.Errorf("WebP first-frame header is %dx%d but its bitstream is %dx%d", width, height, bitstreamWidth, bitstreamHeight)
	}
	if !materialize {
		return frame, sourceBlend, nil, nil
	}
	chunks := make([]byte, 0, len(alphaChunk)+len(imageChunk)+18)
	if alphaChunk != nil {
		vp8x := make([]byte, 10)
		vp8x[0] = 0x10
		writeUint24(vp8x[4:7], width-1)
		writeUint24(vp8x[7:10], height-1)
		chunks = appendWebPChunk(chunks, "VP8X", vp8x)
		chunks = append(chunks, alphaChunk...)
	}
	chunks = append(chunks, imageChunk...)
	result := make([]byte, 12, 12+len(chunks))
	copy(result[:4], "RIFF")
	copy(result[8:12], "WEBP")
	result = append(result, chunks...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return frame, sourceBlend, result, nil
}

func webPBitstreamDimensions(format string, payload []byte) (int, int, error) {
	switch format {
	case "VP8 ":
		if len(payload) < 10 || !bytes.Equal(payload[3:6], []byte{0x9d, 0x01, 0x2a}) {
			return 0, 0, errors.New("invalid WebP VP8 first-frame header")
		}
		return int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff), int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff), nil
	case "VP8L":
		if len(payload) < 5 || payload[0] != 0x2f {
			return 0, 0, errors.New("invalid WebP VP8L first-frame header")
		}
		bits := binary.LittleEndian.Uint32(payload[1:5])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	default:
		return 0, 0, errors.New("missing WebP first-frame bitstream")
	}
}

func appendWebPChunk(destination []byte, chunkType string, payload []byte) []byte {
	destination = append(destination, chunkType...)
	destination = binary.LittleEndian.AppendUint32(destination, uint32(len(payload)))
	destination = append(destination, payload...)
	if len(payload)%2 != 0 {
		destination = append(destination, 0)
	}
	return destination
}

func readUint24(data []byte) int {
	return int(data[0]) | int(data[1])<<8 | int(data[2])<<16
}

func writeUint24(destination []byte, value int) {
	destination[0] = byte(value)
	destination[1] = byte(value >> 8)
	destination[2] = byte(value >> 16)
}

type contextReader struct {
	ctx    context.Context
	reader *bytes.Reader
}

func (reader contextReader) Read(destination []byte) (int, error) {
	if err := contextError(reader.ctx); err != nil {
		return 0, err
	}
	if len(destination) > 32<<10 {
		destination = destination[:32<<10]
	}
	n, err := reader.reader.Read(destination)
	if contextErr := contextError(reader.ctx); contextErr != nil {
		return n, contextErr
	}
	return n, err
}

var _ io.Reader = contextReader{}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	default:
		return ctx.Err()
	}
}
