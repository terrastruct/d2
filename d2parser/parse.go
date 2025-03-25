package d2parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/util-go/go2"
)

type ParseOptions struct {
	// UTF16Pos would be used with input received from a browser where the browser will send the text as UTF-8 but
	// JavaScript keeps strings in memory as UTF-16 and so needs UTF-16 indexes into the text to line up errors correctly.
	// So you want to read UTF-8 still but adjust the indexes to pretend the input is utf16.
	UTF16Pos bool

	ParseError *ParseError
}

// Parse parses a .d2 Map in r.
//
// The returned Map always represents a valid .d2 file. All encountered errors will be in
// []error.
//
// The map may be compiled via Compile even if there are errors to keep language tooling
// operational. Though autoformat should not run.
//
// If UTF16Pos is true, positions will be recorded in UTF-16 codeunits as required by LSP
// and browser clients. See
// https://microsoft.github.io/language-server-protocol/specifications/specification-current/#textDocuments
// TODO: update godocs
func Parse(path string, r io.Reader, opts *ParseOptions) (*d2ast.Map, error) {
	if opts == nil {
		opts = &ParseOptions{
			UTF16Pos: false,
		}
	}

	p := &parser{
		path: path,

		utf16Pos: opts.UTF16Pos,
		err:      opts.ParseError,
	}
	br := bufio.NewReader(r)
	p.reader = br

	bom, err := br.Peek(2)
	if err == nil {
		// 0xFFFE is invalid UTF-8 so this is safe.
		// Also a different BOM is used for UTF-8.
		// See https://unicode.org/faq/utf_bom.html#bom4
		if bom[0] == 0xFF && bom[1] == 0xFE {
			p.utf16Pos = true

			buf := make([]byte, br.Buffered())
			io.ReadFull(br, buf)

			mr := io.MultiReader(bytes.NewBuffer(buf), r)
			tr := transform.NewReader(mr, tunicode.UTF16(tunicode.LittleEndian, tunicode.UseBOM).NewDecoder())
			br.Reset(tr)
		}
	}

	if p.err == nil {
		p.err = &ParseError{}
	}

	m := p.parseMap(true)
	if !p.err.Empty() {
		return m, p.err
	}
	return m, nil
}

func ParseKey(key string) (*d2ast.KeyPath, error) {
	p := &parser{
		reader: strings.NewReader(key),
		err:    &ParseError{},
	}

	k := p.parseKey()
	if !p.err.Empty() {
		return nil, fmt.Errorf("failed to parse key %q: %w", key, p.err)
	}
	if k == nil {
		return nil, fmt.Errorf("empty key: %q", key)
	}
	return k, nil
}

func ParseMapKey(mapKey string) (*d2ast.Key, error) {
	p := &parser{
		reader: strings.NewReader(mapKey),
		err:    &ParseError{},
	}

	mk := p.parseMapKey()
	if !p.err.Empty() {
		return nil, fmt.Errorf("failed to parse map key %q: %w", mapKey, p.err)
	}
	if mk == nil {
		return nil, fmt.Errorf("empty map key: %q", mapKey)
	}
	return mk, nil
}

func ParseValue(value string) (d2ast.Value, error) {
	p := &parser{
		reader: strings.NewReader(value),
		err:    &ParseError{},
	}

	v := p.parseValue()
	if !p.err.Empty() {
		return nil, fmt.Errorf("failed to parse value %q: %w", value, p.err)
	}
	if v.Unbox() == nil {
		return nil, fmt.Errorf("empty value: %q", value)
	}
	return v.Unbox(), nil
}

// TODO: refactor parser to keep entire file in memory as []rune
//   - trivial to then convert positions
//   - lookahead is gone, just forward back as much as you want :)
//   - streaming parser isn't really helpful.
//   - just read into a string even and decode runes forward/back as needed
//   - the whole file essentially exists within the parser as the AST anyway...
//
// TODO: ast struct that combines map & errors and pass that around
type parser struct {
	path     string
	pos      d2ast.Position
	utf16Pos bool

	reader    io.RuneReader
	readerPos d2ast.Position

	readahead    []rune
	lookahead    []rune
	lookaheadPos d2ast.Position

	ioerr bool
	err   *ParseError

	inEdgeGroup bool

	depth int
}

// TODO: rename to Error and make existing Error a private type errorWithRange
type ParseError struct {
	// Errors from globs need to be deduplicated
	ErrorsLookup map[d2ast.Error]struct{} `json:"-"`
	Errors       []d2ast.Error            `json:"errs"`
}

func Errorf(n d2ast.Node, f string, v ...interface{}) error {
	f = "%v: " + f
	v = append([]interface{}{n.GetRange()}, v...)
	return d2ast.Error{
		Range:   n.GetRange(),
		Message: fmt.Sprintf(f, v...),
	}
}

func (pe *ParseError) Empty() bool {
	if pe == nil {
		return true
	}
	return len(pe.Errors) == 0
}

