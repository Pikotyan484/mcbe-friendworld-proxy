// Package protocol は Minecraft Bedrock Edition のパケット処理を提供します
package protocol

import (
"bytes"
"encoding/binary"
"errors"
"io"
)

// パケット ID 定数（Bedrock Edition 26.20 / Protocol 975）
const (
PacketIDStartGame           = 0x01
PacketIDCodeBuilder         = 0x6A // Code Builder パケット
PacketIDCodeBuilderResponse = 0x6B
PacketIDRequestChunkRadius  = 0x54
PacketIDChunkRadiusUpdated  = 0x55
PacketIDPlayerList          = 0xDD
PacketIDMovePlayer          = 0x13
PacketIDUpdateAttributes    = 0x75
PacketIDServerSettingsReq   = 0x68
PacketIDServerSettingsResp  = 0x69
)

// PacketHeader はパケットヘッダーを表します
type PacketHeader struct {
PacketID uint8
}

// StartGamePacket は StartGame パケットを表します
type StartGamePacket struct {
EntityID            int64
RuntimeEntityID     int64
PlayerGamemode      int32
PlayerPosition      [3]float32
Pitch               float32
Yaw                 float32
Seed                int64
Dimension           int16
GeneratorType       int32
GamemodeForAll      int32
Hardcore            bool
Cheats              bool
CommandsEnabled     bool
NoMobs              bool
GameRules           []GameRule
Experiments         []Experiment
BonusChest          bool
StartWithMap        bool
ServerVersion       string
EduFeatures         bool
EduProductUUID      string
RainLevel           float32
LightningLevel      float32
ConfirmedPlatform   bool
PlatformBroadcast   string
XBLBroadcast        string
DSLBroadcast        string
WorldName           string
TemplateContentKey  string
IsFromTemplate      bool
IsTrial             bool
MovementType        int32
ServerAuthoritativeInventory bool
EngineVersion       string
ServerEngineVersion string
}

// GameRule はゲームルールを表します
type GameRule struct {
Name   string
Value  interface{}
Type   uint8 // 1=bool, 2=int, 3=float
}

// Experiment は実験機能を表します
type Experiment struct {
Name    string
Enabled bool
}

// ReadPacketID はストリームからパケット ID を読み取ります
func ReadPacketID(r io.Reader) (uint8, error) {
var id uint8
err := binary.Read(r, binary.LittleEndian, &id)
return id, err
}

// WritePacketID はストリームにパケット ID を書き込みます
func WritePacketID(w io.Writer, id uint8) error {
return binary.Write(w, binary.LittleEndian, id)
}

// DecodeVarInt は可変長整数をデコードします
func DecodeVarInt(r io.Reader) (int64, error) {
var result int64
var shift uint
for {
b, err := r.ReadByte()
if err != nil {
return 0, err
}
result |= int64(b&0x7F) << shift
if b&0x80 == 0 {
break
}
shift += 7
if shift >= 64 {
return 0, errors.New("varint too long")
}
}
return result, nil
}

// EncodeVarInt は可変長整数をエンコードします
func EncodeVarInt(w io.Writer, value int64) error {
for value >= 0x80 || value < 0 {
if err := w.WriteByte(byte(value&0x7F | 0x80)); err != nil {
return err
}
value >>= 7
}
return w.WriteByte(byte(value))
}

// DecodeString は UTF-8 文字列をデコードします（LE 接頭辞付き）
func DecodeString(r io.Reader) (string, error) {
length, err := DecodeVarInt(r)
if err != nil {
return "", err
}
if length < 0 || length > 1000000 {
return "", errors.New("invalid string length")
}
buf := make([]byte, length)
if _, err := io.ReadFull(r, buf); err != nil {
return "", err
}
return string(buf), nil
}

// EncodeString は UTF-8 文字列をエンコードします（LE 接頭辞付き）
func EncodeString(w io.Writer, s string) error {
if err := EncodeVarInt(w, int64(len(s))); err != nil {
return err
}
_, err := w.Write([]byte(s))
return err
}

