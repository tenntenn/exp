# Go AST/SSA Visualizer

GoのソースコードのAST（抽象構文木）とSSA（Static Single Assignment）形式を可視化し、インタラクティブに探索できるWebアプリケーション。

## 特徴

- **AST可視化**: Goコードの抽象構文木をツリー形式で表示
- **SSA可視化**: SSA形式の中間表現を関数・Basic Block単位で表示
- **双方向連動**: ASTノードやSSA命令をクリックすると対応するソースコードがハイライト
- **複数ファイル対応**: txtar形式で複数ファイルのパッケージを解析
- **Go Playground統合**: コードの共有と読み込み
- **WASMベース**: バックエンドサーバー不要、ブラウザで完結

## アーキテクチャ

**WebAssembly (WASM)** を使用してGoコードを直接ブラウザで実行：

- **フロントエンド**: React + TypeScript + Vite + Monaco Editor
- **パーサー**: GoをWASMにコンパイル（バックエンドサーバー不要！）
- **ストレージ**: Go Playgroundでコード共有

## 開発

### 前提条件

- Node.js 18+
- Go 1.24+

### WASMモジュールのビルド

```bash
cd wasm
./build.sh
```

生成されるファイル:
- `frontend/public/parser.wasm` - WASMにコンパイルされたGoパーサー
- `frontend/public/wasm_exec.js` - Go WASMランタイム

### 開発サーバーの起動

```bash
cd frontend
npm install
npm run dev
```

http://localhost:5173 を開く

### プロダクションビルド

```bash
# WASMをビルド
cd wasm
./build.sh

# フロントエンドをビルド
cd ../frontend
npm run build

# 出力: frontend/dist/
```

### テストの実行

```bash
cd frontend
npm test              # ヘッドレスモードで実行
npm run test:ui       # UIモードで実行
```

## 使い方

### 単一ファイル

Goコードを貼り付けて "Parse" をクリック:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
```

### 複数ファイル（txtar形式）

txtar形式で複数ファイルを扱う:

```
-- main.go --
package main

import "fmt"

func main() {
    fmt.Println(Greet("World"))
}

-- greet.go --
package main

func Greet(name string) string {
    return "Hello, " + name + "!"
}
```

### コードの共有

1. "Share" ボタンをクリック
2. URLがクリップボードにコピーされる
3. URLを他の人に送る
4. 相手はコードを表示・編集できる

## 技術詳細

### WASM統合

GoパーサーはWebAssemblyにコンパイルされ、ブラウザで直接実行されます:

1. `wasm/main.go` が `goParse` 関数をJavaScriptにエクスポート
2. `frontend/src/wasm/loader.ts` がWASMモジュールをロード・初期化
3. パースリクエストはHTTP APIの代わりにWASMで処理

メリット:
- ✅ バックエンドサーバー不要
- ✅ 高速なパース（ネットワークレイテンシなし）
- ✅ オフラインで動作
- ✅ 簡単なデプロイ（静的ファイルのみ）

### ファイル構造

```
.
├── wasm/                   # Go WASMソース
│   ├── main.go            # WASMエントリーポイント
│   └── build.sh           # WASMビルドスクリプト
├── backend/               # Goパーサーライブラリ
│   ├── parser/            # AST/SSAパース処理
│   ├── model/             # データモデル
│   └── api/               # APIハンドラ（参考用）
├── frontend/              # Reactフロントエンド
│   ├── src/
│   │   ├── components/    # Reactコンポーネント
│   │   ├── wasm/          # WASMローダー
│   │   └── store/         # Zustand状態管理
│   ├── tests/             # Playwrightテスト
│   └── public/            # 静的ファイル + WASM
└── README.md
```

## デプロイ

`frontend/dist/` を任意の静的ホスティングサービスにデプロイ:

- Vercel
- Netlify
- GitHub Pages
- AWS S3 + CloudFront
- 任意のWebサーバー（nginx、Apacheなど）

バックエンドの設定は不要！

## ライセンス

MIT
