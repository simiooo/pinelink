import 'package:flutter/material.dart';

import '../i18n/controller.dart';
import '../theme/tokens.dart';

String formatBytes(int bytes, [Locale? locale]) {
  final current = locale ?? const Locale('zh', 'CN');
  if (bytes < 0) return translateLocale(current, 'sizeUnknown');
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  if (bytes < 1024 * 1024 * 1024) {
    return '${(bytes / 1024 / 1024).toStringAsFixed(1)} MB';
  }
  return '${(bytes / 1024 / 1024 / 1024).toStringAsFixed(1)} GB';
}

String? displayFileName(String? name, [Locale? locale]) => name == 'encrypted'
    ? translateLocale(locale ?? const Locale('zh', 'CN'), 'encryptedFile')
    : name;

String fileMetadataLabel(String? name, int? size, [Locale? locale]) {
  final current = locale ?? const Locale('zh', 'CN');
  if (name == 'encrypted') {
    return '${translateLocale(current, 'encryptedNameHint')}${size == null ? '' : ' · ${formatBytes(size, current)}'}';
  }
  return size == null
      ? translateLocale(current, 'fileSizePending')
      : formatBytes(size, current);
}

String formatTime(String? value, [Locale? locale]) {
  final current = locale ?? const Locale('zh', 'CN');
  final date = value == null ? null : DateTime.tryParse(value)?.toLocal();
  if (date == null) return translateLocale(current, 'timeUnknown');
  String two(int number) => number.toString().padLeft(2, '0');
  if (current.languageCode != 'zh') {
    final hour = date.hour % 12 == 0 ? 12 : date.hour % 12;
    return '${date.month}/${date.day} $hour:${two(date.minute)} ${date.hour < 12 ? 'AM' : 'PM'}';
  }
  return '${date.month}/${date.day} ${two(date.hour)}:${two(date.minute)}';
}

class FileIcon extends StatelessWidget {
  const FileIcon({super.key, this.fileName});
  final String? fileName;
  @override
  Widget build(BuildContext context) {
    final parts = fileName?.split('.');
    final extension =
        parts != null && parts.length > 1 && fileName != 'encrypted'
        ? parts.last
        : '';
    return Container(
      width: 56,
      height: 56,
      decoration: BoxDecoration(
        color: PinelinkColors.soft,
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(
            Icons.insert_drive_file_outlined,
            size: 22,
            color: PinelinkColors.accent,
          ),
          if (extension.isNotEmpty)
            Text(
              extension
                  .substring(0, extension.length > 4 ? 4 : extension.length)
                  .toUpperCase(),
              style: const TextStyle(
                fontSize: 10,
                color: PinelinkColors.accent,
              ),
            ),
        ],
      ),
    );
  }
}
