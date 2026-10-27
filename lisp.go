package main

// S 式の言語。読み取りと評価は 1 つで、方言ごとの違い (偽の値、出力の関数、
// トップレベルの値を表示するか) だけを dialect で表す。

import (
	"strconv"
	"strings"
)

type sym string
type char rune
type void struct{}

type dialect struct {
	falsy    func(any) bool
	printTop bool // Racket はトップレベルの式の値を表示する
	fold     bool // Common Lisp は名前の大文字小文字を区別しない
	lib      func(e *env)
}

func init() {
	scheme := &dialect{falsy: func(v any) bool { return v == false }, lib: schemeLib}
	racket := &dialect{falsy: scheme.falsy, printTop: true, lib: schemeLib}
	cl := &dialect{falsy: func(v any) bool { l, ok := v.(*list); return v == nil || ok && len(l.v) == 0 }, fold: true, lib: clLib}
	clj := &dialect{falsy: func(v any) bool { return v == nil || v == false }, lib: cljLib}
	for d, exts := range map[*dialect][]string{
		scheme: {"scm", "ss", "sls", "sld", "scheme"},
		racket: {"rkt", "racket"},
		cl:     {"lisp", "lsp", "cl"},
		clj:    {"clj", "cljc", "clojure"},
	} {
		d := d
		register(func(src string) error { return run(func() { d.run(src) }) }, exts...)
	}
}

func (d *dialect) run(src string) {
	toStr = lispDisplay
	if strings.HasPrefix(src, "#lang") {
		line, rest, _ := strings.Cut(src, "\n")
		if f := strings.Fields(line); len(f) != 2 || f[1] != "racket" && f[1] != "racket/base" {
			failf("only #lang racket is supported")
		}
		src = rest
	} else if d.printTop {
		failf("a Racket module needs #lang racket")
	}
	r := &reader{src: src, fold: d.fold}
	e := newEnv(nil)
	d.lib(e)
	for {
		x, ok := r.read()
		if !ok {
			return
		}
		v := d.compile(x)(e)
		if d.printTop {
			if _, isVoid := v.(void); !isVoid && !isDefine(x) {
				write(lispWrite(v) + "\n")
			}
		}
	}
}

func isDefine(x any) bool {
	l, ok := x.(*list)
	return ok && len(l.v) > 0 && l.v[0] == sym("define")
}

// 読み取り。

type reader struct {
	src  string
	i    int
	fold bool
}

