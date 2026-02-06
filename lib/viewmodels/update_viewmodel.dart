import 'dart:io';
import 'package:http/http.dart' as http;
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

    try {
      final response = await http.Client().send(http.Request('GET', Uri.parse(version.apkUrl)));
      final contentLength = response.contentLength ?? 0;
      
      // Use temporary directory to avoid storage permission issues
      final dir = await getTemporaryDirectory();
      final file = File("${dir.path}/update.apk");
      
      // Delete existing file if any
      if (await file.exists()) {
        await file.delete();
      }
      
      final sink = file.openWrite();
      int receivedBytes = 0;

      await for (var chunk in response.stream) {
        sink.add(chunk);
        receivedBytes += chunk.length;
        if (contentLength > 0) {
          _downloadProgress.add(receivedBytes / contentLength);
        }
      }
      
      await sink.flush();
      await sink.close();
      
      print("Download completed: ${file.path} (${file.length()} bytes)");
      _isDownloading.add(false);
      _downloadProgress.add(1.0);
      return file;
    } catch (e) {
      print("Download error details: $e");
      _isDownloading.add(false);
      _downloadProgress.add(-1.0);
      return null;
    }
  }

  void dispose() {
    _availableUpdate.close();
    _downloadProgress.close();
    _isDownloading.close();
  }
}
