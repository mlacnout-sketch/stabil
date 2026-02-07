package tunnel

import (
	"context"
	"fmt"
	"time"

	"github.com/miekg/dns"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/dialer"
	"github.com/xjasonlyu/tun2socks/v2/log"
)

// handleDNS handles UDP DNS queries by resolving them via TCP through the proxy.
func (t *Tunnel) handleDNS(uc adapter.UDPConn) {
	defer uc.Close()
	
	// Level Dewa: Safety first. Recover from any potential panic in DNS parsing.
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[DNS] Recovered from panic: %v", r)
		}
	}()

	for {
		// Set deadline based on configured UDP timeout
		uc.SetReadDeadline(time.Now().Add(t.udpTimeout.Load()))

		buf := make([]byte, 2048)
		n, addr, err := uc.ReadFrom(buf)
		if err != nil {
			// Expected when session idles or app closes
			return
		}

		msg := new(dns.Msg)
		if err := msg.Unpack(buf[:n]); err != nil {
			log.Warnf("[DNS] Failed to unpack DNS query: %v", err)
			continue
		}

		if len(msg.Question) == 0 {
			continue
		}

		domain := msg.Question[0].Name
		log.Infof("[DNS] Resolving %s via TCP-Proxy", domain)

		// Resolve via TCP to ensure reliability inside the tunnel
		resp, err := t.resolveDNSTCP(msg)
		if err != nil {
			log.Warnf("[DNS] Error resolving %s: %v", domain, err)
			continue
		}

		out, err := resp.Pack()
		if err != nil {
			continue
		}

		_, _ = uc.WriteTo(out, addr)
	}
}

func (t *Tunnel) resolveDNSTCP(query *dns.Msg) (*dns.Msg, error) {
	// Level Dewa: We MUST use the internal dialer to go THROUGH the proxy.
	// 8.8.8.8:53 is chosen as a reliable global upstream.
	conn, err := dialer.DefaultDialer.DialContext(context.Background(), "tcp", "8.8.8.8:53")
	if err != nil {
		return nil, fmt.Errorf("proxy dial failed: %w", err)
	}
	defer conn.Close()

	// Set short timeout for DNS over TCP
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	dnsConn := &dns.Conn{Conn: conn}
	if err := dnsConn.WriteMsg(query); err != nil {
		return nil, fmt.Errorf("write msg failed: %w", err)
	}

	resp, err := dnsConn.ReadMsg()
	if err != nil {
		return nil, fmt.Errorf("read msg failed: %w", err)
	}

	if resp == nil {
		return nil, fmt.Errorf("empty dns response")
	}

	// Match ID to ensure security
	resp.Id = query.Id
	return resp, nil
}