func (r *reader) skip() {
	for r.i < len(r.src) {
		switch c := r.src[r.i]; {
		case c == ';':
			for r.i < len(r.src) && r.src[r.i] != '\n' {
				r.i++
			}
		case strings.HasPrefix(r.src[r.i:], "#|"):
			end := strings.Index(r.src[r.i:], "|#")
			if end < 0 {
				failf("unterminated comment")
			}
			r.i += end + 2
		case strings.HasPrefix(r.src[r.i:], "#;") || strings.HasPrefix(r.src[r.i:], "#_"):
			r.i += 2
			if _, ok := r.read(); !ok {
				failf("nothing after datum comment")
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			r.i++
		default:
			return
		}
	}
}

func (r *reader) read() (any, bool) {
	r.skip()
	if r.i >= len(r.src) {
		return nil, false
	}
	c := r.src[r.i]
	switch c {
	case '(', '[':
		r.i++
		end := map[byte]byte{'(': ')', '[': ']'}[c]
		l := &list{}
		for {
			r.skip()
			if r.i >= len(r.src) {
				failf("unclosed list")
			}
			if r.src[r.i] == end {
				r.i++
				if c == '[' {
					return &list{append([]any{sym("vector")}, l.v...)}, true
				}
				return l, true
			}
			x, _ := r.read()
			l.v = append(l.v, x)
		}
	case ')', ']':
		failf("unexpected %q", c)
	case '\'':
		r.i++
		x, ok := r.read()
		if !ok {
			failf("nothing after quote")
		}
		return &list{[]any{sym("quote"), x}}, true
	case '"':
		var b strings.Builder
		for r.i++; ; r.i++ {
			if r.i >= len(r.src) {
				failf("unterminated string")
			}
			switch r.src[r.i] {
			case '"':
				r.i++
				return b.String(), true
			case '\\':
				s, n := unescape(r.src[r.i:])
				b.WriteString(s)
				r.i += n - 1
			default:
				b.WriteByte(r.src[r.i])
			}
		}
	}
	j := r.i
	for j < len(r.src) && !strings.ContainsRune(" \t\n\r()[]\";,", rune(r.src[j])) {
		j++
	}
	if strings.HasPrefix(r.src[r.i:], "#\\") || strings.HasPrefix(r.src[r.i:], "\\") && !r.fold {
		start := strings.Index(r.src[r.i:], "\\") + r.i + 1
		j = start + 1
		for j < len(r.src) && !strings.ContainsRune(" \t\n\r()[]\";", rune(r.src[j])) {
			j++
		}
		name := r.src[start:j]
		r.i = j
		names := map[string]rune{"space": ' ', "newline": '\n', "tab": '\t', "Space": ' ', "Newline": '\n'}
		if v, ok := names[name]; ok {
			return char(v), true
		}
		if rs := []rune(name); len(rs) == 1 {
			return char(rs[0]), true
		}
		failf("unknown character %q", name)
	}
	t := r.src[r.i:j]
	r.i = j
	if t == "" {
		failf("unexpected %q", r.src[j])
	}
	switch t {
	case "#t", "#true":
		return true, true
	case "#f", "#false":
		return false, true
	}
	if v, err := strconv.ParseInt(t, 10, 64); err == nil {
		return v, true
	}
	if v, err := strconv.ParseFloat(t, 64); err == nil && strings.ContainsAny(t, "0123456789") {
		return v, true
	}
	if r.fold {
		t = strings.ToLower(t)
	}
	return sym(t), true
}

// 評価。

func (d *dialect) compile(x any) code {
	switch v := x.(type) {
	case sym:
		n := string(v)
		if strings.HasPrefix(n, ":") {
			return lit(v) // キーワード
		}
		return ref(n)
	case *list:
		if len(v.v) == 0 {
			if d.fold {
				return lit(nil)
			}
			failf("empty application")
		}
		if h, ok := v.v[0].(sym); ok {
			if c := d.special(string(h), v.v[1:]); c != nil {
				return c
			}
		}
		f := d.compile(v.v[0])
		args := make([]code, len(v.v)-1)
		for i, a := range v.v[1:] {
			args[i] = d.compile(a)
		}
		return func(e *env) any {
			fv := f(e)
			vs := make([]any, len(args))
			for i, a := range args {
				vs[i] = a(e)
			}
			return call(fv, vs, nil)
		}
	}
	return lit(x)
}

func (d *dialect) body(xs []any) code {
	cs := make([]code, len(xs))
	for i, x := range xs {
		cs[i] = d.compile(x)
	}
	if len(cs) == 0 {
		return lit(void{})
	}
	return seq(cs...)
}

func (d *dialect) lambda(name string, params any, body []any) code {
	var ps []string
	pl, ok := params.(*list)
	if !ok {
		failf("bad parameter list")
	}
	items := pl.v
	if len(items) > 0 && items[0] == sym("vector") { // Clojure の [a b]
		items = items[1:]
	}
	for _, p := range items {
		s, ok := p.(sym)
		if !ok || s == "&" || s == "&rest" || s == "." || s == "&optional" {
			failf("unsupported parameter %v", p)
		}
		ps = append(ps, string(s))
	}
	b := d.body(body)
	return func(e *env) any {
		f := closure(name, ps, nil, b, e)
		inner := f.call
		f.call = func(args []any, kw map[string]any) any {
			if len(args) != len(ps) {
				failf("%s expects %d arguments, got %d", name, len(ps), len(args))
			}
			return inner(args, kw)
		}
		return f
	}
}

func (d *dialect) special(h string, a []any) code {
	need := func(n int) {
		if len(a) < n {
			failf("%s needs %d parts", h, n)
		}
	}
	falsy := d.falsy
	switch h {
	case "quote":
		need(1)
		return lit(a[0])
	case "if":
		need(2)
		c, t := d.compile(a[0]), d.compile(a[1])
		var f code = lit(nil)
		if len(a) > 2 {
			f = d.compile(a[2])
		} else if !d.fold {
			f = lit(void{})
		}
		return func(e *env) any {
			tick()
			if !falsy(c(e)) {
				return t(e)
			}
			return f(e)
		}
	case "when", "unless":
		need(1)
		c, b := d.compile(a[0]), d.body(a[1:])
		return func(e *env) any {
			if falsy(c(e)) == (h == "unless") {
				return b(e)
			}
			return void{}
		}
	case "cond":
		type clause struct{ c, b code }
		var cs []clause
		for _, x := range a {
			l, ok := x.(*list)
			if !ok || len(l.v) == 0 {
				failf("bad cond clause")
			}
			var c code = d.compile(l.v[0])
			if l.v[0] == sym("else") || l.v[0] == sym(":else") {
				c = lit(true)
			}
			cs = append(cs, clause{c, d.body(l.v[1:])})
		}
		return func(e *env) any {
			for _, cl := range cs {
				if !falsy(cl.c(e)) {
					return cl.b(e)
				}
			}
			return void{}
		}
	case "begin", "progn", "do":
		if h == "do" && len(a) > 0 { // Scheme の do ループは扱わない
			if l, ok := a[0].(*list); ok && len(l.v) > 0 {
				if _, ok := l.v[0].(*list); ok {
					failf("do loops are not supported")
				}
			}
		}
		return d.body(a)
	case "and", "or":
		cs := make([]code, len(a))
		for i, x := range a {
			cs[i] = d.compile(x)
		}
		return func(e *env) any {
			var v any = h == "and"
			for _, c := range cs {
				if v = c(e); falsy(v) == (h == "and") {
					return v
				}
			}
			return v
		}
	case "define", "def", "defvar", "defparameter", "setq", "set!", "setf":
		need(2)
		if l, ok := a[0].(*list); ok && h == "define" { // (define (f x) ...)
			n, _ := l.v[0].(sym)
			f := d.lambda(string(n), &list{l.v[1:]}, a[1:])
			return func(e *env) any { e.vars[string(n)] = f(e); return void{} }
		}
		n, ok := a[0].(sym)
		if !ok {
			failf("bad %s", h)
		}
		v := d.compile(a[1])
		return func(e *env) any {
			if h == "set!" || h == "setq" || h == "setf" {
				if e.find(string(n)) == nil && h == "set!" {
					failf("%s is not defined", n)
				}
				e.set(string(n), v(e))
				return void{}
			}
			e.vars[string(n)] = v(e)
			return void{}
		}
	case "defun", "defn":
		need(3)
		n, _ := a[0].(sym)
		rest := a[1:]
		if _, ok := rest[0].(string); ok && len(rest) > 2 { // 説明の文字列
			rest = rest[1:]
		}
		f := d.lambda(string(n), rest[0], rest[1:])
		return func(e *env) any { e.vars[string(n)] = f(e); return void{} }
	case "lambda", "fn":
		need(1)
		return d.lambda("lambda", a[0], a[1:])
	case "let", "let*":
		need(1)
		bl, ok := a[0].(*list)
		if !ok {
			failf("named let is not supported")
		}
		var names []string
		var vals []code
		items := bl.v
		if len(items) > 0 && items[0] == sym("vector") { // Clojure の [a 1 b 2]
			items = items[1:]
			if len(items)%2 != 0 {
				failf("bad let bindings")
			}
			for i := 0; i < len(items); i += 2 {
				s, _ := items[i].(sym)
				names, vals = append(names, string(s)), append(vals, d.compile(items[i+1]))
			}
		} else {
			for _, b := range items {
				p, ok := b.(*list)
				if !ok || len(p.v) != 2 {
					failf("bad let binding")
				}
				s, _ := p.v[0].(sym)
				names, vals = append(names, string(s)), append(vals, d.compile(p.v[1]))
			}
		}
		b := d.body(a[1:])
		sequential := h == "let*" || len(bl.v) > 0 && bl.v[0] == sym("vector")
		return func(e *env) any {
			in := newEnv(e)
			for i, n := range names {
				src := e
				if sequential {
					src = in
				}
				in.vars[n] = vals[i](src)
			}
			return b(in)
		}
	case "dotimes":
		need(1)
		spec, ok := a[0].(*list)
		if !ok || len(spec.v) < 2 {
			failf("bad dotimes")
		}
		items := spec.v
		if items[0] == sym("vector") {
			items = items[1:]
		}
		n, _ := items[0].(sym)
		count, b := d.compile(items[1]), d.body(a[1:])
		return func(e *env) any {
			k := toInt(count(e))
			in := newEnv(e)
			for i := int64(0); i < k; i++ {
				tick()
				in.vars[string(n)] = i
				b(in)
			}
			return nil
		}
	case "dolist", "doseq":
		need(1)
		spec, ok := a[0].(*list)
		if !ok || len(spec.v) < 2 {
			failf("bad %s", h)
		}
		items := spec.v
		if items[0] == sym("vector") {
			items = items[1:]
		}
		n, _ := items[0].(sym)
		xs, b := d.compile(items[1]), d.body(a[1:])
		return func(e *env) any {
			in := newEnv(e)
			for _, v := range lispSeq(xs(e)) {
				tick()
				in.vars[string(n)] = v
				b(in)
			}
			return nil
		}
	case "loop", "while", "for", "named-lambda", "defmacro", "define-syntax", "ns", "require", "import", "in-package", "defpackage", "eval":
		if h == "ns" || h == "require" && len(a) > 0 {
			// 名前空間の宣言と標準の読み込みは出力に効かない
			return lit(void{})
		}
		failf("%s is not supported", h)
	}
	return nil
}

func lispSeq(v any) []any {
	switch x := v.(type) {
	case *list:
		if len(x.v) > 0 && x.v[0] == sym("vector") {
			return x.v[1:]
		}
		return x.v
	case string:
		var cs []any
		for _, r := range x {
			cs = append(cs, char(r))
		}
		return cs
	case nil:
		return nil
	}
	failf("%s is not a sequence", show(v))
	return nil
}

// lispDisplay は display や princ の表し方。lispWrite は write や prin1 の表し方 (文字列に引用符が付く)。
func lispDisplay(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case char:
		return string(x)
	}
	return lispWrite(v)
}

