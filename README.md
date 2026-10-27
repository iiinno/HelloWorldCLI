# HelloWorld!CLI

ソースファイルを1つ渡すと、そのプログラムが `HelloWorld!` だけを出力するかを判定する CLI です。合格なら `HelloWorld!` を表示し、不合格なら理由を表示して終了コード 1 で終わります。

判定は、このリポジトリの Go のコードだけで行います。コンパイラ、インタプリタ、Docker、ネットワークは使いません。456 の言語と処理系に対応し、バイナリ1つ (約 2MB) で動きます。

A self-contained single-binary judge that decides, without running any external tool, whether a source file in one of 456 languages prints exactly `HelloWorld!`.

```console
$ HelloWorldCLI hello.cob
HelloWorld!

$ HelloWorldCLI hello.py        # print("Hello World!")
error: not a program that prints exactly HelloWorld! (checked as Python)

$ HelloWorldCLI hello.ook       # Ook! で書いた HelloWorld!
HelloWorld!

$ HelloWorldCLI prog.txt python # 拡張子が違うときは言語を2つ目の引数で渡す
HelloWorld!
```

## 入れ方

Go が入っていれば次の1行で入ります。

```sh
go install github.com/iiinno/HelloWorldCLI@latest
```

Go がなければ [Releases](https://github.com/iiinno/HelloWorldCLI/releases) から OS に合うバイナリを1つ落とします。

## しくみ

「任意のプログラムが何を出力するか」は、動かさずに一般に決めることはできません (停止性問題)。そこでこの CLI は問いを裏返し、「確実に `HelloWorld!` だけを出力すると分かっている形か」を判定します。

言語ごとに、確実に `HelloWorld!` だけを出力するプログラムの形 (以下、型) を表に書いておき、ファイルがその型に丸ごと一致したときだけ合格にします。一致しないものはすべて不合格です。このため、合格と言ったプログラムは本当に `HelloWorld!` を出力します。逆に、出力は正しくても書き方が型にないプログラムは不合格になります。正しいプログラムを落とすことはあっても、間違ったプログラムを通すことはない、という方針で作っています。

型は、実際のプログラムをそのまま書き、揺れてよい所に記号を入れたものです。C の型は次のとおりです。

```
#include <stdio.h>
int main(⟦void｜int argc, char **argv｜int argc, char *argv[]⟧) { ⟨puts("HelloWorld!");｜printf("HelloWorld!⟦\n⟧");｜fputs("HelloWorld!⟦\n⟧", stdout);⟩ ⟦return 0;⟧ }
```

記号は7つです。

| 記号 | 意味 |
|---|---|
| `⟨a｜b⟩` | a か b のどちらか |
| `⟦a⟧` | a は省略できる |
| `◇` | 任意の識別子 |
| `◆` | ファイル名 (Java の public class 名や Ada の手続き名のように、ファイル名と一致しないとコンパイルできない所に使う) |
| `␣` | ちょうど1つの空白 (Forth の `."` のように、空白の数が出力に入る所に使う) |
| 空白 | 何文字でもよい空白。ただし `int main` のように語と語の間では1文字以上 |
| 改行 | 1つ以上の改行 |

言語ごとの旗で、空白の扱いを変えます。`free` は改行も空白として扱い (C や Java)、`indent` は字下げを型より浅くさせず (Python や COBOL)、`exact` は空白を1文字もゆるめず (Befunge のような2次元の言語)、`i` は大文字小文字を区別しません (Fortran や COBOL)。`i` の言語でも `HelloWorld!` の部分だけは常に区別します。

この型は書き方の揺れを広く受け付けつつ、出力を変える違いは通しません。たとえば次のものは、すべて不合格になります ([testdata/ng](testdata/ng))。

1. C で `//` の行末に `\` を置き、次の行の `#include` をコメントに取り込んだもの。
2. Python で `print` と `("HelloWorld!")` の間に改行を入れたもの。2つの文になり、何も出力しない。
3. Forth の `."` のあとに空白を2つ置いたもの。2つ目の空白が出力に入る。
4. COBOL を1桁目から書いたもの。固定形式では8桁目より前は命令として読まれない。
5. Java の public class 名がファイル名と違うもの。

Brainfuck とその派生言語 (Ook!、Blub、Alphuck、Spoon、Pikalang) だけは、表の代わりに中の小さな解釈器で実際に評価します。派生言語は、8つの命令に当たる語を表に並べるだけで増やせます。入力を読むものと、1000万歩で終わらないものは不合格です。

## 表は2つある

[langs.txt](langs.txt) は手で書いた表で、129 言語あります。主要な言語に加え、COBOL、Fortran、Ada、ALGOL 68、Simula、Modula-2、Oberon、SNOBOL4、Eiffel、Forth、Unlambda、Befunge などを、書き方の揺れまで含めて書いています。

[tio.txt](tio.txt) は [gen](gen/main.go) が作った表で、380 言語あります。[Try It Online](https://tio.run) の言語一覧 (languages.json) には、言語ごとに「TIO 上で `Hello, World!` を出すと確かめられた見本」が入っています。gen はその文字列を `HelloWorld!` に置き換え、空白も含めて丸ごと一致する型にします。文字列の長さを数で埋め込んだ見本 (アセンブリの `mov edx, 13` など) や、入力に頼る見本 (sed など) は、置き換えると出力が変わるので除いています。除いた理由は gen のコードに書いてあります。TIO を使うのは表を作るときだけで、CLI は TIO に依存しません。

tio.txt の言語は、TIO の言語名を拡張子として使います。たとえば 05AB1E は `hello.05ab1e`、ArnoldC は `hello.arnoldc` です。

## 正しさの確かめ方

`go test ./...` が次の4つを確かめます。

1. 表のすべての型が正規表現に変換でき、TIO 由来の型がその見本自身に一致する。
2. [testdata/ok](testdata/ok) の見本が合格し、[testdata/ng](testdata/ng) の見本が不合格になる。
3. testdata/ok の見本のうち、処理系が手元にある言語は、実際に動かして本当に `HelloWorld!` を出す。処理系がなければ飛ばす。
4. Brainfuck 系の置き換え表で TIO の見本を評価すると `Hello, World!` が出る。これで Alphuck と Spoon の表が正しいと分かる。

3 だけは処理系を使いますが、これは表が正しいことの答え合わせで、判定そのものには使いません。GitHub Actions で Linux、Windows、macOS の3つで動かしています。

## 言語を足す

1. langs.txt に `== 名前 | 拡張子 | 旗 | 行コメントの頭` の見出しと型を足す。
2. testdata/ok に合格する見本を、testdata/ng に紛らわしいが不合格になる見本を置く。
3. `go test ./...` で確かめる。

## 判定の前提

プログラムは標準入力が空の状態で動かすものとして判定します。出力の末尾の改行は1つまで (`\n` か `\r\n`) 許します。言語のバージョンによって動く書き方が違う所 (Zig の標準出力の取り方など) は、主なバージョンの書き方をそれぞれ型にしました。

## 対応言語

<details>
<summary>456 の言語と処理系</summary>

*><>, ,,,, ///, 05AB1E, 05AB1E (legacy), 2sable, 33, ><>, ABC, Ada, Ada (GNAT), Adapt, Add++, Agda, Alchemist, ALGOL 68, ALGOL 68 (Genie), Alice ML, Alphuck, anyfix, APL, APL (Dyalog Classic), APL (Dyalog Extended), APL (Dyalog Unicode), APL (dzaima/APL), APL (ngn/apl), Appleseed, ARBLE, Arcyóu, ArnoldC, AsciiDots, Assembly (as, x64, Linux), Assembly (MIPS, SPIM), Assembly (NASM, Linux x86-64), Assembly (nasm, x64, Linux), ATS2, Attache, AWK, B (ybc), Ballerina, Bash, BASIC (FreeBASIC / QBasic), Batch, bc, BeanShell, Beeswax, Befunge, Befunge-96 (MTFI), Befunge-97 (MTFI), Blub, Boo, bosh, BQN, Brachylog, Brachylog v1, Bracmat, Brainfuck, Braingolf, Brat, Broccoli, Burlesque, BuzzFizz, C, C (clang), C (gcc), C (tcc), C shell, C#, C# (.NET Core), C# (Mono C# compiler), C# (Mono C# Shell), C# (Visual C# Compiler), C# (Visual C# Interactive Compiler), C++, C++ (clang), C++ (gcc), C3, Caboose, CakeML, calc (TTK), Canvas, Cardinal, Carrot, Cauliflower, Ceylon, Chain, Chapel, Charcoal, Charm, Check, Checked C, Cheddar, Chez Scheme, CHICKEN Scheme, CIL (Mono IL assembler), cixl, CJam, CLIPS, Clojure, COBOL, Cobra, Coconut, CoffeeScript, CoffeeScript 1, CoffeeScript 2, Common Lisp, Condit, Convex, Corea, CPY, Crayon, Cryptol, Crystal, Curry (PAKCS), Curry (Sloth), Cy, Cyclone, Cython, D, Dafny, Dart, Dash, dc, Deorst, Detour, dg, Dirty, DScript, Dyvil, eacal, eC, ecpp + C (gcc), ecpp + C++ (gcc), Egel, Eiffel, Elixir, Elvish, Emacs Lisp, Emoji, Emojicode, Emojicode 0.5, Enlist, Erlang (escript), es, Euphoria 3, Euphoria 4, F#, F# (.NET Core), F# (Mono), Factor, FALSE, Fantom, Farnsworth, Felix, Fennel, FEU, FIM++, fish, Fission, Fission 2, Foam, FOCAL-69, Foo, Forte, Forth, Forth (gforth), Fortran, Fortran (GFortran), Fourier, Funky, Funky 2, Fynyl, Gaia, Gambit Scheme (gsi), GAP, Gleam, gnuplot, Go, GolfScript, Groovy, Guile, Gwion, HadesLang, Hare, Haskell, Haskell 1.2 (Gofer), Haskell 98 (Hugs), Haxe, Haystack, Hobbes, Hy, I, Icon, Idris, Implicit, ink, Io, J, J-uby, Janet, Japt, Java, Java (JDK), Java (OpenJDK 8), JavaScript, JavaScript (Babel Node), JavaScript (Node.js), JavaScript (SpiderMonkey), JavaScript (V8), Jelly, Jellyfish, Joy, Julia, Julia 0.4, Julia 0.5, Julia 0.6, Julia 0.7, Julia 1.0, Jx, K (Kona), K (ngn/k), K (oK), Kipple (cipple), Kobeři-C, Koka, Kotlin, Kotlin script, ksh, Lean, Lean 4, Lily, Literate Haskell, LiveScript, LLVM IR, Logo, LOLCODE, Lua, Lua (LuaJIT), Lua (OpenResty), Ly, M, M4, Make, Mamba, Mathematica / Wolfram, MathGolf, Mathics, MATL, Maxima, MaybeLater, Mercury, Milky Way, miniML, Mirror, Modula-2, Mojo, MoonScript, Moonscript, MUMPS, Muriel, MY-BASIC, Neim, Neutrino, Nial, Nim, Noether, Nushell, Oasis, Oberon, Oberon-07, Object Pascal (FPC), Objective-C, Objective-C (clang), Objective-C (gcc), OCaml, occam-pi, Octave, Octave / MATLAB, Odin, Ohm, Ohm v2, OML, Ook!, Oration, ORK, OSH, P#, Pari/GP, Pascal, Pascal (FPC), Perl, Perl 4, Perl 5, Perl 5 (cperl), Perl 6, Perl 6 (Niecza), Phoenix, Phooey, PHP, PHP (text), Physica, PicoLisp, Pikalang, Pike, PILOT (psPILOT), PILOT (RPilot), Pip, pl, Pony, Positron, PostL, PostScript, PostScript (xpost), PowerShell, PowerShell Core, Premier, Preproc, Prolog (Ciao), Prolog (SWI), Proton, Proton 2.0, Pure, PureScript, Pushy, Pyke, Pylons, PynTree, Pyon, Pyret, Pyth, Python, Python 1, Python 2, Python 2 (Cython), Python 2 (IronPython), Python 2 (Jython), Python 2 (PyPy), Python 3, Python 3 (Cython), Python 3 (PyPy), Python 3 (Stackless), Python 3.8 (pre-release), QuadR, Quadrefunge-97 (MTFI), QuadS, Quintefunge-97 (MTFI), R, Racket, RAD, Rail, Raku, Rapira, Reason, ReasonML, Rebol, REBOL, REBOL 3, Recursiva, Red, Reng, ReRegex, ReScript, Reticular, Retina, Retina 0.8.2, REXX, Rexx (Regina), Ring, rk, Rockstar, ROOP, Rotor, RProgN, RProgN 2, Ruby, Runic Enchantments, Rust, Rutger, Röda, S.I.L.O.S, Scala, Scheme, Septefunge-97 (MTFI), Sexefunge-97 (MTFI), sfk, ShapeScript, Shell, Shnap, shortC, Sidef, Simplex, Simula, Simula (cim), SISAL, Sisi, Smalltalk (GNU), smol, SNOBOL4, SNOBOL4 (CSNOBOL4), Snowman, Spoon, SQL (SQLite), SQLite, Squirrel, Stacked, Standard ML, Standard ML (MLton), str, Sumerian, Swift, Syms, TacO, Tampio (imperative), Tamsin, Taxi, Tcl, tcsh, Teal, TemplAt, Threead, Thue, Tidy, tinylisp, Tir, TRANSCRIPT, Trefunge-97 (MTFI), TrumpScript, Turtlèd, TypeScript, uBASIC, Uiua, Underload, Unlambda, Uno, V, V (vim), V (vlang.io), Vala, VAR, Visual Basic .NET, Visual Basic .NET (.NET Core), Visual Basic .NET (Mono), Visual Basic .NET (VBC), VSL, Whispers v1, Whispers v2, Wren, Yabasic, yash, Z shell, Z3, Zephyr, Zig, zkl, Zoidberg, Zsh

</details>

## ライセンス

MIT。tio.txt の見本は Try It Online の languages.json に由来します。
