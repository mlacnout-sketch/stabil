package tunnel

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"

	"github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy"
)

// PacketProxyProvider is an interface to get the proxy.
type PacketProxyProvider interface {
	Proxy() proxy.Proxy
}

type BadVPNClient struct {
	addr     string
	provider PacketProxyProvider

	mu    sync.RWMutex
	conn  net.Conn
	flows map[string]chan []byte

	closed chan struct{}
}

func NewBadVPNClient(addr string, p PacketProxyProvider) *BadVPNClient {
	return &BadVPNClient{
		addr:     addr,
		provider: p,
		flows:    make(map[string]chan []byte),
		closed:   make(chan struct{}),
	}
}

func (c *BadVPNClient) Start() {
	go c.loop()
}

func (c *BadVPNClient) loop() {
	for {
		select {
		case <-c.closed:
			return
		default:
		}

		if err := c.connect(); err != nil {
			log.Errorf("[BadVPN] connect to %s failed: %v", c.addr, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if err := c.readLoop(); err != nil {
			log.Errorf("[BadVPN] connection error: %v", err)
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
			}
			c.mu.Unlock()
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *BadVPNClient) connect() error {
	host, portStr, err := net.SplitHostPort(c.addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return err
	}

	dstIP, err := netip.ParseAddr(host)
	if err != nil {
		// host might be a domain, resolve it?
		// proxy.DialContext usually handles domains if passed as metadata?
		// Metadata expects netip.Addr.
		// If it's a domain, we might need DNS resolution or proxy support for domains.
		// For now, let's assume it's an IP or try to resolve.
		// But wait, tun2socks uses gvisor stack for DNS?
		// Let's assume IP for now, or just use 0.0.0.0 and rely on proxy handling?
		// Actually, Metadata DstIP is used for matching rules in proxy.
		// If we use SOCKS5, we can pass domain.
		// But Metadata struct enforces netip.Addr.
		// If host is a domain, netip.ParseAddr fails.
		// We should resolve it.
		// Or we can use a dummy IP if the proxy handles resolution internally?
		// No, let's look at how tun2socks handles domains.
		// Usually it resolves before creating metadata.
		// Here we are inside tun2socks.
		// Let's just try ResolveIPAddr.
		if ips, err := net.LookupIP(host); err == nil && len(ips) > 0 {
			// Convert net.IP to netip.Addr
			if addr, ok := netip.AddrFromSlice(ips[0]); ok {
				dstIP = addr
			} else {
				return err
			}
		} else {
			return err
		}
	}

	metadata := &M.Metadata{
		Network: M.TCP,
		DstIP:   dstIP,
		DstPort: uint16(port),
	}

	conn, err := c.provider.Proxy().DialContext(context.Background(), metadata)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Infof("[BadVPN] connected to %s", c.addr)
	return nil
}

func (c *BadVPNClient) readLoop() error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return io.EOF
	}

	headerBuf := make([]byte, 2)
	for {
		// Read length (2 bytes, Little Endian)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return err
		}
		length := binary.LittleEndian.Uint16(headerBuf)

		// Read packet
		packet := make([]byte, length)
		if _, err := io.ReadFull(conn, packet); err != nil {
			return err
		}

		// Process packet
		c.dispatch(packet)
	}
}

func (c *BadVPNClient) dispatch(packet []byte) {
	// Parse IP header
	var srcIP, dstIP netip.Addr
	var srcPort, dstPort uint16
	var protocol tcpip.TransportProtocolNumber

	// Try IPv4
	if header.IPv4(packet).IsValid(len(packet)) {
		ipv4 := header.IPv4(packet)
		srcAddr := ipv4.SourceAddress()
		srcIP, _ = netip.AddrFromSlice(srcAddr.AsSlice())
		dstAddr := ipv4.DestinationAddress()
		dstIP, _ = netip.AddrFromSlice(dstAddr.AsSlice())
		protocol = tcpip.TransportProtocolNumber(ipv4.Protocol())
		// Get payload offset
		offset := int(ipv4.HeaderLength())
		if len(packet) > offset {
			payload := packet[offset:]
			if protocol == header.UDPProtocolNumber {
				udp := header.UDP(payload)
				srcPort = udp.SourcePort()
				dstPort = udp.DestinationPort()
			}
		}
	} else if header.IPv6(packet).IsValid(len(packet)) {
		ipv6 := header.IPv6(packet)
		srcAddr := ipv6.SourceAddress()
		srcIP, _ = netip.AddrFromSlice(srcAddr.AsSlice())
		dstAddr := ipv6.DestinationAddress()
		dstIP, _ = netip.AddrFromSlice(dstAddr.AsSlice())
		protocol = ipv6.TransportProtocol()
		// TODO: Handle extension headers?
		// Assume standard UDP for now
		offset := header.IPv6MinimumSize
		if len(packet) > offset {
			payload := packet[offset:]
			if protocol == header.UDPProtocolNumber {
				udp := header.UDP(payload)
				srcPort = udp.SourcePort()
				dstPort = udp.DestinationPort()
			}
		}
	} else {
		// Invalid packet
		return
	}

	if protocol != header.UDPProtocolNumber {
		return
	}

	// Key: SrcIP:SrcPort|DstIP:DstPort
	key := c.makeKey(srcIP, srcPort, dstIP, dstPort)

	c.mu.RLock()
	ch, ok := c.flows[key]
	c.mu.RUnlock()

	if ok {
		select {
		case ch <- packet:
		default:
			// Drop if full
		}
	}
}

func (c *BadVPNClient) makeKey(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) string {
	return srcIP.String() + ":" + strconv.Itoa(int(srcPort)) + "|" + dstIP.String() + ":" + strconv.Itoa(int(dstPort))
}

func (c *BadVPNClient) Write(packet []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return io.EOF
	}

	buf := make([]byte, 2+len(packet))
	binary.LittleEndian.PutUint16(buf[:2], uint16(len(packet)))
	copy(buf[2:], packet)

	_, err := conn.Write(buf)
	return err
}

func (c *BadVPNClient) Register(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) <-chan []byte {
	key := c.makeKey(srcIP, srcPort, dstIP, dstPort)
	ch := make(chan []byte, 100)

	c.mu.Lock()
	c.flows[key] = ch
	c.mu.Unlock()

	return ch
}

func (c *BadVPNClient) Unregister(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) {
	key := c.makeKey(srcIP, srcPort, dstIP, dstPort)

	c.mu.Lock()
	delete(c.flows, key)
	c.mu.Unlock()
}
