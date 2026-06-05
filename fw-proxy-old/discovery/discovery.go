// Package discovery provides LAN discovery for Minecraft Bedrock Edition.
package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	// DiscoveryPort is the UDP port used for LAN discovery.
	DiscoveryPort = 19132
	
	// DiscoveryBroadcastAddress is the broadcast address for LAN discovery.
	DiscoveryBroadcastAddress = "255.255.255.255:19132"
	
	// Magic is the magic number used in discovery packets.
	Magic = int64(0x00ffff00fefefefefdfdfdfd12345678)
)

// PacketType represents the type of discovery packet.
type PacketType byte

const (
	PacketRequest PacketType = 0x01
	PacketResponse PacketType = 0x02
	PacketMessage PacketType = 0x03
)

// RequestPacket represents a discovery request.
type RequestPacket struct {
	PacketType PacketType
	Timestamp  int64
	GUID       int64
}

// ResponsePacket represents a discovery response.
type ResponsePacket struct {
	PacketType     PacketType
	Timestamp      int64
	ServerGUID     int64
	Motd           string
	Protocol       int
	Version        string
	PlayerCount    int
	MaxPlayers     int
	GameMode       string
	MapName        string
	GameID         string
	PortV6         int
	PortV4         int
}

// MessagePacket represents a discovery message.
type MessagePacket struct {
	PacketType PacketType
	Timestamp  int64
	Data       []byte
}

// Listener listens for LAN discovery packets.
type Listener struct {
	mu          sync.Mutex
	conn        *net.UDPConn
	closed      bool
	discovered  map[string]*ResponsePacket
	onDiscover  func(*ResponsePacket)
	ctx         context.Context
	cancel      context.CancelFunc
}

// Config contains configuration for the discovery listener.
type Config struct {
	OnDiscover func(*ResponsePacket)
	Logger     interface{}
}

// NewListener creates a new discovery listener.
func NewListener(cfg Config) (*Listener, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	addr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}
	
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to listen UDP: %w", err)
	}
	
	// Enable broadcast
	if err := conn.SetBroadcastMode(true); err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("failed to set broadcast mode: %w", err)
	}
	
	l := &Listener{
		conn:       conn,
		discovered: make(map[string]*ResponsePacket),
		onDiscover: cfg.OnDiscover,
		ctx:        ctx,
		cancel:     cancel,
	}
	
	go l.listenLoop()
	
	return l, nil
}

// SendRequest sends a discovery request to the broadcast address.
func (l *Listener) SendRequest() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.closed {
		return errors.New("listener closed")
	}
	
	packet := RequestPacket{
		PacketType: PacketRequest,
		Timestamp:  time.Now().UnixNano(),
		GUID:       Magic,
	}
	
	data := marshalRequest(packet)
	
	broadcastAddr, err := net.ResolveUDPAddr("udp4", DiscoveryBroadcastAddress)
	if err != nil {
		return fmt.Errorf("failed to resolve broadcast address: %w", err)
	}
	
	_, err = l.conn.WriteToUDP(data, broadcastAddr)
	return err
}

// SendRequestTo sends a discovery request to a specific address.
func (l *Listener) SendRequestTo(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.closed {
		return errors.New("listener closed")
	}
	
	packet := RequestPacket{
		PacketType: PacketRequest,
		Timestamp:  time.Now().UnixNano(),
		GUID:       Magic,
	}
	
	data := marshalRequest(packet)
	
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}
	
	_, err = l.conn.WriteToUDP(data, udpAddr)
	return err
}

// GetDiscoveredServers returns all discovered servers.
func (l *Listener) GetDiscoveredServers() map[string]*ResponsePacket {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	result := make(map[string]*ResponsePacket)
	for k, v := range l.discovered {
		result[k] = v
	}
	return result
}

// Close closes the listener.
func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.closed {
		return nil
	}
	
	l.closed = true
	l.cancel()
	return l.conn.Close()
}

func (l *Listener) listenLoop() {
	buf := make([]byte, 4096)
	
	for {
		select {
		case <-l.ctx.Done():
			return
		default:
			l.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, addr, err := l.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				return
			}
			
			packet := buf[:n]
			response, err := parseResponse(packet)
			if err != nil {
				continue
			}
			
			l.mu.Lock()
			serverAddr := addr.String()
			if _, exists := l.discovered[serverAddr]; !exists {
				l.discovered[serverAddr] = response
				if l.onDiscover != nil {
					go l.onDiscover(response)
				}
			}
			l.mu.Unlock()
		}
	}
}

func marshalRequest(pkt RequestPacket) []byte {
	buf := make([]byte, 0, 18)
	buf = append(buf, byte(pkt.PacketType))
	
	// Timestamp (8 bytes, big endian)
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, uint64(pkt.Timestamp))
	buf = append(buf, tsBuf...)
	
	// GUID (8 bytes, big endian)
	guidBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(guidBuf, uint64(pkt.GUID))
	buf = append(buf, guidBuf...)
	
	return buf
}

func parseResponse(data []byte) (*ResponsePacket, error) {
	if len(data) < 17 {
		return nil, errors.New("invalid response packet length")
	}
	
	pkt := &ResponsePacket{
		PacketType: PacketType(data[0]),
	}
	
	if pkt.PacketType != PacketResponse {
		return nil, errors.New("not a response packet")
	}
	
	// Parse timestamp
	pkt.Timestamp = int64(binary.BigEndian.Uint64(data[1:9]))
	
	// Parse server GUID
	pkt.ServerGUID = int64(binary.BigEndian.Uint64(data[9:17]))
	
	// Parse remaining data as string (MOTD and other info)
	if len(data) > 17 {
		remaining := string(data[17:])
		// Split by semicolon
		parts := splitString(remaining, ';')
		
		if len(parts) >= 1 {
			pkt.Motd = parts[0]
		}
		if len(parts) >= 2 {
			fmt.Sscanf(parts[1], "%d", &pkt.Protocol)
		}
		if len(parts) >= 3 {
			pkt.Version = parts[2]
		}
		if len(parts) >= 4 {
			fmt.Sscanf(parts[3], "%d", &pkt.PlayerCount)
		}
		if len(parts) >= 5 {
			fmt.Sscanf(parts[4], "%d", &pkt.MaxPlayers)
		}
		if len(parts) >= 6 {
			pkt.GameMode = parts[5]
		}
		if len(parts) >= 7 {
			pkt.MapName = parts[6]
		}
		if len(parts) >= 8 {
			pkt.GameID = parts[7]
		}
		if len(parts) >= 9 {
			fmt.Sscanf(parts[8], "%d", &pkt.PortV6)
		}
		if len(parts) >= 10 {
			fmt.Sscanf(parts[9], "%d", &pkt.PortV4)
		}
	}
	
	return pkt, nil
}

func splitString(s string, sep rune) []string {
	var parts []string
	var current string
	
	for _, r := range s {
		if r == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	
	if current != "" {
		parts = append(parts, current)
	}
	
	return parts
}
