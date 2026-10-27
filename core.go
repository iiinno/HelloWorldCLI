package main

// 共通の実行器。どの言語も、読み取り部がここの code (環境を受けて値を返す関数) に翻訳してから動かす。
// 値は string, int64, float64, bool, nil, *list, *function, unit (言語ごとの「値なし」) のどれか。

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const stepLimit = 10_000_000

type code func(*env) any

type list struct{ v []any }

type function struct {
	name string
	call func(args []any, kw map[string]any) any
}

type undefined struct{} // JavaScript の undefined

// fail は判定を NO にする理由。翻訳でも実行でも、分からないこと・エラーになることは全部これで止める。
type fail string

func failf(f string, a ...any) { panic(fail(fmt.Sprintf(f, a...))) }

type exit struct{ code int64 } // プログラムが自分で終了した

type brk struct{}
type cont struct{}
type ret struct{ v any }

type env struct {
	vars   map[string]any
	up     *env
	fn     bool            // 関数の境目 (JavaScript の var はここに置く)
	consts map[string]bool // 代入できない名前
}

func newEnv(up *env) *env { return &env{vars: map[string]any{}, up: up} }

func (e *env) find(n string) *env {
	for ; e != nil; e = e.up {
		if _, ok := e.vars[n]; ok {
			return e
		}
	}
	return nil
}

func (e *env) get(n string) any {
	if s := e.find(n); s != nil {
		return s.vars[n]
	}
	failf("%s is not defined", n)
	return nil
}

// set は既にある変数に代入し、なければ今の場所に作る。
func (e *env) set(n string, v any) {
	if s := e.find(n); s != nil {
		s.vars[n] = v
		return
	}
	e.vars[n] = v
}

// 実行中の状態。判定は 1 回に 1 つなので大域に置く。
var (
	out   strings.Builder
	steps int
)

func tick() {
	if steps++; steps > stepLimit {
		failf("did not finish within %d steps", stepLimit)
	}
}

// write は標準出力に書く。HelloWorld! の頭から外れた時点で、もう合格しないので止める。
func write(s string) {
	out.WriteString(s)
	o := out.String()
	if !strings.HasPrefix(want+"\r\n", o) && !strings.HasPrefix(want+"\n", o) {
		failf("prints %q", o)
	}
}

// run は翻訳済みのプログラムを動かし、HelloWorld! だけを出したかを返す。
func run(prog func()) (err error) {
	out.Reset()
	steps = 0
	defer func() {
		switch r := recover().(type) {
		case nil:
		case fail:
			err = fmt.Errorf("%s", string(r))
		case exit:
			err = finish(r.code)
		case brk, cont, ret:
			err = fmt.Errorf("break, continue or return outside its place")
		default:
			panic(r)
		}
	}()
	prog()
	return finish(0)
}

func finish(code int64) error {
	if code != 0 {
		return fmt.Errorf("exits with status %d", code)
	}
	if o := out.String(); !printsWant(o) {
		return fmt.Errorf("prints %q", o)
	}
	return nil
}

// 翻訳の部品。

func lit(v any) code    { return func(*env) any { return v } }
func ref(n string) code { return func(e *env) any { tick(); return e.get(n) } }
func seq(cs ...code) code {
	return func(e *env) any {
		var v any
		for _, c := range cs {
			v = c(e)
		}
		return v
	}
}

func loop(truthy func(any) bool, cond, body, step code) code {
	return func(e *env) any {
		for tick(); cond == nil || truthy(cond(e)); tick() {
			if stop := iterate(e, body); stop {
				break
			}
			if step != nil {
				step(e)
			}
		}
		return nil
	}
}

// iterate はループの 1 周を動かし、break されたかを返す。
func iterate(e *env, body code) (stop bool) {
	defer func() {
		switch r := recover(); r.(type) {
		case nil:
		case brk:
			stop = true
		case cont:
		default:
			panic(r)
		}
	}()
	body(e)
	return false
}

