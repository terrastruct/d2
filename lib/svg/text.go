package svg

import (
	"bytes"
	"encoding/base32"
	"encoding/xml"
	"strings"
)

func EscapeText(text string) string {
	buf := new(bytes.Buffer)
	_ = xml.EscapeText(buf, []byte(text))
	return buf.String()
}

func SVGID(text string) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString([]byte(text)), "=")
}
