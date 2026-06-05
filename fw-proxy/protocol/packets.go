package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Minecraft のバージョン情報
const (
	VersionString   = "26.20"
	ProtocolVersion = 975
)

// PacketID はパケット ID です
type PacketID uint32

const (
	PacketIDStartGame PacketID = 0x0b
	PacketIDCodeBuilderPacket PacketID = 0x4d
	PacketIDCodeBuilderRequestPacket PacketID = 0x4e
	PacketIDCodeBuilderResponsePacket PacketID = 0x4f
)

// Packet は Minecraft パケットのインターフェースです
type Packet interface {
	ID() PacketID
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

// StartGamePacket は StartGame パケットです
type StartGamePacket struct {
	EntityID          int64
	RuntimeEntityID   int64
	PlayerGamemode    int32
	PlayerPosition    [3]float32
	Pitch             float32
	Yaw               float32
	Seed              int64
	Dimension         int16
	GeneratorType     int32
	GameMode          int32
	Hardcore          bool
	Cheats            bool
	RequiresPack      bool
	GameRules         []GameRule
	LevelName         string
	TemplateContent   []byte
	HasAchievements   bool
	Time              int32
	EducationEdition  bool
	EduFeatures       bool
	RainLevel         float32
	LightningLevel    float32
	Platform          string
	Multiplayer       bool
	LANBroadcast      bool
	XBLBroadcast      bool
	PlatformBroadcast int32
	InboxNotificationsEnabled bool
	CommandBlockEnabled bool
	RequiresResourcePack bool
	ExperimentalGameplay bool
	BonusChestEnabled bool
	StartWithMapEnabled bool
	Permissions       int32
	ServerChunkTickRange int32
	BehaviorPackLocked bool
	ResourcePackLocked bool
	FromLockedWorldTemplate bool
	UseMSAGamerTagsOnly bool
	FromWorldTemplate bool
	WorldTemplateOptionLocked bool
	OnlySpawnV1Villagers bool
	PersonaDisabled bool
	CustomSkinsDisabled bool
	EmoteChatMuted bool
	DefaultPlayerPermission int32
	ServerAuthoritativeInventory bool
	ExperimentalGameplayOverride bool
	ClientSideGeneration bool
	WorldVersion        int32
	LimitedWorldWidth   int32
	LimitedWorldDepth   int32
	NewNether           bool
	EduSharedURI        string
	EduSharedResourcePatch bool
	ForceExperimentalGameplay bool
	ServerID            string
	WorldID             string
	ScenarioID          string
	Tier                int32
}

type GameRule struct {
	Name     string
	Type     int32
	Value    interface{}
}

func (p *StartGamePacket) ID() PacketID {
	return PacketIDStartGame
}

func (p *StartGamePacket) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	
	// VarInt: EntityID
	writeVarInt(buf, p.EntityID)
	// VarInt: RuntimeEntityID
	writeVarInt(buf, p.RuntimeEntityID)
	// VarInt: PlayerGamemode
	writeVarInt(buf, int64(p.PlayerGamemode))
	// Vec3f: Position
	writeFloat32(buf, p.PlayerPosition[0])
	writeFloat32(buf, p.PlayerPosition[1])
	writeFloat32(buf, p.PlayerPosition[2])
	// Float32: Pitch, Yaw
	writeFloat32(buf, p.Pitch)
	writeFloat32(buf, p.Yaw)
	// VarInt: Seed
	writeVarInt(buf, p.Seed)
	// VarInt: Dimension
	writeVarInt(buf, int64(p.Dimension))
	// VarInt: GeneratorType
	writeVarInt(buf, int64(p.GeneratorType))
	// VarInt: GameMode
	writeVarInt(buf, int64(p.GameMode))
	// Bool: Hardcore
	writeBool(buf, p.Hardcore)
	// Bool: Cheats
	writeBool(buf, p.Cheats)
	// Bool: RequiresPack
	writeBool(buf, p.RequiresPack)
	// VarInt: GameRules count
	writeVarInt(buf, int64(len(p.GameRules)))
	for _, rule := range p.GameRules {
		writeString(buf, rule.Name)
		writeVarInt(buf, int64(rule.Type))
		switch rule.Type {
		case 1: // Bool
			if v, ok := rule.Value.(bool); ok {
				writeBool(buf, v)
			}
		case 2: // Int
			if v, ok := rule.Value.(int32); ok {
				writeVarInt(buf, int64(v))
			}
		case 3: // Float
			if v, ok := rule.Value.(float32); ok {
				writeFloat32(buf, v)
			}
		}
	}
	// String: LevelName
	writeString(buf, p.LevelName)
	// String: TemplateContent (base64)
	writeString(buf, string(p.TemplateContent))
	// Bool: HasAchievements
	writeBool(buf, p.HasAchievements)
	// VarInt: Time
	writeVarInt(buf, int64(p.Time))
	// Bool: EducationEdition
	writeBool(buf, p.EducationEdition)
	// Bool: EduFeatures
	writeBool(buf, p.EduFeatures)
	// Float32: RainLevel
	writeFloat32(buf, p.RainLevel)
	// Float32: LightningLevel
	writeFloat32(buf, p.LightningLevel)
	// Bool: Platform
	writeBool(buf, p.Platform != "")
	if p.Platform != "" {
		writeString(buf, p.Platform)
	}
	// Bool: Multiplayer
	writeBool(buf, p.Multiplayer)
	// Bool: LANBroadcast
	writeBool(buf, p.LANBroadcast)
	// VarInt: XBLBroadcast (bool -> 0/1)
	if p.XBLBroadcast {
		writeVarInt(buf, 1)
	} else {
		writeVarInt(buf, 0)
	}
	// VarInt: PlatformBroadcast
	writeVarInt(buf, int64(p.PlatformBroadcast))
	// Bool: InboxNotificationsEnabled
	writeBool(buf, p.InboxNotificationsEnabled)
	// Bool: CommandBlockEnabled
	writeBool(buf, p.CommandBlockEnabled)
	// Bool: RequiresResourcePack
	writeBool(buf, p.RequiresResourcePack)
	// Bool: ExperimentalGameplay
	writeBool(buf, p.ExperimentalGameplay)
	// Bool: BonusChestEnabled
	writeBool(buf, p.BonusChestEnabled)
	// Bool: StartWithMapEnabled
	writeBool(buf, p.StartWithMapEnabled)
	// VarInt: Permissions
	writeVarInt(buf, int64(p.Permissions))
	// VarInt: ServerChunkTickRange
	writeVarInt(buf, int64(p.ServerChunkTickRange))
	// Bool: BehaviorPackLocked
	writeBool(buf, p.BehaviorPackLocked)
	// Bool: ResourcePackLocked
	writeBool(buf, p.ResourcePackLocked)
	// Bool: FromLockedWorldTemplate
	writeBool(buf, p.FromLockedWorldTemplate)
	// Bool: UseMSAGamerTagsOnly
	writeBool(buf, p.UseMSAGamerTagsOnly)
	// Bool: FromWorldTemplate
	writeBool(buf, p.FromWorldTemplate)
	// Bool: WorldTemplateOptionLocked
	writeBool(buf, p.WorldTemplateOptionLocked)
	// Bool: OnlySpawnV1Villagers
	writeBool(buf, p.OnlySpawnV1Villagers)
	// Bool: PersonaDisabled
	writeBool(buf, p.PersonaDisabled)
	// Bool: CustomSkinsDisabled
	writeBool(buf, p.CustomSkinsDisabled)
	// Bool: EmoteChatMuted
	writeBool(buf, p.EmoteChatMuted)
	// VarInt: DefaultPlayerPermission
	writeVarInt(buf, int64(p.DefaultPlayerPermission))
	// VarInt: ServerAuthoritativeInventory (bool -> 0/1)
	if p.ServerAuthoritativeInventory {
		writeVarInt(buf, 1)
	} else {
		writeVarInt(buf, 0)
	}
	// Bool: ExperimentalGameplayOverride
	writeBool(buf, p.ExperimentalGameplayOverride)
	// Bool: ClientSideGeneration
	writeBool(buf, p.ClientSideGeneration)
	// VarInt: WorldVersion
	writeVarInt(buf, int64(p.WorldVersion))
	// VarInt: LimitedWorldWidth
	writeVarInt(buf, int64(p.LimitedWorldWidth))
	// VarInt: LimitedWorldDepth
	writeVarInt(buf, int64(p.LimitedWorldDepth))
	// Bool: NewNether
	writeBool(buf, p.NewNether)
	// String: EduSharedURI
	writeString(buf, p.EduSharedURI)
	// Bool: EduSharedResourcePatch
	writeBool(buf, p.EduSharedResourcePatch)
	// Bool: ForceExperimentalGameplay
	writeBool(buf, p.ForceExperimentalGameplay)
	// String: ServerID
	writeString(buf, p.ServerID)
	// String: WorldID
	writeString(buf, p.WorldID)
	// String: ScenarioID
	writeString(buf, p.ScenarioID)
	// VarInt: Tier
	writeVarInt(buf, int64(p.Tier))
	
	return buf.Bytes(), nil
}