func (pe *ParseError) Error() string {
	var sb strings.Builder
	for i, err := range pe.Errors {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

func (p *parser) errorf(start d2ast.Position, end d2ast.Position, f string, v ...interface{}) {
	r := d2ast.Range{
		Path:  p.path,
		Start: start,
		End:   end,
	}
	f = "%v: " + f
	v = append([]interface{}{r}, v...)
	p.err.Errors = append(p.err.Errors, d2ast.Error{
		Range:   r,
		Message: fmt.Sprintf(f, v...),
	})
}

// _readRune reads the next rune from the underlying reader or from the p.readahead buffer.
func (p *parser) _readRune() (r rune, eof bool) {
	if len(p.readahead) > 0 {
		r = p.readahead[0]
		p.readahead = append(p.readahead[:0], p.readahead[1:]...)
		return r, false
	}

	if p.ioerr {
		p.rewind()
		return 0, true
	}

	p.readerPos = p.lookaheadPos

	r, _, err := p.reader.ReadRune()
	if err != nil {
		p.ioerr = true
		if err != io.EOF {
			p.err.Errors = append(p.err.Errors, d2ast.Error{
				Range: d2ast.Range{
					Path:  p.path,
					Start: p.readerPos,
					End:   p.readerPos,
				},
				Message: fmt.Sprintf("io error: %v", err),
			})
		}
		p.rewind()
		return 0, true
	}
	return r, false
}

func (p *parser) read() (r rune, eof bool) {
	r, eof = p._readRune()
	if eof {
		return 0, true
	}
	p.pos = p.pos.Advance(r, p.utf16Pos)
	p.lookaheadPos = p.pos
	return r, false
}

func (p *parser) replay(r rune) {
	p.pos = p.pos.Subtract(r, p.utf16Pos)

	// This is more complex than it needs to be to allow reusing the buffer underlying
	// p.lookahead.
	newcap := len(p.lookahead) + 1
	if newcap > cap(p.lookahead) {
		lookahead2 := make([]rune, newcap)
		copy(lookahead2[1:], p.lookahead)
		p.lookahead = lookahead2
	} else {
		p.lookahead = p.lookahead[:newcap]
		copy(p.lookahead[1:], p.lookahead)
	}
	p.lookahead[0] = r

	p.rewind()
}

// peek returns the next rune without advancing the parser.
// You *must* call commit or rewind afterwards.
func (p *parser) peek() (r rune, eof bool) {
	r, eof = p._readRune()
	if eof {
		return 0, true
	}

	p.lookahead = append(p.lookahead, r)
	p.lookaheadPos = p.lookaheadPos.Advance(r, p.utf16Pos)
	return r, false
}

// TODO: this can replace multiple peeks i think, just return []rune instead
func (p *parser) peekn(n int) (s string, eof bool) {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		r, eof := p.peek()
		if eof {
			return sb.String(), true
		}
		sb.WriteRune(r)
	}
	return sb.String(), false
}

func (p *parser) readNotSpace() (r rune, eof bool) {
	for {
		r, eof = p.read()
		if eof {
			return 0, true
		}
		if unicode.IsSpace(r) {
			continue
		}
		return r, false
	}
}

// peekNotSpace returns the next non space rune without advancing the parser.
//
// newline is set if the next non space character is on a different line
// than the current line.
//
// TODO: everywhere this is used, we support newline escapes and so can just
// add the logic here and it should *just* work
// except line comments iirc
// not entirely sure, maybe i can put it into peek somehow
func (p *parser) peekNotSpace() (r rune, newlines int, eof bool) {
	for {
		r, eof = p.peek()
		if eof {
			return 0, 0, true
		}
		if unicode.IsSpace(r) {
			if r == '\n' {
				newlines++
			}
			continue
		}
		return r, newlines, false
	}
}

// commit advances p.pos by all peeked bytes and then resets the p.lookahead buffer.
func (p *parser) commit() {
	p.pos = p.lookaheadPos
	p.lookahead = p.lookahead[:0]
}

// rewind copies p.lookahead to the front of p.readahead and then resets the p.lookahead buffer.
// All peeked bytes will again be available via p.eat or p.peek.
// TODO:
// peek
// peekn
// peekNotSpace
// commit
// rewind
//
// TODO: make each parse function read its delimiter and return nil if not as expected
// TODO: lookahead *must* always be empty in between parse calls. you either commit or
//
//	rewind in each function. if you don't, you pass a hint.
//
// TODO: omg we don't need two buffers, just a single lookahead and an index...
// TODO: get rid of lookaheadPos or at least never use directly. maybe rename to beforePeekPos?
//
//	or better yet keep positions in the lookahead buffer.
//	ok so plan here is to get rid of lookaheadPos and add a rewindPos that stores
//	the pos to rewind to.
func (p *parser) rewind() {
	if len(p.lookahead) == 0 {
		return
	}

	// This is more complex than it needs to be to allow reusing the buffer underlying
	// p.readahead.
	newcap := len(p.lookahead) + len(p.readahead)
	if cap(p.readahead) < newcap {
		readahead2 := make([]rune, newcap)
		copy(readahead2[len(p.lookahead):], p.readahead)
		p.readahead = readahead2
	} else {
		p.readahead = p.readahead[:newcap]
		copy(p.readahead[len(p.lookahead):], p.readahead)
	}
	copy(p.readahead, p.lookahead)

	p.lookahead = p.lookahead[:0]
	p.lookaheadPos = p.pos
}

// TODO: remove isFileMap like in printer. can't rn as we have to subtract delim
func (p *parser) parseMap(isFileMap bool) *d2ast.Map {
	m := &d2ast.Map{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos,
		},
	}
	defer m.Range.End.From(&p.pos)

	if !isFileMap {
		m.Range.Start = m.Range.Start.Subtract('{', p.utf16Pos)
		p.depth++
		defer dec(&p.depth)
	}

	for {
		r, eof := p.readNotSpace()
		if eof {
			if !isFileMap {
				p.errorf(m.Range.Start, p.readerPos, "maps must be terminated with }")
			}
			return m
		}

		switch r {
		case ';':
			continue
		case '}':
			if isFileMap {
				p.errorf(p.pos.Subtract(r, p.utf16Pos), p.pos, "unexpected map termination character } in file map")
				continue
			}
			return m
		}

		n := p.parseMapNode(r)
		if n.Unbox() != nil {
			m.Nodes = append(m.Nodes, n)
			// TODO: all subsequent not comment characters on the current line (or till ;)
			// need to be considered errors.
			// TODO: add specific msg for each bad rune type
		}

		if n.BlockComment != nil {
			// Anything after a block comment is ok.
			continue
		}

		after := p.pos
		for {
			r, newlines, eof := p.peekNotSpace()
			if eof || newlines != 0 || r == ';' || r == '}' || r == '#' {
				p.rewind()
				break
			}
			p.commit()
		}

		// TODO: maybe better idea here is to make parseUnquotedString aware of its delimiters
		// better and so it would read technically invalid characters and just complain.
		// TODO: that way broken syntax will be parsed more "intently". would work better with
		// language tooling I think though not sure. yes definitely, eaterr!
		if after != p.pos {
			if n.Unbox() != nil {
				if n.MapKey != nil && n.MapKey.Value.Unbox() != nil {
					ps := ""
					if _, ok := n.MapKey.Value.Unbox().(*d2ast.BlockString); ok {
						ps = ". See https://d2lang.com/tour/text#advanced-block-strings."
					}
					p.errorf(after, p.pos, "unexpected text after %v%s", n.MapKey.Value.Unbox().Type(), ps)
				} else {
					p.errorf(after, p.pos, "unexpected text after %v", n.Unbox().Type())
				}
			} else {
				p.errorf(after, p.pos, "invalid text beginning unquoted key")
			}
		}
	}
}

