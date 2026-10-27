package main

// Python 3 の読み取り部。

import (
	"strconv"
	"strings"
)

type module struct {
	name string
	m    map[string]any
} // import したものの名前の集まり

var pyLex = &lexer{comments: []string{"#"}, quotes: `"'`, newlines: true, indent: true, pyPrefix: true}

var pyGrammar = &grammar{
	bin: map[string]int{"if": 0, "or": 1, "and": 2, "in": 4, "not": 4, "is": 4, "<": 4, ">": 4, "==": 4, ">=": 4, "<=": 4, "!=": 4,
		"|": 5, "^": 6, "&": 7, "<<": 8, ">>": 8, "+": 9, "-": 9, "*": 10, "/": 10, "//": 10, "%": 10, "**": 12},
	right:  map[string]bool{"**": true},
	prefix: map[string]int{"not": 3, "-": 11, "+": 11, "~": 11},
	and:    []string{"and"}, or: []string{"or"},
	kwargs: true, comp: true,
}

func init() {
	g := pyGrammar
	g.truthy = pyTruthy
	g.binop = pyBinop
	g.unop = func(op string, a any) any {
		switch op {
		case "not":
			return !pyTruthy(a)
		case "-":
			return arith("-", int64(0), pyNum(a))
		case "+":
			return arith("+", int64(0), pyNum(a))
		}
		return ^toInt(pyNum(a))
	}
	g.member = pyMember
	g.index = func(o, i any) any {
		switch x := o.(type) {
		case string:
			cs := chars(x)
			return cs[index(len(cs), toInt(pyNum(i)), true)]
		case *list:
			return x.v[index(len(x.v), toInt(pyNum(i)), true)]
		}
		failf("%s is not subscriptable", show(o))
		return nil
	}
	g.setIdx = func(o, i, v any) {
		l, ok := o.(*list)
		if !ok {
			failf("%s does not support item assignment", show(o))
		}
		l.v[index(len(l.v), toInt(pyNum(i)), true)] = v
	}
	g.slice = pySlice
	g.atom = func(p *parser) code {
		if p.is("lambda") {
			p.next()
			var params []string
			for !p.accept(":") {
				params = append(params, p.name())
				p.accept(",")
			}
			body := p.binary(0)
			return func(e *env) any { return closure("<lambda>", params, nil, body, e) }
		}
		switch p.peek().s {
		case "None":
			p.next()
			return lit(nil)
		case "True":
			p.next()
			return lit(true)
		case "False":
			p.next()
			return lit(false)
		}
		return nil
	}
}

func judgePython(src string) error {
	return run(func() {
		toStr = pyStr
		p := &parser{ts: pyLex.lex(src), g: pyGrammar, lx: pyLex}
		var body []code
		for p.skipNewlines(); p.peek().kind != 0; p.skipNewlines() {
			body = append(body, p.pyStmt()...)
		}
		seq(body...)(pyGlobals())
	})
}

