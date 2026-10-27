package main

// JavaScript と TypeScript の読み取り部。TypeScript は型の注釈を読み飛ばすだけで、意味は JavaScript と同じ。

import (
	"math"
	"strconv"
	"strings"
)

var jsLex = &lexer{comments: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}, quotes: "\"'`", interp: map[byte]string{'`': "${"}, ident: "$"}

var jsGrammar = &grammar{
	bin: map[string]int{"?": 1, "??": 2, "||": 3, "&&": 4, "|": 5, "^": 6, "&": 7, "==": 8, "!=": 8, "===": 8, "!==": 8,
		"<": 9, ">": 9, "<=": 9, ">=": 9, "<<": 10, ">>": 10, "+": 11, "-": 11, "*": 12, "/": 12, "%": 12, "**": 13},
	right:  map[string]bool{"**": true, "?": true},
	prefix: map[string]int{"!": 14, "-": 14, "+": 14, "~": 14, "typeof": 14},
	and:    []string{"&&"}, or: []string{"||", "??"},
}

func init() {
	g := jsGrammar
	g.truthy = jsTruthy
	g.binop = jsBinop
	g.unop = func(op string, a any) any {
		switch op {
		case "!":
			return !jsTruthy(a)
		case "-":
			return -jsNum(a)
		case "+":
			return jsNum(a)
		case "~":
			return float64(^int32(jsNum(a)))
		}
		return jsTypeof(a)
	}
	g.member = jsMember
	g.index = func(o, i any) any {
		k, ok := i.(float64)
		if !ok || k != math.Trunc(k) {
			if s, ok := i.(string); ok {
				return jsMember(o, s)
			}
			failf("unsupported index %s", show(i))
		}
		switch x := o.(type) {
		case string:
			cs := chars(x)
			if k < 0 || int(k) >= len(cs) {
				return undefined{}
			}
			return cs[int(k)]
		case *list:
			if k < 0 || int(k) >= len(x.v) {
				return undefined{}
			}
			return x.v[int(k)]
		}
		failf("cannot index %s", show(o))
		return nil
	}
	g.setIdx = func(o, i, v any) {
		l, ok := o.(*list)
		k, ok2 := i.(float64)
		if !ok || !ok2 || k < 0 || int(k) >= len(l.v) {
			failf("unsupported element assignment")
		}
		l.v[int(k)] = v
	}
	g.atom = jsAtom
	register(func(src string) error { return judgeJS(src, false, false) }, "js", "cjs")
	register(func(src string) error { return judgeJS(src, true, false) }, "mjs")
	register(func(src string) error { return judgeJS(src, true, true) }, "ts", "mts", "cts")
}

type jsParser struct{ *parser }

// jsStrict は厳格モード (宣言していない名前への代入が誤りになる)、jsTyped は TypeScript の型注釈を読み飛ばすか。
var jsStrict, jsTyped bool

func judgeJS(src string, strict, typed bool) error {
	return run(func() {
		toStr = jsStr
		p := &jsParser{&parser{ts: jsLex.lex(src), g: jsGrammar, lx: jsLex}}
		if t := p.peek(); t.kind == 's' && t.s == "use strict" {
			strict = true
		}
		jsStrict, jsTyped = strict, typed
		body := p.block(func() bool { return p.peek().kind == 0 })
		e := jsGlobals()
		e.fn = true
		body(e)
	})
}

// block は stop が真になるまで文を読む。関数の宣言は先に作っておく (巻き上げ)。
func (p *jsParser) block(stop func() bool) code {
	var hoisted, cs []code
	for p.accept(";"); !stop(); p.accept(";") {
		c, isFunc := p.stmt()
		if isFunc {
			hoisted = append(hoisted, c)
		} else {
			cs = append(cs, c)
		}
	}
	return seq(append(hoisted, cs...)...)
}

func (p *jsParser) braced() code {
	p.expect("{")
	b := p.block(func() bool { return p.is("}") })
	p.expect("}")
	return func(e *env) any { return b(newEnv(e)) }
}

