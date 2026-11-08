// Go-only port of the subset of Python `ast.literal_eval` that
// `read_js_variables` relies on (CPython 3.13 grammar and `_convert`).
//
// Result types: str → string, bytes → []byte, int → int64, float →
// float64, complex → complex128, bool, None → nil, list → []any,
// tuple → Tuple, set → Set, dict → map[string]any (all keys str) or
// map[any]any.
//
// Differences with Python (all fail, so the caller falls back to the
// raw string, where Python would succeed or raise):
//   - `\N{name}` escapes and `...` (Ellipsis) are not supported;
//   - unhashable set elements / dict keys (Python raises TypeError) and
//     tuple / bytes dict keys (not Go map keys) fail;
//   - integers outside the int64 range become float64;
//   - lone surrogate escapes become U+FFFD.

package readers

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tuple is a parsed Python tuple (distinct from a list, as in Python).
type Tuple []any

// Set is a parsed Python set (elements in first-seen order).
type Set []any

var errLiteral = errors.New("malformed literal")

// Token kinds
const (
	tokEnd = iota
	tokNumber
	tokString
	tokName
	tokOp
	tokNewline
)

type token struct {
	kind  int
	text  string // op or name
	value any    // number or string/bytes value
	fstr  bool   // f-string
	bytes bool   // bytes literal
}

const (
	litDigitPart = `[0-9](?:_?[0-9])*`
	litPointFlt  = `(?:` + litDigitPart + `)?\.` + litDigitPart + `|` + litDigitPart + `\.`
	litExpFlt    = `(?:` + litPointFlt + `|` + litDigitPart + `)[eE][+-]?` + litDigitPart
	litFloat     = litExpFlt + `|` + litPointFlt
)

var (
	litNumberRe = func() *regexp.Regexp {
		re := regexp.MustCompile(`^(?:` +
			`(?P<imag>(?:` + litFloat + `|` + litDigitPart + `)[jJ])` +
			`|(?P<float>` + litFloat + `)` +
			`|(?P<hex>0[xX](?:_?[0-9a-fA-F])+)` +
			`|(?P<oct>0[oO](?:_?[0-7])+)` +
			`|(?P<bin>0[bB](?:_?[01])+)` +
			`|(?P<dec>[1-9](?:_?[0-9])*|0(?:_?0)*)` +
			`)`)
		re.Longest()
		return re
	}()
	litStringPrefixes = map[string]bool{
		"r": true, "u": true, "f": true, "b": true,
		"br": true, "rb": true, "fr": true, "rf": true,
	}
)

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) ||
		unicode.Is(unicode.Other_ID_Start, r)
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) ||
		unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Pc, r) ||
		unicode.Is(unicode.Other_ID_Continue, r)
}

// litTokenize mirrors the Python tokenizer for an `eval`-mode source.
func litTokenize(src string) ([]token, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	if strings.ContainsRune(src, 0) {
		// "source code string cannot contain null bytes"
		return nil, errLiteral
	}

	var tokens []token
	depth := 0
	lineStart := true
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\f':
			// Indentation at the start of a logical line is an error
			// (unless the line turns out to be blank)
			if lineStart && depth == 0 && i > 0 && src[i-1] == '\n' {
				j := i
				for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\f') {
					j++
				}
				if j < len(src) && src[j] != '\n' && src[j] != '#' && src[j] != '\\' {
					return nil, errLiteral
				}
			}
			i++
			continue
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		case c == '\\':
			// Line continuation
			if i+1 < len(src) && src[i+1] == '\n' {
				i += 2
				continue
			}
			// A backslash outside a string is only a line continuation
			return nil, errLiteral
		case c == '\n':
			i++
			if depth == 0 {
				if len(tokens) > 0 && tokens[len(tokens)-1].kind != tokNewline {
					tokens = append(tokens, token{kind: tokNewline})
				}
				lineStart = true
			}
			continue
		}
		lineStart = false

		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])):
			tok, n, err := litNumber(src[i:])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i += n
		case c == '"' || c == '\'':
			tok, n, err := litString(src[i:], "")
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i += n
		case isIdentStart(r):
			j := i + size
			for j < len(src) {
				r2, s2 := utf8.DecodeRuneInString(src[j:])
				if !isIdentContinue(r2) {
					break
				}
				j += s2
			}
			name := src[i:j]
			if j < len(src) && (src[j] == '"' || src[j] == '\'') &&
				litStringPrefixes[strings.ToLower(name)] {
				tok, n, err := litString(src[j:], strings.ToLower(name))
				if err != nil {
					return nil, err
				}
				tokens = append(tokens, tok)
				i = j + n
				continue
			}
			tokens = append(tokens, token{kind: tokName, text: name})
			i = j
		case strings.ContainsRune("([{", rune(c)):
			depth++
			tokens = append(tokens, token{kind: tokOp, text: string(c)})
			i++
		case strings.ContainsRune(")]}", rune(c)):
			if depth == 0 {
				return nil, errLiteral
			}
			depth--
			tokens = append(tokens, token{kind: tokOp, text: string(c)})
			i++
		case strings.ContainsRune(",:+-", rune(c)):
			tokens = append(tokens, token{kind: tokOp, text: string(c)})
			i++
		default:
			// Any other operator or character is outside the literal subset
			return nil, errLiteral
		}
	}
	if depth != 0 {
		return nil, errLiteral
	}
	return append(tokens, token{kind: tokEnd}), nil
}

