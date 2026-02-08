package tunnel

import (
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/buffer"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
	"github.com/xjasonlyu/tun2socks/v2/badvpn"
)

// TODO: Port Restricted NAT support.
func (t *Tunnel) handleUDPConn(uc adapter.UDPConn) {
	id := uc.ID()

	// DNS HIJACKING: Intercept DNS queries and handle them via TCP resolver
	if id.LocalPort == 53 {
		go t.handleDNS(uc)
		return
	}

	defer uc.Close()

	// BadVPN / UDPGW Path
	t.badvpnMu.RLock()
	client := t.badvpnClient
	t.badvpnMu.RUnlock()

	if client != nil {
		t.handleBadVPNUDP(uc, client)
		return
	}

	metadata := &M.Metadata{
		Network: M.UDP,
		SrcIP:   parseTCPIPAddress(id.RemoteAddress),
		SrcPort: id.RemotePort,
		DstIP:   parseTCPIPAddress(id.LocalAddress),
		DstPort: id.LocalPort,
	}

	pc, err := t.Proxy().DialUDP(metadata)
	if err != nil {
		log.Warnf("[UDP] dial %s: %v", metadata.DestinationAddress(), err)
		return
	}
	metadata.MidIP, metadata.MidPort = parseNetAddr(pc.LocalAddr())

	pc = statistic.NewUDPTracker(pc, metadata, t.manager)
	defer pc.Close()

	var remote net.Addr
	if udpAddr := metadata.UDPAddr(); udpAddr != nil {
		remote = udpAddr
	} else {
		remote = metadata.Addr()
	}
	
	// Use Restricted NAT (Address Restricted Cone) to allow packets from the same IP with different ports.
	pc = newRestrictedNATPacketConn(pc, metadata)

	log.Infof("[UDP] %s <-> %s", metadata.SourceAddress(), metadata.DestinationAddress())
	pipePacket(uc, pc, remote, t.udpTimeout.Load())
}

func (t *Tunnel) handleBadVPNUDP(uc adapter.UDPConn, client *badvpn.Client) {
	id := uc.ID()
	dstIP := parseTCPIPAddress(id.LocalAddress)
	dstPort := id.LocalPort

	connID, ch := client.Register()
	defer client.Unregister(connID)

	// log.Infof("[UDPGW] New Flow %d -> %s:%d", connID, dstIP, dstPort)

	// Goroutine to read from UDPGW and write to TUN
	go func() {
		for packet := range ch {
			fromAddr := &net.UDPAddr{
				IP:   packet.DstIP.AsSlice(),
				Port: int(packet.DstPort),
			}
			uc.WriteTo(packet.Data, fromAddr)
		}
	}()

	// Main loop: Read from TUN and write to UDPGW
	buf := buffer.Get(buffer.MaxSegmentSize)
	defer buffer.Put(buf)

	timeout := t.udpTimeout.Load()

	for {
		uc.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := uc.ReadFrom(buf)
		if err != nil {
			break
		}

		err = client.WritePacket(connID, dstIP, dstPort, buf[:n])
		if err != nil {
			// log.Warnf("[UDPGW] Write failed: %v", err)
			break
		}
	}
}

func pipePacket(origin, remote net.PacketConn, to net.Addr, timeout time.Duration) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go unidirectionalPacketStream(remote, origin, to, "origin->remote", &wg, timeout)
	go unidirectionalPacketStream(origin, remote, nil, "remote->origin", &wg, timeout)

	wg.Wait()
}

func unidirectionalPacketStream(dst, src net.PacketConn, to net.Addr, dir string, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	if err := copyPacketData(dst, src, to, timeout); err != nil {
		log.Debugf("[UDP] copy data for %s: %v", dir, err)
	}
}

func copyPacketData(dst, src net.PacketConn, to net.Addr, timeout time.Duration) error {
	buf := buffer.Get(buffer.MaxSegmentSize)
	defer buffer.Put(buf)

	for {
		src.SetReadDeadline(time.Now().Add(timeout))
		n, addr, err := src.ReadFrom(buf)
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil /* ignore I/O timeout */
		} else if err == io.EOF {
			return nil /* ignore EOF */
		} else if err != nil {
			return err
		}

		target := to
		if target == nil {
			target = addr
		}

		if _, err = dst.WriteTo(buf[:n], target); err != nil {
			return err
		}
		dst.SetReadDeadline(time.Now().Add(timeout))
	}
}

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
			var fromAddr netip.Addr

			if udpAddr, ok := from.(*net.UDPAddr); ok {
				// Fast path: avoid string allocation
				if addrPort := udpAddr.AddrPort(); addrPort.IsValid() {
					fromAddr = addrPort.Addr()
				}
			} else {
				// Fallback path
				if ap, err := netip.ParseAddrPort(from.String()); err == nil {
					fromAddr = ap.Addr()
				}
			}

			if fromAddr.IsValid() && fromAddr.Unmap() != pc.dstIP.Unmap() {
				// Log dropped packet (rate limiting recommended in production)
				// log.Warnf("[UDP] restricted NAT %s->%s: drop packet from %s", pc.src, pc.dstIP, from)
				continue
			}
		}

		return n, from, err
	}
}