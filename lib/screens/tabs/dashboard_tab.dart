import 'package:flutter/foundation.dart'; // Import for ValueListenable
import 'package:flutter/material.dart';
import '../../widgets/ping_button.dart';

class DashboardTab extends StatefulWidget {
  final String vpnState; // "disconnected", "connecting", "connected"
  final VoidCallback onToggle;
  final ValueListenable<String> dl;
  final ValueListenable<String> ul;
  final String duration;
  final ValueListenable<int> sessionRx;
  final ValueListenable<int> sessionTx;

  const DashboardTab({
    super.key,
    required this.vpnState,
    required this.onToggle,
    required this.dl,
    required this.ul,
    required this.duration,
    required this.sessionRx,
    required this.sessionTx,
  });

  @override
  State<DashboardTab> createState() => _DashboardTabState();
}

class _DashboardTabState extends State<DashboardTab> {
  @override
  Widget build(BuildContext context) {
    bool isConnected = widget.vpnState == "connected";
    bool isConnecting = widget.vpnState == "connecting";
    Color statusColor = isConnected ? const Color(0xFF6C63FF) : (isConnecting ? Colors.orange : const Color(0xFF272736));

    return Padding(
      padding: const EdgeInsets.all(20.0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            "ZIVPN",
            style: TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w900,
              letterSpacing: 1.5,
            ),
          ),
          const Text("Turbo Tunnel Engine", style: TextStyle(color: Colors.grey)),
          const SizedBox(height: 20),
          Expanded(
            child: Stack(
              alignment: Alignment.center,
              children: [
                Center(
                  child: GestureDetector(
                    onTap: isConnecting ? null : widget.onToggle,
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 500),
                      width: 220,
                      height: 240,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: statusColor,
                        boxShadow: [
                          BoxShadow(
                            color: (isConnected ? const Color(0xFF6C63FF) : Colors.black)
                                .withValues(alpha: 0.4),
                            blurRadius: 30,
                            spreadRadius: 10,
                          )
                        ],
                      ),
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          if (isConnecting)
                            const SizedBox(
                              width: 64, 
                              height: 64, 
                              child: CircularProgressIndicator(color: Colors.white, strokeWidth: 3)
                            )
                          else
                            Icon(
                              isConnected ? Icons.vpn_lock : Icons.power_settings_new,
                              size: 64,
                              color: Colors.white,
                            ),
                          const SizedBox(height: 15),
                          Text(
                            isConnecting ? "CONNECTING..." : (isConnected ? "CONNECTED" : "TAP TO CONNECT"),
                            style: const TextStyle(
                              fontWeight: FontWeight.bold,
                              fontSize: 16,
                            ),
                          ),
                          if (isConnected) ...[
                            const SizedBox(height: 15),
                            Container(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 16,
                                vertical: 6,
                              ),
                              decoration: BoxDecoration(
                                color: Colors.black26,
                                borderRadius: BorderRadius.circular(20),
                                border: Border.all(color: Colors.white12),
                              ),
                              child: Text(
                                widget.duration,
                                style: const TextStyle(
                                  fontFamily: 'monospace',
                                  fontWeight: FontWeight.bold,
                                  fontSize: 14,
                                  color: Colors.white,
                                ),
                              ),
                            )
                          ]
                        ],
                      ),
                    ),
                  ),
                ),
                if (isConnected)
                  const Positioned(
                    bottom: 20,
                    right: 20,
                    child: PingButton(),
                  ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.all(12),
            margin: const EdgeInsets.only(bottom: 15),
            decoration: BoxDecoration(
              color: const Color(0xFF272736),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: Colors.white.withValues(alpha: 0.05)),
            ),
            child: ValueListenableBuilder<int>(
              valueListenable: widget.sessionRx,
              builder: (context, rx, _) {
                return ValueListenableBuilder<int>(
                  valueListenable: widget.sessionTx,
                  builder: (context, tx, _) {
                    return Row(
                      mainAxisAlignment: MainAxisAlignment.spaceAround,
                      children: [
                        Text(
                          "Session: ${_formatTotalBytes(rx + tx)}",
                          style: const TextStyle(color: Colors.white70, fontSize: 12),
                        ),
                        Container(width: 1, height: 12, color: Colors.white10),
                        Text(
                          "Rx: ${_formatTotalBytes(rx)}",
                          style: const TextStyle(color: Colors.greenAccent, fontSize: 12),
                        ),
                        Container(width: 1, height: 12, color: Colors.white10),
                        Text(
                          "Tx: ${_formatTotalBytes(tx)}",
                          style: const TextStyle(color: Colors.orangeAccent, fontSize: 12),
                        ),
                      ],
                    );
                  }
                );
              }
            ),
          ),
          Row(
            children: [
              Expanded(
                child: ValueListenableBuilder<String>(
                  valueListenable: widget.dl,
                  builder: (context, val, _) => StatCard(
                    label: "Download",
                    value: val,
                    icon: Icons.download,
                    color: Colors.green,
                  ),
                ),
              ),
              const SizedBox(width: 15),
              Expanded(
                child: ValueListenableBuilder<String>(
                  valueListenable: widget.ul,
                  builder: (context, val, _) => StatCard(
                    label: "Upload",
                    value: val,
                    icon: Icons.upload,
                    color: Colors.orange,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 20),
        ],
      ),
    );
  }

  String _formatTotalBytes(int bytes) {
    if (bytes < 1024) return "$bytes B";
    if (bytes < 1024 * 1024) return "${(bytes / 1024).toStringAsFixed(1)} KB";
    if (bytes < 1024 * 1024 * 1024) {
      return "${(bytes / (1024 * 1024)).toStringAsFixed(2)} MB";
    }
    return "${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB";
  }
}

class StatCard extends StatelessWidget {
  final String label, value;
  final IconData icon;
  final Color color;

  const StatCard({
    super.key,
    required this.label,
    required this.value,
    required this.icon,
    required this.color,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF272736),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: color.withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(icon, color: color),
          ),
          const SizedBox(width: 15),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                value,
                style: const TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
              ),
              Text(label, style: const TextStyle(fontSize: 12, color: Colors.grey)),
            ],
          )
        ],
      ),
    );
  }
}
