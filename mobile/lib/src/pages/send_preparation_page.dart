import '../motion/motion.dart';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart';

String sendCommand(String path, String owner, String device) {
  String quote(String value) => "'${value.replaceAll("'", "''")}'";
  return 'pinelink p2p send ${quote(path.trim())} --to-owner ${quote(owner)} --to-device ${quote(device)} --path auto';
}

class SendPreparationPage extends StatefulWidget {
  const SendPreparationPage({
    super.key,
    required this.name,
    required this.size,
    required this.filePath,
  });
  final String name;
  final int size;
  final String filePath;
  @override
  State<SendPreparationPage> createState() => _SendPreparationPageState();
}

class _SendPreparationPageState extends State<SendPreparationPage> {
  late final TextEditingController _path = TextEditingController(
    text: widget.filePath,
  );
  final TextEditingController _owner = TextEditingController();
  final TextEditingController _device = TextEditingController();
  String? _notice;

  bool get _valid {
    final identity = RegExp(r'^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$');
    return identity.hasMatch(_owner.text) &&
        identity.hasMatch(_device.text) &&
        _path.text.trim().isNotEmpty &&
        !_path.text.contains(RegExp(r'[\r\n]'));
  }

  @override
  void dispose() {
    _path.dispose();
    _owner.dispose();
    _device.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final command = _valid
        ? sendCommand(_path.text, _owner.text, _device.text)
        : '';
    return ListView(
      padding: const EdgeInsets.all(22),
      children: [
        Text(
          tr(context, 'prepareTitle'),
          style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          tr(context, 'prepareSubtitle'),
          style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
        ),
        const SizedBox(height: 28),
        Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: PinelinkColors.background,
            borderRadius: BorderRadius.circular(16),
          ),
          child: Row(
            children: [
              FileIcon(fileName: widget.name),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      widget.name,
                      style: const TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      formatBytes(widget.size, i18nLocale(context)),
                      style: const TextStyle(
                        fontSize: 12,
                        color: PinelinkColors.muted,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 24),
        TextField(
          controller: _owner,
          onChanged: (_) => setState(() => _notice = null),
          decoration: InputDecoration(
            labelText: tr(context, 'recipientOwner'),
            hintText: tr(context, 'recipientOwnerHint'),
            border: const OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 20),
        TextField(
          controller: _device,
          onChanged: (_) => setState(() => _notice = null),
          decoration: InputDecoration(
            labelText: tr(context, 'recipientDevice'),
            hintText: tr(context, 'recipientDeviceHint'),
            border: const OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 20),
        TextField(
          controller: _path,
          onChanged: (_) => setState(() => _notice = null),
          decoration: InputDecoration(
            labelText: tr(context, 'filePath'),
            border: const OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 16),
        Text(
          tr(context, 'prepareNote'),
          style: const TextStyle(
            fontSize: 12,
            color: PinelinkColors.muted,
            height: 1.7,
          ),
        ),
        const SizedBox(height: 24),
        MotionButton.primary(
          onPressed: _copy,
          child: Text(tr(context, 'copyCommand')),
        ),
        if (_notice != null)
          Padding(
            padding: const EdgeInsets.only(top: 16),
            child: Text(
              _notice!,
              style: const TextStyle(
                fontSize: 12,
                color: PinelinkColors.accent,
              ),
            ),
          ),
        if (command.isNotEmpty) ...[
          const SizedBox(height: 20),
          Container(
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(
              color: PinelinkColors.background,
              borderRadius: BorderRadius.circular(12),
            ),
            child: SelectableText(
              command,
              style: const TextStyle(fontSize: 12, height: 1.7),
            ),
          ),
        ],
      ],
    );
  }

  Future<void> _copy() async {
    if (!_valid) {
      setState(() => _notice = tr(context, 'prepareInvalid'));
      return;
    }
    final captured = sendCommand(_path.text, _owner.text, _device.text);
    try {
      await Clipboard.setData(ClipboardData(text: captured));
      if (!mounted ||
          captured != sendCommand(_path.text, _owner.text, _device.text)) {
        return;
      }
      setState(() => _notice = tr(context, 'copySuccess'));
    } catch (_) {
      if (!mounted ||
          captured != sendCommand(_path.text, _owner.text, _device.text)) {
        return;
      }
      setState(() => _notice = tr(context, 'copyFailed'));
    }
  }
}
