// helloworld: ソースファイルを読むだけで、そのプログラムが "HelloWorld!" だけを出力するかを判定する。
//
// 言語ごとに「確実に HelloWorld! だけを出力するプログラムの形」を表に書いておき、
// ファイルがその形に丸ごと一致したときだけ合格にする。一致しないものは全部エラーにするので、
// 合格と言ったものは本当にそう出力する。Brainfuck 系だけは表の代わりに中で実際に評価する。
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const want = "HelloWorld!"

// 言語はコードではなくデータ。langs.txt は手で書いた表、tio.txt は gen が作った表。
//
//go:embed langs.txt
var hand string

//go:embed tio.txt
var tio string

func main() {
	if n := len(os.Args); n < 2 || n > 3 {
		fmt.Fprintln(os.Stderr, "usage: helloworld <file> [language]")
		os.Exit(2)
	}
	path := os.Args[1]
	lang := strings.TrimPrefix(filepath.Ext(path), ".")
	if len(os.Args) == 3 {
		lang = os.Args[2]
	}
	src, err := os.ReadFile(path)
	if err == nil {
		err = judge(string(src), lang, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(want)
}

// block は表の 1 言語分。見出しは "== 名前 | 拡張子... | 旗... | 行コメントの頭..."。
type block struct {
	name                  string
	exts, flags, comments []string
	body                  string
}

func blocks() []block {
	var bs []block
	for _, b := range strings.Split("\n"+hand+"\n"+tio, "\n== ")[1:] {
		head, body, _ := strings.Cut(b, "\n")
		f := append(strings.Split(head, " | "), "", "", "")
		bs = append(bs, block{f[0], strings.Fields(f[1]), strings.Fields(f[2]), strings.Fields(f[3]), body})
	}
	return bs
}

func judge(src, lang, base string) error {
	src = strings.ReplaceAll(strings.TrimPrefix(src, "\xef\xbb\xbf"), "\r\n", "\n")
	var names []string
	for _, b := range blocks() {
		if !slices.Contains(b.exts, lang) {
			continue
		}
		names = append(names, b.name)
		if bf := slices.Contains(b.flags, "bf"); bf || slices.Contains(b.flags, "bfw") {
			if out, ok := brainfuck(strings.Fields(b.body), src, !bf); ok && printsWant(out) {
				return nil
			}
			continue
		}
		s := stripComments(src, b.comments)
		for _, p := range patterns(b) {
			if regexp.MustCompile(compile(p, b.flags, base)).MatchString(s) {
				return nil
			}
		}
	}
	if names == nil {
		return fmt.Errorf("unknown language %q (give it as the 2nd argument)", lang)
	}
	return fmt.Errorf("not a program that prints exactly %s (checked as %s)", want, strings.Join(names, ", "))
}

func printsWant(out string) bool {
	return out == want || out == want+"\n" || out == want+"\r\n"
}

// patterns は 1 言語の型を返す。型どうしは "｜" だけの行で区切る。
func patterns(b block) []string {
	body := strings.TrimRight(b.body, "\n")
	switch {
	case slices.Contains(b.flags, "indent"):
		body = strings.TrimLeft(body, "\n") // 1 行目の字下げも意味を持つ
	case !slices.Contains(b.flags, "exact"):
		body = strings.TrimSpace(body)
	}
	return strings.Split(body, "\n｜\n")
}

// stripComments は行コメントだけの行を消す。"\" で終わる行は次の行まで続くことがあるので消さない。
func stripComments(s string, prefixes []string) string {
	if len(prefixes) == 0 {
		return s
	}
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimLeft(l, " \t")
		if strings.HasSuffix(l, `\`) || !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(t, p) }) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// compile は型を正規表現にする。記号は次のとおりで、それ以外の文字はそのままの意味。
//
//	⟨a｜b⟩ どれか 1 つ   ⟦a⟧ 省略可   ◇ 識別子   ◆ ファイル名(拡張子なし)   ␣ ちょうど 1 つの空白
//	空白 何文字でもよい空白。語と語の間では 1 文字以上
//	改行 1 つ以上の改行(行頭の字下げは自由)
//
// 旗 free は改行も空白とみなし、indent は字下げの深さを型以上に保ち、exact は空白を一切ゆるめない。
// 旗 i は大文字小文字を区別しない。ただし "HelloWorld!" の部分だけは常に区別する。
func compile(p string, flags []string, base string) string {
	has := func(f string) bool { return slices.Contains(flags, f) }
	sp := `[ \t]`
	if has("free") {
		sp = `\s`
	}
	var re strings.Builder
	if has("i") {
		re.WriteString("(?i)")
	}
	switch {
	case has("exact"):
		re.WriteString("^")
	case has("free"):
		re.WriteString(`^\s*`)
	default:
		re.WriteString(`^(?:[ \t]*\n)*`)
	}
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case strings.HasPrefix(string(rs[i:]), want):
			re.WriteString("(?-i:" + regexp.QuoteMeta(want) + ")")
			i += len(want) - 1
		case r == '⟨' || r == '⟦':
			re.WriteString("(?:")
		case r == '⟩':
			re.WriteString(")")
		case r == '⟧':
			re.WriteString(")?")
		case r == '｜':
			re.WriteString("|")
		case r == '◇':
			re.WriteString(`[A-Za-z_]\w*`)
		case r == '◆':
			re.WriteString(regexp.QuoteMeta(base))
		case r == '␣':
			re.WriteString(" ")
		case unicode.IsSpace(r) && !has("exact"):
			j := i
			for j < len(rs) && unicode.IsSpace(rs[j]) {
				j++
			}
			ws := string(rs[i:j])
			var prev, next rune
			if i > 0 {
				prev = rs[i-1]
			}
			if j < len(rs) {
				next = rs[j]
			}
			nl := strings.LastIndex(ws, "\n")
			switch {
			case i == 0 && has("indent"):
				fmt.Fprintf(&re, `(?:\t+| {%d,})`, len(ws))
			case nl >= 0 && has("indent") && len(ws)-nl > 1:
				fmt.Fprintf(&re, `[ \t]*\n(?:[ \t]*\n)*(?:\t+| {%d,})`, len(ws)-nl-1)
			case nl >= 0 && has("indent"):
				re.WriteString(`[ \t]*\n(?:[ \t]*\n)*`)
			case nl >= 0:
				re.WriteString(`\s*\n\s*`)
			case has("words") || joins(prev, next) || joins(next, prev):
				re.WriteString(sp + "+")
			default:
				re.WriteString(sp + "*")
			}
			i = j - 1
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if has("exact") {
		re.WriteString(`\n?$`)
	} else {
		re.WriteString(`\s*$`)
	}
	return re.String()
}

// joins は a と b の間の空白を消すと 1 つの語にくっついてしまうかを返す(print"x" も避ける)。
func joins(a, b rune) bool {
	w := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	return w(a) && (w(b) || b == '"' || b == '\'')
}

// brainfuck は Brainfuck 系の言語を実際に評価する。ops は > < + - . , [ ] に当たる 8 つの語。
// words なら空白で区切った語ごとに、そうでなければ空白を除いて最長一致で命令を読む。
// 入力を読む , と、1000 万歩で終わらないものは、出力が決まらないので失敗にする。
func brainfuck(ops []string, src string, words bool) (string, bool) {
	if len(ops) != 8 {
		return "", false
	}
	var code []byte
	if words {
		for _, w := range strings.Fields(src) {
			if k := slices.Index(ops, w); k >= 0 {
				code = append(code, "><+-.,[]"[k])
			}
		}
		src = ""
	}
	src = strings.Join(strings.Fields(src), "")
	for i := 0; i < len(src); {
		best := -1
		for k, op := range ops {
			if strings.HasPrefix(src[i:], op) && (best < 0 || len(op) > len(ops[best])) {
				best = k
			}
		}
		if best < 0 {
			i++ // 命令でない文字は注釈
			continue
		}
		code = append(code, "><+-.,[]"[best])
		i += len(ops[best])
	}
	jump := make([]int, len(code))
	var open []int
	for i, c := range code {
		switch c {
		case '[':
			open = append(open, i)
		case ']':
			if len(open) == 0 {
				return "", false
			}
			j := open[len(open)-1]
			open = open[:len(open)-1]
			jump[i], jump[j] = j, i
		}
	}
	if len(open) > 0 {
		return "", false
	}
	var tape [1 << 16]byte
	var p uint16 // 0 の左は端に回り込む
	var out []byte
	for pc, steps := 0, 0; pc < len(code); pc, steps = pc+1, steps+1 {
		if steps > 1e7 || len(out) > len(want)+2 {
			return "", false
		}
		switch code[pc] {
		case '>':
			p++
		case '<':
			p--
		case '+':
			tape[p]++
		case '-':
			tape[p]--
		case '.':
			out = append(out, tape[p])
		case ',':
			return "", false
		case '[':
			if tape[p] == 0 {
				pc = jump[pc]
			}
		case ']':
			if tape[p] != 0 {
				pc = jump[pc]
			}
		}
	}
	return string(out), true
}
