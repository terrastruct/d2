package compress

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"io"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
)

var compressionDict = "->" +
	"<-" +
	"--" +
	"<->"

var compressionDictBytes []byte

// Compress takes a D2 script and compresses it to a URL-encoded string
func Compress(raw string) (string, error) {
	var b bytes.Buffer

	zw, err := flate.NewWriterDict(&b, flate.DefaultCompression, []byte(compressionDict))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(zw, strings.NewReader(raw)); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	encoded := base64.URLEncoding.EncodeToString(b.Bytes())
	return encoded, nil
}

// Decompress takes a compressed, URL-encoded string and returns the decompressed D2 script
func Decompress(encoded string) (string, error) {
	b64Decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	zr := flate.NewReaderDict(bytes.NewReader(b64Decoded), []byte(compressionDict))
	var b bytes.Buffer
	if _, err := io.Copy(&b, zr); err != nil {
		return "", err
	}
	if err := zr.Close(); err != nil {
		return "", nil
	}
	return b.String(), nil
}

func init() {
	for k := range d2graph.StyleKeywords {
		compressionDict += k
	}
	for k := range d2graph.ReservedKeywords {
		compressionDict += k
	}
	for k := range d2graph.ReservedKeywordHolders {
		compressionDict += k
	}
}
