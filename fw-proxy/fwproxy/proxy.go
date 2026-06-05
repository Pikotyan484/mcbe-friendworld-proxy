// Package fwproxy は Minecraft Bedrock Edition フレンドワールドプロキシを提供します
package fwproxy

import (
"context"
"encoding/binary"
"fmt"
"io"
"log"
"net"
"sync"
"time"

"github.com/df-mc/go-nethernet-fw/nethernet"
"github.com/df-mc/go-nethernet-fw/protocol"
)

// Proxy はフレンドワールドプロキシを表します
type Proxy struct {
listenAddr     string
sessionInfo    *nethernet.SessionInfo
logger         *log.Logger
mu             sync.Mutex
clientConn     net.Conn
serverConn     *nethernet.Conn
packetHandlers map[uint8]func([]byte) ([]byte, error)
codingHook     *CodingHook
}

// CodingHook は Code Builder パケットのフックを管理します
type CodingHook struct {
mu           sync.Mutex
onCodeRequest func(url string) string
onCodeResponse func(code string)
enabled      bool
}

// ProxyConfig はプロキシの設定を表します
type ProxyConfig struct {
ListenAddr  string
SessionInfo *nethernet.SessionInfo
Logger      *log.Logger
}

// NewProxy は新しいプロキシインスタンスを作成します
func NewProxy(config ProxyConfig) *Proxy {
p := &Proxy{
listenAddr:     config.ListenAddr,
sessionInfo:    config.SessionInfo,
logger:         config.Logger,
packetHandlers: make(map[uint8]func([]byte) ([]byte, error)),
codingHook: &CodingHook{
enabled: true,
},
}

// デフォルトのパケットハンドラを登録
p.registerDefaultHandlers()

return p
}

// registerDefaultHandlers はデフォルトのパケットハンドラを登録します
func (p *Proxy) registerDefaultHandlers() {
// Code Builder パケットのフック
p.packetHandlers[protocol.PacketIDCodeBuilder] = p.handleCodeBuilderPacket
p.packetHandlers[protocol.PacketIDCodeBuilderResponse] = p.handleCodeBuilderResponse
}

// handleCodeBuilderPacket は Code Builder リクエストパケットを処理します
func (p *Proxy) handleCodeBuilderPacket(data []byte) ([]byte, error) {
if !p.codingHook.enabled {
return data, nil
}

pkt, err := protocol.ParseCodeBuilderPacket(data)
if err != nil {
p.logger.Printf("CodeBuilder パケット解析エラー：%v", err)
return data, nil
}

p.logger.Printf("CodeBuilder リクエスト受信：URL=%s, Status=%d, ShouldOpen=%v", 
pkt.URL, pkt.CodeStatus, pkt.ShouldOpen)

// フックが設定されていれば呼び出す
if p.codingHook.onCodeRequest != nil && pkt.CodeStatus == 1 {
code := p.codingHook.onCodeRequest(pkt.URL)
if code != "" {
// レスポンスを生成して返す
resp := &protocol.CodeBuilderPacket{
URL:        pkt.URL,
CodeStatus: 2, // response
ShouldOpen: false,
Code:       code,
}
return protocol.EncodeCodeBuilderPacket(resp)
}
}

return data, nil
}

// handleCodeBuilderResponse は Code Builder レスポンスパケットを処理します
func (p *Proxy) handleCodeBuilderResponse(data []byte) ([]byte, error) {
if !p.codingHook.enabled {
return data, nil
}

pkt, err := protocol.ParseCodeBuilderPacket(data)
if err != nil {
p.logger.Printf("CodeBuilder レスポンス解析エラー：%v", err)
return data, nil
}

p.logger.Printf("CodeBuilder レスポンス受信：URL=%s, Code=%s", pkt.URL, pkt.Code)

// フックが設定されていれば呼び出す
if p.codingHook.onCodeResponse != nil && pkt.CodeStatus == 2 {
p.codingHook.onCodeResponse(pkt.Code)
}

return data, nil
}

// SetCodingRequestHandler は Code リクエスト時のハンドラを設定します
func (p *Proxy) SetCodingRequestHandler(handler func(url string) string) {
p.codingHook.mu.Lock()
defer p.codingHook.mu.Unlock()
p.codingHook.onCodeRequest = handler
}

// SetCodingResponseHandler は Code レスポンス時のハンドラを設定します
func (p *Proxy) SetCodingResponseHandler(handler func(code string)) {
p.codingHook.mu.Lock()
defer p.codingHook.mu.Unlock()
p.codingHook.onCodeResponse = handler
}

// EnableCodingHook は Coding フックの有効/無効を設定します
func (p *Proxy) EnableCodingHook(enabled bool) {
p.codingHook.mu.Lock()
defer p.codingHook.mu.Unlock()
p.codingHook.enabled = enabled
}

// Start はプロキシサーバーを開始します
func (p *Proxy) Start(ctx context.Context) error {
addr, err := net.ResolveUDPAddr("udp", p.listenAddr)
if err != nil {
return fmt.Errorf("アドレス解決エラー：%w", err)
}

conn, err := net.ListenUDP("udp", addr)
if err != nil {
return fmt.Errorf("リスニングエラー：%w", err)
}
defer conn.Close()

p.logger.Printf("プロキシ開始：%s", p.listenAddr)

buf := make([]byte, 65535)
for {
select {
case <-ctx.Done():
return ctx.Err()
default:
conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
n, clientAddr, err := conn.ReadFromUDP(buf)
if err != nil {
if ne, ok := err.(net.Error); ok && ne.Timeout() {
continue
}
return fmt.Errorf("読み込みエラー：%w", err)
}

// クライアントからのパケットを処理
go p.handleClientPacket(conn, clientAddr, buf[:n])
}
}
}

