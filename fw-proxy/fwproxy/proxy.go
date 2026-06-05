package fwproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/df-mc/go-nethernet-fw/auth"
	"github.com/df-mc/go-nethernet-fw/nethernet"
	"github.com/df-mc/go-nethernet-fw/protocol"
	"github.com/df-mc/go-nethernet-fw/rta"
)

// Proxy はフレンドワールドプロキシです
type Proxy struct {
	account      *auth.Account
	session      *rta.Session
	listener     net.Listener
	clients      map[net.Addr]*nethernet.Conn
	mu           sync.Mutex
	logger       *log.Logger
	codeHook     func(*protocol.CodeBuilderPacket) *protocol.CodeBuilderPacket
}

// ProxyConfig はプロキシ設定です
type ProxyConfig struct {
	Account  *auth.Account
	Session  *rta.Session
	ListenAddr string
	Logger   *log.Logger
	CodeHook func(*protocol.CodeBuilderPacket) *protocol.CodeBuilderPacket
}

// NewProxy は新しいプロキシを作成します
func NewProxy(cfg ProxyConfig) (*Proxy, error) {
	listener, err := net.Listen("udp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("リスナー作成エラー: %w", err)
	}

	return &Proxy{
		account:   cfg.Account,
		session:   cfg.Session,
		listener:  listener,
		clients:   make(map[net.Addr]*nethernet.Conn),
		logger:    cfg.Logger,
		codeHook:  cfg.CodeHook,
	}, nil
}

// Start はプロキシを開始します
func (p *Proxy) Start(ctx context.Context) error {
	p.logger.Printf("🚀 プロキシ開始: %s", p.listener.Addr())
	p.logger.Printf("   Gamertag: %s (XUID: %s)", p.account.Gamertag, p.account.XUID)
	if p.session != nil {
		p.logger.Printf("   Session: %s", p.session.SessionID)
		p.logger.Printf("   World: %s", p.session.WorldName)
	}

	go p.acceptLoop(ctx)

	return nil
}

func (p *Proxy) acceptLoop(ctx context.Context) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-ctx.Done():
			p.logger.Println("プロキシ終了")
			return
		default:
		}

		p.listener.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := p.listener.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			p.logger.Printf("読み込みエラー: %v", err)
			continue
		}

		// クライアント接続を管理
		conn := p.getOrCreateClient(addr)
		if conn == nil {
			continue
		}

		// パケット処理
		p.handlePacket(conn, addr, buf[:n])
	}
}

func (p *Proxy) getOrCreateClient(addr net.Addr) *nethernet.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.clients[addr]; ok {
		return conn
	}

	// 新しいクライアント接続を作成
	sessionInfo := &nethernet.SessionInfo{
		SessionID:   p.session.SessionID,
		SCID:        p.session.SCID,
		Template:    p.session.Template,
		NetherNetID: p.session.NetherNetID,
		XUID:        p.account.XUID,
	}

	signaling := &WebSocketSignalingAdapter{
		sessionID: p.session.SessionID,
		xuid:      p.account.XUID,
		token:     p.account.Token,
	}

	cfg := nethernet.ConnConfig{
		Signaling:   signaling,
		SessionInfo: sessionInfo,
		Logger:      p.logger,
	}

	conn, err := nethernet.NewConn(cfg)
	if err != nil {
		p.logger.Printf("接続作成エラー: %v", err)
		return nil
	}

	// 接続開始
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := conn.Dial(ctx); err != nil {
			p.logger.Printf("接続エラー: %v", err)
			return
		}
		p.logger.Printf("✅ クライアント接続完了: %s", addr)
	}()

	p.clients[addr] = conn
	return conn
}

func (p *Proxy) handlePacket(conn *nethernet.Conn, addr net.Addr, data []byte) {
	if len(data) < 2 {
		return
	}

	// Minecraft パケットヘッダー解析
	packetID := protocol.PacketID(data[0])
	
	// Code Builder パケットのフック
	if packetID == protocol.PacketIDCodeBuilderRequestPacket || 
	   packetID == protocol.PacketIDCodeBuilderResponsePacket ||
	   packetID == protocol.PacketIDCodeBuilderPacket {
		
		var cbPacket protocol.CodeBuilderPacket
		if err := cbPacket.Unmarshal(data[1:]); err == nil {
			if p.codeHook != nil {
				modified := p.codeHook(&cbPacket)
				if modified != nil {
					modifiedData, _ := modified.Marshal()
					data = append([]byte{byte(packetID)}, modifiedData...)
					p.logger.Printf("📝 Code Builder パケット改変: URL=%s", modified.URL)
				}
			}
		}
	}

	// WebRTC 経由で送信
	if _, err := conn.Write(data); err != nil {
		p.logger.Printf("送信エラー: %v", err)
	}
}

// Stop はプロキシを停止します
func (p *Proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.clients {
		conn.Close()
	}
	p.clients = make(map[net.Addr]*nethernet.Conn)

	return p.listener.Close()
}

// SetCodeHook は Code Builder フックを設定します
func (p *Proxy) SetCodeHook(hook func(*protocol.CodeBuilderPacket) *protocol.CodeBuilderPacket) {
	p.codeHook = hook
}

// WebSocketSignalingAdapter は WebSocket シグナリングをアダプトします
type WebSocketSignalingAdapter struct {
	sessionID string
	xuid      string
	token     string
	ws        *rta.WebSocketSignaling
	mu        sync.Mutex
}

func (w *WebSocketSignalingAdapter) SendOffer(ctx context.Context, info *nethernet.SessionInfo, offer []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ws == nil {
		var err error
		w.ws, err = rta.ConnectWebSocket(ctx, w.sessionID, w.xuid, w.token)
		if err != nil {
			return err
		}
	}

	return w.ws.SendOffer(offer)
}

func (w *WebSocketSignalingAdapter) SendAnswer(ctx context.Context, info *nethernet.SessionInfo, answer []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ws == nil {
		var err error
		w.ws, err = rta.ConnectWebSocket(ctx, w.sessionID, w.xuid, w.token)
		if err != nil {
			return err
		}
	}

	return w.ws.SendAnswer(answer)
}

func (w *WebSocketSignalingAdapter) SendCandidate(ctx context.Context, info *nethernet.SessionInfo, candidate []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ws == nil {
		return nil // まだ接続されていない
	}

	return w.ws.SendCandidate(candidate)
}

func (w *WebSocketSignalingAdapter) Receive(ctx context.Context) (nethernet.Signal, error) {
	if w.ws == nil {
		return nethernet.Signal{}, fmt.Errorf("WebSocket 未接続")
	}

	msg, err := w.ws.Receive(ctx)
	if err != nil {
		return nethernet.Signal{}, err
	}

	signal := nethernet.Signal{}
	if t, ok := msg["type"].(string); ok {
		signal.Type = nethernet.SignalType(t)
	}
	if payload, ok := msg["payload"].(string); ok {
		signal.Data = []byte(payload)
	}
	if connID, ok := msg["connectionId"].(string); ok {
		signal.ConnectionID = connID
	}

	return signal, nil
}

func (w *WebSocketSignalingAdapter) Close() error {
	if w.ws != nil {
		return w.ws.Close()
	}
	return nil
}
