package xgif

import (
	"compress/lzw"
	"context"
	"fmt"
	"image"
	"image/color"
	"sync"
)

// encodeGIF writes validated, normalized frames as GIF89a. One compressor
// workspace is reset for each frame so its dictionary storage can be reused.
func encodeGIF(output *limitedBuffer, images []*image.Paletted, delays []int) error {
	compression := acquireGIFCompression(output)
	defer releaseGIFCompression(compression)
	encoder := gifAssembler{
		output: output, totalFrames: len(images), colorTables: gifLocalColorTables,
		compressor: &compression.writer, blocks: &compression.blocks,
	}
	for index, frame := range images {
		encoder.appendFrame(frame, delays[index])
	}
	encoder.appendByte(0x3b) // Trailer.
	encoder.sync()
	return encoder.err
}

// AnimateCenteredOpaquePalettedImagesWithLimit encodes fully opaque paletted
// frames on a shared white canvas without allocating a full canvas-sized pixel
// buffer for frames that need centering. Input frames may have non-zero origins
// and different dimensions, but none may exceed the requested canvas.
//
// The opaque contract permits the encoder to share one global color table when
// frame palettes match. Callers that need transparent GIF semantics must use
// AnimatePalettedImagesWithLimit, whose per-frame local color tables preserve
// transparent-frame metadata and deterministic byte compatibility.
func AnimateCenteredOpaquePalettedImagesWithLimit(ctx context.Context, images []*image.Paletted, width, height, intervalMs int, maxBytes int64) ([]byte, error) {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("GIF output byte limit must be positive")
	}
	if err := validateGIFCanvasDimensions(width, height); err != nil {
		return nil, err
	}
	if err := validateOpaqueGIFColor("background", BG_COLOR); err != nil {
		return nil, err
	}
	delays, err := gifFrameDelays(len(images), intervalMs)
	if err != nil {
		return nil, err
	}
	for index, frame := range images {
		if err := validatePalettedImage(ctx, frame); err != nil {
			return nil, fmt.Errorf("GIF frame %d: %w", index, err)
		}
		if err := validateOpaqueGIFPalette(frame.Palette); err != nil {
			return nil, fmt.Errorf("GIF frame %d: %w", index, err)
		}
		bounds := frame.Bounds()
		if bounds.Dx() > width || bounds.Dy() > height {
			return nil, fmt.Errorf("GIF frame %d dimensions %dx%d exceed %dx%d canvas", index, bounds.Dx(), bounds.Dy(), width, height)
		}
	}

	output := limitedBuffer{ctx: ctx, maxBytes: maxBytes}
	compression := acquireGIFCompression(&output)
	defer releaseGIFCompression(compression)
	encoder := gifAssembler{
		output: &output, totalFrames: len(images), colorTables: gifOpaqueGlobalColorTable,
		compressor: &compression.writer, blocks: &compression.blocks,
	}
	canvas := image.Rect(0, 0, width, height)
	for index, frame := range images {
		if frame.Bounds() == canvas {
			logicalFrame := *frame
			logicalFrame.Palette, _ = encoder.copyPaletteWithBackground(frame.Palette)
			encoder.appendFrame(&logicalFrame, delays[index])
		} else {
			encoder.appendCenteredFrame(frame, width, height, delays[index])
		}
		if encoder.err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return nil, encoder.err
		}
	}
	encoder.appendByte(0x3b)
	encoder.sync()
	if encoder.err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, encoder.err
	}
	return output.Bytes(), nil
}

func validateOpaqueGIFColor(name string, value color.Color) error {
	if value == nil {
		return fmt.Errorf("GIF %s color is nil", name)
	}
	if _, _, _, alpha := value.RGBA(); alpha != 0xffff {
		return fmt.Errorf("GIF %s color must be fully opaque", name)
	}
	return nil
}

func validateOpaqueGIFPalette(palette color.Palette) error {
	for index, value := range palette {
		if value == nil {
			return fmt.Errorf("GIF palette color %d is nil", index)
		}
		if _, _, _, alpha := value.RGBA(); alpha != 0xffff {
			return fmt.Errorf("GIF palette color %d must be fully opaque", index)
		}
	}
	return nil
}

func validateGIFCanvasDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("GIF canvas dimensions must be positive")
	}
	if width >= 1<<16 || height >= 1<<16 {
		return fmt.Errorf("GIF canvas dimensions must be smaller than 65536")
	}
	return nil
}

type gifPaletteMode uint8

const (
	gifLocalColorTables gifPaletteMode = iota
	gifOpaqueGlobalColorTable
)

type gifAssembler struct {
	output      *limitedBuffer
	totalFrames int
	framesWrote int
	colorTables gifPaletteMode
	err         error

	globalColorTable [3 * 256]byte
	localColorTable  [3 * 256]byte
	globalTableBytes int
	compressor       *lzw.Writer
	blocks           *gifSubblocks
	canvasRow        []byte
	palette          [256]color.Color
}

func (g *gifAssembler) append(data []byte) {
	if g.err == nil {
		_, failure := g.output.Write(data)
		g.capture(failure)
	}
}

func (g *gifAssembler) appendByte(value byte) {
	if g.err == nil {
		g.capture(g.output.WriteByte(value))
	}
}

func (g *gifAssembler) sync() {
	if g.err == nil {
		g.capture(g.output.Flush())
	}
}

func (g *gifAssembler) capture(failure error) {
	if g.err == nil && failure != nil {
		g.err = failure
	}
}

// gifPaletteLayout returns the packed-field exponent and padded entry count
// for GIF's power-of-two color-table representation.
func gifPaletteLayout(length int) (sizeCode, entries int) {
	entries = 2
	for entries < length {
		entries <<= 1
		sizeCode++
	}
	return sizeCode, entries
}

func gifRGB(value color.Color) uint32 {
	if rgba, ok := value.(color.RGBA); ok {
		return uint32(rgba.R)<<16 | uint32(rgba.G)<<8 | uint32(rgba.B)
	}
	r, g, b, _ := value.RGBA()
	return uint32(byte(r>>8))<<16 | uint32(byte(g>>8))<<8 | uint32(byte(b>>8))
}

func marshalGIFPalette(destination []byte, palette color.Palette, paddedEntries int) []byte {
	for index, value := range palette {
		rgb := gifRGB(value)
		offset := index * 3
		destination[offset] = byte(rgb >> 16)
		destination[offset+1] = byte(rgb >> 8)
		destination[offset+2] = byte(rgb)
	}
	encodedLength := 3 * paddedEntries
	if encodedLength > 3*len(palette) {
		clear(destination[3*len(palette) : encodedLength])
	}
	return destination[:encodedLength]
}

func putGIFUint16(destination []byte, value int) {
	destination[0] = byte(value)
	destination[1] = byte(value >> 8)
}

func (g *gifAssembler) start(first *image.Paletted) {
	var logicalScreen [13]byte
	copy(logicalScreen[:6], "GIF89a")
	putGIFUint16(logicalScreen[6:8], first.Rect.Dx())
	putGIFUint16(logicalScreen[8:10], first.Rect.Dy())
	if g.colorTables == gifOpaqueGlobalColorTable {
		sizeCode, entries := gifPaletteLayout(len(first.Palette))
		logicalScreen[10] = 0x80 | byte(sizeCode)
		encoded := marshalGIFPalette(g.globalColorTable[:], first.Palette, entries)
		g.globalTableBytes = len(encoded)
	}
	g.append(logicalScreen[:])
	if g.globalTableBytes != 0 {
		g.append(g.globalColorTable[:g.globalTableBytes])
	}

	if g.totalFrames > 1 {
		g.append([]byte{
			0x21, 0xff, 0x0b,
			'N', 'E', 'T', 'S', 'C', 'A', 'P', 'E', '2', '.', '0',
			0x03, 0x01, 0x00, 0x00, 0x00,
		})
	}
}

func firstTransparentPaletteIndex(palette color.Palette) int {
	for index, value := range palette {
		if _, _, _, alpha := value.RGBA(); alpha == 0 {
			return index
		}
	}
	return -1
}

