package d2cli

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	stdpng "image/png"
	"io"
	"math"

	d2png "github.com/d2lang/d2/lib/png"
)

// rasterPNGEncoder retains compression and row workspaces for the lifetime of
// one multi-board export. D2's native raster frames have an opaque background,
// so the common path writes the same 8-bit truecolor stream as image/png
// without rescanning alpha or allocating each filter row separately. If an
// NRGBA caller violates that invariant, encode detects it while copying the
// pixels, discards the partial attempt, and falls back to the generic encoder.
type rasterPNGEncoder struct {
	native        rasterOpaquePNGEncoder
	bands         rasterPNGBandEncoder
	generic       rasterPNGEncoderBufferPool
	genericWriter rasterPNGContextWriter
	genericImage  image.NRGBA
}

func (e *rasterPNGEncoder) encode(ctx context.Context, img image.Image) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("d2raster: cannot encode a nil image")
	}
	bounds := img.Bounds()
	nrgba, native := img.(*image.NRGBA)
	native = native && bounds.Dx() > 0 && bounds.Dy() > 0 && bounds.Dx() <= rasterMaxDimension
	if !native {
		e.native = rasterOpaquePNGEncoder{}
		return e.encodeGeneric(ctx, img)
	}
	// Transparent D2 canvases normally expose that fact in their first pixel.
	// Route them before constructing a second compression pipeline that would
	// immediately be discarded. The specialized encoder still validates every
	// pixel, so this is only an early exit for the common transparent case.
	if len(nrgba.Pix) < 4 || nrgba.Pix[3] != 0xff {
		e.native = rasterOpaquePNGEncoder{}
		return e.encodeGeneric(ctx, img)
	}
	var out bytes.Buffer
	writer := rasterPNGContextWriter{ctx: ctx, output: &out}
	err := e.native.encode(&writer, nrgba)
	e.native.releaseOutput()
	if errors.Is(err, errRasterPNGTranslucent) {
		e.native = rasterOpaquePNGEncoder{}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return e.encodeGeneric(ctx, img)
	}
	if err != nil {
		e.close()
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("d2raster: encode PNG: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (e *rasterPNGEncoder) encodeGeneric(ctx context.Context, img image.Image) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("d2raster: cannot encode a nil image")
	}
	var out bytes.Buffer
	e.genericWriter.ctx = ctx
	e.genericWriter.output = &out
	encoder := stdpng.Encoder{CompressionLevel: stdpng.BestSpeed}
	encodedImage := img
	if nrgba, ok := img.(*image.NRGBA); ok {
		bounds := nrgba.Bounds()
		if bounds.Dx() > 0 && bounds.Dy() > 0 && bounds.Dx() <= rasterMaxDimension && bounds.Dy() <= rasterMaxDimension {
			// image/png retains both its writer and image inside the opaque
			// EncoderBuffer. Stable operation-owned headers let the pool keep its
			// compression scratch without pinning either call's output or frame.
			e.genericImage = *nrgba
			encodedImage = &e.genericImage
			encoder.BufferPool = &e.generic
		}
	}
	err := encoder.Encode(&e.genericWriter, encodedImage)
	e.genericWriter = rasterPNGContextWriter{}
	e.genericImage = image.NRGBA{}
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("d2raster: encode PNG: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (e *rasterPNGEncoder) close() {
	*e = rasterPNGEncoder{}
}

type rasterPNGEncoderBufferPool struct {
	buffer *stdpng.EncoderBuffer
}

func (p *rasterPNGEncoderBufferPool) Get() *stdpng.EncoderBuffer {
	buffer := p.buffer
	p.buffer = nil
	return buffer
}

func (p *rasterPNGEncoderBufferPool) Put(buffer *stdpng.EncoderBuffer) {
	p.buffer = buffer
}

var errRasterPNGTranslucent = errors.New("translucent NRGBA frame")

type rasterOpaquePNGEncoder struct {
	storage    []byte
	raw        []byte
	prior      []byte
	candidates [2][]byte
	zw         *zlib.Writer
	bw         *bufio.Writer
	stream     rasterPNGIDATStream
	channels   int
}

func (e *rasterOpaquePNGEncoder) encode(output io.Writer, source *image.NRGBA) error {
	bounds := source.Bounds()
	if err := e.start(output, bounds.Dx(), bounds.Dy(), true); err != nil {
		return err
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		sourceOffset := (y - bounds.Min.Y) * source.Stride
		sourceRow := source.Pix[sourceOffset : sourceOffset+bounds.Dx()*4]
		if err := e.writeRow(sourceRow); err != nil {
			return err
		}
	}
	return e.finish()
}

func (e *rasterOpaquePNGEncoder) start(output io.Writer, width, height int, opaque bool) error {
	e.channels = 4
	if opaque {
		e.channels = 3
	}
	rowSize := 1 + e.channels*width
	storageSize := 4 * rowSize
	if cap(e.storage) < storageSize {
		e.storage = make([]byte, storageSize)
	} else {
		e.storage = e.storage[:storageSize]
	}
	e.raw = e.storage[:rowSize]
	e.prior = e.storage[rowSize : 2*rowSize]
	e.candidates[0] = e.storage[2*rowSize : 3*rowSize]
	e.candidates[1] = e.storage[3*rowSize:]
	e.raw[0] = 0
	clear(e.prior)

	e.stream.output = output
	e.stream.err = nil
	if n, err := io.WriteString(output, "\x89PNG\r\n\x1a\n"); err != nil {
		return err
	} else if n != 8 {
		return io.ErrShortWrite
	}
	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8
	ihdr[9] = 2
	if !opaque {
		ihdr[9] = 6
	}
	e.stream.emit(0x49484452, ihdr[:]) // IHDR
	if e.stream.err != nil {
		return e.stream.err
	}
	if e.bw == nil {
		e.bw = bufio.NewWriterSize(&e.stream, 1<<15)
	} else {
		e.bw.Reset(&e.stream)
	}
	if e.zw == nil {
		zw, err := zlib.NewWriterLevel(e.bw, zlib.BestSpeed)
		if err != nil {
			return err
		}
		e.zw = zw
	} else {
		e.zw.Reset(e.bw)
	}
	return nil
}

func (e *rasterOpaquePNGEncoder) writeRow(sourceRow []byte) error {
	destination := e.raw[1:]
	if e.channels == 4 {
		copy(destination, sourceRow)
	} else {
		for len(sourceRow) >= 16 {
			if sourceRow[3]&sourceRow[7]&sourceRow[11]&sourceRow[15] != 0xff {
				return errRasterPNGTranslucent
			}
			destination[0], destination[1], destination[2] = sourceRow[0], sourceRow[1], sourceRow[2]
			destination[3], destination[4], destination[5] = sourceRow[4], sourceRow[5], sourceRow[6]
			destination[6], destination[7], destination[8] = sourceRow[8], sourceRow[9], sourceRow[10]
			destination[9], destination[10], destination[11] = sourceRow[12], sourceRow[13], sourceRow[14]
			sourceRow = sourceRow[16:]
			destination = destination[12:]
		}
		for len(sourceRow) >= 4 {
			if sourceRow[3] != 0xff {
				return errRasterPNGTranslucent
			}
			destination[0], destination[1], destination[2] = sourceRow[0], sourceRow[1], sourceRow[2]
			sourceRow = sourceRow[4:]
			destination = destination[3:]
		}
	}
	row := rasterPNGFilteredRow(e.raw, e.prior, &e.candidates, e.channels)
	if _, err := e.zw.Write(row); err != nil {
		return err
	}
	e.prior, e.raw = e.raw, e.prior
	e.raw[0] = 0
	return nil
}

func (e *rasterOpaquePNGEncoder) finish() error {
	if err := e.zw.Close(); err != nil {
		return err
	}
	if err := e.bw.Flush(); err != nil {
		return err
	}
	e.stream.emit(0x49454e44, nil) // IEND
	return e.stream.err
}

// releaseOutput severs the retained writer chain from zlib through bufio and
// the chunk writer. The reusable compression and row storage remain owned by
// the operation, but the caller's frame and encoded bytes do not.
func (e *rasterOpaquePNGEncoder) releaseOutput() {
	e.stream.output = nil
}

// rasterPNGBandEncoder writes a PNG as consecutive full-width bands arrive.
// Its retained memory depends on the row width, never the image height. The
// caller may reuse a band's pixel storage as soon as append returns.
//
// Opaque mode must be established before start: the stream cannot fall back
// to RGBA after an RGB header has been written. Unknown backgrounds use RGBA.
type rasterPNGBandEncoder struct {
	native rasterOpaquePNGEncoder
	writer rasterPNGContextWriter
	width  int
	height int
	nextY  int
	active bool
	err    error
}

func (e *rasterPNGBandEncoder) start(ctx context.Context, output io.Writer, width, height int, opaque bool) error {
	if e.active {
		return fmt.Errorf("d2raster: PNG stream already started")
	}
	e.close()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if output == nil {
		return fmt.Errorf("d2raster: cannot write PNG to a nil writer")
	}
	// PNG dimensions are limited to 31 bits. Four working RGBA rows also
	// have to fit in an int before their storage size can be computed.
	if width <= 0 || height <= 0 || uint64(width) > math.MaxInt32 || uint64(height) > math.MaxInt32 || width > (math.MaxInt/4-1)/4 {
		return fmt.Errorf("d2raster: invalid PNG dimensions %dx%d", width, height)
	}
	e.writer = rasterPNGContextWriter{ctx: ctx, output: output}
	e.width, e.height, e.active = width, height, true
	if e.err = e.native.start(&e.writer, width, height, opaque); e.err != nil {
		return e.err
	}
	e.err = d2png.WriteExif(&e.writer)
	return e.err
}

func (e *rasterPNGBandEncoder) append(band *image.NRGBA) error {
	if e.err != nil {
		return e.err
	}
	if !e.active {
		return fmt.Errorf("d2raster: PNG stream is not started")
	}
	if e.err = e.writer.ctx.Err(); e.err != nil {
		return e.err
	}
	if band == nil {
		e.err = fmt.Errorf("d2raster: cannot encode a nil PNG band")
		return e.err
	}
	bounds := band.Bounds()
	if bounds.Min.X != 0 || bounds.Max.X != e.width || bounds.Min.Y != e.nextY || bounds.Max.Y <= bounds.Min.Y || bounds.Max.Y > e.height {
		e.err = fmt.Errorf("d2raster: PNG band %v must have width %d and begin at row %d within height %d", bounds, e.width, e.nextY, e.height)
		return e.err
	}
	rowBytes := 4 * e.width
	// Divide before multiplying so hostile stride/dimension values cannot
	// overflow and turn malformed image storage into an indexing panic.
	if band.Stride < rowBytes || len(band.Pix) < rowBytes || bounds.Dy()-1 > (len(band.Pix)-rowBytes)/band.Stride {
		e.err = fmt.Errorf("d2raster: PNG band has invalid pixel storage")
		return e.err
	}
	for row := 0; row < bounds.Dy(); row++ {
		if e.err = e.writer.ctx.Err(); e.err != nil {
			return e.err
		}
		offset := row * band.Stride
		if e.err = e.native.writeRow(band.Pix[offset : offset+rowBytes]); e.err != nil {
			return e.err
		}
		e.nextY++
	}
	e.err = e.writer.ctx.Err()
	return e.err
}

func (e *rasterPNGBandEncoder) finish() error {
	if e.err != nil {
		return e.err
	}
	if !e.active {
		return fmt.Errorf("d2raster: PNG stream is not started")
	}
	if e.nextY != e.height {
		e.err = fmt.Errorf("d2raster: PNG stream has %d rows, want %d", e.nextY, e.height)
		return e.err
	}
	if e.err = e.writer.ctx.Err(); e.err != nil {
		return e.err
	}
	e.err = e.native.finish()
	if e.err == nil {
		e.err = e.writer.ctx.Err()
	}
	e.active = false
	return e.err
}

// close drops the output writer, context, and band state while retaining row
// and compression workspaces for the next board in this export operation.
func (e *rasterPNGBandEncoder) close() {
	e.native.releaseOutput()
	*e = rasterPNGBandEncoder{native: e.native}
}

type rasterPNGIDATStream struct {
	output io.Writer
	err    error
	frame  [12]byte
}

func (w *rasterPNGIDATStream) Write(data []byte) (int, error) {
	w.emit(0x49444154, data) // IDAT
	if w.err != nil {
		return 0, w.err
	}
	return len(data), nil
}

func (w *rasterPNGIDATStream) emit(kind uint32, data []byte) {
	if w.err != nil {
		return
	}
	binary.BigEndian.PutUint32(w.frame[0:4], uint32(len(data)))
	binary.BigEndian.PutUint32(w.frame[4:8], kind)
	checksum := crc32.Update(0, crc32.IEEETable, w.frame[4:8])
	checksum = crc32.Update(checksum, crc32.IEEETable, data)
	binary.BigEndian.PutUint32(w.frame[8:12], checksum)
	if w.err = rasterPNGWriteAll(w.output, w.frame[:8]); w.err != nil {
		return
	}
	if w.err = rasterPNGWriteAll(w.output, data); w.err != nil {
		return
	}
	w.err = rasterPNGWriteAll(w.output, w.frame[8:])
}

func rasterPNGWriteAll(output io.Writer, data []byte) error {
	n, err := output.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func rasterPNGAbs8(value byte) int {
	signed := int(int8(value))
	mask := signed >> (rasterPNGIntSize - 1)
	return (signed ^ mask) - mask
}

const rasterPNGIntSize = 32 << (^uint(0) >> 63)

// rasterPNGFilteredRow preserves image/png's strict filter priority with two
// alternating candidate rows. Each candidate is scored as it is materialized;
// the winning row stays untouched while the other buffer is reused. Together
// with raw and prior rows, this retains four rows instead of six.
func rasterPNGFilteredRow(rawRow, priorRow []byte, candidates *[2][]byte, channels int) []byte {
	raw := rawRow[1:]
	prior := priorRow[1:]
	winner, scratch := candidates[0], candidates[1]
	best := rasterPNGUpFilter(winner, raw, prior)
	selected := byte(2) // Up wins ties, followed by Paeth, None, Sub, Average.
	if cost := rasterPNGPaethFilter(scratch, raw, prior, best, channels); cost < best {
		best, selected = cost, 4
		winner, scratch = scratch, winner
	}
	if cost := rasterPNGNoneCost(raw, best); cost < best {
		best, selected = cost, 0
	}
	if cost := rasterPNGSubFilter(scratch, raw, best, channels); cost < best {
		best, selected = cost, 1
		winner, scratch = scratch, winner
	}
	if cost := rasterPNGAverageFilter(scratch, raw, prior, best, channels); cost < best {
		selected = 3
		winner = scratch
	}
	if selected == 0 {
		return rawRow
	}
	return winner
}

func rasterPNGNoneCost(raw []byte, stopAt int) int {
	cost := 0
	for _, value := range raw {
		cost += rasterPNGAbs8(value)
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGUpFilter(destination, raw, prior []byte) int {
	_ = prior[len(raw)-1]
	destination[0] = 2
	destination = destination[1:]
	cost := 0
	for index, value := range raw {
		destination[index] = value - prior[index]
		cost += rasterPNGAbs8(destination[index])
	}
	return cost
}

func rasterPNGSubFilter(destination, raw []byte, stopAt, channels int) int {
	destination[0] = 1
	destination = destination[1:]
	cost := 0
	for index := 0; index < channels; index++ {
		destination[index] = raw[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := channels; index < len(raw); index++ {
		destination[index] = raw[index] - raw[index-channels]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGAverageFilter(destination, raw, prior []byte, stopAt, channels int) int {
	_ = prior[len(raw)-1]
	destination[0] = 3
	destination = destination[1:]
	cost := 0
	for index := 0; index < channels; index++ {
		destination[index] = raw[index] - prior[index]/2
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := channels; index < len(raw); index++ {
		destination[index] = raw[index] - byte((int(raw[index-channels])+int(prior[index]))/2)
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGPaethFilter(destination, raw, prior []byte, stopAt, channels int) int {
	_ = prior[len(raw)-1]
	destination[0] = 4
	destination = destination[1:]
	cost := 0
	for index := 0; index < channels; index++ {
		destination[index] = raw[index] - prior[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := channels; index < len(raw); index++ {
		left := raw[index-channels]
		upperLeft := prior[index-channels]
		upper := prior[index]
		leftDistance := rasterPNGByteDistance(upper, upperLeft)
		upperDistance := rasterPNGByteDistance(left, upperLeft)
		diagonalDistance := int(left) + int(upper) - 2*int(upperLeft)
		if diagonalDistance < 0 {
			diagonalDistance = -diagonalDistance
		}
		predictor := upperLeft
		if leftDistance <= upperDistance && leftDistance <= diagonalDistance {
			predictor = left
		} else if upperDistance <= diagonalDistance {
			predictor = upper
		}
		destination[index] = raw[index] - predictor
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGByteDistance(left, right byte) int {
	if left >= right {
		return int(left - right)
	}
	return int(right - left)
}

type rasterPNGContextWriter struct {
	ctx    context.Context
	output io.Writer
}

func (w rasterPNGContextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	written, err := w.output.Write(data)
	if err != nil {
		return written, err
	}
	if contextErr := w.ctx.Err(); contextErr != nil {
		return written, contextErr
	}
	return written, nil
}