// litNumber tokenizes a Python numeric literal.
func litNumber(s string) (token, int, error) {
	m := litNumberRe.FindStringSubmatchIndex(s)
	if m == nil {
		return token{}, 0, errLiteral
	}
	end := m[1]
	// A number glued to a name or digit (`0123`, `1_`, `1L`) is invalid
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if isIdentContinue(r) || r == '.' && strings.Contains(s[:end], ".") {
			return token{}, 0, errLiteral
		}
	}
	text := strings.ReplaceAll(s[:end], "_", "")
	group := func(name string) bool {
		i := litNumberRe.SubexpIndex(name)
		return m[2*i] >= 0
	}
	var value any
	switch {
	case group("imag"):
		f, _ := strconv.ParseFloat(text[:len(text)-1], 64)
		value = complex(0, f)
	case group("float"):
		f, _ := strconv.ParseFloat(text, 64)
		value = f
	case group("hex"), group("oct"), group("bin"):
		base := map[byte]int{'x': 16, 'o': 8, 'b': 2}[text[1]|0x20]
		value = parseIntLiteral(text[2:], base)
	default:
		value = parseIntLiteral(text, 10)
	}
	return token{kind: tokNumber, value: value}, end, nil
}

// parseIntLiteral gives an int64, or a float64 beyond the int64 range.
func parseIntLiteral(digits string, base int) any {
	if v, err := strconv.ParseInt(digits, base, 64); err == nil {
		return v
	}
	f := 0.0
	for _, c := range digits {
		f = f*float64(base) + float64(digitOf(c))
	}
	return f
}