func (g *gifAssembler) localTableMatchesGlobal(localLength int) bool {
	for offset := 0; offset < localLength*3; offset++ {
		if g.globalColorTable[offset] != g.localColorTable[offset] {
			return false
		}
	}
	return true
}

func (g *gifAssembler) appendFrame(frame *image.Paletted, delay int) {
	if g.err != nil {
		return
	}
	if g.framesWrote == 0 {
		g.start(frame)
	}
	g.openFrame(frame, delay)
	if g.err != nil {
		return
	}
	bounds := frame.Bounds()
	width := bounds.Dx()
	if width == frame.Stride {
		g.compressPixels(frame.Pix[:width*bounds.Dy()])
	} else {
		for row := 0; row < bounds.Dy() && g.err == nil; row++ {
			start := row * frame.Stride
			g.compressPixels(frame.Pix[start : start+width])
		}
	}
	g.closeFrame()
}

func (g *gifAssembler) openFrame(frame *image.Paletted, delay int) {
	transparentIndex := -1
	if g.colorTables == gifLocalColorTables {
		transparentIndex = firstTransparentPaletteIndex(frame.Palette)
	}
	if delay > 0 || transparentIndex >= 0 {
		control := [8]byte{0x21, 0xf9, 0x04}
		if transparentIndex >= 0 {
			control[3] = 0x01
			control[6] = byte(transparentIndex)
		}
		putGIFUint16(control[4:6], delay)
		g.append(control[:])
	}

	bounds := frame.Bounds()
	sizeCode, entries := gifPaletteLayout(len(frame.Palette))
	packed, localTable := byte(0), g.localColorTable[:0]
	if g.colorTables == gifLocalColorTables {
		packed = 0x80 | byte(sizeCode)
		localTable = marshalGIFPalette(g.localColorTable[:], frame.Palette, entries)
	} else if g.framesWrote != 0 {
		localTable = marshalGIFPalette(g.localColorTable[:], frame.Palette, entries)
		if len(localTable) <= g.globalTableBytes && g.localTableMatchesGlobal(len(frame.Palette)) {
			localTable = localTable[:0]
		} else {
			packed = 0x80 | byte(sizeCode)
		}
	}

	descriptor := [10]byte{0x2c}
	putGIFUint16(descriptor[5:7], bounds.Dx())
	putGIFUint16(descriptor[7:9], bounds.Dy())
	descriptor[9] = packed
	g.append(descriptor[:])
	if len(localTable) != 0 {
		g.append(localTable)
	}

	literalWidth := sizeCode + 1
	if literalWidth < 2 {
		literalWidth = 2
	}
	g.appendByte(byte(literalWidth))
	if g.err != nil {
		return
	}
	g.blocks.reset(g.output)
	g.compressor.Reset(g.blocks, lzw.LSB, literalWidth)
}

func (g *gifAssembler) closeFrame() {
	if closeErr := g.compressor.Close(); g.err == nil {
		g.err = closeErr
	}
	if g.err == nil {
		g.err = g.blocks.finish()
	}
	g.framesWrote++
}

func (g *gifAssembler) appendCenteredFrame(frame *image.Paletted, width, height, delay int) {
	palette, backgroundIndex := g.copyPaletteWithBackground(frame.Palette)
	logicalFrame := image.Paletted{Rect: image.Rect(0, 0, width, height), Stride: width, Palette: palette}
	if g.framesWrote == 0 {
		g.start(&logicalFrame)
	}
	g.openFrame(&logicalFrame, delay)
	if g.err != nil {
		return
	}
	if cap(g.canvasRow) < width {
		g.canvasRow = make([]byte, width)
	} else {
		g.canvasRow = g.canvasRow[:width]
	}
	fillBytes(g.canvasRow, byte(backgroundIndex))
	bounds := frame.Bounds()
	top := (height - bounds.Dy()) / 2
	left := (width - bounds.Dx()) / 2
	writeRow := func(rowIndex int) bool {
		if rowIndex&255 == 0 && g.output.ctx != nil {
			if err := g.output.ctx.Err(); err != nil {
				g.err = err
				return false
			}
		}
		g.compressPixels(g.canvasRow)
		return g.err == nil
	}
	rowIndex := 0
	for ; rowIndex < top && writeRow(rowIndex); rowIndex++ {
	}
	for sourceY := bounds.Min.Y; sourceY < bounds.Max.Y && g.err == nil; sourceY++ {
		sourceOffset := frame.PixOffset(bounds.Min.X, sourceY)
		copy(g.canvasRow[left:left+bounds.Dx()], frame.Pix[sourceOffset:sourceOffset+bounds.Dx()])
		if !writeRow(rowIndex) {
			break
		}
		rowIndex++
	}
	fillBytes(g.canvasRow[left:left+bounds.Dx()], byte(backgroundIndex))
	for ; rowIndex < height && g.err == nil && writeRow(rowIndex); rowIndex++ {
	}
	g.closeFrame()
}

