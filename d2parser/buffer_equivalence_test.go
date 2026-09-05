package d2parser_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2parser"
)

type parserBufferCase struct {
	name, source string
	readerLimit  int
	readerError  bool
}

func parserBufferCases() []parserBufferCase {
	return []parserBufferCase{
		{name: "empty"},
		{name: "ordinary", source: "first.child -> second: an ordinary label\nthird: 42\n"},
		{name: "raw_escape", source: "a: before\\t after\\n end\\ \\*\nb: \\a\\b\\f\\r\\t\\v\\\\\\\""},
		{name: "substitutions", source: "vars: {x: value}\na: before${x} after\\t${x} tail\nb: ${x}${x}\nc: $\n"},
		{name: "quoted", source: "a: \"before${x} after\\t${x} tail\"\nb: 'single\\quote'\n"},
		{name: "escaped_newlines", source: "first\\\n  second: before\\\n\t after\\t\\\n\n"},
		{name: "patterns", source: "*.**.***: value\na\\*.b*: text*\\*\n"},
		{name: "edge_groups", source: "(a -> b)[0].style: {stroke: red}\na-(name): value\n"},
		{name: "crlf_unicode", source: "α.😀: café\r\nβ: 中文\rnext: yes\n"},
		{name: "invalid_utf8", source: "a\xff.\xe2\x82: \xc0\xaf\nnext: \xf0\x9f\x98\n"},
		{name: "utf16_bom", source: "\xff\xfea\x00:\x00 \x00b\x00\r\x00\n\x00c\x00"},
		{name: "short_prefixes", source: "a\nbc\n...@file\nx: ...@file\ny: ...\nz: ..\n"},
		{name: "eof_escape", source: "a: tail\\"},
		{name: "eof_dash", source: "a-"},
		{name: "eof_quote", source: "a: \"unfinished\\"},
		{name: "comments_blocks", source: "# comment\na: |md\n  **bold**\n|\n\"\"\" block comment \"\"\"\n"},
		{name: "long_lookahead", source: "a" + strings.Repeat(" ", 2048) + ".b: label" + strings.Repeat("\t", 2048) + "\nnext: value"},
		{name: "whitespace_rewind", source: "a: \"value\"" + strings.Repeat(" ", 2048) + "\n" + strings.Repeat("\t", 2048) + "b: other"},
		{name: "delimiters", source: "a: [first; second; ${x}; last]\n# end\nb -> c: label; d: another\n"},
		{name: "reader_error_initial", source: "a: value", readerError: true},
		{name: "reader_error_prefix", source: "α: value", readerLimit: 3, readerError: true},
		{name: "reader_error_escape", source: "a: before\\tafter", readerLimit: 10, readerError: true},
	}
}

type parserOracleReader struct {
	data  string
	limit int
	fail  bool
	pos   int
	calls int
}

func (r *parserOracleReader) Read(b []byte) (int, error) {
	r.calls++
	if r.fail && r.pos == r.limit {
		return 0, errors.New("oracle read failure")
	}
	if r.pos == len(r.data) {
		return 0, io.EOF
	}
	// One-byte reads exercise partial UTF-8/BOM input and make read-ahead
	// consumption observable independently of the parser's rune positions.
	b[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

type parserBufferObservation struct {
	Mode       string
	AST        any
	Error      string
	Structured *d2parser.ParseError
	Panic      string
	ReadCalls  int
	ReadBytes  int
}

func parserBufferObservations(tc parserBufferCase) []parserBufferObservation {
	var observations []parserBufferObservation
	for _, utf16 := range []bool{false, true} {
		reader := &parserOracleReader{data: tc.source, limit: tc.readerLimit, fail: tc.readerError}
		observation := parserBufferObservation{Mode: fmt.Sprintf("parse/utf16=%v", utf16)}
		func() {
			defer func() {
				if value := recover(); value != nil {
					observation.Panic = fmt.Sprint(value)
				}
			}()
			ast, err := d2parser.Parse("oracle.d2", reader, &d2parser.ParseOptions{UTF16Pos: utf16})
			observation.AST = ast
			if err != nil {
				observation.Error = err.Error()
				errors.As(err, &observation.Structured)
			}
		}()
		observation.ReadCalls, observation.ReadBytes = reader.calls, reader.pos
		observations = append(observations, observation)
	}
	if !tc.readerError {
		for _, mode := range []string{"key", "map-key", "value"} {
			observation := parserBufferObservation{Mode: mode}
			func() {
				defer func() {
					if value := recover(); value != nil {
						observation.Panic = fmt.Sprint(value)
					}
				}()
				var err error
				switch mode {
				case "key":
					observation.AST, err = d2parser.ParseKey(tc.source)
				case "map-key":
					observation.AST, err = d2parser.ParseMapKey(tc.source)
				case "value":
					observation.AST, err = d2parser.ParseValue(tc.source)
				}
				if err != nil {
					observation.Error = err.Error()
					errors.As(err, &observation.Structured)
				}
			}()
			observations = append(observations, observation)
		}
	}
	return observations
}

func TestParserBufferEquivalence(t *testing.T) {
	for _, tc := range parserBufferCases() {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(parserBufferObservations(tc))
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(data))
			if got != parserBufferLegacyDigests[tc.name] {
				t.Fatalf("AST/errors/positions/reader trace changed for %q: digest %s\n%s", tc.source, got, data)
			}
		})
	}
}

func TestUnquotedRawStringOwnership(t *testing.T) {
	for _, source := range []string{"ordinary", `escaped\ttext`, `before${variable} after`, `before\t${variable} after`} {
		value, err := d2parser.ParseValue(source)
		if err != nil {
			t.Fatal(err)
		}
		unquoted, ok := value.(*d2ast.UnquotedString)
		if !ok {
			t.Fatalf("expected unquoted value, got %T", value)
		}
		for _, part := range unquoted.Value {
			if part.String == nil || part.StringRaw == nil {
				continue
			}
			raw := *part.StringRaw
			*part.String = "changed"
			if *part.StringRaw != raw {
				t.Fatal("decoded string mutation changed raw string")
			}
		}
	}
}

func BenchmarkParserLookaheadWhitespace(b *testing.B) {
	for _, spaces := range []int{16, 1024, 16384} {
		b.Run(fmt.Sprint(spaces), func(b *testing.B) {
			// Finishing a quoted scalar peeks across the following whitespace
			// before rewinding it for the map parser to consume again.
			source := "a: \"value\"" + strings.Repeat(" ", spaces) + "\n" + strings.Repeat("\t", spaces) + "b: other"
			b.ReportAllocs()
			for range b.N {
				if _, err := d2parser.Parse("bench.d2", strings.NewReader(source), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
