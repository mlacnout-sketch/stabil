package tunnel

import (
	"net"
	"net/netip"
	"testing"
	"time"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPacketConn is a mock implementation of net.PacketConn
type MockPacketConn struct {
	mock.Mock
}

func (m *MockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	args := m.Called(p)
	var retAddr net.Addr
	if args.Get(1) != nil {
		retAddr = args.Get(1).(net.Addr)
	}
	return args.Int(0), retAddr, args.Error(2)
}

func (m *MockPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	args := m.Called(p, addr)
	return args.Int(0), args.Error(1)
}

func (m *MockPacketConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPacketConn) LocalAddr() net.Addr {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(net.Addr)
}

func (m *MockPacketConn) SetDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockPacketConn) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockPacketConn) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func TestRestrictedNATPacketConn(t *testing.T) {
	mockPC := new(MockPacketConn)
	dstIP := netip.MustParseAddr("1.2.3.4")
	dstPort := uint16(1234)
	metadata := &M.Metadata{
		DstIP:   dstIP,
		DstPort: dstPort,
	}

	// Create restricted NAT wrapper
	natPC := newRestrictedNATPacketConn(mockPC, metadata)

	// Test case 1: Drop packet from different IP
	diffIP := &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 1234}
	validAddr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 1234}

	mockPC.On("ReadFrom", mock.Anything).Return(10, diffIP, nil).Once()
	mockPC.On("ReadFrom", mock.Anything).Return(10, validAddr, nil).Once()

	buf := make([]byte, 100)
	n, addr, err := natPC.ReadFrom(buf)

	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, validAddr.String(), addr.String())
	mockPC.AssertExpectations(t)

	// Test case 2: Allow packet from same IP but different port
	diffPort := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5678}

	// This time, diffPort should be ACCEPTED because it's Restricted Cone NAT (IP check only)
	mockPC.On("ReadFrom", mock.Anything).Return(10, diffPort, nil).Once()

	n, addr, err = natPC.ReadFrom(buf)

	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, diffPort.String(), addr.String())
	mockPC.AssertExpectations(t)
}
