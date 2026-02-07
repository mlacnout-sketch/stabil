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
	// List of reliable upstream DNS servers (TCP)
	// We iterate through them until one succeeds.
	resolvers := []string{
		"1.1.1.1:53", // Cloudflare
		"8.8.8.8:53", // Google
		"9.9.9.9:53", // Quad9
		"112.215.198.248:53", // ISP Specific (XL/Tsel)
	}

	var lastErr error
	for _, resolver := range resolvers {
		conn, err := dialer.DefaultDialer.DialContext(context.Background(), "tcp", resolver)
		if err != nil {
			lastErr = err
			log.Warnf("[DNS] Failed to dial %s: %v", resolver, err)
			continue
		}
		
		// Set short timeout per attempt
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

		dnsConn := &dns.Conn{Conn: conn}
		if err := dnsConn.WriteMsg(query); err != nil {
			dnsConn.Close()
			lastErr = err
			continue
		}

		resp, err := dnsConn.ReadMsg()
		dnsConn.Close() // Close immediately after read
		
		if err != nil {
			lastErr = err
			continue
		}

		if resp == nil {
			continue
		}

		// Success! Match ID and return.
		resp.Id = query.Id
		return resp, nil
	}

	return nil, fmt.Errorf("all resolvers failed, last error: %v", lastErr)
}