// Package nethernet provides WebRTC-based networking for Minecraft Bedrock Edition.
package nethernet

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// MessageReliability specifies the reliability of a message.
type MessageReliability uint8

const (
	// Reliable messages are guaranteed to be delivered in order.
	Reliable MessageReliability = iota
	// Unreliable messages are not guaranteed to be delivered or in order.
	Unreliable
)

// SessionInfo contains session-specific information for Friend World connections.
type SessionInfo struct {
	SessionID   string `json:"sessionId,omitempty"`
	SCID        string `json:"scid,omitempty"`
	Template    string `json:"template,omitempty"`
	NetherNetID int64  `json:"netherNetId,omitempty"`
	XUID        string `json:"xuid,omitempty"`
}

// SignalType represents the type of signaling message.
type SignalType uint8

const (
	SignalOffer SignalType = iota
	SignalAnswer
	SignalCandidate
	SignalError
	SignalClose
)

// Signal represents a signaling message exchanged between peers.
type Signal struct {
	Type        SignalType     `json:"type"`
	ConnectionID string        `json:"connectionId"`
	Data        []byte         `json:"data,omitempty"`
	NetworkID   string         `json:"networkId,omitempty"`
	SessionInfo *SessionInfo   `json:"sessionInfo,omitempty"`
}

// Signaling is an interface for exchanging signaling messages.
type Signaling interface {
	Send(ctx context.Context, signal Signal) error
	Receive(ctx context.Context) (Signal, error)
	Close() error
	SessionInfo() *SessionInfo
}

// Conn is a WebRTC-based network connection implementing net.Conn.
type Conn struct {
	mu              sync.Mutex
	state           webrtc.ICEConnectionState
	reliableDC      *webrtc.DataChannel
	unreliableDC    *webrtc.DataChannel
	pc              *webrtc.PeerConnection
	readBuf         []byte
	readPos         int
	readMu          sync.Mutex
	writeMu         sync.Mutex
	closeOnce       sync.Once
	closed          bool
	sessionInfo     *SessionInfo
	networkID       string
	connectionID    string
	logger          *log.Logger
	onStateChange   func(webrtc.ICEConnectionState)
}

// DialConfig contains configuration for dialing a connection.
type DialConfig struct {
	Signaling     Signaling
	Logger        *log.Logger
	ICEServers    []webrtc.ICEServer
	SessionInfo   *SessionInfo
	NetworkID     string
	ConnectionID  string
}

// ListenConfig contains configuration for listening for connections.
type ListenConfig struct {
	Signaling     Signaling
	Logger        *log.Logger
	ICEServers    []webrtc.ICEServer
	SessionInfo   *SessionInfo
	NetworkID     string
}

// Credentials contains authentication credentials.
type Credentials struct {
	XUID string
	Token string
}

// ICEServer represents an ICE server configuration.
type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// Error codes for signaling errors.
const (
	ErrorCodeNone uint8 = iota
	ErrorCodeInternal
	ErrorCodeProtocol
	ErrorCodeUnauthorized
	ErrorCodeNotFound
	ErrorCodeTimeout
)

var (
	ErrConnClosed         = errors.New("connection closed")
	ErrInvalidSignal      = errors.New("invalid signal")
	ErrNegotiationFailed  = errors.New("negotiation failed")
	ErrSessionExpired     = errors.New("session expired")
)

