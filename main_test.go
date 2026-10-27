package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func check(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return judge(string(src), strings.TrimPrefix(filepath.Ext(path), "."))
}

// testdata/ok は YES、testdata/ng は NO になる。
func TestExamples(t *testing.T) {
	for _, dir := range []string{"ok", "ng"} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*"))
		for _, f := range files {
			t.Run(dir+"/"+filepath.Base(f), func(t *testing.T) {
				err := check(f)
				if dir == "ok" && err != nil {
					t.Fatal(err)
				}
				if dir == "ng" && err == nil {
					t.Fatal("judged YES")
				}
			})
		}
	}
}

// Brainfuck 系の置き換え表は、Try It Online の見本が "Hello, World!" を出すことで正しいと分かる。
func TestBrainfuckTables(t *testing.T) {
	for _, c := range []struct{ file, ops string }{
		{"brainfuck", "> < + - . , [ ]"},
		{"alphuck", "a c e i j o p s"},
		{"spoon", "010 011 1 000 001010 0010110 00100 0011"},
	} {
		src, err := os.ReadFile(filepath.Join("testdata", "bf", c.file+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		out.Reset()
		func() {
			defer func() { recover() }() // "Hello, World!" は HelloWorld! と違うので途中で止まる
			brainfuck(strings.Fields(c.ops), string(src), false)
		}()
		if got := out.String(); !strings.HasPrefix("Hello, World!", got) || len(got) < 6 {
			t.Errorf("%s: got %q", c.file, got)
		}
	}
}

// 見本を手元の本物の処理系でも動かし、判定が本物の結果と一致することを確かめる。処理系がなければ飛ばす。
var runners = map[string][][]string{
	"py":   {{"py", "-3", "{}"}, {"python3", "{}"}, {"python", "{}"}},
	"js":   {{"node", "{}"}},
	"cjs":  {{"node", "{}"}},
	"mjs":  {{"node", "{}"}},
	"ts":   {{"deno", "run", "-q", "{}"}, {"node", "{}"}},
	"scm":  {{"guile", "--no-auto-compile", "{}"}},
	"rkt":  {{"racket", "{}"}},
	"lisp": {{"sbcl", "--script", "{}"}},
	"clj":  {{"clojure", "-M", "{}"}},
}

func TestAgreesWithRealInterpreters(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*", "*"))
	for _, f := range files {
		cands := runners[strings.TrimPrefix(filepath.Ext(f), ".")]
		i := slices.IndexFunc(cands, func(c []string) bool { _, err := exec.LookPath(c[0]); return err == nil })
		if i < 0 {
			continue
		}
		t.Run(f, func(t *testing.T) {
			dir := t.TempDir()
			src, _ := os.ReadFile(f)
			file := filepath.Join(dir, filepath.Base(f))
			os.WriteFile(file, src, 0o644)
			args := slices.Clone(cands[i])
			for k := range args {
				args[k] = strings.ReplaceAll(args[k], "{}", file)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			real := err == nil && stderr.Len() == 0 && printsWant(stdout.String())
			if ours := check(f) == nil; ours != real {
				t.Fatalf("judged %v but %s really gives %v (stdout %q, stderr %q)", ours, args[0], real, stdout.String(), stderr.String())
			}
		})
	}
}
