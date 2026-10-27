// HelloWorldCLI: ソースファイルのプログラムが "HelloWorld!" だけを出力するかを、自前の解釈器で評価して判定する。
//
// 言語ごとに読み取り部があり、プログラムを共通の実行器 (core.go) の命令に翻訳して動かす。
// 出力が HelloWorld! だけ (末尾の改行は 1 つまで) なら YES、それ以外は全部 NO。
// 外部の処理系やネットワークは使わない。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const want = "HelloWorld!"

// langs は拡張子から読み取り部を引く表。
var langs = map[string]func(src string) error{}

func register(f func(string) error, exts ...string) {
	for _, e := range exts {
		langs[e] = f
	}
}

func init() {
	register(judgePython, "py", "pyw", "python")
}

func main() {
	if n := len(os.Args); n < 2 || n > 3 {
		fmt.Fprintln(os.Stderr, "usage: HelloWorldCLI <file> [language]")
		os.Exit(2)
	}
	lang := strings.TrimPrefix(filepath.Ext(os.Args[1]), ".")
	if len(os.Args) == 3 {
		lang = os.Args[2]
	}
	src, err := os.ReadFile(os.Args[1])
	if err == nil {
		err = judge(string(src), lang)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(want)
}

func judge(src, lang string) error {
	f, ok := langs[strings.ToLower(lang)]
	if !ok {
		return fmt.Errorf("unknown language %q (give it as the 2nd argument)", lang)
	}
	return f(strings.ReplaceAll(strings.TrimPrefix(src, "\xef\xbb\xbf"), "\r\n", "\n"))
}

func printsWant(out string) bool {
	return out == want || out == want+"\n" || out == want+"\r\n"
}
