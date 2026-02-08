package badvpn

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock SOCKS5 Server that accepts BadVPN UDPGW handshake
func startMockServer(t *testing.T) (net.Listener, chan []byte, chan []byte) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	sendCh := make(chan []byte, 10)
	recvCh := make(chan []byte, 10)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return // Test likely finished
		}
		defer conn.Close()

		// 1. SOCKS5 Auth Handshake
		buf := make([]byte, 3) // VER, NMETHODS, METHODS
		io.ReadFull(conn, buf) 
		conn.Write([]byte{0x05, 0x00}) // VER, METHOD (No Auth)

		// 2. SOCKS5 Connect Request
		// VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT
		header := make([]byte, 4)
		io.ReadFull(conn, header)
		
		// Skip Addr/Port (assuming IPv4 for simplicity in mock)
		io.CopyN(io.Discard, conn, 4+2) 

		// Reply Success
		// VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
		conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

		// 3. Data Loop
		go func() {
			for data := range sendCh {
				conn.Write(data)
			}
		}()

		// Read Loop
		readBuf := make([]byte, 1024)
		for {
			n, err := conn.Read(readBuf)
			if err != nil {
				return
			}
			tmp := make([]byte, n)
			copy(tmp, readBuf[:n])
			recvCh <- tmp
		}
	}()

	return l, sendCh, recvCh
}

func TestClient_WritePacket(t *testing.T) {
	// Setup Mock Server
	l, _, recvCh := startMockServer(t)
	defer l.Close()

	// Override SOCKS Addr
	LocalSocksAddr = l.Addr().String()

	// Create Client
	client := NewClient(nil, "127.0.0.1:7300")
	defer client.cancel()

	// Wait for connection (simple sleep or retry logic in client handles it)
	time.Sleep(100 * time.Millisecond)

	// Test Data
	connID := uint16(42)
	dstIP := netip.MustParseAddr("8.8.8.8")
	dstPort := uint16(53)
	payload := []byte("hello")

	// Write
	err := client.WritePacket(connID, dstIP, dstPort, payload)
	assert.NoError(t, err)

	// Verify received data on server
	select {
	case data := <-recvCh:
		// BadVPN Packet:
		// Length (2) Little Endian
		// Flags (1)
		// ConnID (2) Little Endian
		// IP (4)
		// Port (2) Big Endian
		// Payload
		
		expectedLen := 1 + 2 + 4 + 2 + len(payload) // 14
		assert.GreaterOrEqual(t, len(data), 2)
		
		pktLen := binary.LittleEndian.Uint16(data[0:2])
		assert.Equal(t, uint16(expectedLen), pktLen)
		
		body := data[2:]
		assert.Equal(t, uint8(FlagData), body[0]) // Flags
		assert.Equal(t, connID, binary.LittleEndian.Uint16(body[1:3])) // ID
		assert.Equal(t, dstIP.AsSlice(), body[3:7]) // IP
		assert.Equal(t, dstPort, binary.BigEndian.Uint16(body[7:9])) // Port
		assert.Equal(t, payload, body[9:]) // Data

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for packet")
	}
}

func TestClient_ReadPacket(t *testing.T) {
	l, sendCh, _ := startMockServer(t)
	defer l.Close()
	LocalSocksAddr = l.Addr().String()

	client := NewClient(nil, "127.0.0.1:7300")
	defer client.cancel()

	// Register Flow
	id, ch := client.Register()
	
	// Wait connect
	time.Sleep(100 * time.Millisecond)

	// Construct incoming packet
	// Header
	payload := []byte("response")
	totalLen := 1 + 2 + 4 + 2 + len(payload)
	
	buf := make([]byte, 2 + totalLen)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(totalLen))
	buf[2] = 0x00 // Flags
	binary.LittleEndian.PutUint16(buf[3:5], id) // ID
	copy(buf[5:9], []byte{1, 1, 1, 1}) // IP
	binary.BigEndian.PutUint16(buf[9:11], 80) // Port
	copy(buf[11:], payload)

	// Send from server
	sendCh <- buf

	// Read from client channel
	select {
	case pkt := <-ch:
		assert.Equal(t, "1.1.1.1", pkt.DstIP.String())
		assert.Equal(t, uint16(80), pkt.DstPort)
		assert.Equal(t, payload, pkt.Data)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for packet from channel")
	}
	
	// Test Unregister
	client.Unregister(id)
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "Channel should be closed")
	default:
		// might take a moment
	}
}