func (p *parser) parseMapNode(r rune) d2ast.MapNodeBox {
	var box d2ast.MapNodeBox

	switch r {
	case '#':
		box.Comment = p.parseComment()
		return box
	case '"':
		s, eof := p.peekn(2)
		if eof {
			break
		}
		if s != `""` {
			p.rewind()
			break
		}
		p.commit()
		box.BlockComment = p.parseBlockComment()
		return box
	case '.':
		s, eof := p.peekn(2)
		if eof {
			break
		}
		if s != ".." {
			p.rewind()
			break
		}
		r, eof := p.peek()
		if eof {
			break
		}
		if r == '$' {
			p.commit()
			box.Substitution = p.parseSubstitution(true)
			return box
		}
		if r == '@' {
			p.commit()
			box.Import = p.parseImport(true)
			return box
		}
		p.rewind()
		break
	}

	p.replay(r)
	box.MapKey = p.parseMapKey()
	return box
}

func (p *parser) parseComment() *d2ast.Comment {
	c := &d2ast.Comment{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('#', p.utf16Pos),
		},
	}
	defer c.Range.End.From(&p.pos)

	var sb strings.Builder
	defer func() {
		c.Value = sb.String()
	}()
	p.parseCommentLine(c, &sb)

	for {
		r, newlines, eof := p.peekNotSpace()
		if eof {
			return c
		}
		if r != '#' || newlines >= 2 {
			p.rewind()
			return c
		}
		p.commit()

		if newlines == 1 {
			sb.WriteByte('\n')
		}

		p.parseCommentLine(c, &sb)
	}
}

func (p *parser) parseCommentLine(c *d2ast.Comment, sb *strings.Builder) {
	firstRune := true
	for {
		r, eof := p.peek()
		if eof {
			return
		}
		if r == '\n' {
			p.rewind()
			return
		}
		p.commit()

		if firstRune {
			firstRune = false
			if r == ' ' {
				continue
			}
		}
		sb.WriteRune(r)
	}
}

func (p *parser) parseBlockComment() *d2ast.BlockComment {
	bc := &d2ast.BlockComment{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.SubtractString(`"""`, p.utf16Pos),
		},
	}
	defer bc.Range.End.From(&p.pos)

	p.depth++
	defer dec(&p.depth)

	var sb strings.Builder
	defer func() {
		bc.Value = trimSpaceAfterLastNewline(sb.String())
		bc.Value = trimCommonIndent(bc.Value)
	}()

	for {
		r, eof := p.peek()
		if eof {
			p.errorf(bc.Range.Start, p.readerPos, `block comments must be terminated with """`)
			return bc
		}

		if !unicode.IsSpace(r) {
			p.rewind()
			break
		}
		p.commit()
		if r == '\n' {
			break
		}
	}

	for {
		r, eof := p.read()
		if eof {
			p.errorf(bc.Range.Start, p.readerPos, `block comments must be terminated with """`)
			return bc
		}

		if r != '"' {
			sb.WriteRune(r)
			continue
		}

		s, eof := p.peekn(2)
		if eof {
			p.errorf(bc.Range.Start, p.readerPos, `block comments must be terminated with """`)
			return bc
		}
		if s != `""` {
			sb.WriteByte('"')
			p.rewind()
			continue
		}
		p.commit()
		return bc
	}
}

func trimSpaceAfterLastNewline(s string) string {
	lastNewline := strings.LastIndexByte(s, '\n')
	if lastNewline == -1 {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	}

	lastLine := s[lastNewline+1:]
	lastLine = strings.TrimRightFunc(lastLine, unicode.IsSpace)
	if len(lastLine) == 0 {
		return s[:lastNewline]
	}
	return s[:lastNewline+1] + lastLine
}

