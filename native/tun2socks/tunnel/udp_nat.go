package tunnel

import (
	"net"
	"net/netip"

	"github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

type restrictedNATPacketConn struct {
	net.PacketConn
	src   string
	dstIP netip.Addr
}

func newRestrictedNATPacketConn(pc net.PacketConn, metadata *M.Metadata) *restrictedNATPacketConn {
	return &restrictedNATPacketConn{
		PacketConn: pc,
		src:        metadata.SourceAddress(),
		dstIP:      metadata.DstIP,
	}
}

func (pc *restrictedNATPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, from, err := pc.PacketConn.ReadFrom(p)

		if from != nil {
			if addrPort, err := netip.ParseAddrPort(from.String()); err == nil {
				if addrPort.Addr() != pc.dstIP {
					log.Warnf("[UDP] restricted NAT %s->%s: drop packet from %s", pc.src, pc.dstIP, from)
					continue
				}
			} else {
				log.Warnf("[UDP] restricted NAT %s->%s: drop unparsable packet from %s", pc.src, pc.dstIP, from)
				continue
			}
		}

		return n, from, err
	}
}
