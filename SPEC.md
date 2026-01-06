# Go AST/SSA Visualizer - 仕様書

## 1. 概要

GoのソースコードのAST（抽象構文木）とSSA（Static Single Assignment）形式を可視化し、インタラクティブに探索できるWebアプリケーション。

### 1.1 目的

- Goのコンパイラの内部動作を理解するための教育ツール
- ASTとSSAの構造を視覚的に理解できるようにする
- ソースコードとAST/SSAノードを双方向に連動させる

### 1.2 主な特徴

- データベース不要（サーバーレス・ステートレス）
- Go Playgroundとの統合（コード共有・永続化）
- 複数ファイルのサポート（txtar形式）
- ast-grep Playgroundのようなインタラクティブなノード連動UI

## 2. 機能要件

### 2.1 コア機能

#### 2.1.1 コードエディタ
- モナコエディタまたは CodeMirror を使用したGoコードエディタ
- シンタックスハイライト
- 行番号表示
- 選択範囲のハイライト機能

#### 2.1.2 AST可視化
- `go/ast` パッケージを使用したAST解析
- ツリー構造での表示
  - 折りたたみ可能なノード
  - ノードタイプの表示（識別子、式、文など）
  - ノードの詳細情報（位置情報、型情報など）
- ソースコードとの連動
  - ASTノードをクリック → 対応するソースコードをハイライト
  - ソースコードを選択 → 対応するASTノードをハイライト

#### 2.1.3 SSA可視化
- `golang.org/x/tools/go/ssa` パッケージを使用したSSA生成
- 表示形式
  - 関数ごとのSSA命令一覧
  - CFG（Control Flow Graph）の可視化（オプション）
  - Basic Block単位での表示
- ソースコードとの連動
  - SSA命令をクリック → 対応するソースコードをハイライト
  - 可能な範囲でソースコードからSSAへの逆引き

### 2.2 コード管理機能

#### 2.2.1 Go Playground統合
- **保存機能**：`github.com/tenntenn/goplayground` を使用
  - ユーザーがコードを入力/編集
  - "Share" ボタンでGo Playgroundに保存
  - ハッシュIDを取得（例: `play.golang.org/p/AbCdEfGhIj`）
  - URLに反映: `https://example.com/?share=AbCdEfGhIj`

- **読み込み機能**
  - URLパラメータ `?share=<hash>` からコードを復元
  - Go Playground APIを使ってハッシュからコードを取得
  - エディタに自動ロード

#### 2.2.2 複数ファイルサポート（txtar形式）
- **txtar形式の採用理由**
  - Go公式のテストでも使われている標準的なフォーマット
  - シンプルなテキストベースのアーカイブ形式
  - 複数ファイルを1つのテキストとして扱える

- **内部動作**
  ```
  -- main.go --
  package main

  import "fmt"

  func main() {
      fmt.Println(hello())
  }

  -- hello.go --
  package main

  func hello() string {
      return "Hello, World!"
  }
  ```

- **UI設計**
  - タブまたはファイルツリーで複数ファイルを切り替え
  - 新規ファイル追加/削除機能
  - メインファイルの指定（エントリーポイント）

#### 2.2.3 ローカルストレージ
- ブラウザのLocalStorageを使用した一時保存
- 編集中のコードの自動保存（セッション維持）
- 履歴管理（オプション）

### 2.3 UI/UX機能

#### 2.3.1 レイアウト
3ペイン構成（ast-grep Playgroundを参考）:

```
+------------------+------------------+------------------+
|                  |                  |                  |
|  Code Editor     |  AST Viewer      |  SSA Viewer      |
|                  |                  |                  |
|  - Monaco/       |  - Tree View     |  - Instruction   |
|    CodeMirror    |  - Node Details  |    List          |
|  - Multi-file    |  - Highlight     |  - CFG Graph     |
|    Tabs          |                  |    (optional)    |
|                  |                  |                  |
+------------------+------------------+------------------+
|     Controls: [Parse] [Share] [Load] [Settings]       |
+----------------------------------------------------------+
```

レスポンシブ対応:
- デスクトップ: 3ペイン横並び
- タブレット: タブ切り替え
- モバイル: 縦スタック + タブ切り替え

#### 2.3.2 インタラクション

**双方向連動**:
1. Code → AST/SSA
   - コードを選択するとASTノードとSSA命令がハイライト
   - カーソル位置に対応するノード/命令を自動スクロール