func (p *jsParser) stmtOrBlock() code {
	if p.is("{") {
		return p.braced()
	}
	c, _ := p.stmt()
	return c
}

func (p *jsParser) end() {
	if !p.accept(";") && !p.is("}") && p.peek().kind != 0 && !p.peek().nl {
		failf("expected ; but found %q", p.peek().s)
	}
}

func (p *jsParser) stmt() (code, bool) {
	t := p.peek()
	switch t.s {
	case "{":
		return p.braced(), false
	case "function":
		p.next()
		n := p.name()
		f := p.function(n)
		return func(e *env) any { e.vars[n] = f(e); return nil }, true
	case "var", "let", "const":
		p.next()
		c := p.decl(t.s)
		p.end()
		return c, false
	case "if":
		p.next()
		p.expect("(")
		cond := p.expr()
		p.expect(")")
		then := p.stmtOrBlock()
		var els code = lit(nil)
		if p.accept("else") {
			els = p.stmtOrBlock()
		}
		return func(e *env) any {
			tick()
			if jsTruthy(cond(e)) {
				return then(e)
			}
			return els(e)
		}, false
	case "while":
		p.next()
		p.expect("(")
		cond := p.expr()
		p.expect(")")
		return loop(jsTruthy, cond, p.stmtOrBlock(), nil), false
	case "do":
		p.next()
		body := p.stmtOrBlock()
		p.expect("while")
		p.expect("(")
		cond := p.expr()
		p.expect(")")
		p.end()
		return func(e *env) any {
			if iterate(e, body) {
				return nil
			}
			return loop(jsTruthy, cond, body, nil)(e)
		}, false
	case "for":
		return p.forStmt(), false
	case "return":
		p.next()
		if p.is(";") || p.is("}") || p.peek().nl || p.peek().kind == 0 {
			p.accept(";")
			return func(*env) any { panic(ret{undefined{}}) }, false
		}
		x := p.expr()
		p.end()
		return func(e *env) any { panic(ret{x(e)}) }, false
	case "break", "continue":
		p.next()
		p.end()
		if t.s == "break" {
			return func(*env) any { panic(brk{}) }, false
		}
		return func(*env) any { panic(cont{}) }, false
	case "type", "interface", "declare":
		if jsTyped && p.declOnly(t.s) {
			return lit(nil), false
		}
	case "import", "export", "class", "switch", "try", "throw", "with", "async", "enum", "namespace":
		failf("%q is not supported", t.s)
	}
	c := p.simple()
	p.end()
	return c, false
}

// declOnly は TypeScript の型だけの宣言を読み飛ばす。
func (p *jsParser) declOnly(kw string) bool {
	if p.ts2Peek(1).kind != 'i' {
		return false
	}
	p.next()
	p.next()
	if kw == "type" {
		p.expect("=")
		p.skipType(";")
		p.accept(";")
		return true
	}
	p.skipType("{")
	p.skipBalanced()
	return true
}

func (p *jsParser) ts2Peek(n int) tok { return p.parser.ts[p.p+n] }

func (p *jsParser) skipBalanced() {
	depth := 0
	for {
		t := p.next()
		if t.kind == 0 {
			failf("unbalanced braces")
		}
		switch t.s {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			if depth--; depth == 0 {
				return
			}
		}
	}
}

// skipType は型の注釈を、stop のどれかに着くまで読み飛ばす。
func (p *jsParser) skipType(stop ...string) {
	depth, start := 0, p.p
	for {
		t := p.peek()
		if t.kind == 0 || depth == 0 && (contains(stop, t.s) || t.s == ";" || t.s == "=" && !contains(stop, "{") || t.nl && p.p > start) {
			return
		}
		switch t.s {
		case "(", "[", "{", "<":
			depth++
		case ")", "]", "}", ">":
			if depth == 0 {
				return
			}
			depth--
		case "=>":
		}
		p.next()
	}
}

