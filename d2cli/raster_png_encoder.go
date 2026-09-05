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
)

// rasterPNGEncoder retains compression and row workspaces for the lifetime of
// one multi-board export. D2's native raster frames have an opaque background,
// so the common path writes the same 8-bit truecolor stream as image/png
// without rescanning alpha or allocating each filter row separately. If an
// NRGBA caller violates that invariant, encode detects it while copying the
// pixels, discards the partial attempt, and falls back to the generic encoder.
type rasterPNGEncoder struct {
	native        rasterOpaquePNGEncoder
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
}

func (e *rasterOpaquePNGEncoder) encode(output io.Writer, source *image.NRGBA) error {
	bounds := source.Bounds()
	rowSize := 1 + 3*bounds.Dx()
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
	if _, err := io.WriteString(output, "\x89PNG\r\n\x1a\n"); err != nil {
		return err
	}
	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(bounds.Dx()))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(bounds.Dy()))
	ihdr[8] = 8
	ihdr[9] = 2
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

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		sourceOffset := (y - bounds.Min.Y) * source.Stride
		sourceRow := source.Pix[sourceOffset : sourceOffset+bounds.Dx()*4]
		destination := e.raw[1:]
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
		row := rasterPNGFilteredRow(e.raw, e.prior, &e.candidates)
		if _, err := e.zw.Write(row); err != nil {
			return err
		}
		e.prior, e.raw = e.raw, e.prior
		e.raw[0] = 0
	}
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
	if _, w.err = w.output.Write(w.frame[:8]); w.err != nil {
		return
	}
	if _, w.err = w.output.Write(data); w.err != nil {
		return
	}
	_, w.err = w.output.Write(w.frame[8:])
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
func rasterPNGFilteredRow(rawRow, priorRow []byte, candidates *[2][]byte) []byte {
	raw := rawRow[1:]
	prior := priorRow[1:]
	winner, scratch := candidates[0], candidates[1]
	best := rasterPNGUpFilter(winner, raw, prior)
	selected := byte(2) // Up wins ties, followed by Paeth, None, Sub, Average.
	if cost := rasterPNGPaethFilter(scratch, raw, prior, best); cost < best {
		best, selected = cost, 4
		winner, scratch = scratch, winner
	}
	if cost := rasterPNGNoneCost(raw, best); cost < best {
		best, selected = cost, 0
	}
	if cost := rasterPNGSubFilter(scratch, raw, best); cost < best {
		best, selected = cost, 1
		winner, scratch = scratch, winner
	}
	if cost := rasterPNGAverageFilter(scratch, raw, prior, best); cost < best {
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

func rasterPNGSubFilter(destination, raw []byte, stopAt int) int {
	destination[0] = 1
	destination = destination[1:]
	cost := 0
	for index := 0; index < 3; index++ {
		destination[index] = raw[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := 3; index < len(raw); index++ {
		destination[index] = raw[index] - raw[index-3]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGAverageFilter(destination, raw, prior []byte, stopAt int) int {
	_ = prior[len(raw)-1]
	destination[0] = 3
	destination = destination[1:]
	cost := 0
	for index := 0; index < 3; index++ {
		destination[index] = raw[index] - prior[index]/2
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := 3; index < len(raw); index++ {
		destination[index] = raw[index] - byte((int(raw[index-3])+int(prior[index]))/2)
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func rasterPNGPaethFilter(destination, raw, prior []byte, stopAt int) int {
	_ = prior[len(raw)-1]
	destination[0] = 4
	destination = destination[1:]
	cost := 0
	for index := 0; index < 3; index++ {
		destination[index] = raw[index] - prior[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := 3; index < len(raw); index++ {
		left := raw[index-3]
		upperLeft := prior[index-3]
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
