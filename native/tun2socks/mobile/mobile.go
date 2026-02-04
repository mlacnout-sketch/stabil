package mobile

import (
	"time"

	"github.com/xjasonlyu/tun2socks/v2/engine"
	_ "github.com/xjasonlyu/tun2socks/v2/dns"
)

// Start starts the tun2socks engine with the given parameters.
// udpTimeout is in milliseconds.
func Start(proxy, device, loglevel string, mtu int, udpTimeout int64, snb, rcb string, autotune bool) error {
	key := &engine.Key{
		Proxy:                    proxy,
		Device:                   device,
		LogLevel:                 loglevel,
		MTU:                      mtu,
		UDPTimeout:               time.Duration(udpTimeout) * time.Millisecond,
		TCPSendBufferSize:        snb,
		TCPReceiveBufferSize:     rcb,
		TCPModerateReceiveBuffer: autotune,
	}
	engine.Insert(key)
	return engine.Run()
}

// Stop stops the tun2socks engine.
func Stop() {
	engine.Stop()
}
