import 'dart:io';
import 'package:path_provider/path_provider.dart';
import 'package:rxdart/rxdart.dart';
import '../models/app_version.dart';
import '../repositories/update_repository.dart';

class UpdateViewModel {
  final _repository = UpdateRepository();

  final _availableUpdate = BehaviorSubject<AppVersion?>();
  final _downloadProgress = BehaviorSubject<double>.seeded(-1.0);
  final _isDownloading = BehaviorSubject<bool>.seeded(false);

  Stream<AppVersion?> get availableUpdate => _availableUpdate.stream;
  Stream<double> get downloadProgress => _downloadProgress.stream;
  Stream<bool> get isDownloading => _isDownloading.stream;

  Future<bool> checkForUpdate() async {
    final update = await _repository.fetchUpdate();
    _availableUpdate.add(update);
    return update != null;
  }

  Future<File?> startDownload(AppVersion version) async {
    _isDownloading.add(true);
    _downloadProgress.add(0.0);

    // Strategy: Try Proxy First (Tunnel), then Direct
    // This helps users with no quota update via VPN.
    final strategies = [
      "SOCKS5 127.0.0.1:7777", // Priority: Via VPN
      "DIRECT"                 // Fallback: Via WiFi/Data
    ];

    for (final proxy in strategies) {
      print("Attempting download via: $proxy");
      try {
        final file = await _executeDownload(version, proxy);
        if (file != null) {
          _isDownloading.add(false);
          _downloadProgress.add(1.0);
          return file;
        }
      } catch (e) {
        print("Download failed via $proxy: $e");
      }
    }
    
    _isDownloading.add(false);
    _downloadProgress.add(-1.0);
    return null;
  }

  Future<File?> _executeDownload(AppVersion version, String proxyConf) async {
    final client = HttpClient();
    client.connectionTimeout = const Duration(seconds: 15);
    
    // Force Proxy Config
    if (proxyConf != "DIRECT") {
      client.findProxy = (uri) => proxyConf;
    }

    try {
      final request = await client.getUrl(Uri.parse(version.apkUrl));
      final response = await request.close();
      
      if (response.statusCode != 200) {
        throw Exception("HTTP ${response.statusCode}");
      }

      final contentLength = response.contentLength;
      final dir = await getTemporaryDirectory();
      final fileName = "update_${version.name}.apk";
      final file = File("${dir.path}/$fileName");
      
      if (await file.exists()) {
        await file.delete();
      }
      
      final sink = file.openWrite();
      int receivedBytes = 0;

      await for (var chunk in response) {
        sink.add(chunk);
        receivedBytes += chunk.length;
        if (contentLength > 0) {
          _downloadProgress.add(receivedBytes / contentLength.toDouble());
        }
      }
      await sink.flush();
      await sink.close();
      
      if (contentLength > 0 && file.lengthSync() != contentLength) {
          throw Exception("Incomplete download");
      }
      return file;
    } finally {
      client.close();
    }
  }

  void dispose() {
    _availableUpdate.close();
    _downloadProgress.close();
    _isDownloading.close();
  }
}