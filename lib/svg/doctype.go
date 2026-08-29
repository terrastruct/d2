package svg

import "bytes"

const (
	maxSupportedExternalDoctypeBytes = 256
	maxExternalDoctypePrologBytes    = 16 << 10
)

var (
	svg10PublicID = []byte("-//W3C//DTD SVG 1.0//EN")
	svg10SystemID = []byte("http://www.w3.org/TR/2001/REC-SVG-20010904/DTD/svg10.dtd")
	svg11PublicID = []byte("-//W3C//DTD SVG 1.1//EN")
	svg11SystemID = []byte("http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd")
)

// IsSupportedExternalDoctype reports whether directive is the encoding/xml
// token for one canonical W3C SVG 1.0 or SVG 1.1 external DOCTYPE declaration
// in document's raw prolog. Requiring both inputs prevents encoding/xml's
// directive normalization from widening the accepted raw syntax. It accepts no
// internal subset and performs no DTD resolution. Callers remain responsible
// for enforcing that the declaration occurs at most once and before the root.
func IsSupportedExternalDoctype(document, directive []byte) bool {
	if _, ok := parseSupportedExternalDoctype(directive, false); !ok {
		return false
	}
	return hasSupportedExternalDoctypeInProlog(document)
}

func hasSupportedExternalDoctypeInProlog(document []byte) bool {
	limit := len(document)
	if limit > maxExternalDoctypePrologBytes {
		limit = maxExternalDoctypePrologBytes
	}
	prolog := document[:limit]
	offset := 0
	if bytes.HasPrefix(prolog, []byte{0xef, 0xbb, 0xbf}) {
		offset = 3
	}
	for offset < len(prolog) {
		for offset < len(prolog) && isXMLSpace(prolog[offset]) {
			offset++
		}
		switch {
		case bytes.HasPrefix(prolog[offset:], []byte("<!--")):
			end := bytes.Index(prolog[offset+4:], []byte("-->"))
			if end < 0 {
				return false
			}
			offset += 4 + end + 3
		case bytes.HasPrefix(prolog[offset:], []byte("<?")):
			end := bytes.Index(prolog[offset+2:], []byte("?>"))
			if end < 0 {
				return false
			}
			offset += 2 + end + 2
		case bytes.HasPrefix(prolog[offset:], []byte("<!DOCTYPE")):
			_, ok := parseSupportedExternalDoctype(document[offset:], true)
			return ok
		default:
			return false
		}
	}
	return false
}

func parseSupportedExternalDoctype(input []byte, raw bool) (int, bool) {
	if !raw && len(input) > maxSupportedExternalDoctypeBytes {
		return 0, false
	}
	limit := len(input)
	if limit > maxSupportedExternalDoctypeBytes+1 {
		limit = maxSupportedExternalDoctypeBytes + 1
	}
	parser := externalDoctypeParser{input: input[:limit]}
	if raw && !parser.consume("<!") {
		return 0, false
	}
	if !parser.consume("DOCTYPE") || !parser.skipRequiredSpace() || !parser.consume("svg") ||
		!parser.skipRequiredSpace() || !parser.consume("PUBLIC") || !parser.skipRequiredSpace() {
		return 0, false
	}
	publicID, ok := parser.quoted()
	if !ok || !parser.skipRequiredSpace() {
		return 0, false
	}
	systemID, ok := parser.quoted()
	if !ok {
		return 0, false
	}
	parser.skipSpace()
	if raw {
		if !parser.consume(">") || parser.offset > maxSupportedExternalDoctypeBytes {
			return 0, false
		}
	} else if parser.offset != len(parser.input) {
		return 0, false
	}
	supported := bytes.Equal(publicID, svg10PublicID) && bytes.Equal(systemID, svg10SystemID) ||
		bytes.Equal(publicID, svg11PublicID) && bytes.Equal(systemID, svg11SystemID)
	return parser.offset, supported
}

type externalDoctypeParser struct {
	input  []byte
	offset int
}

func (p *externalDoctypeParser) consume(value string) bool {
	if !bytes.HasPrefix(p.input[p.offset:], []byte(value)) {
		return false
	}
	p.offset += len(value)
	return true
}

func (p *externalDoctypeParser) skipRequiredSpace() bool {
	start := p.offset
	p.skipSpace()
	return p.offset != start
}

func (p *externalDoctypeParser) skipSpace() {
	for p.offset < len(p.input) && isXMLSpace(p.input[p.offset]) {
		p.offset++
	}
}

func (p *externalDoctypeParser) quoted() ([]byte, bool) {
	if p.offset >= len(p.input) || p.input[p.offset] != '\'' && p.input[p.offset] != '"' {
		return nil, false
	}
	quote := p.input[p.offset]
	p.offset++
	start := p.offset
	for p.offset < len(p.input) && p.input[p.offset] != quote {
		p.offset++
	}
	if p.offset >= len(p.input) {
		return nil, false
	}
	value := p.input[start:p.offset]
	p.offset++
	return value, true
}

func isXMLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
