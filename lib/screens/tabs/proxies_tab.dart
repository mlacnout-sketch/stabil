import 'package:flutter/material.dart';
import 'dart:io';

class ProxiesTab extends StatefulWidget {
  final List<Map<String, dynamic>> accounts;
  final int activePingIndex;
  final Function(int) onActivate;
  final Function(Map<String, dynamic>) onAdd;
  final Function(int) onDelete;

  const ProxiesTab({
    super.key,
    required this.accounts,
    required this.activePingIndex,
    required this.onActivate,
    required this.onAdd,
    required this.onDelete,
  });

  @override
  State<ProxiesTab> createState() => _ProxiesTabState();
}

class _ProxiesTabState extends State<ProxiesTab> with TickerProviderStateMixin {
  // Ping State
  final Map<int, String> _pingResults = {};
  final Map<int, bool> _isPinging = {};
  final Map<int, AnimationController> _animControllers = {};

  String _formatTotalBytes(int bytes) {
    if (bytes < 1024) return "$bytes B";
    if (bytes < 1024 * 1024) return "${(bytes / 1024).toStringAsFixed(1)} KB";
    if (bytes < 1024 * 1024 * 1024) return "${(bytes / (1024 * 1024)).toStringAsFixed(2)} MB";
    return "${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB";
  }

