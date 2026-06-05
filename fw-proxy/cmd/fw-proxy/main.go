package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/df-mc/go-nethernet-fw/auth"
	"github.com/df-mc/go-nethernet-fw/fwproxy"
	"github.com/df-mc/go-nethernet-fw/protocol"
	"github.com/df-mc/go-nethernet-fw/rta"
)

func main() {
	// コマンドライン引数
	listenAddr := flag.String("listen", "127.0.0.1:19132", "リッスンアドレス")
	sessionID := flag.String("session", "", "セッション ID（省略可能：自動選択）")
	autoSelect := flag.Bool("auto", false, "フレンドのワールドを自動選択")
	customCode := flag.String("code", "", "カスタム Code Builder コード")
	flag.Parse()

	fmt.Println("╔═══════════════════════════════════════════════════╗")
	fmt.Println("║     Minecraft Bedrock 26.20 フレンドワールドプロキシ    ║")
	fmt.Println("║           Protocol Version: 975                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════╝")
	fmt.Println()

	// シグナルハンドリング
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 シャットダウン中...")
		cancel()
	}()

	// 1. 認証（microsoft.com/link フロー）
	fmt.Println("📡 認証中...")
	authenticator := auth.NewAuthenticator()
	account, err := authenticator.DeviceCodeAuthFlow(ctx)
	if err != nil {
		log.Fatalf("認証エラー: %v", err)
	}

	// 2. セッション取得
	var session *rta.Session
	rtaClient := rta.NewRTAClient(account.Token, account.XUID)

	if *autoSelect || *sessionID == "" {
		// フレンドのセッション一覧を取得
		fmt.Println("🔍 フレンドのセッションを検索中...")
		sessions, err := rtaClient.GetFriendSessions(ctx)
		if err != nil {
			log.Fatalf("セッション取得エラー: %v", err)
		}

		if len(sessions) == 0 {
			fmt.Println("⚠️ 開いているフレンドのワールドが見つかりません")
			fmt.Println("   手動でセッション ID を指定してください: -session <ID>")
			return
		}

		// 最初のセッションを選択（またはユーザーに選択させる）
		fmt.Printf("✅ %d 個のセッションを発見:\n", len(sessions))
		for i, s := range sessions {
			fmt.Printf("   [%d] %s (オーナー：%s)\n", i+1, s.WorldName, s.OwnerGamertag)
		}

		if *sessionID == "" {
			session = sessions[0] // 最初のを自動選択
			fmt.Printf("🎮 '%s' に接続します...\n", session.WorldName)
		} else {
			// 指定された ID を探す
			found := false
			for _, s := range sessions {
				if s.SessionID == *sessionID {
					session = s
					found = true
					break
				}
			}
			if !found {
				log.Fatalf("セッション ID '%s' が見つかりません", *sessionID)
			}
		}
	} else {
		// 指定されたセッション ID を使用
		fmt.Printf("🔍 セッション '%s' を取得中...\n", *sessionID)
		session, err = rtaClient.SelectSession(ctx, *sessionID)
		if err != nil {
			log.Fatalf("セッション選択エラー: %v", err)
		}
	}

	// 3. プロキシ設定
	logger := log.New(os.Stdout, "[Proxy] ", log.LstdFlags)

	codeHook := func(pkt *protocol.CodeBuilderPacket) *protocol.CodeBuilderPacket {
		if *customCode != "" {
			fmt.Printf("📝 Code Builder カスタムコード注入: %s\n", *customCode)
			return &protocol.CodeBuilderPacket{
				URL:  pkt.URL,
				Code: *customCode,
			}
		}
		return pkt
	}

	cfg := fwproxy.ProxyConfig{
		Account:    account,
		Session:    session,
		ListenAddr: *listenAddr,
		Logger:     logger,
		CodeHook:   codeHook,
	}

	proxy, err := fwproxy.NewProxy(cfg)
	if err != nil {
		log.Fatalf("プロキシ作成エラー: %v", err)
	}

	// 4. プロキシ開始
	fmt.Println()
	if err := proxy.Start(ctx); err != nil {
		log.Fatalf("プロキシ開始エラー: %v", err)
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ プロキシが稼働中: %s\n", *listenAddr)
	fmt.Printf("   アカウント：%s (%s)\n", account.Gamertag, account.XUID)
	fmt.Printf("   ワールド：%s\n", session.WorldName)
	fmt.Printf("   セッション：%s\n", session.SessionID)
	fmt.Println()
	fmt.Println("🎮 Minecraft から以下に接続:")
	fmt.Printf("   サーバーアドレス：%s\n", *listenAddr)
	fmt.Println()
	fmt.Println("💡 Coding 機能を使用すると、カスタムコードが注入されます")
	fmt.Println("   Ctrl+C で終了")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 待機
	<-ctx.Done()

	// クリーンアップ
	fmt.Println("🔄 プロキシを停止中...")
	if err := proxy.Stop(); err != nil {
		log.Printf("停止エラー: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("👋 さようなら！")
}
