package badvpn

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/dialer"
	"github.com/xjasonlyu/tun2socks/v2/log"
)

const (
	flagKeepAlive = 0x01
	flagIPv6      = 0x08
	headerSize    = 3 // Flags(1) + ConnID(2)
)

// Client handles a single UDPGW session over a TCP connection.
type Client struct {
	conn   net.Conn
	connID uint16
	mu     sync.Mutex
}

// NewClient establishes a new TCP connection to the UDPGW server via the proxy dialer.
func NewClient(serverAddr string) (*Client, error) {
	// Use DefaultDialer to ensure traffic goes through the proxy (SOCKS5/Hysteria)
	conn, err := dialer.DefaultDialer.DialContext(context.Background(), "tcp", serverAddr)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		connID: 0, // Single flow mode, ID can be static or random
	}, nil
}

// Close closes the underlying TCP connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// WriteUDPGW encapsulates a UDP packet and sends it to the server.
func (c *Client) WriteUDPGW(dstIP net.IP, dstPort uint16, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	isIPv6 := dstIP.To4() == nil
	var addrLen int
	if isIPv6 {
		addrLen = 16
	} else {
		addrLen = 4
	}

	// Calculate packet size: Header(3) + IP(4/16) + Port(2) + Data
	packetSize := headerSize + addrLen + 2 + len(data)
	
	// Total frame size: Size(2) + Packet
	buf := make([]byte, 2+packetSize)

	// 1. Frame Size (Little Endian)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(packetSize))

	// 2. Flags
	var flags uint8
	if isIPv6 {
		flags |= flagIPv6
	}
	buf[2] = flags

	// 3. ConnID (Little Endian)
	binary.LittleEndian.PutUint16(buf[3:5], c.connID)

	// 4. Address (Raw Bytes)
	if isIPv6 {
		copy(buf[5:21], dstIP.To16())
		// 5. Port (Big Endian)
		binary.BigEndian.PutUint16(buf[21:23], dstPort)
		// 6. Data
		copy(buf[23:], data)
	} else {
		copy(buf[5:9], dstIP.To4())
		// 5. Port (Big Endian)
		binary.BigEndian.PutUint16(buf[9:11], dstPort)
		// 6. Data
		copy(buf[11:], data)
	}

	_, err := c.conn.Write(buf)
	return err
}

// ReadUDPGW reads a packet from the server, strips the header, and returns the payload.
func (c *Client) ReadUDPGW() ([]byte, error) {
	// Read Frame Size (2 bytes)
	sizeBuf := make([]byte, 2)
	if _, err := io.ReadFull(c.conn, sizeBuf); err != nil {
		return nil, err
	}
	totalSize := binary.LittleEndian.Uint16(sizeBuf)

	// Read Payload
	payload := make([]byte, totalSize)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return nil, err
	}

	// Parse Header
	flags := payload[0]
	// connID := binary.LittleEndian.Uint16(payload[1:3]) // Skip ID check for now

	// Check KeepAlive
	if flags&flagKeepAlive != 0 {
		// It's a keep-alive packet, ignore and recurse
		// NOTE: In recursive read, be careful of stack overflow if too many KA packets.
		// Better to use loop in caller or here. But for simplicity:
		log.Debugf("[UDPGW] Received KeepAlive")
		return c.ReadUDPGW() 
	}

	// Determine Address Length
	var addrLen int
	if flags&flagIPv6 != 0 {
		addrLen = 16
	} else {
		addrLen = 4
	}

	// Validate size
	headerEnd := 3 + addrLen + 2 // Flags(1) + ID(2) + IP + Port(2)
	if int(totalSize) < headerEnd {
		return nil, fmt.Errorf("packet too short")
	}

	// Extract Data
	data := payload[headerEnd:]
	return data, nil
}