// closure は引数名と本体から関数を作る。
func closure(name string, params []string, defaults []code, body code, def *env) *function {
	return &function{name, func(args []any, kw map[string]any) (v any) {
		e := newEnv(def)
		e.fn = true
		for i, p := range params {
			switch {
			case i < len(args):
				e.vars[p] = args[i]
			case kw[p] != nil:
				e.vars[p] = kw[p]
			case i < len(defaults) && defaults[i] != nil:
				e.vars[p] = defaults[i](def)
			default:
				e.vars[p] = nil
			}
		}
		defer func() {
			if r := recover(); r != nil {
				rv, ok := r.(ret)
				if !ok {
					panic(r)
				}
				v = rv.v
			}
		}()
		tick()
		return body(e)
	}}
}

func call(f any, args []any, kw map[string]any) any {
	tick()
	fn, ok := f.(*function)
	if !ok {
		failf("%s is not a function", show(f))
	}
	return fn.call(args, kw)
}

func native(name string, f func(args []any) any) *function {
	return &function{name, func(args []any, _ map[string]any) any { return f(args) }}
}

// 値の扱い。

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// arith は数どうしの四則。整数どうしは整数のまま (除算 / だけは呼び出し側が決める)。
func arith(op string, a, b any) any {
	if x, ok := a.(int64); ok {
		if y, ok := b.(int64); ok {
			switch op {
			case "+":
				return x + y
			case "-":
				return x - y
			case "*":
				return x * y
			case "%":
				if y == 0 {
					failf("division by zero")
				}
				return x % y
			case "//":
				if y == 0 {
					failf("division by zero")
				}
				q := x / y
				if (x%y != 0) && ((x < 0) != (y < 0)) {
					q--
				}
				return q
			}
		}
	}
	x, ok1 := num(a)
	y, ok2 := num(b)
	if !ok1 || !ok2 {
		failf("cannot apply %s to %s and %s", op, show(a), show(b))
	}
	switch op {
	case "+":
		return x + y
	case "-":
		return x - y
	case "*":
		return x * y
	case "/":
		return x / y
	case "%":
		return math.Mod(x, y)
	case "//":
		return math.Floor(x / y)
	case "**":
		return math.Pow(x, y)
	}
	failf("unknown operator %s", op)
	return nil
}

func compare(a, b any) int {
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return strings.Compare(x, y)
		}
	}
	x, ok1 := num(a)
	y, ok2 := num(b)
	if !ok1 || !ok2 {
		failf("cannot compare %s and %s", show(a), show(b))
	}
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func equal(a, b any) bool {
	if x, ok := num(a); ok {
		if y, ok := num(b); ok {
			return x == y
		}
	}
	return a == b
}

// show はエラー文のための表示。出力には使わない。
func show(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return fmt.Sprintf("%v", v)
}

func chars(s string) []any {
	var cs []any
	for _, r := range s {
		cs = append(cs, string(r))
	}
	return cs
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		if x == math.Trunc(x) {
			return int64(x)
		}
	}
	failf("%s is not an integer", show(v))
	return 0
}

// index は負の数を後ろから数える言語向けに、範囲外を失敗にする。
func index(n int, i int64, negative bool) int {
	if negative && i < 0 {
		i += int64(n)
	}
	if i < 0 || i >= int64(n) {
		failf("index %d out of range", i)
	}
	return int(i)
}

func reverse(xs []any) []any {
	r := make([]any, len(xs))
	for i, x := range xs {
		r[len(xs)-1-i] = x
	}
	return r
}

// formatFloat は Python と JavaScript の数の表し方。数字は HelloWorld! に入らないので、桁の細部は判定に効かない。
func formatFloat(f float64, py bool) string {
	switch {
	case math.IsInf(f, 1):
		return map[bool]string{true: "inf", false: "Infinity"}[py]
	case math.IsInf(f, -1):
		return map[bool]string{true: "-inf", false: "-Infinity"}[py]
	case math.IsNaN(f):
		return map[bool]string{true: "nan", false: "NaN"}[py]
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if py && !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}