func (p *jsParser) decl(kind string) code {
	var cs []code
	for {
		n := p.name()
		if jsTyped && p.accept(":") {
			p.skipType(",", "=", ";", ")")
		}
		var x code = lit(undefined{})
		if p.accept("=") {
			x = p.expr()
			if jsTyped && p.accept("as") {
				p.skipType(",", ";", ")")
			}
		} else if kind == "const" {
			failf("const needs a value")
		}
		cs = append(cs, func(e *env) any {
			v := x(e)
			s := e
			if kind == "var" {
				for !s.fn {
					s = s.up
				}
			} else if _, dup := s.vars[n]; dup {
				failf("%s has already been declared", n)
			}
			s.vars[n] = v
			if kind == "const" {
				if s.consts == nil {
					s.consts = map[string]bool{}
				}
				s.consts[n] = true
			}
			return nil
		})
		if !p.accept(",") {
			return seq(cs...)
		}
	}
}

func (p *jsParser) forStmt() code {
	p.next()
	p.expect("(")
	if (p.is("const") || p.is("let") || p.is("var")) && p.ts2Peek(2).s == "of" {
		p.next()
		n := p.name()
		p.expect("of")
		it := p.expr()
		p.expect(")")
		body := p.stmtOrBlock()
		return func(e *env) any {
			for _, v := range jsIter(it(e)) {
				tick()
				in := newEnv(e)
				in.vars[n] = v
				if iterate(in, body) {
					break
				}
			}
			return nil
		}
	}
	var init code = lit(nil)
	if t := p.peek().s; t == "let" || t == "var" || t == "const" {
		p.next()
		init = p.decl(t)
	} else if !p.is(";") {
		init = p.simple()
	}
	p.expect(";")
	var cond code
	if !p.is(";") {
		cond = p.expr()
	}
	p.expect(";")
	var step code
	if !p.is(")") {
		step = p.simple()
	}
	p.expect(")")
	body := p.stmtOrBlock()
	return func(e *env) any {
		in := newEnv(e)
		init(in)
		return loop(jsTruthy, cond, body, step)(in)
	}
}

// simple は代入、++/--、式のどれかを読む。
func (p *jsParser) simple() code {
	if p.peek().kind == 'i' && (p.ts2Peek(1).s == "++" || p.ts2Peek(1).s == "--") {
		n, op := p.name(), p.next().s
		return func(e *env) any {
			old := jsNum(e.get(n))
			jsAssign(e, n, old+map[string]float64{"++": 1, "--": -1}[op])
			return old
		}
	}
	if op := p.assignAhead("=", "+=", "-=", "*=", "/=", "%="); op != "" {
		start := p.p
		set := p.target()
		simple := p.p == start+1
		p.expect(op)
		x := p.expr()
		n := p.parser.ts[start].s
		if simple {
			set = func(e *env, v any) { jsAssign(e, n, v) }
		}
		if op == "=" {
			return func(e *env) any { v := x(e); set(e, v); return v }
		}
		if !simple {
			failf("compound assignment is only supported on names")
		}
		return func(e *env) any { v := jsBinop(op[:1], e.get(n), x(e)); set(e, v); return v }
	}
	return p.expr()
}

func jsAssign(e *env, n string, v any) {
	s := e.find(n)
	switch {
	case s == nil && jsStrict:
		failf("%s is not defined", n)
	case s == nil:
		for e.up != nil {
			e = e.up
		}
		e.vars[n] = v
	case s.consts[n]:
		failf("assignment to constant variable %s", n)
	default:
		s.vars[n] = v
	}
}

// function は (引数) { 本体 } を読む。
func (p *jsParser) function(name string) func(*env) any {
	params, defs := p.params()
	if jsTyped && p.accept(":") {
		p.skipType("{")
	}
	body := p.braced()
	return func(e *env) any { return closure(name, params, defs, seq(body, lit(undefined{})), e) }
}

func (p *jsParser) params() ([]string, []code) {
	p.expect("(")
	var params []string
	var defs []code
	for !p.accept(")") {
		if p.is("...") || p.is("{") || p.is("[") {
			failf("rest and destructured parameters are not supported")
		}
		params = append(params, p.name())
		if jsTyped {
			p.accept("?")
			if p.accept(":") {
				p.skipType(",", "=", ")")
			}
		}
		var d code
		if p.accept("=") {
			d = p.expr()
		}
		defs = append(defs, d)
		p.accept(",")
	}
	return params, defs
}