func pyGlobals() *env {
	e := newEnv(nil)
	e.vars["__name__"] = "__main__"
	e.vars["print"] = &function{"print", func(args []any, kw map[string]any) any {
		sep, end := " ", "\n"
		if s, ok := kw["sep"].(string); ok {
			sep = s
		}
		if s, ok := kw["end"].(string); ok {
			end = s
		}
		if f, ok := kw["file"]; ok && f != stdout {
			failf("prints to somewhere other than stdout")
		}
		var ss []string
		for _, a := range args {
			ss = append(ss, pyStr(a))
		}
		write(strings.Join(ss, sep) + end)
		return nil
	}}
	fns := map[string]func([]any) any{
		"len": func(a []any) any {
			switch x := one(a).(type) {
			case string:
				return int64(len(chars(x)))
			case *list:
				return int64(len(x.v))
			}
			failf("object has no len()")
			return nil
		},
		"str": func(a []any) any {
			if len(a) == 0 {
				return ""
			}
			return pyStr(one(a))
		},
		"repr": func(a []any) any { return pyRepr(one(a)) },
		"int": func(a []any) any {
			switch x := one(a).(type) {
			case string:
				v, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
				if err != nil {
					failf("invalid literal for int(): %q", x)
				}
				return v
			case float64:
				return int64(x)
			}
			return toInt(pyNum(one(a)))
		},
		"chr": func(a []any) any { return string(rune(toInt(one(a)))) },
		"ord": func(a []any) any {
			s, ok := one(a).(string)
			if !ok || len([]rune(s)) != 1 {
				failf("ord() expected a character")
			}
			return int64([]rune(s)[0])
		},
		"range": func(a []any) any {
			lo, hi, st := int64(0), int64(0), int64(1)
			switch len(a) {
			case 1:
				hi = toInt(a[0])
			case 2, 3:
				lo, hi = toInt(a[0]), toInt(a[1])
				if len(a) == 3 {
					st = toInt(a[2])
				}
			default:
				failf("range() takes 1 to 3 arguments")
			}
			if st == 0 {
				failf("range() step must not be zero")
			}
			l := &list{}
			for i := lo; st > 0 && i < hi || st < 0 && i > hi; i += st {
				tick()
				l.v = append(l.v, i)
			}
			return l
		},
		"list":     func(a []any) any { return &list{pyIter(one(a))} },
		"reversed": func(a []any) any { return &list{reverse(pyIter(one(a)))} },
		"abs": func(a []any) any {
			if compare(pyNum(one(a)), int64(0)) < 0 {
				return arith("-", int64(0), pyNum(one(a)))
			}
			return pyNum(one(a))
		},
		"input": func([]any) any { failf("reads input"); return nil },
		"exit":  pyExit,
		"quit":  pyExit,
	}
	for n, f := range fns {
		e.vars[n] = native(n, f)
	}
	return e
}

var stdout = &module{"sys.stdout", map[string]any{"write": native("write", func(a []any) any {
	s, ok := one(a).(string)
	if !ok {
		failf("write() argument must be str")
	}
	write(s)
	return int64(len(chars(s)))
}), "flush": native("flush", func([]any) any { return nil })}}

func pyExit(a []any) any {
	switch {
	case len(a) == 0 || a[0] == nil:
		panic(exit{0})
	case a[0] == false:
		panic(exit{0})
	case a[0] == true:
		panic(exit{1})
	}
	if s, ok := a[0].(string); ok {
		failf("prints %q to stderr", s)
	}
	panic(exit{toInt(a[0])})
}

var pyModules = map[string]*module{
	"sys": {"sys", map[string]any{"stdout": stdout, "exit": native("exit", pyExit), "stderr": &module{"sys.stderr", nil}}},
}

func one(a []any) any {
	if len(a) != 1 {
		failf("expected 1 argument, got %d", len(a))
	}
	return a[0]
}

// 文の読み取り。

func (p *parser) pyStmt() []code {
	t := p.peek()
	switch t.s {
	case "if":
		return []code{p.pyIf()}
	case "while":
		p.next()
		cond := p.expr()
		body := p.pySuite()
		if p.is("else") {
			failf("while-else is not supported")
		}
		return []code{loop(pyTruthy, cond, body, nil)}
	case "for":
		p.next()
		n := p.name()
		if p.is(",") {
			failf("unpacking in for is not supported")
		}
		p.expect("in")
		it := p.expr()
		body := p.pySuite()
		if p.is("else") {
			failf("for-else is not supported")
		}
		return []code{func(e *env) any {
			for _, v := range pyIter(it(e)) {
				tick()
				e.vars[n] = v
				if iterate(e, body) {
					break
				}
			}
			return nil
		}}
	case "def":
		p.next()
		n := p.name()
		p.expect("(")
		var params []string
		var defs []code
		for !p.accept(")") {
			if p.is("*") || p.is("**") || p.is("/") {
				failf("special parameters are not supported")
			}
			params = append(params, p.name())
			if p.accept(":") {
				p.expr()
			}
			var d code
			if p.accept("=") {
				d = p.expr()
			}
			defs = append(defs, d)
			p.accept(",")
		}
		if p.accept("->") {
			p.expr()
		}
		body := p.pySuite()
		return []code{func(e *env) any { e.vars[n] = closure(n, params, defs, body, e); return nil }}
	case "class", "try", "with", "global", "nonlocal", "async", "await", "yield", "del", "match", "@":
		failf("%q is not supported", t.s)
	}
	var cs []code
	for {
		cs = append(cs, p.pySimple())
		if !p.accept(";") || p.peek().kind == ';' || p.peek().kind == 0 {
			break
		}
	}
	if k := p.peek().kind; k != ';' && k != 0 && k != '<' {
		failf("unexpected %q", p.peek().s)
	}
	return cs
}