2. AST → Code
   - ASTノードをクリックすると対応コードをハイライト
   - ツリーをホバーするとコードにアンダーラインプレビュー

3. SSA → Code
   - SSA命令をクリックすると対応コードをハイライト
   - ソース位置情報がある命令のみ連動

**視覚的フィードバック**:
- ハイライトカラー: 選択中（青）、ホバー（黄）、関連（緑）
- スムーズなスクロールアニメーション
- ノード展開/折りたたみのアニメーション

### 2.4 追加機能（Nice to have）

- エラー表示（パースエラー、型エラー）
- SSAの最適化レベル選択
- ASTのJSON/S式でのエクスポート
- SSAのテキストエクスポート
- ダークモード対応
- ショートカットキー対応
- サンプルコードのプリセット

## 3. 技術スタック

### 3.1 バックエンド

**言語**: Go 1.24+

**フレームワーク**:
- 候補1: 標準ライブラリ (`net/http`)
- 候補2: Echo / Gin （軽量Webフレームワーク）
- 推奨: **標準ライブラリ** + `http.ServeMux` (シンプル、依存少ない)

**主要パッケージ**:
- `go/parser`: Goコードのパース
- `go/ast`: AST操作
- `go/token`: ソース位置管理
- `go/types`: 型チェック
- `golang.org/x/tools/go/ssa`: SSA生成
- `golang.org/x/tools/go/packages`: パッケージロード
- `golang.org/x/tools/txtar`: txtar形式のパース
- `github.com/tenntenn/goplayground`: Go Playground統合

**API設計**:
```go
// POST /api/parse
// Request: { "code": "package main...", "format": "single" | "txtar" }
// Response: { "ast": {...}, "ssa": {...}, "errors": [...] }

// POST /api/share
// Request: { "code": "...", "format": "txtar" }
// Response: { "hash": "AbCdEfGhIj", "url": "https://play.golang.org/p/AbCdEfGhIj" }

// GET /api/load?hash=AbCdEfGhIj
// Response: { "code": "...", "format": "txtar" }
```

### 3.2 フロントエンド

**言語**: TypeScript

**フレームワーク**:
- 候補1: React + Vite
- 候補2: Vue 3 + Vite
- 候補3: Svelte + SvelteKit
- 推奨: **React + Vite** （エコシステムが充実、Monaco Editorとの親和性）

**主要ライブラリ**:
- **エディタ**: Monaco Editor (VS Codeと同じエディタエンジン)
  - Goシンタックスハイライト標準搭載
  - 範囲選択、デコレーションAPI

- **UI コンポーネント**:
  - Ant Design / Material-UI / shadcn/ui
  - 推奨: **shadcn/ui** (軽量、カスタマイズ性高い)

- **ツリービュー**:
  - react-arborist / react-tree-view
  - 推奨: **react-arborist** (仮想化対応、パフォーマンス良)

- **状態管理**:
  - Zustand / Jotai / Recoil
  - 推奨: **Zustand** (シンプル、軽量)

- **HTTPクライアント**: axios / fetch API
  - 推奨: **fetch API** (標準、依存なし)

### 3.3 デプロイ

**ホスティング**:
- 候補1: Vercel (フロントエンド + Serverless Functions)
- 候補2: Cloud Run (コンテナベース)
- 候補3: Fly.io (Go アプリに最適)
- 推奨: **Vercel** または **Fly.io**

**ビルド**:
- フロントエンド: Vite でビルド → 静的ファイル
- バックエンド: Go バイナリ
- デプロイ構成: SPA + Go APIサーバー

## 4. データフロー

### 4.1 初回読み込み

```
User訪問
  ↓
URLにshareパラメータあり？
  ↓ Yes
GET /api/load?hash=xxx
  ↓
Go Playground APIからコード取得
  ↓
txtar パース
  ↓
エディタに表示
  ↓
自動的にAST/SSA解析
```

### 4.2 コード編集・解析

```
User入力
  ↓
LocalStorageに自動保存
  ↓
[Parse] ボタンクリック または 自動トリガー
  ↓
POST /api/parse
  ↓
バックエンドでAST/SSA生成
  ↓
JSONレスポンス
  ↓
フロントエンドで可視化
  ↓
連動機能有効化
```

### 4.3 コード共有

```
[Share] ボタンクリック
  ↓
現在のコードをtxtar形式化
  ↓
POST /api/share
  ↓
github.com/tenntenn/goplayground 経由でPlaygroundに保存
  ↓
ハッシュ取得
  ↓
URLを更新 (?share=xxx)
  ↓
共有リンクをクリップボードにコピー
```