  void _showAddDialog(BuildContext context) {
    final nameCtrl = TextEditingController();
    final ipCtrl = TextEditingController();
    final authCtrl = TextEditingController();

    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF272736),
        title: const Text("Add Account"),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: nameCtrl, decoration: const InputDecoration(labelText: "Name (e.g. SG-1)")),
              TextField(controller: ipCtrl, decoration: const InputDecoration(labelText: "IP/Domain:Port")),
              TextField(controller: authCtrl, decoration: const InputDecoration(labelText: "Password")),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text("Cancel")),
          ElevatedButton(
            style: ElevatedButton.styleFrom(backgroundColor: const Color(0xFF6C63FF), foregroundColor: Colors.white),
            onPressed: () {
              if (nameCtrl.text.isNotEmpty && ipCtrl.text.isNotEmpty) {
                widget.onAdd({
                  "name": nameCtrl.text,
                  "ip": ipCtrl.text,
                  "auth": authCtrl.text,
                  "obfs": "hu``hqb`c",
                  "usage": 0,
                });
                Navigator.pop(ctx);
              }
            },
            child: const Text("Save"),
          ),
        ],
      ),
    );
  }

  void _showPingDialog(BuildContext context, int index) {
    final targetCtrl = TextEditingController(text: "connectivitycheck.gstatic.com");
    
    final List<String> suggestions = [
      "http://google.com/generate_204",
      "http://cp.cloudflare.com/generate_204",
      "http://connect.rom.miui.com/generate_204",
      "https://www.gstatic.com/generate_204",
      "connectivitycheck.gstatic.com",
      "1.1.1.1",
      "8.8.8.8"
  ];

    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF272736),
        title: const Text("Ping Destination"),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Autocomplete<String>(
              initialValue: TextEditingValue(text: targetCtrl.text),
              optionsBuilder: (TextEditingValue textEditingValue) {
                if (textEditingValue.text == '') {
                  return const Iterable<String>.empty();
                }
                return suggestions.where((String option) {
                  return option.contains(textEditingValue.text.toLowerCase());
                });
              },
              onSelected: (String selection) {
                targetCtrl.text = selection;
              },
              fieldViewBuilder: (BuildContext context, TextEditingController fieldTextEditingController, FocusNode fieldFocusNode, VoidCallback onFieldSubmitted) {
                // Sync controllers
                fieldTextEditingController.addListener(() {
                  targetCtrl.text = fieldTextEditingController.text;
                });
                if (fieldTextEditingController.text.isEmpty && targetCtrl.text.isNotEmpty) {
                   fieldTextEditingController.text = targetCtrl.text;
                }
                
                return TextField(
                  controller: fieldTextEditingController,
                  focusNode: fieldFocusNode,
                  decoration: const InputDecoration(
                    labelText: "Target (IP/Domain)",
                    prefixIcon: Icon(Icons.network_check),
                    hintText: "e.g. google.com",
                  ),
                );
              },
            ),
            const SizedBox(height: 10),
            Wrap(
              spacing: 8.0,
              children: suggestions.take(3).map((s) => ActionChip(
                label: Text(s),
                backgroundColor: const Color(0xFF6C63FF).withValues(alpha: 0.2),
                onPressed: () {
                  Navigator.pop(ctx);
                  _doPing(index, s);
                },
              )).toList(),
            )
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text("Cancel")),
          ElevatedButton(
            style: ElevatedButton.styleFrom(backgroundColor: const Color(0xFF6C63FF), foregroundColor: Colors.white),
            onPressed: () {
              Navigator.pop(ctx);
              // Clean URL to hostname if needed
              String target = targetCtrl.text.trim();
              if (target.startsWith("http")) {
                try {
                  target = Uri.parse(target).host;
                } catch (e) { /* ignore */ }
              }
              // Remove path if present (e.g. domain.com/path -> domain.com)
              if (target.contains("/")) {
                target = target.split("/")[0];
              }
              _doPing(index, target);
            },
            child: const Text("Ping"),
          ),
        ],
      ),
    );
  }

  Future<void> _doPing(int index, String target) async {
    // Init Animation
    if (!_animControllers.containsKey(index)) {
      _animControllers[index] = AnimationController(
        duration: const Duration(milliseconds: 1000),
        vsync: this,
      );
    }
    _animControllers[index]!.repeat();

    setState(() {
      _isPinging[index] = true;
      _pingResults[index] = "Pinging...";
    });

    final stopwatch = Stopwatch()..start();
    String latency = "Timeout";

    try {
      if (target.startsWith("http")) {
        // HTTP Ping (Real Delay)
        try {
          final client = HttpClient();
          client.connectionTimeout = const Duration(seconds: 5);
          final request = await client.getUrl(Uri.parse(target));
          final response = await request.close();
          stopwatch.stop();
          
          if (response.statusCode == 204 || response.statusCode == 200) {
            latency = "${stopwatch.elapsedMilliseconds} ms";
          } else {
            latency = "HTTP ${response.statusCode}";
          }
        } catch (e) {
          latency = "Error";
        }
      } else {
        // ICMP Ping
        final result = await Process.run('ping', ['-c', '1', '-W', '2', target]);
        stopwatch.stop();
        
        if (result.exitCode == 0) {
          final RegExp regExp = RegExp(r"time=([0-9\.]+) ms");
          final match = regExp.firstMatch(result.stdout.toString());
          if (match != null) {
            latency = "${match.group(1)} ms";
          }
        }
      }

      if (mounted) {
        setState(() {
          _pingResults[index] = latency;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _pingResults[index] = "Error";
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isPinging[index] = false;
        });
        _animControllers[index]!.stop();
        _animControllers[index]!.reset();
      }
    }
  }

  @override
  void dispose() {
    for (var controller in _animControllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.transparent,
      floatingActionButton: FloatingActionButton(
        onPressed: () => _showAddDialog(context),
        backgroundColor: const Color(0xFF6C63FF),
        child: const Icon(Icons.add, color: Colors.white),
      ),
      body: widget.accounts.isEmpty 
          ? Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.no_accounts_outlined, size: 64, color: Colors.grey.withValues(alpha: 0.3)),
                  const SizedBox(height: 16),
                  const Text("No accounts saved", style: TextStyle(color: Colors.grey)),
                ],
              ),
            )
          : ListView.builder(
              padding: const EdgeInsets.all(20),
              itemCount: widget.accounts.length,
              itemBuilder: (context, index) {
                final acc = widget.accounts[index];
                final isSelected = index == widget.activePingIndex;
                final usage = acc['usage'] ?? 0;
                
                final isPinging = _isPinging[index] ?? false;
                final pingResult = _pingResults[index];
                
                return Card(
                  margin: const EdgeInsets.only(bottom: 12),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(16),
                    side: isSelected ? const BorderSide(color: Color(0xFF6C63FF), width: 2) : BorderSide.none,
                  ),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 8),
                    child: ListTile(
                      contentPadding: const EdgeInsets.symmetric(horizontal: 16),
                      leading: Container(
                        padding: const EdgeInsets.all(10),
                        decoration: BoxDecoration(
                          color: isSelected ? const Color(0xFF6C63FF) : const Color(0xFF6C63FF).withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Icon(Icons.dns, color: isSelected ? Colors.white : const Color(0xFF6C63FF)),
                      ),
                      title: Row(
                        mainAxisAlignment: MainAxisAlignment.spaceBetween,
                        children: [
                          Text(acc['name'] ?? "Unknown", style: const TextStyle(fontWeight: FontWeight.bold)),
                          if (pingResult != null)
                            Container(
                              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                              decoration: BoxDecoration(
                                color: pingResult.contains("ms") 
                                    ? (double.tryParse(pingResult.split(" ")[0]) ?? 999) < 150 ? Colors.green.withValues(alpha: 0.2) : Colors.orange.withValues(alpha: 0.2)
                                    : Colors.red.withValues(alpha: 0.2),
                                borderRadius: BorderRadius.circular(8),
                              ),
                              child: Text(
                                pingResult, 
                                style: TextStyle(
                                  fontSize: 12, 
                                  fontWeight: FontWeight.bold,
                                  color: pingResult.contains("ms") 
                                      ? (double.tryParse(pingResult.split(" ")[0]) ?? 999) < 150 ? Colors.green : Colors.orange
                                      : Colors.red
                                ),
                              ),
                            ),
                        ],
                      ),
                      subtitle: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(acc['ip'] ?? "", style: const TextStyle(fontSize: 12)),
                          const SizedBox(height: 6),
                          Row(
                            children: [
                              Container(
                                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                                decoration: BoxDecoration(
                                  color: Colors.black26,
                                  borderRadius: BorderRadius.circular(4)
                                ),
                                child: Text(
                                  "Used: ${_formatTotalBytes(usage)}", 
                                  style: const TextStyle(fontSize: 10, color: Colors.grey),
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                      trailing: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          // Ping Button
                          IconButton(
                            icon: RotationTransition(
                              turns: _animControllers[index] ?? const AlwaysStoppedAnimation(0),
                              child: Icon(
                                Icons.flash_on, 
                                color: isPinging ? Colors.yellow : Colors.grey,
                                size: 20,
                              ),
                            ),
                            onPressed: isPinging ? null : () => _showPingDialog(context, index),
                          ),
                          PopupMenuButton(
                            itemBuilder: (ctx) => [
                              const PopupMenuItem(value: 'delete', child: Text("Delete")),
                            ],
                            onSelected: (val) {
                              if (val == 'delete') widget.onDelete(index);
                            },
                          ),
                        ],
                      ),
                      onTap: () => widget.onActivate(index),
                    ),
                  ),
                );
              },
            ),
    );
  }
}