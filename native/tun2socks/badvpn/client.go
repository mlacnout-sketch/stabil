package badvpn

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/log"
	"github.com/xjasonlyu/tun2socks/v2/proxy"
)

var (
	// LocalSocksAddr is the address of the local SOCKS5 proxy to connect through.
	// Can be modified for testing.
	LocalSocksAddr = "127.0.0.1:7777"
	
	// KeepAliveInterval default 10s
	KeepAliveInterval = 10 * time.Second
)

const (
	FlagKeepAlive = 0x01
	FlagIPv6      = 0x08
)

// Packet represents a received UDP packet
type Packet struct {
	DstIP   netip.Addr
	DstPort uint16
	Data    []byte
}

// Client manages the UDPGW session
type Client struct {
	proxy       proxy.Proxy
	serverAddr  string
	conn        net.Conn
	mu          sync.Mutex
	conns       map[uint16]chan *Packet // Map ConnID -> Channel
	nextID      uint16
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectCh chan struct{}
}

func NewClient(p proxy.Proxy, serverAddr string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		proxy:       p,
		serverAddr:  serverAddr,
		conns:       make(map[uint16]chan *Packet),
		nextID:      1,
		ctx:         ctx,
		cancel:      cancel,
		reconnectCh: make(chan struct{}, 1),
	}
	go c.loop()
	return c
}

// Register registers a new flow and returns a ConnID and a packet channel
func (c *Client) Register() (uint16, <-chan *Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++
	if c.nextID == 0 {
		c.nextID = 1 // Skip 0
	}

	// Simple collision avoidance (loop until free)
	for {
		if _, exists := c.conns[id]; !exists {
			break
		}
		id++
		if id == 0 {
			id = 1
		}
	}

	ch := make(chan *Packet, 64)
	c.conns[id] = ch
	return id, ch
}

// Unregister removes a flow
func (c *Client) Unregister(id uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.conns[id]; ok {
		close(ch)
		delete(c.conns, id)
	}
}

// WritePacket sends a packet to the UDPGW server
func (c *Client) WritePacket(connID uint16, dest netip.Addr, port uint16, data []byte) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return errors.New("not connected")
	}

	isIPv6 := dest.Is6()
	addrLen := 4
	if isIPv6 {
		addrLen = 16
	}

	// Header: 2 bytes Len
	// Payload: 1 Flags + 2 ID + Addr + 2 Port + Data
	payloadLen := 1 + 2 + addrLen + 2 + len(data)
	buf := make([]byte, 2+payloadLen)

	// Length (Little Endian)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(payloadLen))

	// Flags
	if isIPv6 {
		buf[2] = FlagIPv6
	} else {
		buf[2] = 0x00
	}

	// ConnID (Little Endian)
	binary.LittleEndian.PutUint16(buf[3:5], connID)

	// Address
	if isIPv6 {
		copy(buf[5:21], dest.AsSlice())
		// Port (Big Endian)
		binary.BigEndian.PutUint16(buf[21:23], port)
		copy(buf[23:], data)
	} else {
		copy(buf[5:9], dest.AsSlice())
		// Port (Big Endian)
		binary.BigEndian.PutUint16(buf[9:11], port)
		copy(buf[11:], data)
	}

	// Write atomically? No, net.Conn is thread-safe but writing multiple chunks might interleave.
	// We constructed a single buffer, so single Write call is atomic enough.
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(buf)
	return err
}

func (c *Client) loop() {
	backoff := time.Second

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		err := c.connect()
		if err != nil {
			log.Warnf("[UDPGW] Connect failed: %v. Retrying in %v", err, backoff)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			}
		}

		backoff = time.Second // Reset backoff
		log.Infof("[UDPGW] Connected to %s", c.serverAddr)

		// Start KeepAlive
		go c.keepAlive()

		// Read Loop
		c.readLoop()

		// Disconnected
		log.Warnf("[UDPGW] Disconnected")
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}
}