// arrowAhead は今の位置から矢印関数が始まるかを調べる。
func (p *jsParser) arrowAhead() bool {
	t := p.peek()
	if t.kind == 'i' && p.ts2Peek(1).s == "=>" {
		return true
	}
	if !p.is("(") {
		return false
	}
	depth := 0
	for i := p.p; i < len(p.parser.ts); i++ {
		switch p.parser.ts[i].s {
		case "(":
			depth++
		case ")":
			if depth--; depth == 0 {
				next := p.parser.ts[i+1].s
				return next == "=>" || jsTyped && next == ":"
			}
		}
		if p.parser.ts[i].kind == 0 {
			return false
		}
	}
	return false
}

func jsAtom(pp *parser) code {
	p := &jsParser{pp}
	switch t := p.peek(); {
	case t.kind == 'n':
		p.next()
		f, _ := num(t.num)
		return lit(f)
	case p.arrowAhead():
		var params []string
		var defs []code
		if p.peek().kind == 'i' {
			params = []string{p.name()}
		} else {
			params, defs = p.params()
			if jsTyped && p.accept(":") {
				p.skipType("=>")
			}
		}
		p.expect("=>")
		var body code
		if p.is("{") {
			b := p.braced()
			body = seq(b, lit(undefined{}))
		} else {
			x := p.binary(0)
			body = func(e *env) any { panic(ret{x(e)}) }
		}
		return func(e *env) any { return closure("arrow", params, defs, body, e) }
	case t.s == "function":
		p.next()
		n := "anonymous"
		if p.peek().kind == 'i' {
			n = p.name()
		}
		return p.function(n)
	case t.kind == 'i':
		switch t.s {
		case "true", "false":
			p.next()
			return lit(t.s == "true")
		case "null":
			p.next()
			return lit(nil)
		case "undefined":
			p.next()
			return lit(undefined{})
		case "new", "this", "class", "await", "yield", "super", "delete", "void", "in", "instanceof":
			failf("%q is not supported", t.s)
		}
	case t.s == "{":
		failf("object literals are not supported")
	case t.s == "/":
		failf("regular expressions are not supported")
	}
	return nil
}

func jsGlobals() *env {
	e := newEnv(nil)
	logTo := func(stdout bool) *function {
		return native("log", func(a []any) any {
			if !stdout {
				failf("prints to stderr")
			}
			write(jsLogLine(a) + "\n")
			return undefined{}
		})
	}
	e.vars["console"] = &module{"console", map[string]any{
		"log": logTo(true), "info": logTo(true), "debug": logTo(true), "error": logTo(false), "warn": logTo(false)}}
	e.vars["process"] = &module{"process", map[string]any{"stdout": &module{"process.stdout", map[string]any{
		"write": native("write", func(a []any) any {
			s, ok := one(a).(string)
			if !ok {
				failf("process.stdout.write needs a string")
			}
			write(s)
			return true
		})}}, "exit": native("exit", func(a []any) any {
		if len(a) == 0 {
			panic(exit{0})
		}
		panic(exit{int64(jsNum(a[0]))})
	})}}
	e.vars["String"] = &module{"String", map[string]any{"fromCharCode": native("fromCharCode", func(a []any) any {
		var b strings.Builder
		for _, v := range a {
			b.WriteRune(rune(uint16(jsNum(v))))
		}
		return b.String()
	})}}
	e.vars["NaN"] = math.NaN()
	e.vars["Infinity"] = math.Inf(1)
	return e
}

