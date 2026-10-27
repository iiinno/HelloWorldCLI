package main

// 意味の小さい言語は、言語ごとに丸ごと解釈する。

import (
	"strconv"
	"strings"
)

func init() {
	// Brainfuck と、その命令を別の語に置き換えただけの言語。語は > < + - . , [ ] の順。
	bfs := []struct {
		words bool // 空白で区切った語ごとに読む
		ops   string
		exts  []string
	}{
		{false, "> < + - . , [ ]", []string{"bf", "b", "brainfuck"}},
		{false, "Ook.Ook? Ook?Ook. Ook.Ook. Ook!Ook! Ook!Ook. Ook.Ook! Ook!Ook? Ook?Ook!", []string{"ook"}},
		{false, "Blub.Blub? Blub?Blub. Blub.Blub. Blub!Blub! Blub!Blub. Blub.Blub! Blub!Blub? Blub?Blub!", []string{"blub"}},
		{false, "a c e i j o p s", []string{"alphuck"}},
		{false, "010 011 1 000 001010 0010110 00100 0011", []string{"spoon"}},
		{true, "pipi pichu pi ka pikachu pikapi pika chu", []string{"pokeball", "pikalang"}},
	}
	for _, b := range bfs {
		ops, words := strings.Fields(b.ops), b.words
		register(func(src string) error { return run(func() { brainfuck(ops, src, words) }) }, b.exts...)
	}
	register(func(src string) error { return run(func() { befunge(src) }) }, "b93", "befunge")
	register(func(src string) error { return run(func() { slashes(src) }) }, "slashes", "///")
	register(func(src string) error { return run(func() { deadfish(src) }) }, "dfish", "deadfish")
	register(func(src string) error { return run(func() { hq9(src) }) }, "hq9", "hq9+")
}