// ParseStartGamePacket は StartGame パケットを解析します
func ParseStartGamePacket(data []byte) (*StartGamePacket, error) {
r := bytes.NewReader(data)

// パケット ID をスキップ
if _, err := r.ReadByte(); err != nil {
return nil, err
}

pkt := &StartGamePacket{}
var err error

// EntityID
if pkt.EntityID, err = DecodeVarInt(r); err != nil {
return nil, err
}
// RuntimeEntityID
if pkt.RuntimeEntityID, err = DecodeVarInt(r); err != nil {
return nil, err
}
// PlayerGamemode
if err := binary.Read(r, binary.LittleEndian, &pkt.PlayerGamemode); err != nil {
return nil, err
}
// Position
for i := range pkt.PlayerPosition {
if err := binary.Read(r, binary.LittleEndian, &pkt.PlayerPosition[i]); err != nil {
return nil, err
}
}
// Pitch/Yaw
if err := binary.Read(r, binary.LittleEndian, &pkt.Pitch); err != nil {
return nil, err
}
if err := binary.Read(r, binary.LittleEndian, &pkt.Yaw); err != nil {
return nil, err
}
// Seed
if err := binary.Read(r, binary.LittleEndian, &pkt.Seed); err != nil {
return nil, err
}
// Dimension
if err := binary.Read(r, binary.LittleEndian, &pkt.Dimension); err != nil {
return nil, err
}
// GeneratorType
if err := binary.Read(r, binary.LittleEndian, &pkt.GeneratorType); err != nil {
return nil, err
}
// GamemodeForAll
if err := binary.Read(r, binary.LittleEndian, &pkt.GamemodeForAll); err != nil {
return nil, err
}
// Hardcore
if err := binary.Read(r, binary.LittleEndian, &pkt.Hardcore); err != nil {
return nil, err
}
// Cheats
if err := binary.Read(r, binary.LittleEndian, &pkt.Cheats); err != nil {
return nil, err
}
// CommandsEnabled
if err := binary.Read(r, binary.LittleEndian, &pkt.CommandsEnabled); err != nil {
return nil, err
}
// NoMobs
if err := binary.Read(r, binary.LittleEndian, &pkt.NoMobs); err != nil {
return nil, err
}

// GameRules
numRules, err := DecodeVarInt(r)
if err != nil {
return nil, err
}
pkt.GameRules = make([]GameRule, numRules)
for i := int64(0); i < numRules; i++ {
rule := &pkt.GameRules[i]
if rule.Name, err = DecodeString(r); err != nil {
return nil, err
}
if err := binary.Read(r, binary.LittleEndian, &rule.Type); err != nil {
return nil, err
}
switch rule.Type {
case 1: // bool
var v bool
if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
return nil, err
}
rule.Value = v
case 2: // int
var v int32
if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
return nil, err
}
rule.Value = v
case 3: // float
var v float32
if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
return nil, err
}
rule.Value = v
default:
return nil, errors.New("unknown game rule type")
}
}

// Experiments
numExp, err := DecodeVarInt(r)
if err != nil {
return nil, err
}
pkt.Experiments = make([]Experiment, numExp)
for i := int64(0); i < numExp; i++ {
exp := &pkt.Experiments[i]
if exp.Name, err = DecodeString(r); err != nil {
return nil, err
}
if err := binary.Read(r, binary.LittleEndian, &exp.Enabled); err != nil {
return nil, err
}
}