// Dial establishes a new connection using the provided config.
func Dial(ctx context.Context, cfg DialConfig) (*Conn, error) {
	if cfg.Signaling == nil {
		return nil, errors.New("signaling is required")
	}

	api := webrtc.NewAPI()
	if cfg.Logger != nil {
		api = webrtc.NewAPI(webrtc.WithSettingEngine(webrtc.SettingEngine{
			LoggerFactory: &loggerFactory{logger: cfg.Logger},
		}))
	}

	config := webrtc.Configuration{
		ICEServers: cfg.ICEServers,
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	conn := &Conn{
		pc:           pc,
		state:        webrtc.ICEConnectionStateNew,
		sessionInfo:  cfg.SessionInfo,
		networkID:    cfg.NetworkID,
		connectionID: cfg.ConnectionID,
		logger:       cfg.Logger,
		readBuf:      make([]byte, 0, 65536),
	}

	// Setup data channels
	conn.setupDataChannels()

	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	// Send offer via signaling
	signal := Signal{
		Type:        SignalOffer,
		ConnectionID: cfg.ConnectionID,
		Data:        []byte(*offer.SDP),
		NetworkID:   cfg.NetworkID,
		SessionInfo: cfg.SessionInfo,
	}

	if err := cfg.Signaling.Send(ctx, signal); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to send offer: %w", err)
	}

	// Wait for answer
	answerSignal, err := cfg.Signaling.Receive(ctx)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to receive answer: %w", err)
	}

	if answerSignal.Type != SignalAnswer {
		pc.Close()
		return nil, ErrInvalidSignal
	}

	answerSDP := string(answerSignal.Data)
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to set remote description: %w", err)
	}

	// Wait for ICE connection
	select {
	case <-ctx.Done():
		pc.Close()
		return nil, ctx.Err()
	case <-conn.waitForICEGathering():
	}

	return conn, nil
}

// Listen accepts incoming connections.
func (lc *ListenConfig) Listen(ctx context.Context) (*Conn, error) {
	if lc.Signaling == nil {
		return nil, errors.New("signaling is required")
	}

	api := webrtc.NewAPI()
	if lc.Logger != nil {
		api = webrtc.NewAPI(webrtc.WithSettingEngine(webrtc.SettingEngine{
			LoggerFactory: &loggerFactory{logger: lc.Logger},
		}))
	}

	config := webrtc.Configuration{
		ICEServers: lc.ICEServers,
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	conn := &Conn{
		pc:           pc,
		state:        webrtc.ICEConnectionStateNew,
		sessionInfo:  lc.SessionInfo,
		networkID:    lc.NetworkID,
		logger:       lc.Logger,
		readBuf:      make([]byte, 0, 65536),
	}

	conn.setupDataChannels()

	// Wait for offer
	signal, err := lc.Signaling.Receive(ctx)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to receive offer: %w", err)
	}

	if signal.Type != SignalOffer {
		pc.Close()
		return nil, ErrInvalidSignal
	}

	offerSDP := string(signal.Data)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to set remote description: %w", err)
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	// Send answer
	answerSignal := Signal{
		Type:        SignalAnswer,
		ConnectionID: signal.ConnectionID,
		Data:        []byte(*answer.SDP),
		NetworkID:   lc.NetworkID,
		SessionInfo: lc.SessionInfo,
	}

	if err := lc.Signaling.Send(ctx, answerSignal); err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to send answer: %w", err)
	}

	// Wait for ICE connection
	select {
	case <-ctx.Done():
		pc.Close()
		return nil, ctx.Err()
	case <-conn.waitForICEGathering():
	}

	return conn, nil
}

func (c *Conn) setupDataChannels() {
	// Reliable data channel
	reliableDC, err := c.pc.CreateDataChannel("reliable", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err == nil {
		c.reliableDC = reliableDC
		c.setupDataChannelHandlers(reliableDC)
	}

	// Unreliable data channel
	unreliableDC, err := c.pc.CreateDataChannel("unreliable", &webrtc.DataChannelInit{
		Ordered: boolPtr(false),
		MaxRetransmits: uint16Ptr(0),
	})
	if err == nil {
		c.unreliableDC = unreliableDC
		c.setupDataChannelHandlers(unreliableDC)
	}

	c.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		c.mu.Lock()
		c.state = state
		c.mu.Unlock()
		
		if c.onStateChange != nil {
			c.onStateChange(state)
		}
		
		if state == webrtc.ICEConnectionStateFailed || 
		   state == webrtc.ICEConnectionStateClosed ||
		   state == webrtc.ICEConnectionStateDisconnected {
			c.closeOnce.Do(func() {
				c.closed = true
			})
		}
	})
}