func (g *gifAssembler) copyPaletteWithBackground(source color.Palette) (color.Palette, int) {
	length := copy(g.palette[:], source)
	if length == len(g.palette) {
		backgroundIndex := findWhiteIndex(g.palette[:])
		g.palette[backgroundIndex] = BG_COLOR
		return g.palette[:], backgroundIndex
	}
	backgroundIndex := length
	g.palette[length] = BG_COLOR
	return g.palette[:length+1], backgroundIndex
}

func fillBytes(destination []byte, value byte) {
	if len(destination) == 0 {
		return
	}
	if value == 0 {
		clear(destination)
		return
	}
	destination[0] = value
	for initialized := 1; initialized < len(destination); {
		initialized += copy(destination[initialized:], destination[:initialized])
	}
}

// OpaquePalettedAnimationEncoder incrementally encodes normalized, fully
// opaque paletted frames. It is useful when a renderer can release each frame
// before producing the next one. The opaque contract permits matching frames
// to share a global color table. An encoder is single-use and is not safe for
// concurrent calls.
type OpaquePalettedAnimationEncoder struct {
	ctx        context.Context
	width      int
	height     int
	total      int
	delays     gifFrameDelaySchedule
	next       int
	finished   bool
	err        error
	output     limitedBuffer
	blocks     gifSubblocks
	compressor lzw.Writer
	encoder    gifAssembler
}

// NewOpaquePalettedAnimationEncoder creates an incremental GIF encoder for an
// exact frame count and canvas. maxBytes bounds the encoded output before
// growth. WriteFrame rejects any palette entry that is not fully opaque.
func NewOpaquePalettedAnimationEncoder(ctx context.Context, width, height, totalFrames, intervalMs int, maxBytes int64) (*OpaquePalettedAnimationEncoder, error) {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("GIF output byte limit must be positive")
	}
	if err := validateGIFCanvasDimensions(width, height); err != nil {
		return nil, err
	}
	delays, err := newGIFFrameDelaySchedule(totalFrames, intervalMs)
	if err != nil {
		return nil, err
	}
	result := &OpaquePalettedAnimationEncoder{
		ctx: ctx, width: width, height: height, total: totalFrames, delays: delays,
	}
	result.output.ctx = ctx
	result.output.maxBytes = maxBytes
	result.encoder.output = &result.output
	result.encoder.totalFrames = totalFrames
	result.encoder.colorTables = gifOpaqueGlobalColorTable
	result.blocks.output = &result.output
	result.encoder.compressor = &result.compressor
	result.encoder.blocks = &result.blocks
	return result, nil
}

// WriteFrame validates and encodes the next frame. The frame is no longer
// retained when this method returns, except for the palette values copied into
// the GIF stream.
func (e *OpaquePalettedAnimationEncoder) WriteFrame(frame *image.Paletted) error {
	if e == nil {
		return fmt.Errorf("GIF encoder is nil")
	}
	if e.err != nil {
		return e.err
	}
	if e.finished {
		return fmt.Errorf("GIF encoder is already finished")
	}
	if e.next >= e.total {
		e.err = fmt.Errorf("GIF encoder received more than %d frames", e.total)
		return e.err
	}
	if err := validatePalettedImage(e.ctx, frame); err != nil {
		e.err = fmt.Errorf("GIF frame %d: %w", e.next, err)
		return e.err
	}
	if err := validateOpaqueGIFPalette(frame.Palette); err != nil {
		e.err = fmt.Errorf("GIF frame %d: %w", e.next, err)
		return e.err
	}
	if frame.Bounds() != image.Rect(0, 0, e.width, e.height) {
		e.err = fmt.Errorf("GIF frame %d dimensions %dx%d differ from %dx%d canvas", e.next, frame.Bounds().Dx(), frame.Bounds().Dy(), e.width, e.height)
		return e.err
	}
	e.encoder.appendFrame(frame, e.delays.next())
	if e.encoder.err != nil {
		e.err = e.encoder.err
		return e.err
	}
	e.next++
	return nil
}