func (p *parser) pySimple() code {
	t := p.peek()
	switch t.s {
	case "pass":
		p.next()
		return lit(nil)
	case "break":
		p.next()
		return func(*env) any { panic(brk{}) }
	case "continue":
		p.next()
		return func(*env) any { panic(cont{}) }
	case "return":
		p.next()
		if k := p.peek().kind; k == ';' || k == 0 || p.is(";") {
			return func(*env) any { panic(ret{nil}) }
		}
		x := p.expr()
		return func(e *env) any { panic(ret{x(e)}) }
	case "assert":
		p.next()
		x := p.expr()
		return func(e *env) any {
			if !pyTruthy(x(e)) {
				failf("assertion failed")
			}
			return nil
		}
	case "import":
		p.next()
		var names []string
		for {
			n := p.name()
			if p.accept("as") {
				failf("import as is not supported")
			}
			names = append(names, n)
			if !p.accept(",") {
				break
			}
		}
		return func(e *env) any {
			for _, n := range names {
				e.vars[n] = pyModule(n)
			}
			return nil
		}
	case "from":
		p.next()
		m := p.name()
		p.expect("import")
		var names []string
		for {
			names = append(names, p.name())
			if !p.accept(",") {
				break
			}
		}
		return func(e *env) any {
			for _, n := range names {
				if m != "__future__" {
					e.vars[n] = pyMember(pyModule(m), n)
				}
			}
			return nil
		}
	}
	if op := p.assignAhead("=", "+=", "-=", "*=", "/=", "//=", "%="); op != "" {
		start := p.p
		set := p.target()
		simple := p.p == start+1
		if p.next().s != op {
			failf("unsupported assignment")
		}
		if op == "=" && p.assignAhead("=") != "" {
			failf("chained assignment is not supported")
		}
		x := p.expr()
		if p.is(",") {
			failf("tuple assignment is not supported")
		}
		if op == "=" {
			return func(e *env) any { set(e, x(e)); return nil }
		}
		if !simple {
			failf("compound assignment is only supported on names")
		}
		get := ref(p.ts[start].s)
		return func(e *env) any { set(e, pyBinop(op[:len(op)-1], get(e), x(e))); return nil }
	}
	return p.expr()
}

func (p *parser) pyIf() code {
	p.next()
	cond := p.expr()
	then := p.pySuite()
	var els code = lit(nil)
	if p.is("elif") {
		els = p.pyIf()
	} else if p.accept("else") {
		els = p.pySuite()
	}
	return func(e *env) any {
		tick()
		if pyTruthy(cond(e)) {
			return then(e)
		}
		return els(e)
	}
}

func (p *parser) pySuite() code {
	p.expect(":")
	if p.peek().kind != ';' {
		var cs []code
		for {
			cs = append(cs, p.pySimple())
			if !p.accept(";") || p.peek().kind == ';' {
				break
			}
		}
		p.skipNewlines()
		return seq(cs...)
	}
	p.skipNewlines()
	if p.next().kind != '>' {
		failf("expected an indented block")
	}
	var cs []code
	for p.skipNewlines(); p.peek().kind != '<' && p.peek().kind != 0; p.skipNewlines() {
		cs = append(cs, p.pyStmt()...)
	}
	p.next()
	p.skipNewlines()
	return seq(cs...)
}

// pyModule は知らないモジュールも空のまま返す。import するだけなら出力は変わらないので、使ったときに止める。
func pyModule(n string) *module {
	if m, ok := pyModules[n]; ok {
		return m
	}
	return &module{n, nil}
}

// 値の意味。

func pyNum(v any) any {
	if b, ok := v.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	return v
}

func pyTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case *list:
		return len(x.v) > 0
	}
	return true
}