func lispWrite(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case char:
		return "#\\" + string(x)
	case bool:
		if x {
			return "#t"
		}
		return "#f"
	case nil:
		return "nil"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return formatFloat(x, true)
	case sym:
		return string(x)
	case *list:
		var ss []string
		for _, e := range x.v {
			ss = append(ss, lispWrite(e))
		}
		return "(" + strings.Join(ss, " ") + ")"
	case void:
		return ""
	case *function:
		return "#<procedure " + x.name + ">"
	}
	return show(v)
}

func lispStrings(a []any) string {
	var b strings.Builder
	for _, v := range a {
		switch x := v.(type) {
		case string:
			b.WriteString(x)
		case char:
			b.WriteRune(rune(x))
		default:
			failf("%s is not a string", show(v))
		}
	}
	return b.String()
}

func str1(a []any) string {
	s, ok := one(a).(string)
	if !ok {
		failf("%s is not a string", show(one(a)))
	}
	return s
}

// 方言に共通の関数。
func lispCommon(e *env, falsy func(any) bool) {
	def := func(n string, f func([]any) any) { e.vars[n] = native(n, f) }
	nums := func(op string, unit int64) func([]any) any {
		return func(a []any) any {
			if len(a) == 0 {
				return unit
			}
			if len(a) == 1 && (op == "-" || op == "/") {
				return arith(op, unit, a[0])
			}
			v := a[0]
			for _, x := range a[1:] {
				if op == "/" {
					v = lispDiv(v, x)
				} else {
					v = arith(op, v, x)
				}
			}
			return v
		}
	}
	def("+", nums("+", 0))
	def("-", nums("-", 0))
	def("*", nums("*", 1))
	def("/", nums("/", 1))
	cmp := func(ok func(int) bool) func([]any) any {
		return func(a []any) any {
			for i := 1; i < len(a); i++ {
				if !ok(compare(a[i-1], a[i])) {
					return false
				}
			}
			return true
		}
	}
	def("=", cmp(func(c int) bool { return c == 0 }))
	def("<", cmp(func(c int) bool { return c < 0 }))
	def(">", cmp(func(c int) bool { return c > 0 }))
	def("<=", cmp(func(c int) bool { return c <= 0 }))
	def(">=", cmp(func(c int) bool { return c >= 0 }))
	def("not", func(a []any) any { return falsy(one(a)) })
	def("list", func(a []any) any { return &list{append([]any{}, a...)} })
	def("vector", func(a []any) any { return &list{append([]any{sym("vector")}, a...)} })
	def("reverse", func(a []any) any {
		if s, ok := one(a).(string); ok {
			return &list{reverse(lispSeq(s))}
		}
		return &list{reverse(lispSeq(one(a)))}
	})
	def("string-upcase", func(a []any) any { return strings.ToUpper(str1(a)) })
	def("string-downcase", func(a []any) any { return strings.ToLower(str1(a)) })
	def("string-append", func(a []any) any { return lispStrings(a) })
	def("string", func(a []any) any { return lispStrings(a) })
	def("list->string", func(a []any) any { return lispStrings(lispSeq(one(a))) })
	def("string->list", func(a []any) any { return &list{lispSeq(str1(a))} })
	def("integer->char", func(a []any) any { return char(rune(toInt(one(a)))) })
	def("code-char", func(a []any) any { return char(rune(toInt(one(a)))) })
	def("char->integer", func(a []any) any {
		c, ok := one(a).(char)
		if !ok {
			failf("not a character")
		}
		return int64(c)
	})
	def("string-length", func(a []any) any { return int64(len([]rune(str1(a)))) })
	def("substring", func(a []any) any {
		if len(a) < 2 || len(a) > 3 {
			failf("substring takes 2 or 3 arguments")
		}
		rs := []rune(str1(a[:1]))
		lo, hi := toInt(a[1]), int64(len(rs))
		if len(a) == 3 {
			hi = toInt(a[2])
		}
		if lo < 0 || hi > int64(len(rs)) || lo > hi {
			failf("substring out of range")
		}
		return string(rs[lo:hi])
	})
	def("car", func(a []any) any {
		xs := lispSeq(one(a))
		if len(xs) == 0 {
			failf("car of empty list")
		}
		return xs[0]
	})
	def("cdr", func(a []any) any {
		xs := lispSeq(one(a))
		if len(xs) == 0 {
			failf("cdr of empty list")
		}
		return &list{xs[1:]}
	})
	def("cons", func(a []any) any {
		if len(a) != 2 {
			failf("cons takes 2 arguments")
		}
		return &list{append([]any{a[0]}, lispSeq(a[1])...)}
	})
	def("null?", func(a []any) any { return len(lispSeq(one(a))) == 0 })
	def("apply", func(a []any) any {
		if len(a) < 2 {
			failf("apply needs a function and a list")
		}
		args := append(append([]any{}, a[1:len(a)-1]...), lispSeq(a[len(a)-1])...)
		return call(a[0], args, nil)
	})
	def("map", func(a []any) any {
		if len(a) != 2 {
			failf("map with several lists is not supported")
		}
		l := &list{}
		for _, v := range lispSeq(a[1]) {
			l.v = append(l.v, call(a[0], []any{v}, nil))
		}
		return l
	})
}

