import '../motion/motion.dart';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart';
import 'send_preparation_page.dart';

class NewTransferPage extends StatefulWidget {
  const NewTransferPage({super.key, required this.api});
  final PinelinkApi api;

  @override
  State<NewTransferPage> createState() => _NewTransferPageState();
}

class _NewTransferPageState extends State<NewTransferPage> {
  PlatformFile? _file;
  bool _preparing = false;
  late Future<List<TransferTask>> _tasks = widget.api.getTasks(
    widget.api.currentOwner,
    widget.api.currentDevice,
  );

  @override
  void didUpdateWidget(covariant NewTransferPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      _file = null;
      _preparing = false;
      _tasks = widget.api.getTasks(
        widget.api.currentOwner,
        widget.api.currentDevice,
      );
    }
  }

  Future<void> _choose() async {
    final api = widget.api;
    final result = await FilePicker.platform.pickFiles(withData: false);
    if (!mounted || widget.api != api || result == null) return;
    setState(() => _file = result.files.first);
  }

  @override
  Widget build(BuildContext context) {
    if (_preparing && _file != null) {
      return SendPreparationPage(
        name: _file!.name,
        size: _file!.size,
        filePath: _file!.path ?? _file!.name,
      );
    }
    return ListView(
      padding: const EdgeInsets.all(22),
      children: [
        Text(
          tr(context, 'homeTitle'),
          style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          tr(context, 'homeSubtitle'),
          style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
        ),
        const SizedBox(height: 28),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.all(22),
          decoration: BoxDecoration(
            color: PinelinkColors.background,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: const Color(0xFFC9D0D6)),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 50,
                height: 50,
                decoration: BoxDecoration(
                  color: PinelinkColors.soft,
                  borderRadius: BorderRadius.circular(16),
                ),
                child: const Icon(
                  Icons.download,
                  color: PinelinkColors.accent,
                  size: 26,
                ),
              ),
              const SizedBox(height: 20),
              Text(
                _file?.name ?? tr(context, 'dropTitle'),
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                _file == null
                    ? tr(context, 'dropHint')
                    : '${formatBytes(_file!.size, i18nLocale(context))} · ${tr(context, 'fileReady')}',
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                ),
              ),
              const SizedBox(height: 28),
              MotionButton.primary(
                onPressed: _file == null
                    ? _choose
                    : () => setState(() => _preparing = true),
                child: Text(
                  tr(context, _file == null ? 'chooseFile' : 'continue'),
                ),
              ),
              const SizedBox(height: 20),
              Text(
                tr(context, 'fileSupport'),
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 28),
        Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            color: PinelinkColors.background,
            borderRadius: BorderRadius.circular(12),
          ),
          child: Text(
            '${tr(context, 'relayNote')}\n${tr(context, 'persistenceNote')}',
            style: const TextStyle(
              fontSize: 12,
              color: PinelinkColors.muted,
              height: 1.7,
            ),
          ),
        ),
        const SizedBox(height: 32),
        Text(
          tr(context, 'recentTasks'),
          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 12),
        FutureBuilder<List<TransferTask>>(
          future: _tasks,
          builder: (context, snapshot) {
            final text = snapshot.connectionState == ConnectionState.waiting
                ? tr(context, 'loadingTasks')
                : snapshot.hasError
                ? tr(context, 'actionFailed')
                : (snapshot.data?.isEmpty ?? true)
                ? tr(context, 'noTasks')
                : tr(context, 'taskCount', {'count': snapshot.data!.length});
            return Container(
              width: double.infinity,
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: PinelinkColors.background,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Text(
                text,
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                ),
              ),
            );
          },
        ),
      ],
    );
  }
}
