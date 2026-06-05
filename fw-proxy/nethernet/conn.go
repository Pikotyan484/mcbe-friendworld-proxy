package nethernet

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// Version は Minecraft のバージョン情報です
const (
	VersionString  = "26.20"
	ProtocolVersion = 975
)

// SessionInfo はセッション情報を表します
type SessionInfo struct {
	SessionID   string `json:"sessionId"`
	SCID        string `json:"scid"`
	Template    string `json:"template"`
	NetherNetID int64  `json:"netherNetId"`
	XUID        string `json:"xuid,omitempty"`
}

// Signaling はシグナリングインターフェースです
type Signaling interface {
	SendOffer(ctx context.Context, info *SessionInfo, offer []byte) error
	SendAnswer(ctx context.Context, info *SessionInfo, answer []byte) error
	SendCandidate(ctx context.Context, info *SessionInfo, candidate []byte) error
	Receive(ctx context.Context) (Signal, error)
	Close() error
}

// SignalType はシグナルの種類です
type SignalType string

const (
	SignalTypeOffer     SignalType = "offer"
	SignalTypeAnswer    SignalType = "answer"
	SignalTypeCandidate SignalType = "candidate"
	SignalTypeError     SignalType = "error"
)

// Signal はシグナルメッセージです
type Signal struct {
	Type        SignalType   `json:"type"`
	ConnectionID string      `json:"connectionId"`
	Data        []byte       `json:"data"`
	SessionInfo *SessionInfo `json:"sessionInfo,omitempty"`
}

// MessageReliability はメッセージの信頼性です
type MessageReliability int

const (
	MessageReliabilityUnreliable MessageReliability = iota
	MessageReliabilityReliable
)

// ConnConfig は接続設定です
type ConnConfig struct {
	Signaling   Signaling
	SessionInfo *SessionInfo
	Logger      *log.Logger
}

// Conn は WebRTC 接続です
type Conn struct {
	signaling     Signaling
	sessionInfo   *SessionInfo
	logger        *log.Logger
	pc            *webrtc.PeerConnection
	reliableDC    *webrtc.DataChannel
	unreliableDC  *webrtc.DataChannel
	readBuf       []byte
	readPos       int
	writeMu       sync.Mutex
	closeOnce     sync.Once
	closed        chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	localConnID   string
	remoteConnID  string
}

// NewConn は新しい接続を作成します
func NewConn(cfg ConnConfig) (*Conn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		cancel()
		return nil, err
	}

	conn := &Conn{
		signaling:    cfg.Signaling,
		sessionInfo:  cfg.SessionInfo,
		logger:       cfg.Logger,
		pc:           pc,
		closed:       make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
		localConnID:  generateConnID(),
		readBuf:      make([]byte, 65536),
	}

	// データチャネルの設定
	conn.setupDataChannels()

	return conn, nil
}

func (c *Conn) setupDataChannels() {
	// Reliable チャネル
	reliableDC, err := c.pc.CreateDataChannel("reliable", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		c.logger.Printf("reliable チャネル作成エラー: %v", err)
		return
	}
	c.reliableDC = reliableDC
	reliableDC.OnOpen(func() {
		c.logger.Println("reliable チャネルが開きました")
	})
	reliableDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		c.handleMessage(msg, MessageReliabilityReliable)
	})

	// Unreliable チャネル
	unreliableDC, err := c.pc.CreateDataChannel("unreliable", &webrtc.DataChannelInit{
		Ordered:   boolPtr(false),
		MaxRetransmits: uint16Ptr(0),
	})
	if err != nil {
		c.logger.Printf("unreliable チャネル作成エラー: %v", err)
		return
	}
	c.unreliableDC = unreliableDC
	unreliableDC.OnOpen(func() {
		c.logger.Println("unreliable チャネルが開きました")
	})
	unreliableDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		c.handleMessage(msg, MessageReliabilityUnreliable)
	})

	// リモートデータチャネル用
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		c.logger.Printf("リモートデータチャネル受信: %s", dc.Label())
		if dc.Label() == "reliable" {
			c.reliableDC = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				c.handleMessage(msg, MessageReliabilityReliable)
			})
		} else if dc.Label() == "unreliable" {
			c.unreliableDC = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				c.handleMessage(msg, MessageReliabilityUnreliable)
			})
		}
	})
}

func (c *Conn) handleMessage(msg webrtc.DataChannelMessage, reliability MessageReliability) {
	// メッセージ処理ロジック
	select {
	case <-c.closed:
		return
	default:
		// 簡易実装：バッファにコピー
		copy(c.readBuf[c.readPos:], msg.Data)
		c.readPos += len(msg.Data)
	}
}

