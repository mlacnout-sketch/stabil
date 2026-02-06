import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

class SettingsTab extends StatefulWidget {
  const SettingsTab({super.key});

  @override
  State<SettingsTab> createState() => _SettingsTabState();
}

class _SettingsTabState extends State<SettingsTab> {
  final _mtuCtrl = TextEditingController();
  
  bool _autoTuning = true;
  String _bufferSize = "4m";
  String _logLevel = "info";
  double _coreCount = 4.0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    setState(() {
      _mtuCtrl.text = prefs.getString('mtu') ?? "1200";
      _autoTuning = prefs.getBool('auto_tuning') ?? true;
      _bufferSize = prefs.getString('buffer_size') ?? "4m";
      _logLevel = prefs.getString('log_level') ?? "info";
      _coreCount = (prefs.getInt('core_count') ?? 4).toDouble();
    });
  }

  Future<void> _save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('mtu', _mtuCtrl.text);
    await prefs.setBool('auto_tuning', _autoTuning);
    await prefs.setString('buffer_size', _bufferSize);
    await prefs.setString('log_level', _logLevel);
    await prefs.setInt('core_count', _coreCount.toInt());
    if(mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text("Settings Saved")));
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        const Text("Core Settings", style: TextStyle(fontSize: 24, fontWeight: FontWeight.bold)),
        const SizedBox(height: 20),
        
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16.0),
            child: Column(
              children: [
                _buildInput(_mtuCtrl, "MTU (Default: 1500)", Icons.settings_ethernet),
                const SizedBox(height: 15),
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16),
                      child: Text("Hysteria Cores: ${_coreCount.toInt()}", style: const TextStyle(fontWeight: FontWeight.bold)),
                    ),
                    Slider(
                      value: _coreCount,
                      min: 1,
                      max: 8,
                      divisions: 7,
                      label: "${_coreCount.toInt()} Cores",
                      onChanged: (val) => setState(() => _coreCount = val),
                    ),
                    const Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16),
                      child: Text("More cores = Higher speed but more battery usage", style: TextStyle(fontSize: 12, color: Colors.grey)),
                    ),
                  ],
                ),
                const Divider(),
                SwitchListTile(
                  title: const Text("TCP Auto Tuning"),
                  subtitle: const Text("Dynamic buffer sizing for stability"),
                  value: _autoTuning,
                  onChanged: (val) => setState(() => _autoTuning = val),
                ),
                const Divider(),
                ListTile(
                  title: const Text("TCP Buffer Size"),
                  subtitle: const Text("Max window size per connection"),
                  trailing: DropdownButton<String>(
                    value: _bufferSize,
                    items: const [
                      DropdownMenuItem(value: "1m", child: Text("1 MB")),
                      DropdownMenuItem(value: "2m", child: Text("2 MB")),
                      DropdownMenuItem(value: "4m", child: Text("4 MB")),
                      DropdownMenuItem(value: "8m", child: Text("8 MB")),
                    ],
                    onChanged: (val) => setState(() => _bufferSize = val!),
                  ),
                ),
                ListTile(
                  title: const Text("Log Level"),
                  subtitle: const Text("Verbosity of logs"),
                  trailing: DropdownButton<String>(
                    value: _logLevel,
                    items: const [
                      DropdownMenuItem(value: "debug", child: Text("Debug (Verbose)")),
                      DropdownMenuItem(value: "info", child: Text("Info (Standard)")),
                      DropdownMenuItem(value: "error", child: Text("Error (Minimal)")),
                      DropdownMenuItem(value: "silent", child: Text("Silent (None)")),
                    ],
                    onChanged: (val) => setState(() => _logLevel = val!),
                  ),
                )
              ],
            ),
          ),
        ),

        const SizedBox(height: 30),
        ElevatedButton.icon(
          onPressed: _save,
          icon: const Icon(Icons.save),
          label: const Text("Save Configuration"),
          style: ElevatedButton.styleFrom(
            backgroundColor: const Color(0xFF6C63FF),
            foregroundColor: Colors.white,
            padding: const EdgeInsets.symmetric(vertical: 16),
          ),
        )
      ],
    );
  }

  Widget _buildInput(TextEditingController ctrl, String label, IconData icon) {
    return TextField(
      controller: ctrl,
      decoration: InputDecoration(
        labelText: label,
        prefixIcon: Icon(icon),
        border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
        filled: true,
        fillColor: const Color(0xFF272736),
      ),
    );
  }
}