func (p *StartGamePacket) Unmarshal(data []byte) error {
	buf := bytes.NewReader(data)
	
	var err error
	p.EntityID, err = readVarInt(buf)
	if err != nil {
		return err
	}
	p.RuntimeEntityID, err = readVarInt(buf)
	if err != nil {
		return err
	}
	gamemode, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.PlayerGamemode = int32(gamemode)
	
	p.PlayerPosition[0], err = readFloat32(buf)
	if err != nil {
		return err
	}
	p.PlayerPosition[1], err = readFloat32(buf)
	if err != nil {
		return err
	}
	p.PlayerPosition[2], err = readFloat32(buf)
	if err != nil {
		return err
	}
	
	p.Pitch, err = readFloat32(buf)
	if err != nil {
		return err
	}
	p.Yaw, err = readFloat32(buf)
	if err != nil {
		return err
	}
	
	seed, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.Seed = seed
	
	dim, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.Dimension = int16(dim)
	
	genType, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.GeneratorType = int32(genType)
	
	mode, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.GameMode = int32(mode)
	
	p.Hardcore, err = readBool(buf)
	if err != nil {
		return err
	}
	p.Cheats, err = readBool(buf)
	if err != nil {
		return err
	}
	p.RequiresPack, err = readBool(buf)
	if err != nil {
		return err
	}
	
	ruleCount, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.GameRules = make([]GameRule, ruleCount)
	for i := int64(0); i < ruleCount; i++ {
		p.GameRules[i].Name, err = readString(buf)
		if err != nil {
			return err
		}
		ruleType, err := readVarInt(buf)
		if err != nil {
			return err
		}
		p.GameRules[i].Type = int32(ruleType)
		switch p.GameRules[i].Type {
		case 1:
			p.GameRules[i].Value, err = readBool(buf)
		case 2:
			v, err := readVarInt(buf)
			if err != nil {
				return err
			}
			p.GameRules[i].Value = int32(v)
		case 3:
			p.GameRules[i].Value, err = readFloat32(buf)
		}
		if err != nil {
			return err
		}
	}
	
	p.LevelName, err = readString(buf)
	if err != nil {
		return err
	}
	templateContent, err := readString(buf)
	if err != nil {
		return err
	}
	p.TemplateContent = []byte(templateContent)
	
	p.HasAchievements, err = readBool(buf)
	if err != nil {
		return err
	}
	timeVal, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.Time = int32(timeVal)
	
	p.EducationEdition, err = readBool(buf)
	if err != nil {
		return err
	}
	p.EduFeatures, err = readBool(buf)
	if err != nil {
		return err
	}
	p.RainLevel, err = readFloat32(buf)
	if err != nil {
		return err
	}
	p.LightningLevel, err = readFloat32(buf)
	if err != nil {
		return err
	}
	
	hasPlatform, err := readBool(buf)
	if err != nil {
		return err
	}
	if hasPlatform {
		p.Platform, err = readString(buf)
		if err != nil {
			return err
		}
	}
	
	p.Multiplayer, err = readBool(buf)
	if err != nil {
		return err
	}
	p.LANBroadcast, err = readBool(buf)
	if err != nil {
		return err
	}
	xblBroadcast, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.XBLBroadcast = xblBroadcast != 0
	
	platformBroadcast, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.PlatformBroadcast = int32(platformBroadcast)
	
	p.InboxNotificationsEnabled, err = readBool(buf)
	if err != nil {
		return err
	}
	p.CommandBlockEnabled, err = readBool(buf)
	if err != nil {
		return err
	}
	p.RequiresResourcePack, err = readBool(buf)
	if err != nil {
		return err
	}
	p.ExperimentalGameplay, err = readBool(buf)
	if err != nil {
		return err
	}
	p.BonusChestEnabled, err = readBool(buf)
	if err != nil {
		return err
	}
	p.StartWithMapEnabled, err = readBool(buf)
	if err != nil {
		return err
	}
	
	permissions, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.Permissions = int32(permissions)
	
	chunkRange, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.ServerChunkTickRange = int32(chunkRange)
	
	p.BehaviorPackLocked, err = readBool(buf)
	if err != nil {
		return err
	}
	p.ResourcePackLocked, err = readBool(buf)
	if err != nil {
		return err
	}
	p.FromLockedWorldTemplate, err = readBool(buf)
	if err != nil {
		return err
	}
	p.UseMSAGamerTagsOnly, err = readBool(buf)
	if err != nil {
		return err
	}
	p.FromWorldTemplate, err = readBool(buf)
	if err != nil {
		return err
	}
	p.WorldTemplateOptionLocked, err = readBool(buf)
	if err != nil {
		return err
	}
	p.OnlySpawnV1Villagers, err = readBool(buf)
	if err != nil {
		return err
	}
	p.PersonaDisabled, err = readBool(buf)
	if err != nil {
		return err
	}
	p.CustomSkinsDisabled, err = readBool(buf)
	if err != nil {
		return err
	}
	p.EmoteChatMuted, err = readBool(buf)
	if err != nil {
		return err
	}
	
	defaultPerm, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.DefaultPlayerPermission = int32(defaultPerm)
	
	serverAuthInv, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.ServerAuthoritativeInventory = serverAuthInv != 0
	
	p.ExperimentalGameplayOverride, err = readBool(buf)
	if err != nil {
		return err
	}
	p.ClientSideGeneration, err = readBool(buf)
	if err != nil {
		return err
	}
	
	worldVer, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.WorldVersion = int32(worldVer)
	
	limitedWidth, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.LimitedWorldWidth = int32(limitedWidth)
	
	limitedDepth, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.LimitedWorldDepth = int32(limitedDepth)
	
	p.NewNether, err = readBool(buf)
	if err != nil {
		return err
	}
	
	p.EduSharedURI, err = readString(buf)
	if err != nil {
		return err
	}
	
	p.EduSharedResourcePatch, err = readBool(buf)
	if err != nil {
		return err
	}
	
	p.ForceExperimentalGameplay, err = readBool(buf)
	if err != nil {
		return err
	}
	
	p.ServerID, err = readString(buf)
	if err != nil {
		return err
	}
	
	p.WorldID, err = readString(buf)
	if err != nil {
		return err
	}
	
	p.ScenarioID, err = readString(buf)
	if err != nil {
		return err
	}
	
	tier, err := readVarInt(buf)
	if err != nil {
		return err
	}
	p.Tier = int32(tier)
	
	return nil
}