func digitOf(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// litString tokenizes a string literal starting at its opening quote.
func litString(s string, prefix string) (token, int, error) {
	raw := strings.Contains(prefix, "r")
	isBytes := strings.Contains(prefix, "b")
	quote := s[0]
	triple := strings.HasPrefix(s, strings.Repeat(string(quote), 3))
	i := 1
	if triple {
		i = 3
	}
	start := i
	for {
		if i >= len(s) {
			// Unterminated string literal
			return token{}, 0, errLiteral
		}
		c := s[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == '\n' && !triple {
			return token{}, 0, errLiteral
		}
		if c == quote {
			if !triple {
				break
			}
			if strings.HasPrefix(s[i:], strings.Repeat(string(quote), 3)) {
				break
			}
		}
		i++
	}
	body := s[start:i]
	end := i + 1
	if triple {
		end = i + 3
	}

	tok := token{kind: tokString, fstr: strings.Contains(prefix, "f"), bytes: isBytes}
	if isBytes {
		for j := 0; j < len(body); j++ {
			if body[j] >= 0x80 {
				// bytes can only contain ASCII literal characters
				return token{}, 0, errLiteral
			}
		}
	}
	if raw {
		if isBytes {
			tok.value = []byte(body)
		} else {
			tok.value = body
		}
		return tok, end, nil
	}
	value, err := unescape(body, isBytes)
	if err != nil {
		return token{}, 0, err
	}
	if isBytes {
		tok.value = []byte(value)
	} else {
		tok.value = value
	}
	return tok, end, nil
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

func isHex(c byte) bool {
	return isDigit(c) || (c|0x20 >= 'a' && c|0x20 <= 'f')
}

// unescape mirrors Python string literal escape processing. For bytes
// the result holds raw bytes (not UTF-8 text).
func unescape(body string, isBytes bool) (string, error) {
	var b strings.Builder
	writeCode := func(code rune) {
		if isBytes {
			b.WriteByte(byte(code))
		} else {
			b.WriteRune(code)
		}
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			return "", errLiteral
		}
		e := body[i]
		switch e {
		case '\n':
			// Line continuation inside the string
		case '\\', '\'', '"':
			b.WriteByte(e)
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(body) && j < i+3 && isOctal(body[j]) {
				j++
			}
			code, _ := strconv.ParseUint(body[i:j], 8, 32)
			if isBytes {
				// Python keeps the low byte of an out-of-range octal escape
				code &= 0xff
			}
			writeCode(rune(code))
			i = j - 1
		case 'x':
			if i+2 >= len(body) || !isHex(body[i+1]) || !isHex(body[i+2]) {
				return "", errLiteral
			}
			code, _ := strconv.ParseUint(body[i+1:i+3], 16, 32)
			writeCode(rune(code))
			i += 2
		case 'u', 'U':
			if isBytes {
				b.WriteByte('\\')
				b.WriteByte(e)
				continue
			}
			n := 4
			if e == 'U' {
				n = 8
			}
			if i+n >= len(body) {
				return "", errLiteral
			}
			for k := 1; k <= n; k++ {
				if !isHex(body[i+k]) {
					return "", errLiteral
				}
			}
			code, _ := strconv.ParseUint(body[i+1:i+1+n], 16, 32)
			if code > unicode.MaxRune {
				return "", errLiteral
			}
			// Lone surrogates cannot be held by a Go string
			b.WriteRune(rune(code))
			i += n
		case 'N':
			if isBytes {
				b.WriteString(`\N`)
				continue
			}
			// \N{name} is not supported
			return "", errLiteral
		default:
			// Unknown escapes are kept as written
			b.WriteByte('\\')
			b.WriteByte(e)
		}
	}
	return b.String(), nil
}

// Parsed nodes, mirroring the `ast` node types literal_eval inspects.
type litNode struct {
	kind  string // const, unary, binop, tuple, list, set, dict, call, name
	value any
	op    string
	elts  []*litNode
	keys  []*litNode
	fstr  bool
}

type litParser struct {
	tokens []token
	i      int
}

func (p *litParser) peek() token { return p.tokens[p.i] }

func (p *litParser) next() token {
	t := p.tokens[p.i]
	if t.kind != tokEnd {
		p.i++
	}
	return t
}

func (p *litParser) isOp(op string) bool {
	t := p.peek()
	return t.kind == tokOp && t.text == op
}

// parseEval mirrors `expressions NEWLINE* ENDMARKER`.
func (p *litParser) parseEval() (*litNode, error) {
	node, err := p.expressions("")
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokNewline {
		p.next()
	}
	if p.peek().kind != tokEnd {
		return nil, errLiteral
	}
	return node, nil
}

// expressions parses `expr (',' expr)* [',']`, giving a tuple when a
// comma is present. `closer` is the bracket that ends the sequence.
func (p *litParser) expressions(closer string) (*litNode, error) {
	first, err := p.expr()
	if err != nil {
		return nil, err
	}
	if !p.isOp(",") {
		return first, nil
	}
	elts := []*litNode{first}
	for p.isOp(",") {
		p.next()
		if p.atSequenceEnd(closer) {
			break
		}
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		elts = append(elts, e)
	}
	return &litNode{kind: "tuple", elts: elts}, nil
}

func (p *litParser) atSequenceEnd(closer string) bool {
	t := p.peek()
	if closer == "" {
		return t.kind == tokEnd || t.kind == tokNewline
	}
	return t.kind == tokOp && t.text == closer
}

// expr parses `term (('+'|'-') term)*` (left associative).
func (p *litParser) expr() (*litNode, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.isOp("+") || p.isOp("-") {
		op := p.next().text
		right, err := p.unary()
		if err != nil {
			return nil, err
		}
		left = &litNode{kind: "binop", op: op, elts: []*litNode{left, right}}
	}
	return left, nil
}

func (p *litParser) unary() (*litNode, error) {
	if p.isOp("+") || p.isOp("-") {
		op := p.next().text
		operand, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &litNode{kind: "unary", op: op, elts: []*litNode{operand}}, nil
	}
	return p.atom()
}

func (p *litParser) atom() (*litNode, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return &litNode{kind: "const", value: t.value}, nil
	case tokString:
		return p.strings(t)
	case tokName:
		switch t.text {
		case "True":
			return &litNode{kind: "const", value: true}, nil
		case "False":
			return &litNode{kind: "const", value: false}, nil
		case "None":
			return &litNode{kind: "const", value: nil}, nil
		}
		if p.isOp("(") {
			p.next()
			if !p.isOp(")") {
				return nil, errLiteral
			}
			p.next()
			return &litNode{kind: "call", value: t.text}, nil
		}
		return &litNode{kind: "name", value: t.text}, nil
	case tokOp:
		switch t.text {
		case "(":
			if p.isOp(")") {
				p.next()
				return &litNode{kind: "tuple"}, nil
			}
			node, err := p.expressions(")")
			if err != nil {
				return nil, err
			}
			if !p.isOp(")") {
				return nil, errLiteral
			}
			p.next()
			return node, nil
		case "[":
			elts, err := p.sequence("]")
			if err != nil {
				return nil, err
			}
			return &litNode{kind: "list", elts: elts}, nil
		case "{":
			return p.braces()
		}
	}
	return nil, errLiteral
}

// strings concatenates adjacent string literals.
func (p *litParser) strings(first token) (*litNode, error) {
	parts := []token{first}
	for p.peek().kind == tokString {
		parts = append(parts, p.next())
	}
	isBytes := first.bytes
	fstr := false
	var b strings.Builder
	for _, part := range parts {
		if part.bytes != isBytes {
			// cannot mix bytes and nonbytes literals
			return nil, errLiteral
		}
		fstr = fstr || part.fstr
		switch v := part.value.(type) {
		case string:
			b.WriteString(v)
		case []byte:
			b.Write(v)
		}
	}
	if isBytes {
		return &litNode{kind: "const", value: []byte(b.String())}, nil
	}
	return &litNode{kind: "const", value: b.String(), fstr: fstr}, nil
}

// sequence parses `[expr (',' expr)* [',']] closer`.
func (p *litParser) sequence(closer string) ([]*litNode, error) {
	elts := []*litNode{}
	for !p.isOp(closer) {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		elts = append(elts, e)
		if p.isOp(",") {
			p.next()
			continue
		}
		if !p.isOp(closer) {
			return nil, errLiteral
		}
	}
	p.next()
	return elts, nil
}

// braces parses a dict or a set display.
func (p *litParser) braces() (*litNode, error) {
	if p.isOp("}") {
		p.next()
		return &litNode{kind: "dict"}, nil
	}
	first, err := p.expr()
	if err != nil {
		return nil, err
	}
	if !p.isOp(":") {
		// Set display
		elts := []*litNode{first}
		for p.isOp(",") {
			p.next()
			if p.isOp("}") {
				break
			}
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			elts = append(elts, e)
		}
		if !p.isOp("}") {
			return nil, errLiteral
		}
		p.next()
		return &litNode{kind: "set", elts: elts}, nil
	}

	node := &litNode{kind: "dict"}
	key := first
	for {
		if !p.isOp(":") {
			return nil, errLiteral
		}
		p.next()
		value, err := p.expr()
		if err != nil {
			return nil, err
		}
		node.keys = append(node.keys, key)
		node.elts = append(node.elts, value)
		if p.isOp("}") {
			p.next()
			return node, nil
		}
		if !p.isOp(",") {
			return nil, errLiteral
		}
		p.next()
		if p.isOp("}") {
			p.next()
			return node, nil
		}
		if key, err = p.expr(); err != nil {
			return nil, err
		}
	}
}

// literalEval mirrors `ast.literal_eval(text)`.
func literalEval(text string) (any, error) {
	tokens, err := litTokenize(strings.TrimLeft(text, " \t"))
	if err != nil {
		return nil, err
	}
	p := &litParser{tokens: tokens}
	if p.peek().kind == tokEnd || p.peek().kind == tokNewline {
		// Empty source: invalid syntax
		return nil, errLiteral
	}
	node, err := p.parseEval()
	if err != nil {
		return nil, err
	}
	return litConvert(node)
}

// litConvert mirrors literal_eval's `_convert`.
func litConvert(node *litNode) (any, error) {
	switch node.kind {
	case "const":
		if node.fstr {
			// f-strings are JoinedStr nodes, not constants
			return nil, errLiteral
		}
		return node.value, nil
	case "tuple", "list", "set":
		elts := make([]any, 0, len(node.elts))
		for _, e := range node.elts {
			v, err := litConvert(e)
			if err != nil {
				return nil, err
			}
			elts = append(elts, v)
		}
		switch node.kind {
		case "tuple":
			return Tuple(elts), nil
		case "list":
			return elts, nil
		}
		return newSet(elts)
	case "call":
		if node.value == "set" {
			return Set{}, nil
		}
	case "dict":
		keys := make([]any, len(node.keys))
		values := make([]any, len(node.elts))
		for i := range node.keys {
			k, err := litConvert(node.keys[i])
			if err != nil {
				return nil, err
			}
			v, err := litConvert(node.elts[i])
			if err != nil {
				return nil, err
			}
			keys[i], values[i] = k, v
		}
		return newDict(keys, values)
	case "binop":
		left, err := litSignedNum(node.elts[0])
		if err != nil {
			return nil, err
		}
		right, err := litNum(node.elts[1])
		if err != nil {
			return nil, err
		}
		r, isComplex := right.(complex128)
		_, leftComplex := left.(complex128)
		if isComplex && !leftComplex {
			l := toComplex(left)
			if node.op == "+" {
				return l + r, nil
			}
			return l - r, nil
		}
		return nil, errLiteral
	}
	return litSignedNum(node)
}

func toComplex(v any) complex128 {
	switch n := v.(type) {
	case int64:
		return complex(float64(n), 0)
	case float64:
		return complex(n, 0)
	case complex128:
		return n
	}
	return 0
}

// litNum mirrors `_convert_num`: an int, float or complex constant.
func litNum(node *litNode) (any, error) {
	if node.kind == "const" {
		switch node.value.(type) {
		case int64, float64, complex128:
			return node.value, nil
		}
	}
	return nil, errLiteral
}

// litSignedNum mirrors `_convert_signed_num`.
func litSignedNum(node *litNode) (any, error) {
	if node.kind != "unary" {
		return litNum(node)
	}
	operand, err := litNum(node.elts[0])
	if err != nil {
		return nil, err
	}
	if node.op == "+" {
		return operand, nil
	}
	switch n := operand.(type) {
	case int64:
		if n == math.MinInt64 {
			return -float64(n), nil
		}
		return -n, nil
	case float64:
		return -n, nil
	case complex128:
		return -n, nil
	}
	return nil, errLiteral
}

// hashKey returns a key honoring Python equality (1 == 1.0 == True), or
// false when the value is unhashable.
func hashKey(v any) (string, bool) {
	switch n := v.(type) {
	case nil:
		return "None", true
	case bool:
		if n {
			return "n:1", true
		}
		return "n:0", true
	case int64:
		return "n:" + strconv.FormatInt(n, 10), true
	case float64:
		if n == math.Trunc(n) && math.Abs(n) < 1<<63 {
			return "n:" + strconv.FormatInt(int64(n), 10), true
		}
		return "n:" + strconv.FormatFloat(n, 'g', -1, 64), true
	case complex128:
		if imag(n) == 0 {
			return hashKey(real(n))
		}
		return "c:" + strconv.FormatComplex(n, 'g', -1, 128), true
	case string:
		return "s:" + n, true
	case []byte:
		return "b:" + string(n), true
	case Tuple:
		parts := make([]string, len(n))
		for i, e := range n {
			k, ok := hashKey(e)
			if !ok {
				return "", false
			}
			parts[i] = strconv.Quote(k)
		}
		return "t:" + strings.Join(parts, ","), true
	}
	return "", false
}

func newSet(elts []any) (any, error) {
	seen := map[string]bool{}
	result := Set{}
	for _, e := range elts {
		k, ok := hashKey(e)
		if !ok {
			return nil, errLiteral
		}
		if !seen[k] {
			seen[k] = true
			result = append(result, e)
		}
	}
	return result, nil
}

func newDict(keys, values []any) (any, error) {
	allStrings := true
	for _, k := range keys {
		if _, ok := k.(string); !ok {
			allStrings = false
		}
	}
	if allStrings {
		result := make(map[string]any, len(keys))
		for i, k := range keys {
			result[k.(string)] = values[i]
		}
		return result, nil
	}

	// Python keeps the first key object and the last value
	result := make(map[any]any, len(keys))
	first := map[string]any{}
	for i, k := range keys {
		hk, ok := hashKey(k)
		if !ok {
			return nil, errLiteral
		}
		switch k.(type) {
		case []byte, Tuple:
			// Not usable as Go map keys
			return nil, errLiteral
		}
		if prev, ok := first[hk]; ok {
			k = prev
		} else {
			first[hk] = k
		}
		result[k] = values[i]
	}
	return result, nil
}
