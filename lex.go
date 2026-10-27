package main

// 字句の切り出し。言語ごとの違い (コメント、引用符、改行や字下げの意味) は lexer の設定で表す。

import (
	"strconv"
	"strings"
	"unicode"
)

type tok struct {
	kind  byte   // 'n' 数, 's' 文字列, 'i' 名前, 'o' 記号, ';' 改行, '>' 字下げ, '<' 字戻し, 0 終わり
	s     string // 名前や記号の文字。文字列なら中身
	parts []part // 埋め込み式を持つ文字列の部品。持たなければ nil
	num   any
	sp    bool // 直前に空白があった
	nl    bool // 直前に改行があった
}

type part struct {
	s    string
	expr bool // s が式の源 ("${...}" の中身など)
}

type lexer struct {
	comments []string        // 行コメントの頭
	blocks   [][2]string     // 範囲コメントの頭と尻
	quotes   string          // エスケープを解く引用符
	raws     string          // エスケープを解かない引用符 (\\ と \引用符 だけ解く)
	interp   map[byte]string // 引用符ごとの式埋め込みの頭 ("${", "#{")
	newlines bool            // 改行を文の区切りとして返す
	indent   bool            // 字下げを返す (Python)
	pyPrefix bool            // r"..." f"..." のような Python の接頭辞
	ident    string          // 名前に使える追加の文字
}

var ops = strings.Fields(`... === !== **= //= >>= <<= == != <= >= && || ++ -- += -= *= /= %= -> => :: .. ** // << >> ?. ?? ~= + - * / % = < > ! & | ^ ~ ? : . , ; ( ) [ ] { } @ # $ \`)

func (lx *lexer) lex(src string) []tok {
	var ts []tok
	depth := 0
	levels := []int{0}
	atLine := true
	i, sp, nl := 0, false, false
	emit := func(t tok) {
		t.sp, t.nl = sp, nl
		ts = append(ts, t)
		sp, nl = false, false
		if t.kind != '>' && t.kind != '<' {
			atLine = false
		}
	}
	for i < len(src) {
		c := src[i]
		if atLine && lx.indent && depth == 0 {
			j := i
			for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
				j++
			}
			if j < len(src) && src[j] != '\n' && !lx.isComment(src[j:]) {
				w := j - i
				if strings.Contains(src[i:j], "\t") && strings.Contains(src[i:j], " ") {
					failf("mixed tabs and spaces")
				}
				switch top := levels[len(levels)-1]; {
				case w > top:
					levels = append(levels, w)
					emit(tok{kind: '>'})
				case w < top:
					for w < levels[len(levels)-1] {
						levels = levels[:len(levels)-1]
						emit(tok{kind: '<'})
					}
					if w != levels[len(levels)-1] {
						failf("bad indentation")
					}
				}
				atLine = false
			}
			i = j
			if i >= len(src) {
				break
			}
			c = src[i]
		}
		switch {
		case c == '\n':
			if lx.newlines && depth == 0 && len(ts) > 0 && ts[len(ts)-1].kind != ';' {
				emit(tok{kind: ';', s: "\n"})
			}
			atLine, sp, nl = true, true, true
			i++
		case c == ' ' || c == '\t' || c == '\r':
			sp = true
			i++
		case lx.isComment(src[i:]):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case lx.blockAt(src[i:]) >= 0:
			b := lx.blocks[lx.blockAt(src[i:])]
			end := strings.Index(src[i+len(b[0]):], b[1])
			if end < 0 {
				failf("unterminated comment")
			}
			i += len(b[0]) + end + len(b[1])
			sp = true
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' && (len(ts) == 0 || ts[len(ts)-1].kind != 'i'):
			j := i
			for j < len(src) && (isWord(src[j]) || src[j] == '.' && j+1 < len(src) && src[j+1] >= '0' && src[j+1] <= '9') {
				j++
			}
			emit(tok{kind: 'n', s: src[i:j], num: parseNum(src[i:j])})
			i = j
		case lx.pyPrefix && strings.ContainsRune("rRfFbBuU", rune(c)) && lx.prefixLen(src[i:]) > 0:
			n := lx.prefixLen(src[i:])
			pre := strings.ToLower(src[i : i+n])
			if strings.Contains(pre, "b") {
				failf("bytes literals are not supported")
			}
			t, j := lx.str(src, i+n, !strings.Contains(pre, "r"), strings.Contains(pre, "f"))
			emit(t)
			i = j
		case isWord(c) || c >= 0x80 || strings.IndexByte(lx.ident, c) >= 0:
			j := i
			for j < len(src) && (isWord(src[j]) || src[j] >= 0x80 || strings.IndexByte(lx.ident, src[j]) >= 0) {
				j++
			}
			emit(tok{kind: 'i', s: src[i:j]})
			i = j
		case strings.IndexByte(lx.quotes, c) >= 0 || strings.IndexByte(lx.raws, c) >= 0:
			t, j := lx.str(src, i, strings.IndexByte(lx.quotes, c) >= 0, false)
			emit(t)
			i = j
		default:
			op := ""
			for _, o := range ops {
				if strings.HasPrefix(src[i:], o) {
					op = o
					break
				}
			}
			if op == "" {
				failf("unexpected character %q", c)
			}
			switch op {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			}
			emit(tok{kind: 'o', s: op})
			i += len(op)
		}
	}
	if lx.newlines {
		emit(tok{kind: ';', s: "\n"})
	}
	for len(levels) > 1 {
		levels = levels[:len(levels)-1]
		emit(tok{kind: '<'})
	}
	return append(ts, tok{})
}

func isWord(c byte) bool {
	return c == '_' || c < 0x80 && (unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)))
}

func (lx *lexer) isComment(s string) bool {
	for _, c := range lx.comments {
		if strings.HasPrefix(s, c) {
			return true
		}
	}
	return false
}

func (lx *lexer) blockAt(s string) int {
	for k, b := range lx.blocks {
		if strings.HasPrefix(s, b[0]) {
			return k
		}
	}
	return -1
}

func (lx *lexer) prefixLen(s string) int {
	for n := 1; n <= 2 && n < len(s); n++ {
		if strings.ContainsRune(`"'`, rune(s[n])) && strings.Trim(strings.ToLower(s[:n]), "rfbu") == "" {
			return n
		}
		if !strings.ContainsRune("rRfFbBuU", rune(s[n])) {
			return 0
		}
	}
	return 0
}

