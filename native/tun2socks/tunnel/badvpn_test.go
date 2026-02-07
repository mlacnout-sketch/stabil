package tunnel

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy"
)

// MockProxy is a mock implementation of proxy.Proxy
type MockProxy struct {
	mock.Mock
}

func (m *MockProxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	args := m.Called(ctx, metadata)
	if conn, ok := args.Get(0).(net.Conn); ok {
		return conn, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockProxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	args := m.Called(metadata)
	if conn, ok := args.Get(0).(net.PacketConn); ok {
		return conn, args.Error(1)
	}
	return nil, args.Error(1)
}

// MockProvider is a mock implementation of PacketProxyProvider
type MockProvider struct {
	proxy proxy.Proxy
}

func (m *MockProvider) Proxy() proxy.Proxy {
	return m.proxy
}

// MockConn is a simple in-memory net.Conn for testing
type MockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
}

func NewMockConn() *MockConn {
	return &MockConn{
		readBuf:  new(bytes.Buffer),
		writeBuf: new(bytes.Buffer),
	}
}

func (c *MockConn) Read(b []byte) (n int, err error) {
	return c.readBuf.Read(b)
}

func (c *MockConn) Write(b []byte) (n int, err error) {
	return c.writeBuf.Write(b)
}

func (c *MockConn) Close() error {
	return nil
}

func (c *MockConn) LocalAddr() net.Addr                { return nil }
func (c *MockConn) RemoteAddr() net.Addr               { return nil }
func (c *MockConn) SetDeadline(t time.Time) error      { return nil }
func (c *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestBadVPNClient_Write(t *testing.T) {
	mockProxy := new(MockProxy)
	provider := &MockProvider{proxy: mockProxy}
	client := NewBadVPNClient("127.0.0.1:7300", provider)

	// Mock connection
	mockConn := NewMockConn()

	// Inject connection manually since connect() is private/blocking
	client.mu.Lock()
	client.conn = mockConn
	client.mu.Unlock()

	payload := []byte("hello world")
	err := client.Write(payload)
	assert.NoError(t, err)

	// Check output framing: Length (2 bytes, LE) + Payload
	output := mockConn.writeBuf.Bytes()
	assert.Equal(t, 2+len(payload), len(output))

	length := binary.LittleEndian.Uint16(output[:2])
	assert.Equal(t, uint16(len(payload)), length)
	assert.Equal(t, payload, output[2:])
}

func TestBadVPNClient_Dispatch(t *testing.T) {
	mockProxy := new(MockProxy)
	provider := &MockProvider{proxy: mockProxy}
	client := NewBadVPNClient("127.0.0.1:7300", provider)

	// 1. Create a dummy IPv4 UDP packet
	srcIP := netip.MustParseAddr("10.0.0.1")
	dstIP := netip.MustParseAddr("1.1.1.1")
	srcPort := uint16(12345)
	dstPort := uint16(53)

	payload := []byte("dns query")
	totalLen := header.IPv4MinimumSize + header.UDPMinimumSize + len(payload)
	packet := make([]byte, totalLen)

	ip := header.IPv4(packet)
	ip.Encode(&header.IPv4Fields{
		TotalLength: uint16(totalLen),
		Protocol:    uint8(header.UDPProtocolNumber),
		TTL:         64,
		SrcAddr:     tcpip.AddrFromSlice(srcIP.AsSlice()),
		DstAddr:     tcpip.AddrFromSlice(dstIP.AsSlice()),
	})

	udp := header.UDP(packet[header.IPv4MinimumSize:])
	udp.Encode(&header.UDPFields{
		SrcPort: srcPort,
		DstPort: dstPort,
		Length:  uint16(header.UDPMinimumSize + len(payload)),
	})
	copy(udp.Payload(), payload)

	// 2. Register a flow listener
	// Note: Dispatch logic matches flow based on packet contents.
	// SrcIP/Port in packet is 10.0.0.1:12345 -> 1.1.1.1:53
	// The client flow key is typically based on the tunnel metadata.
	// In UDP tunnel, we register: Remote -> Local.
	// If the packet comes FROM BadVPN (which is effectively "Remote"), it has Src=Remote, Dst=Local.
	// So we should register with Src=Remote, Dst=Local to match the packet's Src/Dst.

	// Packet Src: 10.0.0.1 (Remote), Dst: 1.1.1.1 (Local - assuming purely for matching)
	// Wait, usually:
	// App sends: Local -> Remote.
	// BadVPN receives: Local -> Remote.
	// BadVPN replies: Remote -> Local.
	// Dispatcher receives: Remote -> Local.

	// So if packet Src is 10.0.0.1 and Dst is 1.1.1.1.
	// The key should match Src=10.0.0.1, Dst=1.1.1.1.
	ch := client.Register(srcIP, srcPort, dstIP, dstPort)

	// 3. Dispatch the packet
	client.dispatch(packet)

	// 4. Verify reception
	select {
	case received := <-ch:
		assert.Equal(t, packet, received)
	case <-time.After(1 * time.Second):
		t.Fatal("Packet not received")
	}
}