func lispDiv(a, b any) any {
	x, xi := a.(int64)
	y, yi := b.(int64)
	if yi && y == 0 || !yi && b == 0.0 {
		failf("division by zero")
	}
	if xi && yi && x%y == 0 {
		return x / y
	}
	if xi && yi {
		failf("exact fractions are not supported")
	}
	return arith("/", a, b)
}

func schemeLib(e *env) {
	lispCommon(e, func(v any) bool { return v == false })
	def := func(n string, f func([]any) any) { e.vars[n] = native(n, f) }
	def("display", func(a []any) any { write(lispDisplay(one(a))); return void{} })
	def("write", func(a []any) any { write(lispWrite(one(a))); return void{} })
	def("write-string", func(a []any) any { write(str1(a)); return void{} })
	def("write-char", func(a []any) any { write(lispDisplay(one(a))); return void{} })
	def("newline", func([]any) any { write("\n"); return void{} })
	def("displayln", func(a []any) any { write(lispDisplay(one(a)) + "\n"); return void{} })
	def("printf", func(a []any) any { write(lispFormat(a, "~n")); return void{} })
	def("number->string", func(a []any) any { return lispWrite(one(a)) })
	def("exit", func(a []any) any {
		if len(a) == 0 || a[0] == true {
			panic(exit{0})
		}
		panic(exit{toInt(a[0])})
	})
}

