# Go AST/SSA Visualizer

GoのソースコードのAST（抽象構文木）とSSA（Static Single Assignment）形式を可視化し、インタラクティブに探索できるWebアプリケーション。

## 特徴

- **AST可視化**: Goコードの抽象構文木をツリー形式で表示
- **SSA可視化**: SSA形式の中間表現を関数・Basic Block単位で表示
- **インタラクティブ**: Monaco Editorによる高機能なコードエディタ
- **リアルタイム解析**: コードを編集して即座に解析結果を確認
- **位置情報表示**: ASTノードとSSA命令のソースコード上の位置を表示

## 技術スタック

### バックエンド
- Go 1.24+
- 標準ライブラリ (`go/parser`, `go/ast`, `go/token`, `go/types`)
- `golang.org/x/tools/go/ssa` - SSA生成
- `golang.org/x/tools/go/packages` - パッケージ解析

### フロントエンド
- React 18
- TypeScript
- Vite
- Monaco Editor
- Zustand (状態管理)

## セットアップ

### 前提条件

- Go 1.24+ がインストールされていること
- Node.js 18+ と npm がインストールされていること

### インストール

1. リポジトリをクローン
```bash
git clone https://github.com/tenntenn/exp.git
cd exp
```

2. Go依存関係をインストール
```bash
go mod tidy
```

3. フロントエンド依存関係をインストール
```bash
cd frontend
npm install
```

## 開発

### フロントエンド開発サーバーを起動

```bash
cd frontend
npm run dev
```

ブラウザで http://localhost:3000 を開く

### バックエンドサーバーを起動

別のターミナルで:

```bash
go run ./cmd/server
```

APIサーバーが http://localhost:8080 で起動します

## プロダクションビルド

### 1. フロントエンドをビルド

```bash
cd frontend
npm run build
```

### 2. ビルド済みファイルをサーバーにコピー

```bash
cp -r dist ../cmd/server/frontend/
```

### 3. Goサーバーをビルド

```bash
cd ..
go build -o bin/server ./cmd/server
```

### 4. サーバーを起動

```bash
./bin/server
```

ブラウザで http://localhost:8080 を開く

## 使い方

1. **コードを編集**: 左ペインのエディタでGoコードを編集
2. **解析実行**: 右上の "Parse" ボタンをクリック
3. **ASTを確認**: 中央ペインでAST構造を確認
4. **SSAを確認**: 右ペインでSSA形式の中間表現を確認
5. **ノードをクリック**: ASTノードやSSA命令をクリックすると、対応するソースコードの位置情報が表示されます

## プロジェクト構造

```
.
├── SPEC.md                      # 仕様書
├── README.md                    # 本ファイル
├── backend/                     # バックエンドコード
│   ├── api/                    # APIハンドラ
│   │   └── parse.go           # 解析APIエンドポイント
│   ├── model/                  # データモデル
│   │   └── types.go           # 型定義
│   └── parser/                 # 解析ロジック
│       ├── ast.go             # AST解析
│       └── ssa.go             # SSA生成
├── cmd/
│   └── server/                 # サーバーエントリーポイント
│       └── main.go
├── frontend/                    # フロントエンドコード
│   ├── src/
│   │   ├── components/        # Reactコンポーネント
│   │   │   ├── Editor.tsx    # コードエディタ
│   │   │   ├── ASTViewer.tsx # AST表示
│   │   │   ├── SSAViewer.tsx # SSA表示
│   │   │   └── Header.tsx    # ヘッダー
│   │   ├── store/            # 状態管理
│   │   │   └── app.ts        # Zustandストア
│   │   ├── types.ts          # TypeScript型定義
│   │   ├── App.tsx           # メインアプリ
│   │   └── main.tsx          # エントリーポイント
│   ├── package.json
│   └── vite.config.ts
├── go.mod
└── go.sum
```

## API仕様

### POST /api/parse

Goコードを解析してASTとSSAを返す

**リクエスト**:
```json
{
  "code": "package main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
  "format": "single"
}
```

**レスポンス**:
```json
{
  "ast": {
    "type": "File",
    "start": { "line": 1, "column": 1, "offset": 0 },
    "end": { "line": 5, "column": 1, "offset": 100 },
    "children": [...]
  },
  "ssa": [
    {
      "name": "main",
      "package": "main",
      "location": "main.go:3:1",
      "instructions": [...],
      "blocks": [...]
    }
  ],
  "errors": []
}
```

## 今後の機能拡張（仕様書参照）

Phase 2以降で実装予定:
- Go Playground統合（コード共有機能）
- txtar形式による複数ファイルサポート
- 双方向連動（AST/SSA → Code のハイライト）
- SSA Control Flow Graph (CFG) の可視化
- エラー表示の改善
- ダークモード対応

詳細は [SPEC.md](./SPEC.md) を参照してください。

## ライセンス

MIT

## 関連リンク

- [仕様書 (SPEC.md)](./SPEC.md)
- toolsinternal: mirror of golang.org/x/tools/internal