// jsLogLine は console.log の 1 行。最初の引数に %s があれば書式として読む。
func jsLogLine(a []any) string {
	if len(a) > 0 {
		if f, ok := a[0].(string); ok && strings.Contains(f, "%") {
			var b strings.Builder
			rest := a[1:]
			for i := 0; i < len(f); i++ {
				if f[i] != '%' || i+1 >= len(f) {
					b.WriteByte(f[i])
					continue
				}
				switch f[i+1] {
				case 's':
					if len(rest) == 0 {
						b.WriteString("%s")
					} else {
						b.WriteString(jsStr(rest[0]))
						rest = rest[1:]
					}
					i++
				case '%':
					b.WriteByte('%')
					i++
				case 'd', 'i', 'f', 'o', 'O', 'j', 'c':
					failf("console.log format %%%c is not supported", f[i+1])
				default:
					b.WriteByte('%')
				}
			}
			a = append([]any{b.String()}, rest...)
		}
	}
	var ss []string
	for _, v := range a {
		if s, ok := v.(string); ok {
			ss = append(ss, s)
			continue
		}
		switch v.(type) {
		case *list, *module, *function:
			failf("printing objects is not supported")
		}
		ss = append(ss, jsStr(v))
	}
	return strings.Join(ss, " ")
}

func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil, undefined:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	}
	return true
}

func jsNum(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case nil:
		return 0
	case undefined:
		return math.NaN()
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	case *list:
		return jsNum(jsStr(x))
	}
	failf("cannot convert %s to a number", show(v))
	return 0
}

func jsStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == 0 {
			return "0"
		}
		return formatFloat(x, false)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return "null"
	case undefined:
		return "undefined"
	case *list:
		var ss []string
		for _, e := range x.v {
			if e == nil || e == (undefined{}) {
				ss = append(ss, "")
			} else {
				ss = append(ss, jsStr(e))
			}
		}
		return strings.Join(ss, ",")
	}
	failf("cannot convert %s to a string", show(v))
	return ""
}

func jsTypeof(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case undefined:
		return "undefined"
	case *function:
		return "function"
	}
	return "object"
}

func jsBinop(op string, a, b any) any {
	switch op {
	case "+":
		_, as := a.(string)
		_, bs := b.(string)
		_, al := a.(*list)
		_, bl := b.(*list)
		if as || bs || al || bl {
			return jsStr(a) + jsStr(b)
		}
		return jsNum(a) + jsNum(b)
	case "-":
		return jsNum(a) - jsNum(b)
	case "*":
		return jsNum(a) * jsNum(b)
	case "/":
		return jsNum(a) / jsNum(b)
	case "%":
		return math.Mod(jsNum(a), jsNum(b))
	case "**":
		return math.Pow(jsNum(a), jsNum(b))
	case "===":
		return jsStrictEq(a, b)
	case "!==":
		return !jsStrictEq(a, b)
	case "==":
		return jsLooseEq(a, b)
	case "!=":
		return !jsLooseEq(a, b)
	case "<", ">", "<=", ">=":
		x, xs := a.(string)
		y, ys := b.(string)
		var c int
		if xs && ys {
			c = strings.Compare(x, y)
		} else {
			m, n := jsNum(a), jsNum(b)
			if math.IsNaN(m) || math.IsNaN(n) {
				return false
			}
			c = compare(m, n)
		}
		return map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op]
	case "&", "|", "^", "<<", ">>":
		x, y := int32(jsNum(a)), int32(jsNum(b))
		switch op {
		case "&":
			return float64(x & y)
		case "|":
			return float64(x | y)
		case "^":
			return float64(x ^ y)
		case "<<":
			return float64(x << (uint32(y) & 31))
		}
		return float64(x >> (uint32(y) & 31))
	}
	failf("operator %s is not supported", op)
	return nil
}

func jsStrictEq(a, b any) bool {
	switch a.(type) {
	case *list, *function, *module:
		return a == b
	}
	if x, ok := a.(float64); ok {
		y, ok := b.(float64)
		return ok && x == y
	}
	return a == b
}

func jsLooseEq(a, b any) bool {
	isNullish := func(v any) bool { return v == nil || v == (undefined{}) }
	if isNullish(a) || isNullish(b) {
		return isNullish(a) && isNullish(b)
	}
	_, as := a.(string)
	_, bs := b.(string)
	if as && bs {
		return a == b
	}
	switch a.(type) {
	case *list, *function, *module:
		if _, ok := b.(*list); ok {
			return a == b
		}
	}
	return jsNum(a) == jsNum(b)
}