func (p *parser) parseMapKey() (mk *d2ast.Key) {
	mk = &d2ast.Key{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos,
		},
	}
	defer mk.Range.End.From(&p.pos)

	defer func() {
		if mk.Key == nil && len(mk.Edges) == 0 {
			mk = nil
		}
	}()

	// Check for not ampersand/@.
	r, eof := p.peek()
	if eof {
		return mk
	}
	if r == '!' {
		r, eof := p.peek()
		if eof {
			return mk
		}
		if r == '&' {
			p.commit()
			mk.NotAmpersand = true
		} else {
			p.rewind()
		}
	} else if r == '&' {
		p.commit()
		mk.Ampersand = true
	} else {
		p.rewind()
	}

	r, eof = p.peek()
	if eof {
		return mk
	}
	if r == '(' {
		p.commit()
		p.parseEdgeGroup(mk)
		return mk
	}
	p.rewind()

	k := p.parseKey()
	if k != nil {
		mk.Key = k
	}

	r, newlines, eof := p.peekNotSpace()
	if eof {
		return mk
	}
	if newlines > 0 {
		p.rewind()
		return mk
	}
	switch r {
	case '(':
		p.commit()
		p.parseEdgeGroup(mk)
		return mk
	case '<', '>', '-':
		p.rewind()
		mk.Key = nil
		p.parseEdges(mk, k)
		p.parseMapKeyValue(mk)
		return mk
	default:
		p.rewind()
		p.parseMapKeyValue(mk)
		return mk
	}
}

func (p *parser) parseMapKeyValue(mk *d2ast.Key) {
	r, newlines, eof := p.peekNotSpace()
	if eof {
		return
	}
	if newlines > 0 {
		p.rewind()
		return
	}

	switch r {
	case '{':
		p.rewind()
		if mk.Key == nil && len(mk.Edges) == 0 {
			return
		}
	case ':':
		p.commit()
		if mk.Key == nil && len(mk.Edges) == 0 {
			p.errorf(mk.Range.Start, p.pos, "map value without key")
		}
	default:
		p.rewind()
		return
	}
	mk.Value = p.parseValue()
	if mk.Value.Unbox() == nil {
		p.errorf(p.pos.Subtract(':', p.utf16Pos), p.pos, "missing value after colon")
	}

	sb := mk.Value.ScalarBox()
	// If the value is a scalar, then check if it's the primary value.
	if sb.Unbox() != nil {
		r, newlines, eof := p.peekNotSpace()
		if eof || newlines > 0 || r != '{' {
			p.rewind()
			return
		}
		// Next character is on the same line without ; separator so it must mean
		// our current value is the Primary and the next is the Value.
		p.commit()
		p.replay(r)
		mk.Primary = sb
		mk.Value = p.parseValue()
	}
}

func (p *parser) parseEdgeGroup(mk *d2ast.Key) {
	// To prevent p.parseUnquotedString from consuming terminating parentheses.
	p.inEdgeGroup = true
	defer func() {
		p.inEdgeGroup = false
	}()

	src := p.parseKey()
	p.parseEdges(mk, src)

	r, newlines, eof := p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return
	}
	if r != ')' {
		p.rewind()
		p.errorf(mk.Range.Start, p.pos, "edge groups must be terminated with )")
		return
	}
	p.commit()

	r, newlines, eof = p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return
	}
	if r == '[' {
		p.commit()
		mk.EdgeIndex = p.parseEdgeIndex()
	} else {
		p.rewind()
	}

	r, newlines, eof = p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return
	}
	if r == '.' {
		p.commit()
		mk.EdgeKey = p.parseKey()
	} else {
		p.rewind()
	}

	p.inEdgeGroup = false
	p.parseMapKeyValue(mk)
}

func (p *parser) parseEdgeIndex() *d2ast.EdgeIndex {
	ei := &d2ast.EdgeIndex{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('[', p.utf16Pos),
		},
	}
	defer ei.Range.End.From(&p.pos)

	r, newlines, eof := p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return nil
	}

	if unicode.IsDigit(r) {
		p.commit()
		var sb strings.Builder
		sb.WriteRune(r)
		for {
			r, newlines, eof = p.peekNotSpace()
			if eof || newlines > 0 {
				p.rewind()
				p.errorf(ei.Range.Start, p.pos, "unterminated edge index")
				return nil
			}
			if r == ']' {
				p.rewind()
				break
			}
			p.commit()
			if !unicode.IsDigit(r) {
				p.errorf(p.pos.Subtract(r, p.utf16Pos), p.pos, "unexpected character in edge index")
				continue
			}
			sb.WriteRune(r)
		}
		i, _ := strconv.Atoi(sb.String())
		ei.Int = &i
	} else if r == '*' {
		p.commit()
		ei.Glob = true
	} else {
		p.errorf(p.pos.Subtract(r, p.utf16Pos), p.pos, "unexpected character in edge index")
		// TODO: skip to ], maybe add a p.skipTo to skip to certain characters
	}

	r, newlines, eof = p.peekNotSpace()
	if eof || newlines > 0 || r != ']' {
		p.rewind()
		p.errorf(ei.Range.Start, p.pos, "unterminated edge index")
		return ei
	}
	p.commit()
	return ei
}

func (p *parser) parseEdges(mk *d2ast.Key, src *d2ast.KeyPath) {
	for {
		e := &d2ast.Edge{
			Range: d2ast.Range{
				Path: p.path,
			},
			Src: src,
		}
		if src != nil {
			e.Range.Start = src.Range.Start
		} else {
			e.Range.Start = p.pos
		}

		r, newlines, eof := p.peekNotSpace()
		if eof {
			return
		}
		if newlines > 0 {
			p.rewind()
			return
		}
		if r == '<' || r == '*' {
			e.SrcArrow = string(r)
		} else if r != '-' {
			p.rewind()
			return
		}
		if src == nil {
			p.errorf(p.lookaheadPos.Subtract(r, p.utf16Pos), p.lookaheadPos, "connection missing source")
			e.Range.Start = p.lookaheadPos.Subtract(r, p.utf16Pos)
		}
		p.commit()

		if !p.parseEdge(e) {
			return
		}

		dst := p.parseKey()
		if dst == nil {
			p.errorf(e.Range.Start, p.pos, "connection missing destination")
		} else {
			e.Dst = dst
			e.Range.End = e.Dst.Range.End
		}
		mk.Edges = append(mk.Edges, e)
		src = dst
	}
}

