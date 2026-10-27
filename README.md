# HelloWorldCLI

ソースファイルを1つ渡すと、そのプログラムが `HelloWorld!` だけを出力するかを判定する CLI です。答えは2つだけです。出力するなら `HelloWorld!` を表示して終了コード 0、しないなら理由を表示して終了コード 1 で終わります。

判定は、このリポジトリの Go のコードだけで行います。コンパイラ、インタプリタ、Docker、ネットワークは使いません。

```console
$ HelloWorldCLI hello.py          # print("Hello" + "World!")
HelloWorld!

$ HelloWorldCLI hello.js          # console.log("Hello", "World!")
error: prints "Hello World!\n"

$ HelloWorldCLI hello.ook
HelloWorld!

$ HelloWorldCLI prog.txt python   # 拡張子が違うときは言語を2つ目の引数で渡す
HelloWorld!
```

## 入れ方

```sh
go install github.com/iiinno/HelloWorldCLI@latest
```

## しくみ

プログラムが何を出力するかは、その言語の意味に沿って評価しない限り分かりません。同じ1行の `HelloWorld!` でも、PHP なら出力され、Python なら文法の誤りになります。そこでこの CLI は、言語ごとの解釈器を自前で持ち、プログラムを実際に評価します。

構造は次の3層です。

1. 字句の切り出し ([lex.go](lex.go)) と式の読み取り ([parse.go](parse.go)) は全言語で共有する。コメント、引用符、演算子の強さといった違いは、言語ごとの設定で渡す。
2. 言語ごとの読み取り部 ([py.go](py.go)、[js.go](js.go)、[lisp.go](lisp.go)) が、プログラムを共通の実行器 ([core.go](core.go)) の命令に翻訳する。文字列と数の足し算の結果や、何を偽とみなすかといった意味の違いは、この層で言語の決まりに合わせる。
3. 共通の実行器が翻訳した命令を動かし、標準出力に書かれた文字を集める。

意味の小さい難解言語 ([eso.go](eso.go)) は、言語ごとに丸ごと解釈します。

次のときは NO です。

1. 出力が `HelloWorld!` と違う (末尾の改行は1つまで許す)。出力が `HelloWorld!` の頭から外れた時点で評価を止める。
2. 標準エラー出力に書く、0 以外の終了コードで終わる、実行時の誤りで止まる。
3. 1000万歩で終わらない。止まらないプログラムを見分けることは一般にできない (停止性問題) ため、歩数で打ち切る。
4. 入力を読む。入力は空として扱う。
5. この解釈器が扱わない書き方を使う。理由はエラー文に出る。

## 対応言語

| 系統 | 言語 |
|---|---|
| スクリプト | Python 3、JavaScript (`.js` `.cjs` `.mjs`)、TypeScript |
| S 式 | Scheme、Racket、Common Lisp、Clojure |
| 難解言語 | Brainfuck、Ook!、Blub、Alphuck、Spoon、Pikalang、Befunge-93、`///`、Deadfish、HQ9+ |

## 正しさの確かめ方

`go test ./...` が次の3つを確かめます。

1. [testdata/ok](testdata/ok) の見本が YES、[testdata/ng](testdata/ng) の見本が NO になる。
2. 見本を手元の本物の処理系 (Python、Node.js、Deno、Guile、Racket、SBCL、Clojure のうち入っているもの) でも動かし、YES と NO が本物の結果と一致する。
3. Brainfuck 系の置き換え表で Try It Online の見本を評価すると `Hello, World!` が出る。これで Alphuck と Spoon の表が正しいと分かる。

## ライセンス

MIT
