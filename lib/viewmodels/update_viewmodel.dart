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

    int attempts = 0;
    const maxAttempts = 3;

    while (attempts < maxAttempts) {
      attempts++;
      print("Download attempt $attempts of $maxAttempts");
      
      try {
        final response = await http.Client().send(http.Request('GET', Uri.parse(version.apkUrl)));
        
        if (response.statusCode != 200) {
          throw Exception("HTTP ${response.statusCode}");
        }

        final contentLength = response.contentLength ?? 0;
        
        // Use temporary directory with unique filename to prevent stale APKs
        final dir = await getTemporaryDirectory();
        final fileName = "update_${version.name}.apk";
        final file = File("${dir.path}/$fileName");
        
        // Delete existing file if any to ensure fresh download
        if (await file.exists()) {
          await file.delete();
        }
        
        final sink = file.openWrite();
        int receivedBytes = 0;

        try {
          await for (var chunk in response.stream) {
            sink.add(chunk);
            receivedBytes += chunk.length;
            if (contentLength > 0) {
              _downloadProgress.add(receivedBytes / contentLength);
            }
          }
          await sink.flush();
          await sink.close();
          
          // Verify size if possible
          if (contentLength > 0 && file.lengthSync() != contentLength) {
             throw Exception("Incomplete download");
          }

          print("Download completed: ${file.path}");
          _isDownloading.add(false);
          _downloadProgress.add(1.0);
          return file;

        } catch (e) {
          await sink.close();
          rethrow; // Catch in outer loop
        }

      } catch (e) {
        print("Download error (Attempt $attempts): $e");
        if (attempts >= maxAttempts) {
          _isDownloading.add(false);
          _downloadProgress.add(-1.0);
          return null;
        }
        // Wait before retry (1s, 2s, ...)
        await Future.delayed(Duration(seconds: attempts));
      }
    }
    return null;
  }

  void dispose() {
    _availableUpdate.close();
    _downloadProgress.close();
    _isDownloading.close();
  }
}