func TestClient_IPv6(t *testing.T) {
	l, _, recvCh := startMockServer(t)
	defer l.Close()
	LocalSocksAddr = l.Addr().String()

	client := NewClient(nil, "127.0.0.1:7300")
	defer client.cancel()
	time.Sleep(100 * time.Millisecond)

	connID := uint16(99)
	dstIP := netip.MustParseAddr("2001:db8::1") // IPv6
	dstPort := uint16(443)
	payload := []byte("ipv6test")

	client.WritePacket(connID, dstIP, dstPort, payload)

	select {
	case data := <-recvCh:
		// Verify Flag
		flags := data[2]
		assert.Equal(t, uint8(FlagIPv6|FlagData), flags)
		
		// Verify Length (16 bytes IP)
		// 1 + 2 + 16 + 2 + len = 21 + len
		expectedLen := 21 + len(payload)
		pktLen := binary.LittleEndian.Uint16(data[0:2])
		assert.Equal(t, uint16(expectedLen), pktLen)
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout")
	}
}

func TestClient_ConnectionError(t *testing.T) {
	// 1. Test invalid SOCKS address (Connection Refused)
	LocalSocksAddr = "127.0.0.1:1" // Closed port
	
	client := NewClient(nil, "127.0.0.1:7300")
	defer client.cancel()
	
	// Give it time to try and fail
	time.Sleep(100 * time.Millisecond)
	
	// 2. Test SOCKS Handshake Failure (Server sends bad auth)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer l.Close()
	LocalSocksAddr = l.Addr().String()
	
	go func() {
		conn, err := l.Accept()
		if err == nil {
			conn.Close() // Immediate close = EOF or Bad Handshake
		}
	}()
	
	// Wait for retry
	time.Sleep(100 * time.Millisecond)
}

func TestClient_KeepAlive(t *testing.T) {
	// 1. Setup Mock Server
	l, _, recvCh := startMockServer(t)
	defer l.Close()
	LocalSocksAddr = l.Addr().String()

	// 2. Set very short KeepAlive interval for testing
	oldInterval := KeepAliveInterval
	KeepAliveInterval = 50 * time.Millisecond
	defer func() { KeepAliveInterval = oldInterval }()

	// 3. Create Client
	client := NewClient(nil, "127.0.0.1:7300")
	defer client.cancel()

	// 4. Wait for KeepAlive packet
	select {
	case data := <-recvCh:
		// KeepAlive Packet: Len=3 (2 bytes LittleEndian), Flag=0x01
		assert.GreaterOrEqual(t, len(data), 2)
		pktLen := binary.LittleEndian.Uint16(data[0:2])
		assert.Equal(t, uint16(3), pktLen)
		assert.Equal(t, uint8(FlagKeepAlive), data[2])
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for KeepAlive")
	}
}

func TestClient_RegisterCollision(t *testing.T) {
	client := &Client{
		conns:  make(map[uint16]chan *Packet),
		nextID: 1,
	}
	
	// Pre-fill some IDs
	client.conns[1] = make(chan *Packet)
	client.conns[2] = make(chan *Packet)
	
	// Should pick 3
	id, ch := client.Register()
	assert.Equal(t, uint16(3), id)
	assert.NotNil(t, ch)
	
	// Wrap around test
	client.nextID = 65535
	id2, _ := client.Register()
	assert.Equal(t, uint16(65535), id2)
	id3, _ := client.Register()
	assert.Equal(t, uint16(4), id3) // 1, 2, 3 were taken
}

func TestClient_SOCKS5Errors(t *testing.T) {
	// Test SOCKS5 Connect Failure
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer l.Close()
	LocalSocksAddr = l.Addr().String()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Auth handshake
		buf := make([]byte, 3)
		io.ReadFull(conn, buf)
		conn.Write([]byte{0x05, 0x00})
		
		// Connect request
		header := make([]byte, 4)
		io.ReadFull(conn, header)
		io.CopyN(io.Discard, conn, 4+2)
		
		// Return failure (REP=0x01)
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}()

	client := &Client{serverAddr: "127.0.0.1:7300"}
	err = client.connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SOCKS5 connect failed")
}

func TestClient_Misc(t *testing.T) {
	// 1. WritePacket when not connected
	client := &Client{}
	err := client.WritePacket(1, netip.Addr{}, 80, nil)
	assert.Error(t, err)
	assert.Equal(t, "not connected", err.Error())
	
	// 2. Unregister non-existent
	client.conns = make(map[uint16]chan *Packet)
	client.Unregister(999) // Should not panic
	
	// 3. Invalid server address
	client.serverAddr = "invalid"
	err = client.connect()
	assert.Error(t, err)
}



