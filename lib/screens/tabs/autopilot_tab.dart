import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../app_colors.dart';
import '../../services/autopilot_service.dart';
import '../../models/autopilot_state.dart';
import '../../models/autopilot_config.dart';

class AutoPilotTab extends StatefulWidget {
  const AutoPilotTab({super.key});

  @override
  State<AutoPilotTab> createState() => _AutoPilotTabState();
}

class _AutoPilotTabState extends State<AutoPilotTab> {
  final _service = AutoPilotService();
  bool _isStarting = false;

  @override
  Widget build(BuildContext context) {
    return StreamBuilder<AutoPilotState>(
      stream: _service.stateStream,
      initialData: _service.currentState,
      builder: (context, snapshot) {
        final state = snapshot.data ?? _service.currentState;
        final isRunning = state.status != AutoPilotStatus.stopped;

        return SingleChildScrollView(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text(
                "lexpesawat",
                style: TextStyle(fontSize: 24, fontWeight: FontWeight.bold),
              ),
              const SizedBox(height: 20),
              _buildStatusCard(state),
              const SizedBox(height: 16),
              _buildControlCard(isRunning),
              const SizedBox(height: 16),
              _buildConfigurationSection(),
              const SizedBox(height: 16),
              _buildConnectionCard(state),
            ],
          ),
        );
      },
    );
  }

  Widget _buildStatusCard(AutoPilotState state) {
    Color color;
    IconData icon;
    String label;

    switch (state.status) {
      case AutoPilotStatus.stopped: color = Colors.grey; icon = Icons.stop_circle; label = "STOPPED"; break;
      case AutoPilotStatus.monitoring: color = Colors.green; icon = Icons.radar; label = "MONITORING"; break;
      case AutoPilotStatus.checking: color = Colors.blue; icon = Icons.sync; label = "CHECKING..."; break;
      case AutoPilotStatus.recovering: color = Colors.orange; icon = Icons.airplane_ticket; label = "RESETTING NET"; break;
      case AutoPilotStatus.stabilizing: color = Colors.purple; icon = Icons.bolt; label = "STABILIZING"; break;
      case AutoPilotStatus.error: color = Colors.red; icon = Icons.error; label = "ERROR"; break;
    }

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(icon, size: 64, color: color),
            const SizedBox(height: 12),
            Text(label, style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold, color: color)),
            if (state.message != null) ...[
              const SizedBox(height: 8),
              Text(state.message!, textAlign: TextAlign.center, style: const TextStyle(color: Colors.white70, fontSize: 13)),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildControlCard(bool isRunning) {
    return Column(
      children: [
        SizedBox(
          width: double.infinity,
          child: ElevatedButton.icon(
            onPressed: _isStarting ? null : (isRunning ? _stop : _start),
            icon: Icon(isRunning ? Icons.stop : Icons.play_arrow),
            label: Text(isRunning ? "STOP WATCHDOG" : "START WATCHDOG"),
            style: ElevatedButton.styleFrom(
              backgroundColor: isRunning ? Colors.redAccent : AppColors.primary,
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(vertical: 16),
            ),
          ),
        ),
        const SizedBox(height: 12),
        SizedBox(
          width: double.infinity,
          child: OutlinedButton.icon(
            onPressed: _testConnection,
            icon: const Icon(Icons.network_check),
            label: const Text("TEST CONNECTION NOW"),
            style: OutlinedButton.styleFrom(padding: const EdgeInsets.symmetric(vertical: 16)),
          ),
        ),
      ],
    );
  }

  Widget _buildConfigurationSection() {
    final cfg = _service.config;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text("Configuration", style: TextStyle(fontWeight: FontWeight.bold, fontSize: 16)),
                IconButton(icon: const Icon(Icons.settings, size: 20), onPressed: _showSettingsDialog),
              ],
            ),
            const Divider(),
            _cfgRow("Interval", "${cfg.checkIntervalSeconds}s", Icons.timer),
            _cfgRow("Max Fail", "${cfg.maxFailCount}x", Icons.replay),
            _cfgRow("Reset Delay", "${cfg.airplaneModeDelaySeconds}s", Icons.airplanemode_active),
            _cfgRow("Recovery", "${cfg.recoveryWaitSeconds}s", Icons.hourglass_empty),
            _cfgRow("Stabilizer", cfg.enableStabilizer ? "ON (${cfg.stabilizerSizeMb}MB)" : "OFF", Icons.bolt),
          ],
        ),
      ),
    );
  }

  Widget _cfgRow(String label, String val, IconData icon) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          Icon(icon, size: 16, color: Colors.grey),
          const SizedBox(width: 12),
          Expanded(child: Text(label, style: const TextStyle(color: Colors.white70))),
          Text(val, style: const TextStyle(fontWeight: FontWeight.bold)),
        ],
      ),
    );
  }

  Widget _buildConnectionCard(AutoPilotState state) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: state.hasInternet ? Colors.green.withValues(alpha: 0.1) : Colors.red.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: state.hasInternet ? Colors.green.withValues(alpha: 0.3) : Colors.red.withValues(alpha: 0.3)),
      ),
      child: Row(
        children: [
          Icon(state.hasInternet ? Icons.wifi : Icons.wifi_off, color: state.hasInternet ? Colors.green : Colors.red),
          const SizedBox(width: 16),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(state.hasInternet ? "CONNECTED" : "OFFLINE", style: const TextStyle(fontWeight: FontWeight.bold)),
              if (state.failCount > 0) Text("Fails: ${state.failCount}/${_service.config.maxFailCount}", style: const TextStyle(fontSize: 12, color: Colors.redAccent)),
            ],
          )
        ],
      ),
    );
  }

  void _start() async {
    setState(() => _isStarting = true);
    try {
      await _service.start();
    } catch (e) {
      if (e.toString().contains("Shizuku")) _showShizukuTutorial();
      else ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text("Error: $e")));
    } finally {
      setState(() => _isStarting = false);
    }
  }

  void _stop() => _service.stop();

  void _testConnection() async {
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text("Testing connection..."), duration: Duration(seconds: 1)));
    final ok = await _service.checkInternet();
    showDialog(
      context: context,
      builder: (c) => AlertDialog(
        title: Text(ok ? "Online" : "Offline"),
        content: Text(ok ? "Connection is working properly." : "Unable to reach target server."),
        actions: [TextButton(onPressed: () => Navigator.pop(c), child: const Text("OK"))],
      ),
    );
  }

  void _showShizukuTutorial() {
    showDialog(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text("Shizuku Error"),
        content: const Text("Shizuku is not running or permission denied.

Please start Shizuku and grant permission."),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c), child: const Text("CANCEL")),
          ElevatedButton(onPressed: () => launchUrl(Uri.parse("https://shizuku.rikka.app/download/")), child: const Text("GET SHIZUKU")),
        ],
      ),
    );
  }

  void _showSettingsDialog() {
    final cfg = _service.config;
    final intvl = TextEditingController(text: cfg.checkIntervalSeconds.toString());
    final timeout = TextEditingController(text: cfg.connectionTimeoutSeconds.toString());
    final maxFail = TextEditingController(text: cfg.maxFailCount.toString());
    final delay = TextEditingController(text: cfg.airplaneModeDelaySeconds.toString());
    final wait = TextEditingController(text: cfg.recoveryWaitSeconds.toString());
    final stabSize = TextEditingController(text: cfg.stabilizerSizeMb.toString());
    bool stab = cfg.enableStabilizer;

    showDialog(
      context: context,
      builder: (c) => StatefulBuilder(
        builder: (c, setDialogState) => AlertDialog(
          title: const Text("AutoPilot Settings"),
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _tf(intvl, "Check Interval (s)"),
                _tf(timeout, "Ping Timeout (s)"),
                _tf(maxFail, "Max Fail Count"),
                _tf(delay, "Airplane Duration (s)"),
                _tf(wait, "Recovery Wait (s)"),
                SwitchListTile(
                  title: const Text("Stabilizer"),
                  value: stab,
                  onChanged: (v) => setDialogState(() => stab = v),
                ),
                if (stab) _tf(stabSize, "Stabilizer Size (MB)"),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(c), child: const Text("CANCEL")),
            ElevatedButton(
              onPressed: () {
                _service.updateConfig(AutoPilotConfig(
                  checkIntervalSeconds: int.tryParse(intvl.text) ?? 15,
                  connectionTimeoutSeconds: int.tryParse(timeout.text) ?? 5,
                  maxFailCount: int.tryParse(maxFail.text) ?? 3,
                  airplaneModeDelaySeconds: int.tryParse(delay.text) ?? 2,
                  recoveryWaitSeconds: int.tryParse(wait.text) ?? 10,
                  enableStabilizer: stab,
                  stabilizerSizeMb: int.tryParse(stabSize.text) ?? 1,
                ));
                Navigator.pop(c);
                setState(() {});
              },
              child: const Text("SAVE"),
            )
          ],
        ),
      ),
    );
  }

  Widget _tf(TextEditingController ctrl, String label) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: TextField(
        controller: ctrl,
        keyboardType: TextInputType.number,
        decoration: InputDecoration(labelText: label, border: const OutlineInputBorder()),
      ),
    );
  }
}