func (c *Conn) setupDataChannelHandlers(dc *webrtc.DataChannel) {
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		c.readMu.Lock()
		defer c.readMu.Unlock()
		c.readBuf = append(c.readBuf, msg.Data...)
	})
}

func (c *Conn) waitForICEGathering() <-chan struct{} {
	done := make(chan struct{})
	
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				c.mu.Lock()
				state := c.state
				c.mu.Unlock()
				
				if state == webrtc.ICEConnectionStateConnected {
					close(done)
					return
				}
			case <-time.After(30 * time.Second):
				close(done)
				return
			}
		}
	}()
	
	return done
}

// Read implements net.Conn.
func (c *Conn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	
	if len(c.readBuf) == 0 {
		return 0, io.EOF
	}
	
	n := copy(b, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

// Write implements net.Conn.
func (c *Conn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	
	if c.closed {
		return 0, ErrConnClosed
	}
	
	// Use reliable channel by default
	if c.reliableDC != nil && c.reliableDC.ReadyState() == webrtc.DataChannelStateOpen {
		if err := c.reliableDC.Send(b); err != nil {
			return 0, err
		}
		return len(b), nil
	}
	
	return 0, ErrConnClosed
}

// Send sends a message with specified reliability.
func (c *Conn) Send(b []byte, reliability MessageReliability) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	
	if c.closed {
		return ErrConnClosed
	}
	
	switch reliability {
	case Reliable:
		if c.reliableDC == nil || c.reliableDC.ReadyState() != webrtc.DataChannelStateOpen {
			return ErrConnClosed
		}
		return c.reliableDC.Send(b)
	case Unreliable:
		if c.unreliableDC == nil || c.unreliableDC.ReadyState() != webrtc.DataChannelStateOpen {
			return ErrConnClosed
		}
		return c.unreliableDC.Send(b)
	default:
		return errors.New("unknown reliability")
	}
}

// Close implements net.Conn.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.closed = true
		if c.pc != nil {
			err = c.pc.Close()
		}
	})
	return err
}

// LocalAddr implements net.Conn.
func (c *Conn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

// RemoteAddr implements net.Conn.
func (c *Conn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

// SetDeadline implements net.Conn.
func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline implements net.Conn.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline implements net.Conn.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

// SessionInfo returns the session information.
func (c *Conn) SessionInfo() *SessionInfo {
	return c.sessionInfo
}

// NetworkID returns the network ID.
func (c *Conn) NetworkID() string {
	return c.networkID
}

// ConnectionID returns the connection ID.
func (c *Conn) ConnectionID() string {
	return c.connectionID
}

func boolPtr(b bool) *bool {
	return &b
}

func uint16Ptr(u uint16) *uint16 {
	return &u
}

type loggerFactory struct {
	logger *log.Logger
}

func (f *loggerFactory) NewLogger(scope string) webrtc.LoggerFactory {
	return &pionLogger{logger: f.logger}
}

type pionLogger struct {
	logger *log.Logger
}

func (l *pionLogger) Trace(msg string) {}
func (l *pionLogger) Tracef(format string, args ...interface{}) {}
func (l *pionLogger) Debug(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Debugf(format string, args ...interface{}) { l.logger.Printf(format, args...) }
func (l *pionLogger) Info(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Infof(format string, args ...interface{}) { l.logger.Printf(format, args...) }
func (l *pionLogger) Warn(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Warnf(format string, args ...interface{}) { l.logger.Printf(format, args...) }
func (l *pionLogger) Error(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Errorf(format string, args ...interface{}) { l.logger.Printf(format, args...) }
func (l *pionLogger) Fatal(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Fatalf(format string, args ...interface{}) { l.logger.Printf(format, args...) }
func (l *pionLogger) Panic(msg string) { l.logger.Println(msg) }
func (l *pionLogger) Panicf(format string, args ...interface{}) { l.logger.Printf(format, args...) }