func (p *parser) parseEdge(e *d2ast.Edge) (ok bool) {
	defer e.Range.End.From(&p.pos)

	for {
		r, eof := p.peek()
		if eof {
			p.errorf(e.Range.Start, p.readerPos, "unterminated connection")
			return false
		}
		switch r {
		case '>', '*':
			e.DstArrow = string(r)
			p.commit()
			return true
		case '\\':
			p.commit()
			r, newlines, eof := p.peekNotSpace()
			if eof {
				continue
			}
			if newlines == 0 {
				p.rewind()
				p.errorf(e.Range.Start, p.readerPos, "only newline escapes are allowed in connections")
				return false
			}
			if newlines > 1 {
				p.rewind()
				continue
			}
			p.commit()
			p.replay(r)
		case '-':
			p.commit()
		default:
			p.rewind()
			return true
		}
	}
}

func (p *parser) parseKey() (k *d2ast.KeyPath) {
	k = &d2ast.KeyPath{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos,
		},
	}

	defer func() {
		if len(k.Path) == 0 {
			k = nil
		} else {
			k.Range.End = k.Path[len(k.Path)-1].Unbox().GetRange().End
			for _, part := range k.Path {
				if part.Unbox() != nil {
					if len(part.Unbox().ScalarString()) > 518 {
						p.errorf(k.Range.Start, k.Range.End, "key length %d exceeds maximum allowed length of 518", len(part.Unbox().ScalarString()))
						break
					}
				}
			}
		}
	}()

	for {
		r, newlines, eof := p.peekNotSpace()
		if eof {
			return k
		}
		if newlines > 0 || r == '(' {
			p.rewind()
			return k
		}
		// TODO: error if begin, but see below too
		if r == '.' {
			continue
		}
		p.rewind()

		sb := p.parseString(true)
		s := sb.Unbox()
		if s == nil {
			return k
		}
		if sb.UnquotedString != nil && strings.HasPrefix(s.ScalarString(), "@") {
			p.errorf(s.GetRange().Start, s.GetRange().End, "%s is not a valid import, did you mean ...%[2]s?", s.ScalarString())
		}

		if len(k.Path) == 0 {
			k.Range.Start = s.GetRange().Start
		}
		k.Path = append(k.Path, &sb)

		r, newlines, eof = p.peekNotSpace()
		if eof {
			return k
		}
		if newlines > 0 || r != '.' {
			p.rewind()
			return k
		}
		// TODO: error if not string or ( after, see above too
		p.commit()
	}
}

// TODO: inKey -> p.inKey (means I have to restore though)
func (p *parser) parseString(inKey bool) d2ast.StringBox {
	var box d2ast.StringBox

	r, newlines, eof := p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return box
	}
	p.commit()

	switch r {
	case '"':
		box.DoubleQuotedString = p.parseDoubleQuotedString(inKey)
		return box
	case '\'':
		box.SingleQuotedString = p.parseSingleQuotedString()
		return box
	case '|':
		box.BlockString = p.parseBlockString()
		return box
	default:
		p.replay(r)
		box.UnquotedString = p.parseUnquotedString(inKey)
		return box
	}
}

func (p *parser) parseUnquotedString(inKey bool) (s *d2ast.UnquotedString) {
	s = &d2ast.UnquotedString{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos,
		},
	}
	// TODO: fix unquoted end whitespace handling to peekNotSpace
	lastNonSpace := p.pos
	defer s.Range.End.From(&lastNonSpace)

	var sb strings.Builder
	var rawb strings.Builder
	lastPatternIndex := 0
	defer func() {
		sv := strings.TrimRightFunc(sb.String(), unicode.IsSpace)
		rawv := strings.TrimRightFunc(rawb.String(), unicode.IsSpace)
		if s.Pattern != nil {
			if lastPatternIndex < len(sv) {
				s.Pattern = append(s.Pattern, sv[lastPatternIndex:])
			}
		}
		if sv == "" {
			if len(s.Value) > 0 {
				return
			}
			s = nil
			// TODO: this should be in the parent and instead they check the delimiters first
			// 			 or last really. only in parseMapNode && parseArrayNode
			// TODO: give specific descriptions for each kind of special character that could have caused this.
			return
		}
		s.Value = append(s.Value, d2ast.InterpolationBox{String: &sv, StringRaw: &rawv})
	}()

	_s, eof := p.peekn(4)
	p.rewind()
	if !eof {
		if _s == "...@" {
			p.errorf(p.pos, p.pos.AdvanceString("...@", p.utf16Pos), "unquoted strings cannot begin with ...@ as that's import spread syntax")
		}
	}

	for {
		r, eof := p.peek()
		if eof {
			return s
		}

		if p.inEdgeGroup && r == ')' {
			// TODO: need a peekNotSpace across escaped newlines
			r2, newlines, eof := p.peekNotSpace()
			if eof || newlines > 0 {
				p.rewind()
				return s
			}
			switch r2 {
			case '\n', '#', '{', '}', '[', ']', ':', '.':
				p.rewind()
				return s
			}
			p.rewind()
			p.peek()
			p.commit()
			lastNonSpace = p.pos
			sb.WriteRune(r)
			rawb.WriteRune(r)
			continue
		}

		// top:   '\n', '#', '{', '}', '[', ']'
		// keys:  ':', '.'
		// edges: '<', '>', '(', ')',
		// edges: --, ->, -*, *-
		switch r {
		case '\n', ';', '#', '{', '}', '[', ']':
			p.rewind()
			return s
		}
		if inKey {
			switch r {
			case ':', '.', '<', '>', '&':
				p.rewind()
				return s
			case '-':
				// TODO: need a peekNotSpace across escaped newlines
				r2, eof := p.peek()
				if eof {
					return s
				}
				switch r2 {
				case '\n', ';', '#', '{', '}', '[', ']':
					p.rewind()
					p.peek()
					p.commit()
					sb.WriteRune(r)
					rawb.WriteRune(r)
					return s
				}
				if r2 == '-' || r2 == '>' || r2 == '*' {
					p.rewind()
					return s
				}
				sb.WriteRune(r)
				rawb.WriteRune(r)
				r = r2
			}
		}

		if r == '*' {
			if sb.Len() == 0 {
				s.Pattern = append(s.Pattern, "*")
			} else {
				s.Pattern = append(s.Pattern, sb.String()[lastPatternIndex:], "*")
			}
			lastPatternIndex = len(sb.String()) + 1
		}

		p.commit()

		if !unicode.IsSpace(r) {
			lastNonSpace = p.pos
		}

		if !inKey && r == '$' {
			subst := p.parseSubstitution(false)
			if subst != nil {
				if sb.Len() > 0 {
					sv := sb.String()
					rawv := rawb.String()
					s.Value = append(s.Value, d2ast.InterpolationBox{String: &sv, StringRaw: &rawv})
					sb.Reset()
					rawb.Reset()
				}
				s.Value = append(s.Value, d2ast.InterpolationBox{Substitution: subst})
				continue
			}
			continue
		}

		if r != '\\' {
			sb.WriteRune(r)
			rawb.WriteRune(r)
			continue
		}

		r2, eof := p.read()
		if eof {
			p.errorf(p.pos.Subtract('\\', p.utf16Pos), p.readerPos, "unfinished escape sequence")
			return s
		}

		if r2 == '\n' {
			r, newlines, eof := p.peekNotSpace()
			if eof || newlines > 0 {
				p.rewind()
				return s
			}
			p.commit()
			p.replay(r)
			continue
		}

		sb.WriteRune(decodeEscape(r2))
		rawb.WriteByte('\\')
		rawb.WriteRune(r2)
	}
}