// Dial は接続を開始します
func (c *Conn) Dial(ctx context.Context) error {
	// Offer SDP を生成
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		return err
	}

	if err := c.pc.SetLocalDescription(offer); err != nil {
		return err
	}

	// ICE 候補収集を待機
	gatherComplete := webrtc.GatheringCompletePromise(c.pc)
	<-gatherComplete

	// Offer を送信
	offerData := []byte(c.pc.LocalDescription().SDP)
	if err := c.signaling.SendOffer(ctx, c.sessionInfo, offerData); err != nil {
		return err
	}

	c.logger.Println("Offer を送信しました")

	// Answer を待機
	return c.waitForAnswer(ctx)
}

func (c *Conn) waitForAnswer(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closed:
			return errors.New("connection closed")
		default:
			signal, err := c.signaling.Receive(ctx)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if signal.Type == SignalTypeAnswer {
				answer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  string(signal.Data),
				}
				if err := c.pc.SetRemoteDescription(answer); err != nil {
					return err
				}
				c.remoteConnID = signal.ConnectionID
				c.logger.Println("Answer を受信しました")
				return nil
			} else if signal.Type == SignalTypeCandidate {
				// ICE Candidate 処理
				c.handleCandidate(signal.Data)
			}
		}
	}
}

// Listen は接続を受け入れます
func (c *Conn) Listen(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closed:
			return errors.New("connection closed")
		default:
			signal, err := c.signaling.Receive(ctx)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if signal.Type == SignalTypeOffer {
				c.remoteConnID = signal.ConnectionID
				
				// Offer SDP をセット
				offer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  string(signal.Data),
				}
				if err := c.pc.SetRemoteDescription(offer); err != nil {
					return err
				}

				// Answer SDP を生成
				answer, err := c.pc.CreateAnswer(nil)
				if err != nil {
					return err
				}

				if err := c.pc.SetLocalDescription(answer); err != nil {
					return err
				}

				// ICE 候補収集を待機
				gatherComplete := webrtc.GatheringCompletePromise(c.pc)
				<-gatherComplete

				// Answer を送信
				answerData := []byte(c.pc.LocalDescription().SDP)
				if err := c.signaling.SendAnswer(ctx, c.sessionInfo, answerData); err != nil {
					return err
				}

				c.logger.Println("Answer を送信しました")
				return nil
			} else if signal.Type == SignalTypeCandidate {
				c.handleCandidate(signal.Data)
			}
		}
	}
}

func (c *Conn) handleCandidate(data []byte) {
	// ICE Candidate 処理
	var candidate webrtc.ICECandidateInit
	if err := parseCandidate(data, &candidate); err != nil {
		c.logger.Printf("Candidate パースエラー: %v", err)
		return
	}
	if err := c.pc.AddICECandidate(candidate); err != nil {
		c.logger.Printf("Candidate 追加エラー: %v", err)
	}
}

// Read はデータを読み込みます
func (c *Conn) Read(b []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	if c.readPos > 0 {
		n := copy(b, c.readBuf[:c.readPos])
		c.readBuf = c.readBuf[n:]
		c.readPos -= n
		return n, nil
	}

	// データがない場合はブロック
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return 0, io.EOF
		case <-ticker.C:
			if c.readPos > 0 {
				n := copy(b, c.readBuf[:c.readPos])
				c.readBuf = c.readBuf[n:]
				c.readPos -= n
				return n, nil
			}
		}
	}
}

// Write はデータを書き込みます
func (c *Conn) Write(b []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Reliable で送信（必要に応じて分割）
	if c.reliableDC == nil || c.reliableDC.ReadyState() != webrtc.DataChannelStateOpen {
		return 0, errors.New("reliable channel not open")
	}

	if err := c.reliableDC.Send(b); err != nil {
		return 0, err
	}

	return len(b), nil
}

// Close は接続を閉じます
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		c.cancel()
		if c.pc != nil {
			err = c.pc.Close()
		}
		if c.signaling != nil {
			c.signaling.Close()
		}
	})
	return err
}

// LocalAddr はローカルアドレスを返します
func (c *Conn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

// RemoteAddr はリモートアドレスを返します
func (c *Conn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

// SetDeadline はデッドラインを設定します
func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline は読み込みデッドラインを設定します
func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline は書き込みデッドラインを設定します
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

// SessionInfo はセッション情報を返します
func (c *Conn) SessionInfo() *SessionInfo {
	return c.sessionInfo
}

func generateConnID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func boolPtr(b bool) *bool {
	return &b
}

func uint16Ptr(u uint16) *uint16 {
	return &u
}

func parseCandidate(data []byte, candidate *webrtc.ICECandidateInit) error {
	// 簡易パース
	candidate.Candidate = string(data)
	return nil
}