func pyBinop(op string, a, b any) any {
	switch op {
	case "==":
		return pyEq(a, b)
	case "!=":
		return !pyEq(a, b)
	case "<":
		return pyCompare(a, b) < 0
	case ">":
		return pyCompare(a, b) > 0
	case "<=":
		return pyCompare(a, b) <= 0
	case ">=":
		return pyCompare(a, b) >= 0
	case "in", "not in":
		var in bool
		switch y := b.(type) {
		case string:
			x, ok := a.(string)
			if !ok {
				failf("'in <string>' requires string as left operand")
			}
			in = strings.Contains(y, x)
		case *list:
			for _, v := range y.v {
				in = in || pyEq(a, v)
			}
		default:
			failf("argument of type %s is not iterable", show(b))
		}
		return in == (op == "in")
	case "is", "is not":
		if a != nil && b != nil {
			failf("'is' is only supported with None")
		}
		return (a == b) == (op == "is")
	}
	a, b = pyNum(a), pyNum(b)
	switch x := a.(type) {
	case string:
		switch op {
		case "+":
			if y, ok := b.(string); ok {
				return x + y
			}
			failf("can only concatenate str to str")
		case "*":
			if n, ok := b.(int64); ok {
				return strings.Repeat(x, int(max(n, 0)))
			}
		case "%":
			return pyFormat(x, b)
		}
		failf("unsupported operand for %s: str and %s", op, show(b))
	case *list:
		switch op {
		case "+":
			if y, ok := b.(*list); ok {
				return &list{append(append([]any{}, x.v...), y.v...)}
			}
		case "*":
			if n, ok := b.(int64); ok {
				l := &list{}
				for i := int64(0); i < n; i++ {
					l.v = append(l.v, x.v...)
				}
				return l
			}
		}
		failf("unsupported operand for %s: list", op)
	}
	if n, ok := a.(int64); ok && op == "*" {
		if _, ok := b.(string); ok {
			return pyBinop("*", b, n)
		}
		if _, ok := b.(*list); ok {
			return pyBinop("*", b, n)
		}
	}
	switch op {
	case "/":
		x, ok1 := num(a)
		y, ok2 := num(b)
		if ok1 && ok2 {
			if y == 0 {
				failf("division by zero")
			}
			return x / y
		}
	case "%":
		x, xi := a.(int64)
		y, yi := b.(int64)
		if xi && yi {
			if y == 0 {
				failf("division by zero")
			}
			m := x % y
			if m != 0 && (m < 0) != (y < 0) {
				m += y
			}
			return m
		}
	case "**":
		x, xi := a.(int64)
		y, yi := b.(int64)
		if xi && yi && y >= 0 {
			r := int64(1)
			for i := int64(0); i < y; i++ {
				r *= x
			}
			return r
		}
	case "&", "|", "^", "<<", ">>":
		x, y := toInt(a), toInt(b)
		switch op {
		case "&":
			return x & y
		case "|":
			return x | y
		case "^":
			return x ^ y
		case "<<":
			return x << y
		}
		return x >> y
	}
	return arith(op, a, b)
}

func pyEq(a, b any) bool {
	a, b = pyNum(a), pyNum(b)
	if x, ok := a.(*list); ok {
		y, ok := b.(*list)
		if !ok || len(x.v) != len(y.v) {
			return false
		}
		for i := range x.v {
			if !pyEq(x.v[i], y.v[i]) {
				return false
			}
		}
		return true
	}
	return equal(a, b)
}

func pyCompare(a, b any) int {
	a, b = pyNum(a), pyNum(b)
	_, as := a.(string)
	_, bs := b.(string)
	if as != bs {
		failf("'<' not supported between %s and %s", show(a), show(b))
	}
	return compare(a, b)
}

func pyStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return formatFloat(x, true)
	case *list:
		var ss []string
		for _, e := range x.v {
			ss = append(ss, pyRepr(e))
		}
		return "[" + strings.Join(ss, ", ") + "]"
	case *function:
		return "<function " + x.name + ">"
	}
	failf("cannot convert %s to str", show(v))
	return ""
}

func pyRepr(v any) string {
	s, ok := v.(string)
	if !ok {
		return pyStr(v)
	}
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	r := strconv.Quote(s)
	r = r[1 : len(r)-1]
	if q == "'" {
		r = strings.ReplaceAll(strings.ReplaceAll(r, `\"`, `"`), "'", `\'`)
	}
	return q + r + q
}

// pyFormat は "%s" % x だけを扱う。
func pyFormat(f string, arg any) string {
	args := []any{arg}
	if l, ok := arg.(*list); ok {
		args = l.v
	}
	var b strings.Builder
	k := 0
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		if i++; i >= len(f) {
			failf("incomplete format")
		}
		switch f[i] {
		case '%':
			b.WriteByte('%')
		case 's':
			if k >= len(args) {
				failf("not enough arguments for format string")
			}
			b.WriteString(pyStr(args[k]))
			k++
		default:
			failf("format %%%c is not supported", f[i])
		}
	}
	if k != len(args) {
		failf("not all arguments converted during string formatting")
	}
	return b.String()
}

