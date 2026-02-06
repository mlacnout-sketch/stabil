import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class LogsTab extends StatelessWidget {
  final List<String> logs;
  final ScrollController scrollController;

  const LogsTab({super.key, required this.logs, required this.scrollController});

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(16.0),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text("Live Logs", style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold)),
              Row(
                children: [
                  IconButton(
                    icon: const Icon(Icons.copy_all),
                    tooltip: "Copy All",
                    onPressed: () {
                      Clipboard.setData(ClipboardData(text: logs.join("\n")));
                      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text("All logs copied")));
                    },
                  ),
                  IconButton(icon: const Icon(Icons.delete), onPressed: () => logs.clear()),
                ],
              ),
            ],
          ),
        ),
        Expanded(
          child: Container(
            margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 0),
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: const Color(0xFF0F0F12),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: Colors.white10),
            ),
            child: ListView.builder(
              controller: scrollController,
              itemCount: logs.length,
              itemBuilder: (context, index) {
                final log = logs[index];
                final isError = log.toLowerCase().contains("error") || log.toLowerCase().contains("fail");
                return Padding(
                  padding: const EdgeInsets.only(bottom: 4),
                  child: SelectableText(
                    log,
                    style: TextStyle(
                      fontFamily: 'monospace',
                      fontSize: 11,
                      color: isError ? Colors.redAccent : Colors.greenAccent,
                    ),
                  ),
                );
              },
            ),
          ),
        ),
        const SizedBox(height: 16),
      ],
    );
  }
}
