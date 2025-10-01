# バッチ処理システム仕様書

## 概要

GitHubリポジトリを自動収集し、Gemini CLIを使用してリポジトリの内容を分析するバッチ処理システムの仕様書です。

## 目的

- `repositories` テーブルに登録されているリポジトリのうち、未チェックのものを自動分析
- 事前定義されたチェック項目に基づいてGemini CLIで分析実行
- 分析結果を `easy_checked_repositories` テーブルに保存
- 効率的なリソース管理とクリーンアップ機能

## システム要件

### 必須環境
- Go 1.21以上
- Node.js 18以上
- PostgreSQL
- Git
- ネットワーク接続（GitHub API、Gemini API）

### 事前準備
```bash
# Gemini CLI OAuth認証
npx -y @google/gemini-cli auth
```

## データベース設計

### 既存テーブル

#### repositories
```sql
CREATE TABLE repositories (
    id SERIAL PRIMARY KEY,
    url VARCHAR(255) UNIQUE NOT NULL,
    name_with_owner VARCHAR(255) NOT NULL,
    stargazer_count INTEGER NOT NULL,
    primary_language VARCHAR(100),
    has_dockerfile BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 追加テーブル

#### check_queries
```sql
CREATE TABLE check_queries (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    query_template TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### easy_checked_repositories
```sql
CREATE TABLE easy_checked_repositories (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id),
    check_query_id INTEGER REFERENCES check_queries(id),
    gemini_response TEXT,
    status VARCHAR(50) DEFAULT 'pending', -- pending, completed, failed
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repository_id, check_query_id)
);
```

#### batch_progress
```sql
CREATE TABLE batch_progress (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(255) UNIQUE NOT NULL,
    current_repository_id INTEGER,
    total_repositories INTEGER,
    completed_repositories INTEGER,
    status VARCHAR(50) DEFAULT 'running', -- running, paused, completed, failed
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## 処理フロー

### 1. 対象リポジトリ抽出
```sql
SELECT r.* FROM repositories r
LEFT JOIN easy_checked_repositories ecr ON r.id = ecr.repository_id
WHERE ecr.repository_id IS NULL
ORDER BY r.stargazer_count DESC;
```

### 2. リポジトリ単位処理サイクル

```
FOR each repository:
  1. tmp_repositories/{repository_id}/ にクローン
  2. 全チェック項目を順次実行
     - チェック項目取得
     - プロンプトファイル生成
     - npx -y @google/gemini-cli --prompt-file {prompt} 実行
     - 結果を easy_checked_repositories に保存
  3. tmp_repositories/{repository_id}/ を完全削除
  4. 次のリポジトリへ継続
```

### 3. エラーハンドリング
- **Git操作エラー**: 次のリポジトリへスキップ
- **Gemini API エラー**: 3回まで再試行、失敗時はstatusを'failed'に設定
- **ネットワークエラー**: 指数バックオフで再試行
- **ディスク容量不足**: 古いクローンを強制削除

## 実装構成

### ディレクトリ構造
```
api/
├── cmd/
│   └── batch_analyzer/
│       └── main.go                 # バッチ処理エントリーポイント
├── internal/
│   ├── batch/
│   │   ├── analyzer.go            # メイン処理ロジック
│   │   ├── git_manager.go         # Git操作管理
│   │   ├── gemini_client.go       # Gemini CLI クライアント
│   │   └── repository_processor.go # リポジトリ処理
│   └── database/
│       ├── models.go              # 既存モデル
│       └── batch_models.go        # バッチ処理用モデル
└── tmp_repositories/              # 一時クローンディレクトリ
```

### 主要コンポーネント

#### GeminiClient
```go
type GeminiClient struct {
    workingDir string
    timeout    time.Duration
}

func (gc *GeminiClient) AnalyzeRepository(repoPath string, query CheckQuery) (string, error)
func (gc *GeminiClient) CheckAuthentication() error
```

#### GitManager
```go
type GitManager struct {
    baseDir string
}

func (gm *GitManager) Clone(repoURL, destPath string) error
func (gm *GitManager) Cleanup(repoPath string) error
```

#### RepositoryProcessor
```go
type RepositoryProcessor struct {
    db           *database.DB
    geminiClient *GeminiClient
    gitManager   *GitManager
    workDir      string
}

func (rp *RepositoryProcessor) ProcessRepository(repo Repository) error
```

## Gemini CLI連携

### 実行コマンド
```bash
npx -y @google/gemini-cli --prompt-file {prompt_file}
```

### プロンプトファイル構造
```
チェック項目: {check_query.name}
説明: {check_query.description}

質問: {check_query.query_template}

対象リポジトリの構造と主要ファイル:
{repository_content}

上記の情報に基づいて分析してください。
```

### リポジトリ内容収集
- ディレクトリ構造の取得
- 重要ファイル（README.md、package.json、Dockerfile等）の内容抽出
- プログラミング言語ファイルの部分読み込み
- バイナリファイルと.gitディレクトリの除外

## パフォーマンスとリソース管理

### 並行処理制限
- 同時処理リポジトリ数: 1（メモリ効率重視）
- Gemini API呼び出し間隔: 1秒
- Git操作タイムアウト: 30秒
- Gemini CLI実行タイムアウト: 60秒

### ディスク管理
- 各リポジトリ処理完了後の即座クリーンアップ
- 最大ディスク使用量制限: 1GB
- 大容量リポジトリ（>100MB）の部分読み込み

### メモリ管理
- リポジトリ単位での処理によるメモリ使用量最小化
- プロンプトファイルの一時作成と削除
- defer文による確実なリソース解放

## 運用コマンド

### 新規バッチ開始
```bash
go run cmd/batch_analyzer/main.go --start
```

### 中断されたバッチ再開
```bash
go run cmd/batch_analyzer/main.go --resume --session-id={session_id}
```

### 進捗確認
```bash
go run cmd/batch_analyzer/main.go --status
```

### 失敗したリポジトリ再試行
```bash
go run cmd/batch_analyzer/main.go --retry-failed
```

## ログとモニタリング

### ログレベル
- INFO: 処理進捗、完了通知
- WARN: リトライ実行、スキップ処理
- ERROR: 処理失敗、認証エラー

### メトリクス
- 処理済みリポジトリ数
- 成功/失敗率
- 平均処理時間
- API呼び出し頻度

## エラー対応

### 認証エラー
```
Error: gemini-cli authentication failed
Solution: npx -y @google/gemini-cli auth
```

### Git操作エラー
```
Error: Repository clone failed
Action: Skip to next repository, log error
```

### API制限エラー
```
Error: Gemini API rate limit
Action: Exponential backoff retry
```

## セキュリティ考慮事項

- OAuth認証による安全なAPI アクセス
- 一時ファイルの確実な削除
- プライベートリポジトリへの適切なアクセス制御
- ログ出力における機密情報の除外

## 今後の拡張予定

- 並行処理数の動的調整
- 処理結果の統計分析機能
- Web UIでの進捗監視
- Docker化による環境標準化
- CI/CD パイプラインとの統合