func pyIter(v any) []any {
	switch x := v.(type) {
	case string:
		return chars(x)
	case *list:
		return append([]any{}, x.v...)
	}
	failf("%s is not iterable", show(v))
	return nil
}

func pySlice(o, lo, hi, st any) any {
	var xs []any
	_, isStr := o.(string)
	switch x := o.(type) {
	case string:
		xs = chars(x)
	case *list:
		xs = x.v
	default:
		failf("%s is not subscriptable", show(o))
	}
	n := int64(len(xs))
	step := int64(1)
	if st != nil {
		step = toInt(st)
	}
	if step == 0 {
		failf("slice step cannot be zero")
	}
	clamp := func(v any, def int64) int64 {
		if v == nil {
			return def
		}
		i := toInt(pyNum(v))
		if i < 0 {
			i += n
		}
		if step > 0 {
			return min(max(i, 0), n)
		}
		return min(max(i, -1), n-1)
	}
	var a, b int64
	if step > 0 {
		a, b = clamp(lo, 0), clamp(hi, n)
	} else {
		a, b = clamp(lo, n-1), clamp(hi, -1)
	}
	var r []any
	for i := a; step > 0 && i < b || step < 0 && i > b; i += step {
		r = append(r, xs[i])
	}
	if isStr {
		var sb strings.Builder
		for _, c := range r {
			sb.WriteString(c.(string))
		}
		return sb.String()
	}
	return &list{r}
}

func pyMember(o any, n string) any {
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
		case "upper":
			return m(func([]any) any { return strings.ToUpper(x) })
		case "lower":
			return m(func([]any) any { return strings.ToLower(x) })
		case "strip":
			return m(func([]any) any { return strings.TrimSpace(x) })
		case "capitalize":
			return m(func([]any) any {
				if x == "" {
					return x
				}
				r := []rune(strings.ToLower(x))
				return strings.ToUpper(string(r[0])) + string(r[1:])
			})
		case "replace":
			return m(func(a []any) any {
				if len(a) != 2 {
					failf("replace() takes 2 arguments")
				}
				old, ok1 := a[0].(string)
				nw, ok2 := a[1].(string)
				if !ok1 || !ok2 {
					failf("replace() arguments must be str")
				}
				return strings.ReplaceAll(x, old, nw)
			})
		case "join":
			return m(func(a []any) any {
				var ss []string
				for _, v := range pyIter(one(a)) {
					s, ok := v.(string)
					if !ok {
						failf("join() expects strings")
					}
					ss = append(ss, s)
				}
				return strings.Join(ss, x)
			})
		case "split":
			return m(func(a []any) any {
				var parts []string
				if len(a) == 0 {
					parts = strings.Fields(x)
				} else {
					sep, ok := one(a).(string)
					if !ok || sep == "" {
						failf("bad separator")
					}
					parts = strings.Split(x, sep)
				}
				l := &list{}
				for _, s := range parts {
					l.v = append(l.v, s)
				}
				return l
			})
		case "format":
			return m(func(a []any) any {
				var b strings.Builder
				k := 0
				for i := 0; i < len(x); i++ {
					switch {
					case strings.HasPrefix(x[i:], "{{") || strings.HasPrefix(x[i:], "}}"):
						b.WriteByte(x[i])
						i++
					case strings.HasPrefix(x[i:], "{}"):
						if k >= len(a) {
							failf("format() index out of range")
						}
						b.WriteString(pyStr(a[k]))
						k++
						i++
					case x[i] == '{' || x[i] == '}':
						failf("format() field is not supported")
					default:
						b.WriteByte(x[i])
					}
				}
				return b.String()
			})
		}
	case *list:
		switch n {
		case "append":
			return m(func(a []any) any { x.v = append(x.v, one(a)); return nil })
		case "reverse":
			return m(func([]any) any { x.v = reverse(x.v); return nil })
		case "pop":
			return m(func([]any) any {
				if len(x.v) == 0 {
					failf("pop from empty list")
				}
				v := x.v[len(x.v)-1]
				x.v = x.v[:len(x.v)-1]
				return v
			})
		}
	}
	failf("attribute %q is not supported", n)
	return nil
}
