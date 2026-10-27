package main

// 式の読み取り。演算子の強さと意味は grammar で言語ごとに渡し、読み取りの手順は全言語で共有する。

type grammar struct {
	bin     map[string]int // 2 項演算子の強さ。大きいほど先に結びつく
	right   map[string]bool
	prefix  map[string]int // 前置演算子と、その後ろを読む強さ
	binop   func(op string, a, b any) any
	unop    func(op string, a any) any
	truthy  func(any) bool
	atom    func(p *parser) code // 言語固有の項。扱わなければ nil
	member  func(obj any, name string) any
	index   func(obj, i any) any
	setIdx  func(obj, i, v any)
	slice   func(obj any, lo, hi, step any) any // nil なら切り出しは文法にない
	kwargs  bool
	comp    bool // Python の内包表記 [x for x in xs]
	and, or []string
}

type parser struct {
	ts []tok
	p  int
	g  *grammar
	lx *lexer
}

func (p *parser) peek() tok { return p.ts[p.p] }
func (p *parser) next() tok { t := p.ts[p.p]; p.p++; return t }
func (p *parser) is(s string) bool {
	t := p.peek()
	return (t.kind == 'o' || t.kind == 'i') && t.s == s
}
func (p *parser) accept(s string) bool {
	if p.is(s) {
		p.p++
		return true
	}
	return false
}
func (p *parser) expect(s string) {
	if !p.accept(s) {
		failf("expected %q but found %q", s, p.peek().s)
	}
}
func (p *parser) name() string {
	t := p.next()
	if t.kind != 'i' {
		failf("expected a name but found %q", t.s)
	}
	return t.s
}
func (p *parser) skipNewlines() {
	for p.peek().kind == ';' {
		p.p++
	}
}

func (p *parser) expr() code { return p.binary(0) }