func clLib(e *env) {
	falsy := func(v any) bool { l, ok := v.(*list); return v == nil || ok && len(l.v) == 0 }
	lispCommon(e, falsy)
	e.vars["t"] = true
	e.vars["nil"] = nil
	def := func(n string, f func([]any) any) { e.vars[n] = native(n, f) }
	def("princ", func(a []any) any { write(lispDisplay(one(a))); return one(a) })
	def("prin1", func(a []any) any { write(lispWrite(one(a))); return one(a) })
	def("print", func(a []any) any { write("\n" + lispWrite(one(a)) + " "); return one(a) })
	def("write-string", func(a []any) any { write(str1(a)); return one(a) })
	def("write-line", func(a []any) any { write(str1(a) + "\n"); return one(a) })
	def("terpri", func([]any) any { write("\n"); return nil })
	def("format", func(a []any) any {
		if len(a) == 0 || a[0] != true {
			failf("only (format t ...) is supported")
		}
		write(lispFormat(a[1:], "~%"))
		return nil
	})
	def("concatenate", func(a []any) any {
		if len(a) == 0 || a[0] != sym("string") {
			failf("only (concatenate 'string ...) is supported")
		}
		return lispStrings(a[1:])
	})
}

func cljLib(e *env) {
	falsy := func(v any) bool { return v == nil || v == false }
	lispCommon(e, falsy)
	e.vars["nil"] = nil
	e.vars["true"] = true
	e.vars["false"] = false
	def := func(n string, f func([]any) any) { e.vars[n] = native(n, f) }
	str := func(a []any) string {
		var b strings.Builder
		for _, v := range a {
			if v != nil {
				b.WriteString(lispDisplay(v))
			}
		}
		return b.String()
	}
	def("str", func(a []any) any { return str(a) })
	joined := func(a []any) string {
		var ss []string
		for _, v := range a {
			ss = append(ss, cljShow(v))
		}
		return strings.Join(ss, " ")
	}
	def("println", func(a []any) any { write(joined(a) + "\n"); return nil })
	def("print", func(a []any) any { write(joined(a)); return nil })
	def("clojure.string/upper-case", func(a []any) any { return strings.ToUpper(str1(a)) })
	def("clojure.string/lower-case", func(a []any) any { return strings.ToLower(str1(a)) })
	def("clojure.string/join", func(a []any) any {
		sep, xs := "", a[len(a)-1]
		if len(a) == 2 {
			sep = str1(a[:1])
		}
		var ss []string
		for _, v := range lispSeq(xs) {
			ss = append(ss, lispDisplay(v))
		}
		return strings.Join(ss, sep)
	})
	def("clojure.string/reverse", func(a []any) any { return string(reverseRunes(str1(a))) })
}

