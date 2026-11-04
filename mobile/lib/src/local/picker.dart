import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';

Future<Uint8List> readLocalPick(
  PlatformFile file,
  int limit, {
  String overflow = 'FILE_TOO_LARGE',
}) async {
  final length = await file.length();
  if (length != null && length > limit) throw StateError(overflow);
  final output = BytesBuilder(copy: false);
  final stream = file.readAsByteStream();
  await for (final chunk in stream) {
    if (output.length + chunk.length > limit) {
      throw StateError(overflow);
    }
    output.add(chunk);
  }
  if (length != null && output.length != length) {
    throw StateError('FILE_SOURCE_CHANGED');
  }
  return output.takeBytes();
}
