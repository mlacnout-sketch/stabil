#!/bin/bash

# Configuration
TUN_NAME="tun0"
TUN_IP="198.18.0.1/15"
PROXY_ADDR="socks5://127.0.0.1:7777"
# Detect primary interface (eth0, wlan0, etc)
PRIMARY_IF=$(ip route | grep default | awk '{print $5}' | head -n1)
GATEWAY_IP=$(ip route | grep default | awk '{print $3}' | head -n1)

echo "--- Initializing tun2socks for Linux ---"
echo "Primary Interface: $PRIMARY_IF"
echo "Gateway: $GATEWAY_IP"

# 1. Create TUN Interface
sudo ip tuntap add mode tun dev $TUN_NAME
sudo ip addr add $TUN_IP dev $TUN_NAME
sudo ip link set dev $TUN_NAME up

# 2. Setup Routing (The "Smart" way)
# We add a specific route to our proxy server via the primary gateway first to avoid loop
# (Assuming proxy server is remote, if local 127.0.0.1, we skip this)

# 3. Redirect Default Traffic
# We use metrics to prioritize TUN interface
sudo ip route add default via 198.18.0.1 dev $TUN_NAME metric 1
# Keep original default with higher metric as fallback
sudo ip route add default via $GATEWAY_IP dev $PRIMARY_IF metric 10

# 4. Disable Reverse Path Filtering (Crucial for transparent proxy)
sudo sysctl -w net.ipv4.conf.all.rp_filter=0
sudo sysctl -w net.ipv4.conf.$TUN_NAME.rp_filter=0
sudo sysctl -w net.ipv4.conf.$PRIMARY_IF.rp_filter=0

# 5. Run tun2socks
# Build it first if you haven't: go build -o tun2socks
./tun2socks -device $TUN_NAME -proxy $PROXY_ADDR -interface $PRIMARY_IF
