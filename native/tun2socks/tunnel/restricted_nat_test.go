package tunnel

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

// Smart Mock PacketConn
type smartMockPacketConn struct {
	net.PacketConn
	data []byte
	addr net.Addr
	calls int
}

func (m *smartMockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	m.calls++
	if m.calls > 1 {
		return 0, nil, net.ErrClosed
	}
	copy(p, m.data)
	return len(m.data), m.addr, nil
}

func TestRestrictedNATPacketConn_ReadFrom(t *testing.T) {
	// Destination (Expected)
	dstIP := netip.MustParseAddr("8.8.8.8")
	dstPort := uint16(53)
	
	meta := &M.Metadata{
		DstIP:   dstIP,
		DstPort: dstPort,
	}

	tests := []struct {
		name        string
		srcAddr     net.Addr
		shouldDrop  bool
	}{
		{
			name: "Same IP, Same Port (Symmetric Match)",
			// Use 4-byte IP explicitly to match netip.MustParseAddr("8.8.8.8") which is IPv4
			srcAddr: &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 53},
			shouldDrop: false,
		},
		{
			name: "Same IP, Diff Port (Restricted Cone Match)",
			srcAddr: &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 9999},
			shouldDrop: false,
		},
		{
			name: "Diff IP (Should Drop)",
			srcAddr: &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 53},
			shouldDrop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smartMock := &smartMockPacketConn{
				data: []byte("test"),
				addr: tt.srcAddr,
			}
			
			rn := newRestrictedNATPacketConn(smartMock, meta)
			
			buf := make([]byte, 1024)
			n, addr, err := rn.ReadFrom(buf)
			
			if tt.shouldDrop {
				// If dropped, it calls ReadFrom again -> ErrClosed
				assert.Error(t, err)
			} else {
				// If accepted, it returns immediately
				assert.NoError(t, err)
				assert.Equal(t, 4, n)
				assert.Equal(t, tt.srcAddr, addr)
			}
		})
	}
}