func (c *Client) connect() error {
	// Parse address
	host, portStr, err := net.SplitHostPort(c.serverAddr)
	if err != nil {
		return err
	}
	port, _ := net.LookupPort("tcp", portStr)

	// Dial via Proxy (SOCKS5 -> Hysteria -> 127.0.0.1:7300)
	// We use Metadata to describe the target
	// wait... Proxy.Dial usually takes Metadata or Address string?
	// The interface is Proxy.Dial(metadata *Metadata) (net.Conn, error)

	// But wait, Proxy.Dial expects us to route *TO* the proxy, or *THROUGH* the proxy?
	// We want to connect THROUGH the proxy TO 127.0.0.1:7300 (on the server side).
	
	// BUT, `c.proxy` is usually the SOCKS5 handler which routes packet based on metadata.
	// If we use SOCKS5, the metadata destination IS 127.0.0.1:7300.
	
	// We need to construct metadata. But we don't have metadata package here yet.
	// Let's assume we can just use net.Dial to the LOCAL SOCKS5 port (e.g. 7777)
	// and ask it to CONNECT to 7300.
	// This is cleaner than using internal Proxy interface which might be complex.
	
	// However, we are INSIDE tun2socks process. We can use the Proxy interface directly.
	
	// Let's assume we connect to local SOCKS5 (7777) via TCP.
	conn, err := net.DialTimeout("tcp", LocalSocksAddr, 5*time.Second)
	if err != nil {
		return err
	}

	// Perform SOCKS5 Handshake to 127.0.0.1:7300
	// 1. Auth (No Auth)
	conn.Write([]byte{0x05, 0x01, 0x00})
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		conn.Close()
		return err
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		conn.Close()
		return errors.New("SOCKS5 handshake failed")
	}

	// 2. Connect
	// CMD=0x01 (CONNECT), ATYP=0x01 (IPv4)
	req := []byte{0x05, 0x01, 0x00, 0x01}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		conn.Close()
		return errors.New("invalid IPv4 for UDPGW")
	}
	req = append(req, ip...)
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, uint16(port))
	req = append(req, portBuf...)

	conn.Write(req)

	// Read Reply
	// VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
	// Minimal 10 bytes (IPv4)
	respHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, respHeader); err != nil {
		conn.Close()
		return err
	}
	if respHeader[1] != 0x00 {
		conn.Close()
		return fmt.Errorf("SOCKS5 connect failed: %d", respHeader[1])
	}
	
	// Skip bind address (variable length)
	addrType := respHeader[3]
	var skip int
	switch addrType {
	case 0x01: skip = 4 + 2 // IPv4
	case 0x03: // Domain
		lenBuf := make([]byte, 1)
		io.ReadFull(conn, lenBuf)
		skip = int(lenBuf[0]) + 2
	case 0x04: skip = 16 + 2 // IPv6
	}
	io.CopyN(io.Discard, conn, int64(skip))

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Client) keepAlive() {
	ticker := time.NewTicker(KeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()
			if conn == nil {
				return
			}
			// KeepAlive: Len=3 (1 Flag + 2 ID), Flag=0x01, ID=0
			buf := []byte{0x03, 0x00, FlagKeepAlive, 0x00, 0x00}
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write(buf); err != nil {
				conn.Close() // Will trigger readLoop failure
				return
			}
		}
	}
}

func (c *Client) readLoop() {
	// Re-usable buffers?
	// Header is small (2 bytes len)
	header := make([]byte, 2)
	
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}

		// Read Length
		conn.SetReadDeadline(time.Now().Add(120 * time.Second)) // Keepalive is 10s, so 120s is ample
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := binary.LittleEndian.Uint16(header)

		// Read Payload
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}

		if length < 3 {
			continue // Too short
		}

		flags := payload[0]
		connID := binary.LittleEndian.Uint16(payload[1:3])

		if flags&FlagKeepAlive != 0 {
			// Log keepalive? No, noise.
			continue
		}

		var ip netip.Addr
		var port uint16
		var data []byte

		offset := 3
		if flags&FlagIPv6 != 0 {
			if len(payload) < offset+16+2 {
				continue
			}
			ip, _ = netip.AddrFromSlice(payload[offset : offset+16])
			offset += 16
		} else {
			if len(payload) < offset+4+2 {
				continue
			}
			ip, _ = netip.AddrFromSlice(payload[offset : offset+4])
			offset += 4
		}

		port = binary.BigEndian.Uint16(payload[offset : offset+2])
		offset += 2
		data = payload[offset:]

		// Dispatch
		c.mu.Lock()
		ch, ok := c.conns[connID]
		c.mu.Unlock()

		if ok {
			select {
			case ch <- &Packet{
				DstIP:   ip,
				DstPort: port,
				Data:    data,
			}:
			default:
				// Drop if channel full
			}
		}
	}
}