// https://go.dev/ref/spec#Rune_literals
// TODO: implement all Go escapes like the unicode ones
func decodeEscape(r2 rune) rune {
	switch r2 {
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	case '\\':
		return '\\'
	case '"':
		return '"'
	default:
		return r2
	}
}

func (p *parser) parseDoubleQuotedString(inKey bool) *d2ast.DoubleQuotedString {
	s := &d2ast.DoubleQuotedString{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('"', p.utf16Pos),
		},
	}
	defer s.Range.End.From(&p.pos)

	var sb strings.Builder
	var rawb strings.Builder
	defer func() {
		if sb.Len() > 0 {
			sv := sb.String()
			rawv := rawb.String()
			s.Value = append(s.Value, d2ast.InterpolationBox{String: &sv, StringRaw: &rawv})
		}
	}()

	for {
		r, eof := p.peek()
		if eof {
			p.errorf(s.Range.Start, p.readerPos, `double quoted strings must be terminated with "`)
			return s
		}
		if r == '\n' {
			p.rewind()
			p.errorf(s.Range.Start, p.pos, `double quoted strings must be terminated with "`)
			return s
		}

		p.commit()
		if !inKey && r == '$' {
			subst := p.parseSubstitution(false)
			if subst != nil {
				if sb.Len() > 0 {
					s.Value = append(s.Value, d2ast.InterpolationBox{String: go2.Pointer(sb.String())})
					sb.Reset()
				}
				s.Value = append(s.Value, d2ast.InterpolationBox{Substitution: subst})
				continue
			}
		}

		if r == '"' {
			return s
		}

		if r != '\\' {
			sb.WriteRune(r)
			rawb.WriteRune(r)
			continue
		}

		r2, eof := p.read()
		if eof {
			p.errorf(p.pos.Subtract('\\', p.utf16Pos), p.readerPos, "unfinished escape sequence")
			p.errorf(s.Range.Start, p.readerPos, `double quoted strings must be terminated with "`)
			return s
		}

		if r2 == '\n' {
			// TODO: deindent
			continue
		}
		sb.WriteRune(decodeEscape(r2))
		rawb.WriteByte('\\')
		rawb.WriteRune(r2)
	}
}

func (p *parser) parseSingleQuotedString() *d2ast.SingleQuotedString {
	s := &d2ast.SingleQuotedString{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('\'', p.utf16Pos),
		},
	}
	defer s.Range.End.From(&p.pos)

	var sb strings.Builder
	defer func() {
		s.Value = sb.String()
	}()

	for {
		r, eof := p.peek()
		if eof {
			p.errorf(s.Range.Start, p.readerPos, `single quoted strings must be terminated with '`)
			return s
		}
		if r == '\n' {
			p.rewind()
			p.errorf(s.Range.Start, p.pos, `single quoted strings must be terminated with '`)
			return s
		}
		p.commit()

		if r == '\'' {
			r, eof = p.peek()
			if eof {
				return s
			}
			if r == '\'' {
				p.commit()
				sb.WriteByte('\'')
				continue
			}
			p.rewind()
			return s
		}

		if r != '\\' {
			sb.WriteRune(r)
			continue
		}

		r2, eof := p.peek()
		if eof {
			continue
		}

		switch r2 {
		case '\n':
			p.commit()
			continue
		default:
			sb.WriteRune(r)
			p.rewind()
		}
	}
}