// BonusChest
if err := binary.Read(r, binary.LittleEndian, &pkt.BonusChest); err != nil {
return nil, err
}
// StartWithMap
if err := binary.Read(r, binary.LittleEndian, &pkt.StartWithMap); err != nil {
return nil, err
}
// ServerVersion
if pkt.ServerVersion, err = DecodeString(r); err != nil {
return nil, err
}
// EduFeatures
if err := binary.Read(r, binary.LittleEndian, &pkt.EduFeatures); err != nil {
return nil, err
}
// EduProductUUID
if pkt.EduProductUUID, err = DecodeString(r); err != nil {
return nil, err
}
// RainLevel
if err := binary.Read(r, binary.LittleEndian, &pkt.RainLevel); err != nil {
return nil, err
}
// LightningLevel
if err := binary.Read(r, binary.LittleEndian, &pkt.LightningLevel); err != nil {
return nil, err
}
// ConfirmedPlatform
if err := binary.Read(r, binary.LittleEndian, &pkt.ConfirmedPlatform); err != nil {
return nil, err
}
// PlatformBroadcast
if pkt.PlatformBroadcast, err = DecodeString(r); err != nil {
return nil, err
}
// XBLBroadcast
if pkt.XBLBroadcast, err = DecodeString(r); err != nil {
return nil, err
}
// DSLBroadcast
if pkt.DSLBroadcast, err = DecodeString(r); err != nil {
return nil, err
}
// WorldName
if pkt.WorldName, err = DecodeString(r); err != nil {
return nil, err
}
// TemplateContentKey
if pkt.TemplateContentKey, err = DecodeString(r); err != nil {
return nil, err
}
// IsFromTemplate
if err := binary.Read(r, binary.LittleEndian, &pkt.IsFromTemplate); err != nil {
return nil, err
}
// IsTrial
if err := binary.Read(r, binary.LittleEndian, &pkt.IsTrial); err != nil {
return nil, err
}
// MovementType
if err := binary.Read(r, binary.LittleEndian, &pkt.MovementType); err != nil {
return nil, err
}
// ServerAuthoritativeInventory
if err := binary.Read(r, binary.LittleEndian, &pkt.ServerAuthoritativeInventory); err != nil {
return nil, err
}
// EngineVersion
if pkt.EngineVersion, err = DecodeString(r); err != nil {
return nil, err
}
// ServerEngineVersion
if pkt.ServerEngineVersion, err = DecodeString(r); err != nil {
return nil, err
}

return pkt, nil
}

// CodeBuilderPacket は Code Builder パケットを表します
type CodeBuilderPacket struct {
URL         string
CodeStatus  uint8 // 0=none, 1=request, 2=response
ShouldOpen  bool
Code        string
}

// ParseCodeBuilderPacket は Code Builder パケットを解析します
func ParseCodeBuilderPacket(data []byte) (*CodeBuilderPacket, error) {
r := bytes.NewReader(data)

// パケット ID をスキップ
if _, err := r.ReadByte(); err != nil {
return nil, err
}

pkt := &CodeBuilderPacket{}
var err error

// URL
if pkt.URL, err = DecodeString(r); err != nil {
return nil, err
}
// CodeStatus
if err := binary.Read(r, binary.LittleEndian, &pkt.CodeStatus); err != nil {
return nil, err
}
// ShouldOpen
if err := binary.Read(r, binary.LittleEndian, &pkt.ShouldOpen); err != nil {
return nil, err
}
// Code (optional)
if pkt.CodeStatus == 2 { // response
if pkt.Code, err = DecodeString(r); err != nil {
return nil, err
}
}

return pkt, nil
}

// EncodeCodeBuilderPacket は Code Builder パケットをエンコードします
func EncodeCodeBuilderPacket(pkt *CodeBuilderPacket) ([]byte, error) {
var buf bytes.Buffer

// PacketID
if err := buf.WriteByte(PacketIDCodeBuilder); err != nil {
return nil, err
}
// URL
if err := EncodeString(&buf, pkt.URL); err != nil {
return nil, err
}
// CodeStatus
if err := binary.Write(&buf, binary.LittleEndian, pkt.CodeStatus); err != nil {
return nil, err
}
// ShouldOpen
if err := binary.Write(&buf, binary.LittleEndian, pkt.ShouldOpen); err != nil {
return nil, err
}
// Code (if response)
if pkt.CodeStatus == 2 && pkt.Code != "" {
if err := EncodeString(&buf, pkt.Code); err != nil {
return nil, err
}
}

return buf.Bytes(), nil
}