// str は i にある引用符から文字列を 1 つ読む。三重引用符にも対応する。
func (lx *lexer) str(src string, i int, escapes, fmtStr bool) (tok, int) {
	q := src[i : i+1]
	if lx.pyPrefix && strings.HasPrefix(src[i:], q+q+q) {
		q = q + q + q
	}
	i += len(q)
	open := lx.interp[q[0]]
	if fmtStr {
		open = "{"
	}
	var parts []part
	var b strings.Builder
	for {
		if i >= len(src) || len(q) == 1 && src[i] == '\n' && q != "`" {
			failf("unterminated string")
		}
		switch {
		case strings.HasPrefix(src[i:], q):
			parts = append(parts, part{s: b.String()})
			t := tok{kind: 's', s: b.String()}
			if len(parts) > 1 || parts[0].expr {
				t.parts = parts
			}
			return t, i + len(q)
		case fmtStr && (strings.HasPrefix(src[i:], "{{") || strings.HasPrefix(src[i:], "}}")):
			b.WriteByte(src[i])
			i += 2
		case open != "" && strings.HasPrefix(src[i:], open):
			parts = append(parts, part{s: b.String()})
			b.Reset()
			j, depth := i+len(open), 1
			for ; j < len(src) && depth > 0; j++ {
				switch src[j] {
				case '{':
					depth++
				case '}':
					depth--
				case '\n':
					if q != "`" {
						failf("unterminated string")
					}
				}
			}
			if depth > 0 {
				failf("unterminated string")
			}
			parts = append(parts, part{s: src[i+len(open) : j-1], expr: true})
			i = j
		case src[i] == '\\' && i+1 < len(src):
			if !escapes {
				if src[i+1] == '\\' || src[i+1] == q[0] {
					b.WriteByte(src[i+1])
					i += 2
				} else {
					b.WriteByte('\\')
					i++
				}
				continue
			}
			s, n := unescape(src[i:])
			b.WriteString(s)
			i += n
		default:
			b.WriteByte(src[i])
			i++
		}
	}
}

// unescape は "\n" のような逆斜線の並びを 1 つ解き、文字と読んだ長さを返す。
func unescape(s string) (string, int) {
	switch c := s[1]; c {
	case 'n':
		return "\n", 2
	case 't':
		return "\t", 2
	case 'r':
		return "\r", 2
	case '0':
		return "\x00", 2
	case 'a':
		return "\a", 2
	case 'b':
		return "\b", 2
	case 'f':
		return "\f", 2
	case 'v':
		return "\v", 2
	case 'e':
		return "\x1b", 2
	case 'x', 'u':
		n := map[byte]int{'x': 2, 'u': 4}[c]
		if len(s) >= 2+n {
			if v, err := strconv.ParseUint(s[2:2+n], 16, 32); err == nil {
				return string(rune(v)), 2 + n
			}
		}
		failf("bad escape %q", s[:2])
	case '\n':
		return "", 2
	case '\\', '\'', '"', '`', '$', '/', '?', '{', '}', '#':
		return s[1:2], 2
	}
	failf("unknown escape %q", s[:2])
	return "", 0
}

func parseNum(s string) any {
	t := strings.ReplaceAll(s, "_", "")
	if v, err := strconv.ParseInt(t, 0, 64); err == nil {
		return v
	}
	if v, err := strconv.ParseFloat(t, 64); err == nil {
		return v
	}
	failf("bad number %q", s)
	return nil
}