// handleClientPacket はクライアントからのパケットを処理します
func (p *Proxy) handleClientPacket(conn *net.UDPConn, clientAddr *net.UDPAddr, data []byte) {
// パケット ID を取得
if len(data) < 1 {
return
}
packetID := data[0]

// ハンドラがあれば適用
if handler, ok := p.packetHandlers[packetID]; ok {
modifiedData, err := handler(data)
if err != nil {
p.logger.Printf("パケット処理エラー：%v", err)
return
}
data = modifiedData
}

// サーバーに転送（未実装：現在はログのみ）
p.logger.Printf("クライアント→サーバー：PacketID=0x%02X, Size=%d", packetID, len(data))
}

// ConnectToFriendWorld はフレンドワールドに接続します
func (p *Proxy) ConnectToFriendWorld(ctx context.Context, signaling nethernet.Signaling) error {
p.mu.Lock()
defer p.mu.Unlock()

p.logger.Println("フレンドワールドに接続中...")

// NetherNet 接続を確立
conn, err := nethernet.DialContext(ctx, signaling)
if err != nil {
return fmt.Errorf("NetherNet 接続エラー：%w", err)
}

p.serverConn = conn
p.logger.Println("フレンドワールドに接続完了")

// パケット転送スレッドを開始
go p.forwardPackets()

return nil
}

// forwardPackets はパケットを双方向に転送します
func (p *Proxy) forwardPackets() {
if p.serverConn == nil {
return
}

// サーバー→クライアント
go func() {
buf := make([]byte, 65535)
for {
n, err := p.serverConn.Read(buf)
if err != nil {
p.logger.Printf("サーバー読み込みエラー：%v", err)
return
}

data := buf[:n]
if len(data) < 1 {
continue
}

packetID := data[0]

// ハンドラがあれば適用
if handler, ok := p.packetHandlers[packetID]; ok {
modifiedData, err := handler(data)
if err != nil {
p.logger.Printf("パケット処理エラー：%v", err)
continue
}
data = modifiedData
}

// クライアントに転送（未実装：現在はログのみ）
p.logger.Printf("サーバー→クライアント：PacketID=0x%02X, Size=%d", packetID, len(data))
}
}()
}

// Close はプロキシを閉じます
func (p *Proxy) Close() error {
p.mu.Lock()
defer p.mu.Unlock()

if p.serverConn != nil {
if err := p.serverConn.Close(); err != nil {
return err
}
}

return nil
}

// InjectCodeBuilderPacket は Code Builder パケットを注入します
func (p *Proxy) InjectCodeBuilderPacket(url string, shouldOpen bool) error {
if p.serverConn == nil {
return fmt.Errorf("接続されていません")
}

pkt := &protocol.CodeBuilderPacket{
URL:        url,
CodeStatus: 1, // request
ShouldOpen: shouldOpen,
}

data, err := protocol.EncodeCodeBuilderPacket(pkt)
if err != nil {
return fmt.Errorf("エンコードエラー：%w", err)
}

_, err = p.serverConn.Write(data)
return err
}

// GetSessionInfo はセッション情報を取得します
func (p *Proxy) GetSessionInfo() *nethernet.SessionInfo {
return p.sessionInfo
}

// LogPacket はパケットをログ出力します（デバッグ用）
func LogPacket(direction string, data []byte, logger *log.Logger) {
if len(data) < 1 {
return
}
packetID := data[0]
logger.Printf("%s: PacketID=0x%02X (%d), Size=%d", direction, packetID, packetID, len(data))

// StartGame パケットの場合は詳細を表示
if packetID == protocol.PacketIDStartGame && len(data) > 1 {
pkt, err := protocol.ParseStartGamePacket(data)
if err == nil {
logger.Printf("  StartGame: WorldName=%s, ServerVersion=%s, Dimension=%d",
pkt.WorldName, pkt.ServerVersion, pkt.Dimension)
}
}
}

// ReadVarInt は可変長整数を読み取ります
func ReadVarInt(r io.Reader) (int64, error) {
return protocol.DecodeVarInt(r)
}

// WriteVarInt は可変長整数を書き込みます
func WriteVarInt(w io.Writer, value int64) error {
return protocol.EncodeVarInt(w, value)
}

// ReadString は文字列を読み取ります
func ReadString(r io.Reader) (string, error) {
return protocol.DecodeString(r)
}

// WriteString は文字列を書き込みます
func WriteString(w io.Writer, s string) error {
return protocol.EncodeString(w, s)
}

// WriteUint16LE は 16 ビット整数をリトルエンディアンで書き込みます
func WriteUint16LE(w io.Writer, v uint16) error {
return binary.Write(w, binary.LittleEndian, v)
}

// WriteUint32LE は 32 ビット整数をリトルエンディアンで書き込みます
func WriteUint32LE(w io.Writer, v uint32) error {
return binary.Write(w, binary.LittleEndian, v)
}

// WriteUint64LE は 64 ビット整数をリトルエンディアンで書き込みます
func WriteUint64LE(w io.Writer, v uint64) error {
return binary.Write(w, binary.LittleEndian, v)
}

// WriteFloat32LE は 32 ビット浮動小数点をリトルエンディアンで書き込みます
func WriteFloat32LE(w io.Writer, v float32) error {
return binary.Write(w, binary.LittleEndian, v)
}

// WriteBool はブール値を書き込みます
func WriteBool(w io.Writer, v bool) error {
var b byte
if v {
b = 1
}
return w.WriteByte(b)
}
