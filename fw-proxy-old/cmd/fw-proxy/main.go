package main

import (
"context"
"flag"
"log"
"os"
"os/signal"
"syscall"

"github.com/df-mc/go-nethernet-fw/fwproxy"
"github.com/df-mc/go-nethernet-fw/nethernet"
)

func main() {
// コマンドライン引数の解析
token := flag.String("token", "", "Xbox Live トークン")
xuid := flag.String("xuid", "", "Xbox User ID")
sessionID := flag.String("session", "", "Session ID")
scid := flag.String("scid", "", "SCID (Service Configuration ID)")
template := flag.String("template", "", "Template ID")
netherNetID := flag.Int64("nethernet-id", 0, "NetherNet ID")
listenAddr := flag.String("listen", "127.0.0.1:19132", "リッスンアドレス")
flag.Parse()

// 必須パラメータのチェック
if *token == "" || *xuid == "" || *sessionID == "" || *scid == "" {
log.Fatal("エラー：-token, -xuid, -session, -scid は必須です")
}

logger := log.New(os.Stdout, "[FW-Proxy] ", log.LstdFlags)
logger.Println("Minecraft Bedrock Edition フレンドワールドプロキシ")
logger.Printf("バージョン：1.21.90 (26.20), プロトコル：975")

// セッション情報の作成
sessionInfo := &nethernet.SessionInfo{
SessionID:   *sessionID,
SCID:        *scid,
Template:    *template,
NetherNetID: *netherNetID,
}

// プロキシの作成
proxy := fwproxy.NewProxy(fwproxy.ProxyConfig{
ListenAddr:  *listenAddr,
SessionInfo: sessionInfo,
Logger:      logger,
})

// Coding フックのハンドラを設定
proxy.SetCodingRequestHandler(func(url string) string {
logger.Printf("Code リクエスト：%s", url)
// ここで独自のコードを返すことができます
return "console.log('Hello from FW-Proxy!');"
})

proxy.SetCodingResponseHandler(func(code string) {
logger.Printf("Code レスポンス：%s", code)
})

// シグナル処理
ctx, cancel := context.WithCancel(context.Background())
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

go func() {
<-sigChan
logger.Println("シャットダウン中...")
cancel()
}()

// プロキシ開始
logger.Printf("リスニング開始：%s", *listenAddr)
if err := proxy.Start(ctx); err != nil && err != context.Canceled {
logger.Fatalf("プロキシエラー：%v", err)
}

logger.Println("プロキシ終了")
}