## 5. AST/SSA データ構造

### 5.1 AST JSON形式

```json
{
  "type": "File",
  "pos": {"line": 1, "col": 1, "offset": 0},
  "end": {"line": 10, "col": 1, "offset": 200},
  "name": "main.go",
  "decls": [
    {
      "type": "FuncDecl",
      "pos": {...},
      "end": {...},
      "name": "main",
      "body": {
        "type": "BlockStmt",
        "list": [...]
      }
    }
  ]
}
```

### 5.2 SSA テキスト形式

```
# Name: main.main
# Package: main
# Location: main.go:5:1

0:entry
    t0 = new [1]interface{} (varargs)
    t1 = &t0[0:int]
    t2 = make interface{} <- string ("Hello, World!":string)
    *t1 = t2
    t3 = slice t0[:]
    t4 = fmt.Println(t3...)
    return

# Instructions count: 7
# Blocks count: 1
```

## 6. 非機能要件

### 6.1 パフォーマンス

- AST/SSA解析: 1,000行のコードを1秒以内で処理
- UI応答性: ノードクリックから200ms以内でハイライト
- 大規模コード対応: 10,000行まで動作（ただし警告表示）

### 6.2 互換性

- ブラウザ: Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- Go バージョン: 1.24.0+

### 6.3 セキュリティ

- サンドボックス実行: コード実行は行わない（解析のみ）
- XSS対策: ユーザー入力のサニタイズ
- CORS設定: 適切なオリジン制限

## 7. 開発フェーズ

### Phase 1: MVP (Minimum Viable Product)
- [ ] 単一ファイルのAST可視化
- [ ] シンプルなコードエディタ
- [ ] 基本的なAST/SSA解析API
- [ ] AST → Code の一方向連動

### Phase 2: Core Features
- [ ] txtar形式サポート
- [ ] Go Playground統合
- [ ] 双方向連動（Code ↔ AST ↔ SSA）
- [ ] UI/UXの改善

### Phase 3: Enhancement
- [ ] SSA CFGグラフ可視化
- [ ] エラー表示の充実
- [ ] サンプルコードプリセット
- [ ] エクスポート機能

### Phase 4: Polish
- [ ] パフォーマンス最適化
- [ ] ダークモード
- [ ] ショートカットキー
- [ ] ドキュメント整備

## 8. 参考実装

- **ast-grep Playground**: https://ast-grep.github.io/playground.html
  - UI/UX設計の参考
  - ノード連動の実装パターン

- **Go Playground**: https://play.golang.org/
  - コード共有の仕組み

- **Go AST Viewer**: https://yuroyoro.github.io/goast-viewer/
  - AST可視化の参考

- **SSA Viewer（gopherjs.org）**: https://gopherjs.github.io/playground/
  - SSA出力の表示例

## 9. ディレクトリ構成（案）

```
go-ast-ssa-visualizer/
├── backend/
│   ├── main.go                 # エントリーポイント
│   ├── api/
│   │   ├── parse.go           # AST/SSA解析API
│   │   ├── share.go           # Playground統合API
│   │   └── load.go            # コード読み込みAPI
│   ├── parser/
│   │   ├── ast.go             # AST解析ロジック
│   │   ├── ssa.go             # SSA生成ロジック
│   │   └── txtar.go           # txtar処理
│   └── model/
│       └── types.go           # データ型定義
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── Editor.tsx     # コードエディタ
│   │   │   ├── ASTViewer.tsx  # AST表示
│   │   │   ├── SSAViewer.tsx  # SSA表示
│   │   │   └── Header.tsx     # ヘッダー・コントロール
│   │   ├── hooks/
│   │   │   ├── useParser.ts   # 解析フック
│   │   │   └── useSync.ts     # 連動フック
│   │   ├── store/
│   │   │   └── app.ts         # 状態管理
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
│
├── go.mod
├── go.sum
├── README.md
├── SPEC.md                     # 本ドキュメント
└── Makefile
```

## 10. 今後の検討事項

- WebAssembly化: Go解析ロジックをWasmにしてブラウザで実行？
- リアルタイムコラボレーション: 複数人での同時編集
- プラグインシステム: カスタム解析ルール
- 型情報の可視化: 型推論結果の表示
- パッケージ依存関係の可視化: import graphの表示
