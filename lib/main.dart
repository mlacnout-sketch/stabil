import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.blueAccent, brightness: Brightness.light),
        inputDecorationTheme: InputDecorationTheme(
          border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
          filled: true,
          fillColor: Colors.grey.withValues(alpha: 0.05),
        ),
      ),
      home: const HysteriaConfigPage(),
    );
  }
}

class HysteriaConfigPage extends StatefulWidget {
  const HysteriaConfigPage({super.key});

  @override
  State<HysteriaConfigPage> createState() => _HysteriaConfigPageState();
}

class _HysteriaConfigPageState extends State<HysteriaConfigPage> {
  static const platform = MethodChannel('com.minizivpn.app/core');
  static const logChannel = EventChannel('com.minizivpn.app/logs');

  // Controllers
  final _ipCtrl = TextEditingController();
  final _portRangeCtrl = TextEditingController();
  final _authCtrl = TextEditingController();
  final _obfsCtrl = TextEditingController();
  final _mtuCtrl = TextEditingController();
  
  // State
  double _recvWindowMultiplier = 1.0;
  bool _isRunning = false;
  String _udpMode = "tcp";
  final List<String> _logs = [];
  List<String> _logBuffer = [];
  bool _isLogUpdatePending = false;
  final ScrollController _scrollCtrl = ScrollController();

  @override
  void initState() {
    super.initState();
    _loadConfig();
    _startLogListener();
  }

  Future<void> _loadConfig() async {
    final prefs = await SharedPreferences.getInstance();
    setState(() {
      _ipCtrl.text = prefs.getString('ip') ?? "";
      _portRangeCtrl.text = prefs.getString('port_range') ?? "6000-19999";
      _authCtrl.text = prefs.getString('auth') ?? "";
      _obfsCtrl.text = prefs.getString('obfs') ?? "hu``hqb`c";
      _mtuCtrl.text = prefs.getString('mtu') ?? "1280";
      _recvWindowMultiplier = prefs.getDouble('multiplier') ?? 1.0;
      _udpMode = prefs.getString('udp_mode') ?? "tcp";
      _isRunning = prefs.getBool('vpn_running') ?? false;
    });
  }