func jsIter(v any) []any {
	switch x := v.(type) {
	case string:
		return chars(x)
	case *list:
		return append([]any{}, x.v...)
	}
	failf("%s is not iterable", show(v))
	return nil
}

func jsMember(o any, n string) any {
	if m, ok := o.(*module); ok {
		if v, ok := m.m[n]; ok {
			return v
		}
		failf("%s.%s is not supported", m.name, n)
	}
	m := func(f func([]any) any) any { return native(n, f) }
	switch x := o.(type) {
	case string:
		switch n {
		case "length":
			return float64(len(utf16(x)))
		case "toUpperCase":
			return m(func([]any) any { return strings.ToUpper(x) })
		case "toLowerCase":
			return m(func([]any) any { return strings.ToLower(x) })
		case "trim":
			return m(func([]any) any { return strings.TrimSpace(x) })
		case "repeat":
			return m(func(a []any) any { return strings.Repeat(x, int(max(jsNum(one(a)), 0))) })
		case "concat":
			return m(func(a []any) any {
				for _, v := range a {
					x += jsStr(v)
				}
				return x
			})
		case "split":
			return m(func(a []any) any {
				sep, ok := one(a).(string)
				if !ok {
					failf("split needs a string")
				}
				l := &list{}
				if sep == "" {
					l.v = chars(x)
				} else {
					for _, s := range strings.Split(x, sep) {
						l.v = append(l.v, s)
					}
				}
				return l
			})
		case "charAt":
			return m(func(a []any) any {
				cs, k := chars(x), int(jsNum(one(a)))
				if k < 0 || k >= len(cs) {
					return ""
				}
				return cs[k]
			})
		case "slice", "substring":
			return m(func(a []any) any {
				rs := []rune(x)
				lo, hi := 0, len(rs)
				if len(a) > 0 {
					lo = int(jsNum(a[0]))
				}
				if len(a) > 1 {
					hi = int(jsNum(a[1]))
				}
				if n == "slice" {
					if lo < 0 {
						lo += len(rs)
					}
					if hi < 0 {
						hi += len(rs)
					}
				}
				lo, hi = min(max(lo, 0), len(rs)), min(max(hi, 0), len(rs))
				if lo > hi {
					if n == "substring" {
						lo, hi = hi, lo
					} else {
						return ""
					}
				}
				return string(rs[lo:hi])
			})
		case "replace":
			return m(func(a []any) any {
				if len(a) != 2 {
					failf("replace needs 2 arguments")
				}
				old, ok1 := a[0].(string)
				nw, ok2 := a[1].(string)
				if !ok1 || !ok2 {
					failf("replace with non-strings is not supported")
				}
				return strings.Replace(x, old, nw, 1)
			})
		}
	case *list:
		switch n {
		case "length":
			return float64(len(x.v))
		case "join":
			return m(func(a []any) any {
				sep := ","
				if len(a) > 0 {
					sep = jsStr(a[0])
				}
				var ss []string
				for _, v := range x.v {
					ss = append(ss, jsStr(&list{[]any{v}}))
				}
				return strings.Join(ss, sep)
			})
		case "reverse":
			return m(func([]any) any { x.v = reverse(x.v); return x })
		case "push":
			return m(func(a []any) any { x.v = append(x.v, a...); return float64(len(x.v)) })
		case "map":
			return m(func(a []any) any {
				l := &list{}
				for i, v := range x.v {
					l.v = append(l.v, call(one(a), []any{v, float64(i)}, nil))
				}
				return l
			})
		case "forEach":
			return m(func(a []any) any {
				for i, v := range x.v {
					call(one(a), []any{v, float64(i)}, nil)
				}
				return undefined{}
			})
		}
	}
	failf("property %q is not supported", n)
	return nil
}

func utf16(s string) []uint16 {
	var u []uint16
	for _, r := range s {
		if r >= 0x10000 {
			r -= 0x10000
			u = append(u, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
		} else {
			u = append(u, uint16(r))
		}
	}
	return u
}