// brainfuck は Brainfuck 系の言語を評価する。words なら空白で区切った語ごとに、
// そうでなければ空白を除いて最長一致で命令を読む。入力を読む , は、入力が空なので失敗にする。
func brainfuck(ops []string, src string, words bool) {
	var code []byte
	if words {
		for _, w := range strings.Fields(src) {
			for k, op := range ops {
				if w == op {
					code = append(code, "><+-.,[]"[k])
				}
			}
		}
	} else {
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
	}
	jump := make([]int, len(code))
	var open []int
	for i, c := range code {
		switch c {
		case '[':
			open = append(open, i)
		case ']':
			if len(open) == 0 {
				failf("unmatched ]")
			}
			j := open[len(open)-1]
			open = open[:len(open)-1]
			jump[i], jump[j] = j, i
		}
	}
	if len(open) > 0 {
		failf("unmatched [")
	}
	var tape [1 << 16]byte
	var p uint16 // 0 の左は端に回り込む
	for pc := 0; pc < len(code); pc++ {
		tick()
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
			write(string([]byte{tape[p]}))
		case ',':
			failf("reads input")
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
}

// befunge は Befunge-93 を評価する。80x25 の盤面を、上下左右に回り込みながら進む。
func befunge(src string) {
	var g [25][80]byte
	for y, line := range strings.Split(src, "\n") {
		for x := 0; x < len(line) && x < 80 && y < 25; x++ {
			g[y][x] = line[x]
		}
	}
	for y := range g {
		for x := range g[y] {
			if g[y][x] == 0 {
				g[y][x] = ' '
			}
		}
	}
	var st []int64
	pop := func() int64 {
		if len(st) == 0 {
			return 0
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	push := func(v int64) { st = append(st, v) }
	x, y, dx, dy, strMode := 0, 0, 1, 0, false
	for {
		tick()
		c := g[y][x]
		if strMode && c != '"' {
			push(int64(c))
		} else {
			switch c {
			case '"':
				strMode = !strMode
			case '+', '-', '*', '/', '%', '`':
				b, a := pop(), pop()
				switch c {
				case '+':
					push(a + b)
				case '-':
					push(a - b)
				case '*':
					push(a * b)
				case '/', '%':
					if b == 0 {
						failf("division by zero")
					}
					if c == '/' {
						push(a / b)
					} else {
						push(a % b)
					}
				case '`':
					push(map[bool]int64{true: 1}[a > b])
				}
			case '!':
				push(map[bool]int64{true: 1}[pop() == 0])
			case '>':
				dx, dy = 1, 0
			case '<':
				dx, dy = -1, 0
			case '^':
				dx, dy = 0, -1
			case 'v':
				dx, dy = 0, 1
			case '?':
				failf("? moves at random, so the output is not fixed")
			case '_':
				if pop() == 0 {
					dx, dy = 1, 0
				} else {
					dx, dy = -1, 0
				}
			case '|':
				if pop() == 0 {
					dx, dy = 0, 1
				} else {
					dx, dy = 0, -1
				}
			case ':':
				v := pop()
				push(v)
				push(v)
			case '\\':
				b, a := pop(), pop()
				push(b)
				push(a)
			case '$':
				pop()
			case '.':
				write(strconv.FormatInt(pop(), 10) + " ")
			case ',':
				write(string([]byte{byte(pop())}))
			case '#':
				x, y = (x+dx+80)%80, (y+dy+25)%25
			case 'p':
				py, px, v := pop(), pop(), pop()
				if px < 0 || px >= 80 || py < 0 || py >= 25 {
					failf("p outside the board")
				}
				g[py][px] = byte(v)
			case 'g':
				py, px := pop(), pop()
				if px < 0 || px >= 80 || py < 0 || py >= 25 {
					push(0)
				} else {
					push(int64(g[py][px]))
				}
			case '&', '~':
				failf("reads input")
			case '@':
				return
			case ' ':
			default:
				if c >= '0' && c <= '9' {
					push(int64(c - '0'))
				} else {
					failf("unknown instruction %q", c)
				}
			}
		}
		x, y = (x+dx+80)%80, (y+dy+25)%25
	}
}

// slashes は /// を評価する。/型/置換/ を見つけると、残りの中の型を置換がなくなるまで置き換える。
func slashes(p string) {
	for p != "" {
		tick()
		switch p[0] {
		case '\\':
			if len(p) < 2 {
				return
			}
			write(p[1:2])
			p = p[2:]
		case '/':
			pat, rest, ok := slashPart(p[1:])
			if !ok {
				return
			}
			rep, rest, ok := slashPart(rest)
			if !ok {
				return
			}
			for strings.Contains(rest, pat) {
				tick()
				if pat == "" {
					failf("empty pattern never stops")
				}
				rest = strings.Replace(rest, pat, rep, 1)
			}
			p = rest
		default:
			write(p[:1])
			p = p[1:]
		}
	}
}

func slashPart(s string) (string, string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			}
		case '/':
			return b.String(), s[i+1:], true
		default:
			b.WriteByte(s[i])
		}
	}
	return "", "", false
}

// deadfish は数しか出力できないので、どう書いても HelloWorld! にはならない。それでも正しく評価して答える。
func deadfish(src string) {
	var a int64
	for _, c := range src {
		tick()
		if a == -1 || a == 256 {
			a = 0
		}
		switch c {
		case 'i':
			a++
		case 'd':
			a--
		case 's':
			a *= a
		case 'o':
			write(strconv.FormatInt(a, 10) + "\n")
		}
	}
}

// hq9 は HQ9+ を評価する。H は "Hello, world!"、Q は自分のソース、9 は 99 Bottles を出す。
func hq9(src string) {
	for _, c := range src {
		tick()
		switch c {
		case 'H', 'h':
			write("Hello, world!\n")
		case 'Q', 'q':
			write(src)
		case '9':
			write("99 bottles of beer on the wall")
		} // + は見えない数を 1 増やすだけで、ほかの文字は無視される

	}
}