  Future<void> _saveConfig() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('ip', _ipCtrl.text);
    await prefs.setString('port_range', _portRangeCtrl.text);
    await prefs.setString('auth', _authCtrl.text);
    await prefs.setString('obfs', _obfsCtrl.text);
    await prefs.setString('mtu', _mtuCtrl.text);
    await prefs.setDouble('multiplier', _recvWindowMultiplier);
    await prefs.setString('udp_mode', _udpMode);
  }

  void _startLogListener() {
    logChannel.receiveBroadcastStream().listen((event) {
      if (event is String) {
        _logBuffer.add(event);
        if (!_isLogUpdatePending) {
          _isLogUpdatePending = true;
          Future.delayed(const Duration(milliseconds: 200), () {
            if (!mounted) return;
            setState(() {
              _logs.addAll(_logBuffer);
              _logBuffer.clear();
              if (_logs.length > 500) {
                _logs.removeRange(0, _logs.length - 500);
              }
              _isLogUpdatePending = false;
            });
            if (_scrollCtrl.hasClients) {
              _scrollCtrl.jumpTo(_scrollCtrl.position.maxScrollExtent);
            }
          });
        }
      }
    });
  }

  @override
  void dispose() {
    _ipCtrl.dispose();
    _portRangeCtrl.dispose();
    _authCtrl.dispose();
    _obfsCtrl.dispose();
    _mtuCtrl.dispose();
    _scrollCtrl.dispose();
    super.dispose();
  }

  Future<void> _toggleEngine() async {
    if (_isRunning) {
      try {
        await platform.invokeMethod('stopCore');
        setState(() {
          _isRunning = false;
          _logs.add("Requesting Stop...");
        });
      } catch (e) {
        setState(() => _logs.add("Error stopping: $e"));
      }
    } else {
      await _saveConfig(); // Auto-save on start

      if (_ipCtrl.text.isEmpty) {
        setState(() => _logs.add("Error: Server IP/Domain is required"));
        return;
      }

      setState(() => _logs.add("Initializing Engine..."));

      try {
        final args = {
          "ip": _ipCtrl.text.trim(),
          "port_range": _portRangeCtrl.text.trim(),
          "pass": _authCtrl.text.trim(),
          "obfs": _obfsCtrl.text.trim(),
          "mtu": int.tryParse(_mtuCtrl.text) ?? 9000,
          "recv_window_multiplier": _recvWindowMultiplier,
          "udp_mode": _udpMode,
        };

        final result = await platform.invokeMethod('startCore', args);
        setState(() => _logs.add("Core Result: $result"));
        
        final vpnResult = await platform.invokeMethod('startVpn');
        setState(() {
          _isRunning = true;
          _logs.add("VPN Result: $vpnResult");
        });
      } on PlatformException catch (e) {
        setState(() {
          _isRunning = false;
          _logs.add("Failed: ${e.message}");
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text("ZIVPN Turbo v1.0"),
        centerTitle: true,
        actions: [
          IconButton(
            icon: const Icon(Icons.copy),
            tooltip: "Copy Logs",
            onPressed: () {
              final text = _logs.join("\n");
              Clipboard.setData(ClipboardData(text: text));
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text("Logs copied to clipboard")),
              );
            },
          ),
          IconButton(
            icon: const Icon(Icons.delete_outline),
            onPressed: () => setState(() => _logs.clear()),
          )
        ],
      ),
      body: Column(
        children: [
          Expanded(
            flex: 4,
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16.0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildStatusCard(),
                  const SizedBox(height: 20),
                  const Text("Core Configuration", style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _ipCtrl,
                    decoration: const InputDecoration(labelText: "Server IP / Domain", hintText: "1.1.1.1 or example.com"),
                  ),
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _portRangeCtrl,
                          decoration: const InputDecoration(labelText: "Port Range", hintText: "Start-End"),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _authCtrl,
                    decoration: const InputDecoration(labelText: "Password / Auth"),
                  ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _obfsCtrl,
                    decoration: const InputDecoration(labelText: "Obfuscation Salt"),
                  ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _mtuCtrl,
                    keyboardType: TextInputType.number,
                    decoration: const InputDecoration(labelText: "MTU (Recommended: 1280-1500)"),
                  ),
                  const SizedBox(height: 10),
                  DropdownButtonFormField<String>(
                    initialValue: _udpMode,
                    decoration: const InputDecoration(labelText: "UDP Relay Mode"),
                    items: const [
                      DropdownMenuItem(value: "tcp", child: Text("UDP over TCP (Default)")),
                      DropdownMenuItem(value: "udp", child: Text("UDP Native")),
                    ],
                    onChanged: (val) => setState(() => _udpMode = val!),
                  ),
                  const SizedBox(height: 20),
                  Text("Receive Window Multiplier: {_recvWindowMultiplier.toStringAsFixed(1)}x"),
                  Slider(
                    value: _recvWindowMultiplier,
                    min: 0.1,
                    max: 4.0,
                    divisions: 39,
                    label: "{_recvWindowMultiplier.toStringAsFixed(1)}x",
                    onChanged: (val) => setState(() => _recvWindowMultiplier = val),
                  ),
                  const SizedBox(height: 10),
                  SizedBox(
                    height: 50,
                    child: ElevatedButton.icon(
                      onPressed: _toggleEngine,
                      icon: Icon(_isRunning ? Icons.stop : Icons.rocket_launch),
                      label: Text(
                        _isRunning ? "STOP TURBO ENGINE" : "START TURBO ENGINE",
                        style: const TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
                      ),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: _isRunning ? Colors.red.shade100 : Colors.blue.shade100,
                        foregroundColor: _isRunning ? Colors.red.shade900 : Colors.blue.shade900,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
          const Divider(height: 1),
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Text("Live Logs", style: TextStyle(fontWeight: FontWeight.bold, color: Colors.grey)),
          ),
          Expanded(
            flex: 3,
            child: Container(
              color: Colors.black,
              width: double.infinity,
              child: ListView.builder(
                controller: _scrollCtrl,
                padding: const EdgeInsets.all(8),
                itemCount: _logs.length,
                itemBuilder: (ctx, i) {
                  final log = _logs[i];
                  final isError = log.contains("ERR") || log.contains("Failed") || log.contains("exited");
                  return Text(
                    log,
                    style: TextStyle(
                      fontFamily: "monospace",
                      fontSize: 11,
                      color: isError ? Colors.redAccent : Colors.greenAccent,
                    ),
                  );
                },
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStatusCard() {
    return Card(
      elevation: 0,
      color: _isRunning ? Colors.green.withValues(alpha: 0.1) : Colors.orange.withValues(alpha: 0.1),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: _isRunning ? Colors.green : Colors.orange, width: 1),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Icon(
              _isRunning ? Icons.check_circle : Icons.pause_circle_filled,
              color: _isRunning ? Colors.green : Colors.orange,
              size: 32,
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Text(
                _isRunning ? "Engine Running" : "Engine Stopped",
                style: const TextStyle(fontFamily: "monospace", fontSize: 13),
              ),
            ),
          ],
        ),
      ),
    );
  }
}