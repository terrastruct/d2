package svg

import (
	"bytes"
	"encoding/xml"
)

func EscapeText(text string) string {
	buf := new(bytes.Buffer)
	_ = xml.EscapeText(buf, []byte(text))
	return buf.String()
}
