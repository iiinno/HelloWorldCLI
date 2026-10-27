// gen は Try It Online の languages.json から tio.txt を作る。
//
//	curl -o languages.json https://tio.run/languages.json
//	go run ./gen languages.json > tio.txt
//
// TIO の各言語の Hello World は TIO 上で "Hello, World!" を出すと確かめられている。
// その文字列を HelloWorld! に置き換えて、空白も含めて丸ごと一致する型にする。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// 置き換えると出力が変わる見本。文字列の長さを数で持つもの、入力やファイルに頼るもの、意味を確かめられないもの。
var skip = map[string]string{
	"a-pear-tree":    "検査和が本文に依存する",
	"assembly-fasm":  "長さ 13 を埋め込んでいる",
	"assembly-gcc":   "長さ 13 を埋め込んでいる",
	"assembly-jwasm": "長さ 13 を埋め込んでいる",
	"blc":            "16 進ダンプで、文字列は飾り",
	"bot-engine":     "文字数ぶんの命令を並べている",
	"commercial":     "意味を確かめられない",
	"cquents":        "意味を確かめられない",
	"d2":             "意味を確かめられない",
	"dstack":         "意味を確かめられない",
	"ed":             "ファイルへ書き出す",
	"face":           "意味を確かめられない",
	"llvm":           "長さ 14 を埋め込んでいる",
	"lmbm":           "文字数ぶんの命令を並べている",
	"minkolang":      "文字数ぶんの命令を並べている",
	"nqt":            "意味を確かめられない",
	"sed":            "入力に頼る",
	"sed-gnu":        "入力に頼る",
	"thutu":          "意味を確かめられない",
	"verbosity":      "意味を確かめられない",
	"verbosity2":     "意味を確かめられない",
	"wasm":           "長さ 13 を埋め込んでいる",
}

func main() {
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var langs map[string]struct {
		Name  string
		Tests struct {
			HelloWorld struct {
				Request []struct {
					Command string
					Payload map[string]any
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &langs); err != nil {
		panic(err)
	}
	ids := make([]string, 0, len(langs))
	for id := range langs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		l := langs[id]
		code, ok := "", true
		for _, r := range l.Tests.HelloWorld.Request {
			if r.Command == "V" {
				ok = false // 実行時の引数や旗が要る
			}
			if c, isStr := r.Payload[".code.tio"].(string); r.Command == "F" && isStr {
				code = c
			}
		}
		code = strings.TrimRight(strings.ReplaceAll(code, "Hello, World!", "HelloWorld!"), "\n")
		if !ok || skip[id] != "" || !strings.Contains(code, "HelloWorld!") ||
			strings.ContainsAny(code, "⟨⟩⟦⟧｜◇◆") || strings.Contains(code, "\n== ") {
			continue
		}
		fmt.Printf("== %s (TIO) | %s | exact |\n%s\n", l.Name, id, code)
	}
}
