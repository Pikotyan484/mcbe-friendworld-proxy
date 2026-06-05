# Minecraft Bedrock 26.20 フレンドワールドプロキシ

Minecraft Bedrock Edition **26.20** (プロトコルバージョン **975**) のフレンドワールドに、
ローカルプロキシ経由で接続し、Coding 機能でカスタムコードを実行するためのツールです。

## 特徴

- ✅ **自動認証**: `microsoft.com/link` を使用したデバイスコード認証
- ✅ **セッション自動選択**: フレンドが開いているワールドを自動検出・選択
- ✅ **Code Builder フック**: カスタムコードの注入が可能
- ✅ **127.0.0.1:19132 でリッスン**: 通常の Minecraft クライアントとして接続可能
- ✅ **WebRTC/NetherNet**: 公式のプロトコルを使用した安全な接続

## インストール

```bash
cd /workspace/fw-proxy
go mod tidy
go build -o fw-proxy ./cmd/fw-proxy
```

## 使い方

### 基本（自動認証 + 自動セッション選択）

```bash
./fw-proxy -auto
```

1. ブラウザで `https://microsoft.com/link` を開く
2. 表示されたコードを入力
3. フレンドのワールドが自動検出され、最初のワールドに接続
4. Minecraft から `127.0.0.1:19132` に接続

### カスタムコードを注入

```bash
./fw-proxy -auto -code "player.chat('Hello World!')"
```

Coding 機能を使用すると、指定したコードが自動的に実行されます。

### 全オプション

```bash
./fw-proxy \
  -listen "127.0.0.1:19132" \   # リッスンアドレス（デフォルト）
  -auto \                       # フレンドのワールドを自動選択
  -session "SESSION_ID" \       # 特定のセッション ID を指定（省略可能）
  -code "CODE"                  # カスタム Code Builder コード（省略可能）
```

## アーキテクチャ

```
┌─────────────┐     UDP      ┌─────────────┐    WebRTC    ┌──────────────┐
│  Minecraft  │ ───────────► │   fw-proxy  │ ───────────► │ Friend World │
│   Client    │  127.0.0.1   │   Proxy     │   NetherNet  │   Server     │
└─────────────┘              └─────────────┘               └──────────────┘
                                   │
                                   ▼
                          ┌─────────────────┐
                          │ Xbox Live Auth  │
                          │ RTA WebSocket   │
                          │ Session API     │
                          └─────────────────┘
```

## パッケージ構成

| パッケージ | 説明 |
|------------|------|
| `auth` | Xbox Live 認証（デバイスコードフロー） |
| `rta` | Real-Time API（セッション管理、WebSocket シグナリング） |
| `nethernet` | WebRTC コア接続（NetherNet プロトコル） |
| `protocol` | Minecraft パケット処理（26.20/975 対応） |
| `fwproxy` | フレンドワールドプロキシ本体 |
| `cmd/fw-proxy` | CLI エントリーポイント |

## 対応バージョン

- **Minecraft Bedrock**: 26.20
- **プロトコルバージョン**: 975

## 注意事項

- このツールは教育目的です。利用は自己責任で行ってください。
- フレンドのワールドに接続するには、そのフレンドと Xbox Live で友達である必要があります。
- Coding 機能の悪用は避けてください。

## ライセンス

MIT License