// CodeBuilderPacket は Code Builder パケットです
type CodeBuilderPacket struct {
	URL string
	Code string
}

func (p *CodeBuilderPacket) ID() PacketID {
	return PacketIDCodeBuilderPacket
}

func (p *CodeBuilderPacket) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	writeString(buf, p.URL)
	writeString(buf, p.Code)
	return buf.Bytes(), nil
}

func (p *CodeBuilderPacket) Unmarshal(data []byte) error {
	buf := bytes.NewReader(data)
	var err error
	p.URL, err = readString(buf)
	if err != nil {
		return err
	}
	p.Code, err = readString(buf)
	if err != nil {
		return err
	}
	return nil
}

// Helper functions
func writeVarInt(buf *bytes.Buffer, val int64) {
	for val >= 0x80 || val < -0x80 {
		buf.WriteByte(byte(val&0x7f) | 0x80)
		val >>= 7
	}
	buf.WriteByte(byte(val))
}

func readVarInt(r io.Reader) (int64, error) {
	var result int64
	var shift uint
	for {
		b, err := readByte(r)
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7f) << shift
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

func writeFloat32(buf *bytes.Buffer, val float32) {
	binary.Write(buf, binary.LittleEndian, val)
}

func readFloat32(r io.Reader) (float32, error) {
	var val float32
	err := binary.Read(r, binary.LittleEndian, &val)
	return val, err
}

func writeBool(buf *bytes.Buffer, val bool) {
	if val {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func readBool(r io.Reader) (bool, error) {
	b, err := readByte(r)
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

func writeString(buf *bytes.Buffer, val string) {
	writeVarInt(buf, int64(len(val)))
	buf.WriteString(val)
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", errors.New("invalid string length")
	}
	buf := make([]byte, length)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func readByte(r io.Reader) (byte, error) {
	if br, ok := r.(*bytes.Reader); ok {
		b, err := br.ReadByte()
		return b, err
	}
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	return buf[0], err
}

// ParsePacket はパケット ID からパケットをパースします
func ParsePacket(packetID PacketID, data []byte) (Packet, error) {
	var pkt Packet
	switch packetID {
	case PacketIDStartGame:
		pkt = &StartGamePacket{}
	case PacketIDCodeBuilderPacket:
		pkt = &CodeBuilderPacket{}
	default:
		return nil, fmt.Errorf("unknown packet ID: %d", packetID)
	}
	if err := pkt.Unmarshal(data); err != nil {
		return nil, err
	}
	return pkt, nil
}
