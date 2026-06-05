# go-nethernet-fw - Minecraft Bedrock Edition フレンドワールドプロキシ

Minecraft Bedrock Edition 26.20 (バージョン 1.21.90, プロトコル 975) のフレンドワールドに接続するための WebRTC ベースプロキシです。

## 特徴

- **NetherNet 対応**: WebRTC を使用した P2P 接続
- **SessionInfo サポート**: SessionID, SCID, Template, NetherNetID を含むセッション管理
- **Code Builder フック**: Coding 機能のパケットを傍受・改変可能
- **127.0.0.1:19132 でリッスン**: 通常のマインクラフトクライアントとして接続可能

## インストール

```bash
go build -o fw-proxy ./cmd/fw-proxy
```

## 使い方

```bash
./fw-proxy \
  -token "XBOX_LIVE_TOKEN" \
  -xuid "XBOX_USER_ID" \
  -session "SESSION_ID" \
  -scid "SERVICE_CONFIG_ID" \
  -template "TEMPLATE_ID" \
  -nethernet-id 123456789 \
  -listen "127.0.0.1:19132"
```

### 引数説明

| 引数 | 説明 | 必須 |
|------|------|------|
| `-token` | Xbox Live 認証トークン | はい |
| `-xuid` | Xbox User ID | はい |
| `-session` | Session ID | はい |
| `-scid` | Service Configuration ID | はい |
| `-template` | Template ID | いいえ |
| `-nethernet-id` | NetherNet ID | いいえ |
| `-listen` | リッスンアドレス (デフォルト：127.0.0.1:19132) | いいえ |

## Code Builder 機能

このプロキシは Code Builder パケットをフックできます。デフォルトでは以下のコードを返します：

```javascript
console.log('Hello from FW-Proxy!');
```

独自のコードを返すには、`proxy.go` の `SetCodingRequestHandler` をカスタマイズしてください。

## パケット処理

サポートされているパケット：

- `StartGame` (0x01)
- `CodeBuilder` (0x6A)
- `CodeBuilderResponse` (0x6B)
- `MovePlayer` (0x13)
- `PlayerList` (0xDD)
- その他多数

## アーキテクチャ

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Minecraft │────▶│  FW-Proxy    │────▶│ Friend World │
│   Client    │◀────│  (127.0.0.1) │◀────│   Server     │
└─────────────┘     └──────────────┘     └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │ Code Builder │
                    │    Hook      │
                    └──────────────┘
```

## ライセンス

MIT License
