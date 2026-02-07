package tunnel

import (
	"io"
	"net"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"

	"github.com/xjasonlyu/tun2socks/v2/buffer"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
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

	metadata := &M.Metadata{
		Network: M.UDP,
		SrcIP:   parseTCPIPAddress(id.RemoteAddress),
		SrcPort: id.RemotePort,
		DstIP:   parseTCPIPAddress(id.LocalAddress),
		DstPort: id.LocalPort,
	}

	if t.badvpnClient != nil {
		t.handleBadVPN(uc, metadata)
		return
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
	pc = newSymmetricNATPacketConn(pc, metadata)

	log.Infof("[UDP] %s <-> %s", metadata.SourceAddress(), metadata.DestinationAddress())
	pipePacket(uc, pc, remote, t.udpTimeout.Load())
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
		n, _, err := src.ReadFrom(buf)
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil /* ignore I/O timeout */
		} else if err == io.EOF {
			return nil /* ignore EOF */
		} else if err != nil {
			return err
		}

		if _, err = dst.WriteTo(buf[:n], to); err != nil {
			return err
		}
		dst.SetReadDeadline(time.Now().Add(timeout))
	}
}

type symmetricNATPacketConn struct {
	net.PacketConn
	src string
	dst string
}

func newSymmetricNATPacketConn(pc net.PacketConn, metadata *M.Metadata) *symmetricNATPacketConn {
	return &symmetricNATPacketConn{
		PacketConn: pc,
		src:        metadata.SourceAddress(),
		dst:        metadata.DestinationAddress(),
	}
}

func (pc *symmetricNATPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, from, err := pc.PacketConn.ReadFrom(p)

		if from != nil && from.String() != pc.dst {
			log.Warnf("[UDP] symmetric NAT %s->%s: drop packet from %s", pc.src, pc.dst, from)
			continue
		}

		return n, from, err
	}
}

func (t *Tunnel) handleBadVPN(uc adapter.UDPConn, metadata *M.Metadata) {
	// Register flow: Remote -> Local
	inCh := t.badvpnClient.Register(metadata.DstIP, metadata.DstPort, metadata.SrcIP, metadata.SrcPort)
	defer t.badvpnClient.Unregister(metadata.DstIP, metadata.DstPort, metadata.SrcIP, metadata.SrcPort)

	// Close flow signal
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() {
		once.Do(func() {
			close(done)
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// App -> BadVPN (Write)
	go func() {
		defer wg.Done()
		defer closeDone()
		buf := buffer.Get(buffer.MaxSegmentSize)
		defer buffer.Put(buf)

		srcIP := metadata.SrcIP // Local
		dstIP := metadata.DstIP // Remote
		srcPort := metadata.SrcPort
		dstPort := metadata.DstPort

		// Pre-compute addresses for gvisor
		srcAddrBytes := srcIP.AsSlice()
		dstAddrBytes := dstIP.AsSlice()
		srcAddr := tcpip.AddrFromSlice(srcAddrBytes[:])
		dstAddr := tcpip.AddrFromSlice(dstAddrBytes[:])

		for {
			uc.SetReadDeadline(time.Now().Add(t.udpTimeout.Load()))
			n, _, err := uc.ReadFrom(buf)
			if err != nil {
				return
			}

			payload := buf[:n]
			var packet []byte

			if srcIP.Is4() {
				totalLen := header.IPv4MinimumSize + header.UDPMinimumSize + n
				packet = make([]byte, totalLen)

				ip := header.IPv4(packet)
				ip.Encode(&header.IPv4Fields{
					TotalLength: uint16(totalLen),
					Protocol:    uint8(header.UDPProtocolNumber),
					TTL:         64,
					SrcAddr:     srcAddr,
					DstAddr:     dstAddr,
				})
				ip.SetChecksum(^ip.CalculateChecksum())

				udp := header.UDP(packet[header.IPv4MinimumSize:])
				udp.Encode(&header.UDPFields{
					SrcPort: srcPort,
					DstPort: dstPort,
					Length:  uint16(header.UDPMinimumSize + n),
				})
				xsum := header.PseudoHeaderChecksum(header.UDPProtocolNumber, srcAddr, dstAddr, uint16(len(udp)))
				udp.SetChecksum(^udp.CalculateChecksum(xsum))

				copy(udp.Payload(), payload)
			} else {
				totalLen := header.IPv6MinimumSize + header.UDPMinimumSize + n
				packet = make([]byte, totalLen)

				ip := header.IPv6(packet)
				ip.Encode(&header.IPv6Fields{
					PayloadLength:     uint16(header.UDPMinimumSize + n),
					TransportProtocol: header.UDPProtocolNumber,
					HopLimit:          64,
					SrcAddr:           srcAddr,
					DstAddr:           dstAddr,
				})
				// IPv6 doesn't have header checksum

				udp := header.UDP(packet[header.IPv6MinimumSize:])
				udp.Encode(&header.UDPFields{
					SrcPort: srcPort,
					DstPort: dstPort,
					Length:  uint16(header.UDPMinimumSize + n),
				})
				xsum := header.PseudoHeaderChecksum(header.UDPProtocolNumber, srcAddr, dstAddr, uint16(len(udp)))
				udp.SetChecksum(^udp.CalculateChecksum(xsum))

				copy(udp.Payload(), payload)
			}

			if err := t.badvpnClient.Write(packet); err != nil {
				return
			}
		}
	}()

	// BadVPN -> App (Read)
	go func() {
		defer wg.Done()
		defer closeDone()
		remoteAddr := metadata.UDPAddr()

		for {
			select {
			case <-done:
				return
			case packet, ok := <-inCh:
				if !ok {
					return
				}

				var payload []byte
				if header.IPv4(packet).IsValid(len(packet)) {
					ip := header.IPv4(packet)
					payload = packet[ip.HeaderLength():]
				} else if header.IPv6(packet).IsValid(len(packet)) {
					payload = packet[header.IPv6MinimumSize:] // Simplified
				} else {
					continue
				}

				udp := header.UDP(payload)
				if len(payload) < header.UDPMinimumSize {
					continue
				}
				data := udp.Payload()

				uc.WriteTo(data, remoteAddr)
			}
		}
	}()

	wg.Wait()
}
