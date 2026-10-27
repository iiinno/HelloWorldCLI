package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	ext := filepath.Ext(path)
	return judge(string(src), strings.TrimPrefix(ext, "."), strings.TrimSuffix(filepath.Base(path), ext))
}

// 表の型はすべて正規表現になり、TIO の見本はその見本自身に一致する。
func TestTable(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range blocks() {
		if len(b.exts) == 0 {
			t.Errorf("%s: no extension", b.name)
			continue
		}
		for _, p := range patterns(b) {
			if _, err := regexp.Compile(compile(p, b.flags, "hello")); err != nil {
				t.Errorf("%s: %v", b.name, err)
			}
		}
		if strings.HasSuffix(b.name, "(TIO)") {
			if err := judge(b.body, b.exts[0], "hello"); err != nil {
				t.Errorf("%s: its own sample is rejected: %v", b.name, err)
			}
		}
		seen[strings.TrimSuffix(b.name, " (TIO)")] = true
	}
	t.Logf("%d languages", len(seen))
}

// testdata/ok は合格し、testdata/ng は不合格になる。
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
					t.Fatal("judged as HelloWorld!")
				}
			})
		}
	}
}

// Brainfuck 系の置き換え表は、TIO の見本が "Hello, World!" を出すことで正しいと分かる。
func TestBrainfuckTables(t *testing.T) {
	for _, b := range blocks() {
		src, err := os.ReadFile(filepath.Join("testdata", "bf", b.exts[len(b.exts)-1]+".txt"))
		if err != nil {
			continue
		}
		if out, ok := brainfuck(strings.Fields(b.body), string(src), false); !ok || out != "Hello, World!" {
			t.Errorf("%s: got %q", b.name, out)
		}
	}
}

// 合格する見本を、手元にある処理系で実際に動かす。見本が本当に HelloWorld! を出すことを現物で確かめる。
var runners = map[string][][]string{
	"py":   {{"py", "{}"}, {"python3", "{}"}, {"python", "{}"}},
	"js":   {{"node", "{}"}},
	"mjs":  {{"node", "{}"}},
	"cjs":  {{"node", "{}"}},
	"ts":   {{"deno", "run", "-q", "{}"}, {"node", "{}"}},
	"rb":   {{"ruby", "{}"}},
	"pl":   {{"perl", "{}"}},
	"php":  {{"php", "{}"}},
	"lua":  {{"lua", "{}"}, {"lua5.4", "{}"}},
	"tcl":  {{"tclsh", "{}"}},
	"awk":  {{"awk", "-f", "{}"}},
	"sh":   {{"sh", "{}"}},
	"bash": {{"bash", "{}"}},
	"zsh":  {{"zsh", "{}"}},
	"fish": {{"fish", "{}"}},
	"ps1":  {{"pwsh", "-NoProfile", "-File", "{}"}, {"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "{}"}},
	"bat":  {{"cmd", "/d", "/c", "{}"}},
	"sql":  {{"sqlite3", "-batch", "-init", "{}", ":memory:", ".quit"}},
	"go":   {{"go", "run", "{}"}},
	"java": {{"java", "{}"}},
	"exs":  {{"elixir", "{}"}},
	"r":    {{"Rscript", "{}"}},
	"c":    {{"gcc", "{}", "-o", "{}.exe", "&&", "{}.exe"}},
	"cpp":  {{"g++", "{}", "-o", "{}.exe", "&&", "{}.exe"}},
	"rs":   {{"rustc", "{}", "-o", "{}.exe", "&&", "{}.exe"}},
}

func TestRealOutput(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "ok", "*"))
	for _, f := range files {
		cands := runners[strings.TrimPrefix(filepath.Ext(f), ".")]
		i := slices.IndexFunc(cands, func(c []string) bool { _, err := exec.LookPath(c[0]); return err == nil })
		if i < 0 {
			continue
		}
		t.Run(filepath.Base(f), func(t *testing.T) {
			dir := t.TempDir()
			src, _ := os.ReadFile(f)
			file := filepath.Join(dir, filepath.Base(f))
			os.WriteFile(file, src, 0o644)
			var out []byte
			for _, step := range strings.Split(strings.Join(cands[i], "\x00"), "\x00&&\x00") {
				args := strings.Split(strings.ReplaceAll(step, "{}", file), "\x00")
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				cmd := exec.CommandContext(ctx, args[0], args[1:]...)
				cmd.Dir = dir
				var err error
				if out, err = cmd.Output(); err != nil {
					t.Fatalf("%v: %v", args, err)
				}
			}
			if !printsWant(string(out)) {
				t.Fatalf("really printed %q", out)
			}
		})
	}
}