// Finish closes the animation and returns its encoded bytes. It fails unless
// the configured number of frames has been written.
func (e *OpaquePalettedAnimationEncoder) Finish() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("GIF encoder is nil")
	}
	if e.err != nil {
		return nil, e.err
	}
	if e.finished {
		return nil, fmt.Errorf("GIF encoder is already finished")
	}
	if e.next != e.total {
		return nil, fmt.Errorf("GIF encoder received %d frames, want %d", e.next, e.total)
	}
	e.encoder.appendByte(0x3b)
	e.encoder.sync()
	e.finished = true
	if e.encoder.err != nil {
		e.err = e.encoder.err
		return nil, e.err
	}
	if err := e.ctx.Err(); err != nil {
		e.err = err
		return nil, err
	}
	return e.output.Bytes(), nil
}

func (g *gifAssembler) compressPixels(data []byte) {
	if g.err != nil || len(data) == 0 {
		return
	}
	_, g.err = g.compressor.Write(data)
}

type gifCompression struct {
	writer lzw.Writer
	blocks gifSubblocks
}

var gifCompressionPool = sync.Pool{
	New: func() any { return new(gifCompression) },
}

func acquireGIFCompression(output *limitedBuffer) *gifCompression {
	compression := gifCompressionPool.Get().(*gifCompression)
	compression.blocks.reset(output)
	return compression
}

func releaseGIFCompression(compression *gifCompression) {
	compression.blocks.output = nil
	gifCompressionPool.Put(compression)
}

// gifSubblocks groups the compressor's byte stream into GIF's length-prefixed
// data records, each of which can carry at most 255 bytes.
type gifSubblocks struct {
	output  *limitedBuffer
	payload [255]byte
	used    int
}

func (blocks *gifSubblocks) reset(output *limitedBuffer) {
	blocks.output = output
	blocks.used = 0
}

func (blocks *gifSubblocks) WriteByte(value byte) error {
	blocks.payload[blocks.used] = value
	blocks.used++
	if blocks.used == len(blocks.payload) {
		return blocks.emit(false)
	}
	return nil
}

func (blocks *gifSubblocks) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		count := min(len(data), len(blocks.payload)-blocks.used)
		copy(blocks.payload[blocks.used:], data[:count])
		blocks.used += count
		written += count
		data = data[count:]
		if blocks.used == len(blocks.payload) {
			if err := blocks.emit(false); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// Flush satisfies the compressor's flushable-writer contract. A partial GIF
// subblock stays buffered until finish can append its terminator atomically.
func (blocks *gifSubblocks) Flush() error {
	return blocks.output.Flush()
}

func (blocks *gifSubblocks) finish() error {
	return blocks.emit(true)
}

func (blocks *gifSubblocks) emit(final bool) error {
	output := blocks.output
	if output.ctx != nil {
		if err := output.ctx.Err(); err != nil {
			return err
		}
	}
	bytesNeeded := 1 + blocks.used
	if final && blocks.used != 0 {
		bytesNeeded++
	}
	if int64(bytesNeeded) > output.maxBytes-int64(output.Len()) {
		return fmt.Errorf("GIF output exceeds the %d-byte limit", output.maxBytes)
	}
	if blocks.used != 0 {
		_ = output.Buffer.WriteByte(byte(blocks.used))
		_, _ = output.Buffer.Write(blocks.payload[:blocks.used])
		blocks.used = 0
	}
	if final {
		_ = output.Buffer.WriteByte(0)
	}
	if output.ctx != nil {
		return output.ctx.Err()
	}
	return nil
}
