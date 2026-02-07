import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'dart:async';
import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:open_filex/open_filex.dart';

import 'tabs/dashboard_tab.dart';
import 'tabs/proxies_tab.dart';
import 'tabs/logs_tab.dart';
import 'tabs/settings_tab.dart';
import '../viewmodels/update_viewmodel.dart';
import '../models/app_version.dart';

class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  int _selectedIndex = 0;
  final _updateViewModel = UpdateViewModel();

  static const platform = MethodChannel('com.minizivpn.app/core');
  static const logChannel = EventChannel('com.minizivpn.app/logs');
  static const statsChannel = EventChannel('com.minizivpn.app/stats');

  String _vpnState = "disconnected"; // disconnected, connecting, connected
  final List<String> _logs = [];
  final ScrollController _logScrollCtrl = ScrollController();
  
  List<Map<String, dynamic>> _accounts = [];
  int _activeAccountIndex = -1;
  
  Timer? _timer;
  DateTime? _startTime;
  String _durationString = "00:00:00";
  
  // Optimized: Use ValueNotifier to prevent full rebuilds on stats update
  final ValueNotifier<String> _dlSpeed = ValueNotifier("0 KB/s");
  final ValueNotifier<String> _ulSpeed = ValueNotifier("0 KB/s");
  final ValueNotifier<int> _sessionRx = ValueNotifier(0);
  final ValueNotifier<int> _sessionTx = ValueNotifier(0);
  
  @override
  void initState() {
    super.initState();
    _loadData();
    _initLogListener();
    _initStatsListener();
    
    // Auto-update check
    _updateViewModel.availableUpdate.listen((update) {
      if (update != null && mounted) {
        _showUpdateDialog(update);
      }
    });
    _updateViewModel.checkForUpdate();
  }

  // ... (existing code for _showUpdateDialog, _executeDownload) ...

  @override
  void dispose() {
    _timer?.cancel();
    _updateViewModel.dispose();
    _dlSpeed.dispose();
    _ulSpeed.dispose();
    _sessionRx.dispose();
    _sessionTx.dispose();
    super.dispose();
  }

  // ... (existing code for _loadData, _saveAccounts, _startTimer, _initLogListener) ...

  void _initStatsListener() {
    statsChannel.receiveBroadcastStream().listen((event) {
      if (event is String && mounted) {
        final parts = event.split('|');
        if (parts.length == 2) {
          final rx = int.tryParse(parts[0]) ?? 0;
          final tx = int.tryParse(parts[1]) ?? 0;

          // Optimized: Update notifiers directly, no setState
          _dlSpeed.value = _formatBytes(rx);
          _ulSpeed.value = _formatBytes(tx);
          _sessionRx.value += rx;
          _sessionTx.value += tx;

          if (_activeAccountIndex != -1) {
            _accounts[_activeAccountIndex]['usage'] =
                (_accounts[_activeAccountIndex]['usage'] ?? 0) + rx + tx;
          }
        }
      }
    });
  }

  // ... (existing code for _formatBytes) ...

  Future<void> _toggleVpn() async {
    HapticFeedback.mediumImpact();
    final prefs = await SharedPreferences.getInstance();
    await prefs.reload(); // Force reload to get latest settings from other tabs

    if (_vpnState == "connected") {
      try {
        await platform.invokeMethod('stopCore');
        _timer?.cancel();
        setState(() {
          _vpnState = "disconnected";
          _durationString = "00:00:00";
          _startTime = null;
        });
        // Reset stats via notifier
        _sessionRx.value = 0;
        _sessionTx.value = 0;
        
        await prefs.remove('vpn_start_time');
        await _saveAccounts();
      } catch (e) {
        _logs.add("Error stopping: $e");
      }
    } else {
        // ... (existing code) ...
    }
  }

  Future<void> _handleAccountSwitch(int index) async {
    final account = _accounts[index];
    final prefs = await SharedPreferences.getInstance();

    await prefs.setString('ip', account['ip']);
    await prefs.setString('auth', account['auth']);
    await prefs.setString('obfs', account['obfs']);

    setState(() {
      _activeAccountIndex = index;
    });
    // Reset stats via notifier
    _sessionRx.value = 0;
    _sessionTx.value = 0;

    if (_vpnState == "connected") {
      await _toggleVpn(); // Stop
      await Future.delayed(const Duration(milliseconds: 500));
      await _toggleVpn(); // Start
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: IndexedStack(
          index: _selectedIndex,
          children: [
            DashboardTab(
              vpnState: _vpnState,
              onToggle: _toggleVpn,
              dl: _dlSpeed,
              ul: _ulSpeed,
              duration: _durationString,
              sessionRx: _sessionRx,
              sessionTx: _sessionTx,
            ),
            // ... (rest of the build method) ...