func (p *parser) binary(min int) code {
	var left code
	if t := p.peek(); (t.kind == 'o' || t.kind == 'i') && p.g.prefix[t.s] > 0 && p.g.prefix[t.s] >= min {
		p.next()
		x, op, un := p.binary(p.g.prefix[t.s]), t.s, p.g.unop
		left = func(e *env) any { tick(); return un(op, x(e)) }
	} else {
		left = p.postfix(p.primary())
	}
	for {
		t := p.peek()
		prec, ok := p.g.bin[t.s]
		if !ok || prec < min || t.kind != 'o' && t.kind != 'i' {
			return left
		}
		p.next()
		op := t.s
		if op == "not" { // Python の not in
			p.expect("in")
			op = "not in"
		}
		if op == "is" && p.accept("not") {
			op = "is not"
		}
		next := prec + 1
		if p.g.right[op] {
			next = prec
		}
		l, truthy := left, p.g.truthy
		switch {
		case contains(p.g.and, op):
			r := p.binary(next)
			left = func(e *env) any {
				if v := l(e); !truthy(v) {
					return v
				}
				return r(e)
			}
		case contains(p.g.or, op):
			r := p.binary(next)
			left = func(e *env) any {
				if v := l(e); truthy(v) {
					return v
				}
				return r(e)
			}
		case op == "?": // c ? a : b
			a := p.binary(0)
			p.expect(":")
			b := p.binary(prec)
			left = func(e *env) any {
				if truthy(l(e)) {
					return a(e)
				}
				return b(e)
			}
		case op == "if": // a if c else b
			c := p.binary(next)
			p.expect("else")
			b := p.binary(prec)
			left = func(e *env) any {
				if truthy(c(e)) {
					return l(e)
				}
				return b(e)
			}
		default:
			r, f := p.binary(next), p.g.binop
			left = func(e *env) any { tick(); return f(op, l(e), r(e)) }
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func (p *parser) primary() code {
	if p.g.atom != nil {
		if c := p.g.atom(p); c != nil {
			return c
		}
	}
	t := p.next()
	switch t.kind {
	case 'n':
		return lit(t.num)
	case 's':
		x := p.str(t)
		for p.g.comp && p.peek().kind == 's' { // "a" "b" は 1 つの文字列
			a, b := x, p.str(p.next())
			x = func(e *env) any { return toStr(a(e)) + toStr(b(e)) }
		}
		return x
	case 'i':
		return ref(t.s)
	}
	switch t.s {
	case "(":
		if p.accept(")") {
			return lit(&list{})
		}
		x := p.expr()
		if p.g.comp && p.is("for") {
			return p.comprehension(x, ")")
		}
		if p.is(",") { // (a, b) は組。リストとして扱う
			xs := []code{x}
			for p.accept(",") && !p.is(")") {
				xs = append(xs, p.expr())
			}
			p.expect(")")
			return listOf(xs)
		}
		p.expect(")")
		return x
	case "[":
		if p.g.comp && !p.is("]") {
			start := p.p
			x := p.expr()
			if p.is("for") {
				return p.comprehension(x, "]")
			}
			p.p = start
		}
		xs := p.items("]")
		return listOf(xs)
	}
	failf("unexpected %q", t.s)
	return nil
}

// comprehension は "for 名前 in 式 [if 式]" を読み、elem を集めたリストを作る。
func (p *parser) comprehension(elem code, end string) code {
	p.expect("for")
	n := p.name()
	p.expect("in")
	it := p.binary(1)
	var cond code
	if p.accept("if") {
		cond = p.binary(1)
	}
	p.expect(end)
	truthy := p.g.truthy
	return func(e *env) any {
		l := &list{}
		in := newEnv(e)
		for _, v := range pyIter(it(e)) {
			tick()
			in.vars[n] = v
			if cond == nil || truthy(cond(in)) {
				l.v = append(l.v, elem(in))
			}
		}
		return l
	}
}

func listOf(xs []code) code {
	return func(e *env) any {
		l := &list{}
		for _, x := range xs {
			l.v = append(l.v, x(e))
		}
		return l
	}
}

// items は "," で区切った式を閉じ記号まで読む。
func (p *parser) items(end string) []code {
	var xs []code
	p.skipNewlines()
	for !p.accept(end) {
		xs = append(xs, p.expr())
		p.skipNewlines()
		if !p.accept(",") {
			p.skipNewlines()
			p.expect(end)
			break
		}
		p.skipNewlines()
	}
	return xs
}

// str は文字列の字句を式にする。埋め込み式は同じ言語の式として読み直す。
func (p *parser) str(t tok) code {
	if t.parts == nil {
		return lit(t.s)
	}
	var cs []code
	for _, pt := range t.parts {
		if !pt.expr {
			cs = append(cs, lit(pt.s))
			continue
		}
		sub := &parser{ts: p.lx.lex(pt.s), g: p.g, lx: p.lx}
		x := sub.expr()
		if sub.peek().kind != 0 && sub.peek().kind != ';' {
			failf("unsupported expression in string: %q", pt.s)
		}
		cs = append(cs, x)
	}
	return func(e *env) any {
		var s string
		for _, c := range cs {
			s += toStr(c(e))
		}
		return s
	}
}

// toStr は文字列への埋め込みで使う変換。言語ごとに読み取り部が差し替える。
var toStr func(any) string

func (p *parser) postfix(x code) code {
	for {
		t := p.peek()
		if t.kind != 'o' {
			return x
		}
		switch t.s {
		case "(":
			p.next()
			args, kw := p.args(")")
			f := x
			x = func(e *env) any {
				vs := make([]any, len(args))
				for i, a := range args {
					vs[i] = a(e)
				}
				ks := map[string]any{}
				for k, c := range kw {
					ks[k] = c(e)
				}
				return call(f(e), vs, ks)
			}
		case ".":
			p.next()
			n, o, m := p.name(), x, p.g.member
			x = func(e *env) any { tick(); return m(o(e), n) }
		case "[":
			p.next()
			x = p.subscript(x)
		default:
			return x
		}
	}
}

func (p *parser) subscript(o code) code {
	var lo, hi, st code
	isSlice := false
	if p.g.slice != nil && p.accept("::") { // a[::-1]
		if !p.is("]") {
			st = p.expr()
		}
		p.expect("]")
		sl := p.g.slice
		return func(e *env) any {
			tick()
			var s any
			if st != nil {
				s = st(e)
			}
			return sl(o(e), nil, nil, s)
		}
	}
	if !p.is(":") {
		lo = p.expr()
	}
	if p.g.slice != nil && p.accept(":") {
		isSlice = true
		if !p.is(":") && !p.is("]") {
			hi = p.expr()
		}
		if p.accept(":") && !p.is("]") {
			st = p.expr()
		}
	}
	p.expect("]")
	if !isSlice {
		ix := p.g.index
		return func(e *env) any { tick(); return ix(o(e), lo(e)) }
	}
	ev := func(c code, e *env) any {
		if c == nil {
			return nil
		}
		return c(e)
	}
	sl := p.g.slice
	return func(e *env) any { tick(); return sl(o(e), ev(lo, e), ev(hi, e), ev(st, e)) }
}

func (p *parser) args(end string) ([]code, map[string]code) {
	var args []code
	kw := map[string]code{}
	p.skipNewlines()
	for !p.accept(end) {
		if p.g.comp && len(args) == 0 && len(kw) == 0 {
			start := p.p
			x := p.expr()
			if p.is("for") {
				return []code{p.comprehension(x, end)}, kw
			}
			p.p = start
		}
		if p.g.kwargs && p.peek().kind == 'i' && p.ts[p.p+1].s == "=" {
			n := p.name()
			p.next()
			kw[n] = p.expr()
		} else {
			if p.is("*") || p.is("...") {
				failf("spread arguments are not supported")
			}
			args = append(args, p.expr())
		}
		p.skipNewlines()
		if !p.accept(",") {
			p.skipNewlines()
			p.expect(end)
			break
		}
		p.skipNewlines()
	}
	return args, kw
}

// assignAhead は今の文が代入 (= か += など) かを、括弧の外を先読みして調べる。
func (p *parser) assignAhead(ops ...string) string {
	depth := 0
	for i := p.p; i < len(p.ts); i++ {
		t := p.ts[i]
		if t.kind == 0 || t.kind == ';' && depth == 0 {
			return ""
		}
		if t.kind != 'o' {
			continue
		}
		switch t.s {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth--; depth < 0 {
				return ""
			}
		case ":", "?", "=>":
			if depth == 0 && t.s != ":" {
				return ""
			}
		}
		if depth == 0 && contains(ops, t.s) {
			return t.s
		}
		if depth == 0 && t.s == ";" {
			return ""
		}
	}
	return ""
}

// target は代入の左辺を読み、値を入れる関数を返す。
func (p *parser) target() func(*env, any) {
	n := p.name()
	if !p.is("[") && !p.is(".") {
		return func(e *env, v any) { e.set(n, v) }
	}
	obj := ref(n)
	for {
		switch {
		case p.accept("["):
			k := p.expr()
			p.expect("]")
			if !p.is("[") && !p.is(".") {
				o, set := obj, p.g.setIdx
				return func(e *env, v any) { set(o(e), k(e), v) }
			}
			o, ix := obj, p.g.index
			obj = func(e *env) any { return ix(o(e), k(e)) }
		case p.accept("."):
			failf("assigning to attributes is not supported")
		default:
			failf("bad assignment target")
		}
	}
}
