package urlenc

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"io"
	"strings"

	"oss.terrastruct.com/util-go/xdefer"
)

// Encode takes a D2 script and encodes it as a compressed base64 string for embedding in URLs.
func Encode(raw string) (_ string, err error) {
	defer xdefer.Errorf(&err, "failed to encode d2 script")

	b := &bytes.Buffer{}

	zw, err := flate.NewWriterDict(b, flate.BestCompression, nil)
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

// Decode decodes a compressed base64 D2 string.
func Decode(encoded string) (_ string, err error) {
	defer xdefer.Errorf(&err, "failed to decode d2 script")

	b64Decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	zr := flate.NewReaderDict(bytes.NewReader(b64Decoded), nil)
	var b bytes.Buffer
	if _, err := io.Copy(&b, zr); err != nil {
		return "", err
	}
	if err := zr.Close(); err != nil {
		return "", nil
	}
	return b.String(), nil
}
