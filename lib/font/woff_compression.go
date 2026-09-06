package font

import (
	"bytes"
	"compress/zlib"
	"sync"
)

// Pool compressor workspace, never font inputs or completed WOFF output.
// Large custom-font tables must not increase the retained output buffer size.
const maxRetainedWoffBuffer = 64 << 10

type woffCompression struct {
	buffer bytes.Buffer
	writer *zlib.Writer
}

var woffCompressionPool = sync.Pool{
	New: func() any { return new(woffCompression) },
}

func acquireWoffCompression() *woffCompression {
	return woffCompressionPool.Get().(*woffCompression)
}

func releaseWoffCompression(compression *woffCompression) {
	resetWoffCompression(compression)
	woffCompressionPool.Put(compression)
}

func resetWoffCompression(compression *woffCompression) {
	if compression.buffer.Cap() > maxRetainedWoffBuffer {
		compression.buffer = bytes.Buffer{}
	} else {
		compression.buffer.Reset()
		clear(compression.buffer.Bytes()[:compression.buffer.Cap()])
	}
}

// compressTable starts an independent zlib stream, retaining only the working buffers.
// The returned bytes must be copied before another table or release.
func (compression *woffCompression) compressTable(data []byte) []byte {
	compression.buffer.Reset()
	if compression.writer == nil {
		compression.writer = zlib.NewWriter(&compression.buffer)
	} else {
		compression.writer.Reset(&compression.buffer)
	}
	compression.writer.Write(data)
	compression.writer.Flush()
	compression.writer.Close()
	return compression.buffer.Bytes()
}