func (p *parser) parseBlockString() *d2ast.BlockString {
	bs := &d2ast.BlockString{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('|', p.utf16Pos),
		},
	}
	defer bs.Range.End.From(&p.pos)

	p.depth++
	defer dec(&p.depth)

	var sb strings.Builder
	defer func() {
		bs.Value = trimSpaceAfterLastNewline(sb.String())
		bs.Value = trimCommonIndent(bs.Value)
	}()

	// Do we have more symbol quotes?
	bs.Quote = ""
	for {
		r, eof := p.peek()
		if eof {
			p.errorf(bs.Range.Start, p.readerPos, `block string must be terminated with %v`, bs.Quote+"|")
			return bs
		}

		if unicode.IsSpace(r) || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			p.rewind()
			break
		}
		p.commit()
		bs.Quote += string(r)
	}

	// Do we have a tag?
	for {
		r, eof := p.peek()
		if eof {
			p.errorf(bs.Range.Start, p.readerPos, `block string must be terminated with %v`, bs.Quote+"|")
			return bs
		}

		if unicode.IsSpace(r) {
			p.rewind()
			break
		}
		p.commit()
		bs.Tag += string(r)
	}
	if bs.Tag == "" {
		// TODO: no and fix compiler to not set text/markdown shape always.
		//       reason being not all multiline text is markdown by default.
		//       for example markdown edge labels or other random text.
		//       maybe we can be smart about this at some point and only set
		//       if the block string is being interpreted as markdown.
		bs.Tag = "md"
	}

	// Skip non newline whitespace.
	for {
		r, eof := p.peek()
		if eof {
			p.errorf(bs.Range.Start, p.readerPos, `block string must be terminated with %v`, bs.Quote+"|")
			return bs
		}
		if !unicode.IsSpace(r) {
			// Non whitespace characters on the first line have an implicit indent.
			sb.WriteString(p.getIndent())
			p.rewind()
			break
		}
		p.commit()
		if r == '\n' {
			break
		}
	}

	endHint := '|'
	endRest := ""
	if len(bs.Quote) > 0 {
		var size int
		endHint, size = utf8.DecodeLastRuneInString(bs.Quote)
		endRest = bs.Quote[size:] + "|"
	}

	for {
		r, eof := p.read()
		if eof {
			p.errorf(bs.Range.Start, p.readerPos, `block string must be terminated with %v`, bs.Quote+"|")
			return bs
		}

		if r != endHint {
			sb.WriteRune(r)
			continue
		}

		s, eof := p.peekn(len(endRest))
		if eof {
			p.errorf(bs.Range.Start, p.readerPos, `block string must be terminated with %v`, bs.Quote+"|")
			return bs
		}
		if s != endRest {
			sb.WriteRune(endHint)
			p.rewind()
			continue
		}
		p.commit()
		return bs
	}
}

func (p *parser) parseArray() *d2ast.Array {
	a := &d2ast.Array{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.Subtract('[', p.utf16Pos),
		},
	}
	defer a.Range.End.From(&p.readerPos)

	p.depth++
	defer dec(&p.depth)

	for {
		r, eof := p.readNotSpace()
		if eof {
			p.errorf(a.Range.Start, p.readerPos, "arrays must be terminated with ]")
			return a
		}

		switch r {
		case ';':
			continue
		case ']':
			return a
		}

		n := p.parseArrayNode(r)
		if n.Unbox() != nil {
			a.Nodes = append(a.Nodes, n)
		}

		if n.BlockComment != nil {
			// Anything after a block comment is ok.
			continue
		}

		after := p.pos
		for {
			r, newlines, eof := p.peekNotSpace()
			if eof || newlines != 0 || r == ';' || r == ']' || r == '#' {
				p.rewind()
				break
			}
			p.commit()
		}

		if after != p.pos {
			if n.Unbox() != nil {
				p.errorf(after, p.pos, "unexpected text after %v", n.Unbox().Type())
			} else {
				p.errorf(after, p.pos, "invalid text beginning unquoted string")
			}
		}
	}
}

func (p *parser) parseArrayNode(r rune) d2ast.ArrayNodeBox {
	var box d2ast.ArrayNodeBox

	switch r {
	case '#':
		box.Comment = p.parseComment()
		return box
	case '"':
		s, eof := p.peekn(2)
		if eof {
			break
		}
		if s != `""` {
			p.rewind()
			break
		}
		p.commit()
		box.BlockComment = p.parseBlockComment()
		return box
	case '.':
		s, eof := p.peekn(2)
		if eof {
			break
		}
		if s != ".." {
			p.rewind()
			break
		}
		r, eof := p.peek()
		if eof {
			break
		}
		if r == '$' {
			p.commit()
			box.Substitution = p.parseSubstitution(true)
			return box
		}
		if r == '@' {
			p.commit()
			box.Import = p.parseImport(true)
			return box
		}
		p.rewind()
		break
	}

	p.replay(r)
	vbox := p.parseValue()
	if vbox.UnquotedString != nil && vbox.UnquotedString.ScalarString() == "" &&
		!(len(vbox.UnquotedString.Value) > 0 && vbox.UnquotedString.Value[0].Substitution != nil) {
		p.errorf(p.pos, p.pos.Advance(r, p.utf16Pos), "unquoted strings cannot start on %q", r)
	}
	box.Null = vbox.Null
	box.Boolean = vbox.Boolean
	box.Number = vbox.Number
	box.UnquotedString = vbox.UnquotedString
	box.DoubleQuotedString = vbox.DoubleQuotedString
	box.SingleQuotedString = vbox.SingleQuotedString
	box.BlockString = vbox.BlockString
	box.Array = vbox.Array
	box.Map = vbox.Map
	box.Import = vbox.Import
	return box
}