func reverseRunes(s string) []rune {
	rs := []rune(s)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return rs
}

// cljShow は println が並べるときの表し方。nil は "nil"、文字列はそのまま。
func cljShow(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(x)
	case char:
		return string(x)
	case *list:
		items := x.v
		open, close := "(", ")"
		if len(items) > 0 && items[0] == sym("vector") {
			items, open, close = items[1:], "[", "]"
		}
		var ss []string
		for _, e := range items {
			ss = append(ss, cljShow(e))
		}
		return open + strings.Join(ss, " ") + close
	}
	return lispDisplay(v)
}

// lispFormat は format や printf の指示 (~a ~s ~% ~n ~~) を解く。nl は改行の指示。
func lispFormat(a []any, nl string) string {
	f, ok := a[0].(string)
	if !ok {
		failf("format string must be a string")
	}
	args := a[1:]
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '~' {
			b.WriteByte(f[i])
			continue
		}
		if i++; i >= len(f) {
			failf("incomplete format directive")
		}
		switch d := strings.ToLower(f[i : i+1]); {
		case "~"+d == nl || d == "%" && nl == "~%" || d == "n" && nl == "~n":
			b.WriteByte('\n')
		case d == "&" && nl == "~%":
			if s := out.String() + b.String(); s != "" && !strings.HasSuffix(s, "\n") {
				b.WriteByte('\n')
			}
		case d == "~":
			b.WriteByte('~')
		case d == "a" || d == "s":
			if len(args) == 0 {
				failf("not enough format arguments")
			}
			if d == "a" {
				b.WriteString(lispDisplay(args[0]))
			} else {
				b.WriteString(lispWrite(args[0]))
			}
			args = args[1:]
		default:
			failf("format directive ~%s is not supported", d)
		}
	}
	if len(args) > 0 && nl == "~n" {
		failf("too many format arguments")
	}
	return b.String()
}