func (p *parser) parseValue() d2ast.ValueBox {
	var box d2ast.ValueBox

	r, newlines, eof := p.peekNotSpace()
	if eof || newlines > 0 {
		p.rewind()
		return box
	}
	p.commit()

	switch r {
	case '[':
		box.Array = p.parseArray()
		return box
	case '{':
		box.Map = p.parseMap(false)
		return box
	case '@':
		box.Import = p.parseImport(false)
		return box
	}

	p.replay(r)
	sb := p.parseString(false)
	if sb.DoubleQuotedString != nil {
		box.DoubleQuotedString = sb.DoubleQuotedString
		return box
	}
	if sb.SingleQuotedString != nil {
		box.SingleQuotedString = sb.SingleQuotedString
		return box
	}
	if sb.BlockString != nil {
		box.BlockString = sb.BlockString
		return box
	}

	if sb.UnquotedString == nil {
		return box
	}

	s := sb.UnquotedString
	if strings.EqualFold(s.ScalarString(), "null") {
		box.Null = &d2ast.Null{
			Range: s.Range,
		}
		return box
	}
	if strings.EqualFold(s.ScalarString(), "suspend") {
		box.Suspension = &d2ast.Suspension{
			Range: s.Range,
			Value: true,
		}
		return box
	}
	if strings.EqualFold(s.ScalarString(), "unsuspend") {
		box.Suspension = &d2ast.Suspension{
			Range: s.Range,
			Value: false,
		}
		return box
	}

	if strings.EqualFold(s.ScalarString(), "true") {
		box.Boolean = &d2ast.Boolean{
			Range: s.Range,
			Value: true,
		}
		return box
	}

	if strings.EqualFold(s.ScalarString(), "false") {
		box.Boolean = &d2ast.Boolean{
			Range: s.Range,
			Value: false,
		}
		return box
	}

	// TODO: only if matches regex
	rat, ok := big.NewRat(0, 1).SetString(s.ScalarString())
	if ok {
		box.Number = &d2ast.Number{
			Range: s.Range,
			Raw:   s.ScalarString(),
			Value: rat,
		}
		return box
	}

	box.UnquotedString = s
	return box
}

func (p *parser) parseSubstitution(spread bool) *d2ast.Substitution {
	subst := &d2ast.Substitution{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.SubtractString("$", p.utf16Pos),
		},
		Spread: spread,
	}
	defer subst.Range.End.From(&p.pos)

	if subst.Spread {
		subst.Range.Start = subst.Range.Start.SubtractString("...", p.utf16Pos)
	}

	r, newlines, eof := p.peekNotSpace()
	if eof {
		return nil
	}
	if newlines > 0 {
		p.rewind()
		return nil
	}
	if r != '{' {
		p.rewind()
		p.errorf(subst.Range.Start, p.readerPos, "substitutions must begin on {")
		return nil
	} else {
		p.commit()
	}

	k := p.parseKey()
	if k != nil {
		subst.Path = k.Path
	}

	r, newlines, eof = p.peekNotSpace()
	if eof {
		p.errorf(subst.Range.Start, p.readerPos, "substitutions must be terminated by }")
		return subst
	}
	if newlines > 0 || r != '}' {
		p.rewind()
		p.errorf(subst.Range.Start, p.pos, "substitutions must be terminated by }")
		return subst
	}
	p.commit()

	return subst
}

func (p *parser) parseImport(spread bool) *d2ast.Import {
	imp := &d2ast.Import{
		Range: d2ast.Range{
			Path:  p.path,
			Start: p.pos.SubtractString("$", p.utf16Pos),
		},
		Spread: spread,
	}
	defer imp.Range.End.From(&p.pos)

	if imp.Spread {
		imp.Range.Start = imp.Range.Start.SubtractString("...", p.utf16Pos)
	}

	var pre strings.Builder
	for {
		r, eof := p.peek()
		if eof {
			break
		}
		if r != '.' && r != '/' {
			p.rewind()
			break
		}
		pre.WriteRune(r)
		p.commit()
	}
	imp.Pre = pre.String()

	k := p.parseKey()
	if k == nil {
		return imp
	}
	if k.Path[0].UnquotedString != nil && len(k.Path) > 1 && k.Path[1].UnquotedString != nil && k.Path[1].Unbox().ScalarString() == "d2" {
		k.Path = append(k.Path[:1], k.Path[2:]...)
	}
	imp.Path = k.Path
	return imp
}

// func marshalKey(k *d2ast.Key) string {
// 	var sb strings.Builder
// 	for i, s := range k.Path {
// 		// TODO: Need to encode specials and quotes.
// 		sb.WriteString(s.Unbox().ScalarString())
// 		if i < len(k.Path)-1 {
// 			sb.WriteByte('.')
// 		}
// 	}
// 	return sb.String()
// }

func dec(i *int) {
	*i -= 1
}

func (p *parser) getIndent() string {
	return strings.Repeat(" ", p.depth*2)
}

func trimIndent(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		_, l = splitLeadingIndent(l, len(indent))
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

func trimCommonIndent(s string) string {
	commonIndent := ""
	for _, l := range strings.Split(s, "\n") {
		if l == "" {
			continue
		}
		lineIndent, l := splitLeadingIndent(l, -1)
		if lineIndent == "" {
			// No common indent return as is.
			return s
		}
		if l == "" {
			// Whitespace only line.
			continue
		}
		if commonIndent == "" || len(lineIndent) < len(commonIndent) {
			commonIndent = lineIndent
		}
	}
	if commonIndent == "" {
		return s
	}
	return trimIndent(s, commonIndent)
}

func splitLeadingIndent(s string, maxSpaces int) (indent, rets string) {
	var indentb strings.Builder
	i := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			break
		}
		i++
		if r != '\t' {
			indentb.WriteRune(r)
		} else {
			indentb.WriteByte(' ')
			indentb.WriteByte(' ')
		}
		if maxSpaces > -1 && indentb.Len() == maxSpaces {
			break
		}
	}
	return indentb.String(), s[i:]
}
