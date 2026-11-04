import 'dart:async';
import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../motion/motion.dart';
import 'connect.dart';
import 'controller.dart';
import 'copy.dart';
import 'picker.dart';

class SmartConnect extends StatefulWidget {
  const SmartConnect({super.key, required this.controller});
  final LocalController controller;
  @override
  State<SmartConnect> createState() => _SmartConnectState();
}

class _SmartConnectState extends State<SmartConnect> {
  LocalController get r => widget.controller;
  final input = TextEditingController();
  String view = 'show', error = '';
  bool busy = false, nearby = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && r.pairing['phase'] == 'idle') {
        unawaited(
          run(() async {
            await r.invite();
          }),
        );
      }
    });
  }

  @override
  void dispose() {
    input.dispose();
    super.dispose();
  }

  Future<void> run(Future<void> Function() operation) async {
    if (busy) return;
    setState(() {
      busy = true;
      error = '';
    });
    try {
      await operation();
    } catch (exception) {
      if (mounted) setState(() => error = localError(context, exception));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> join(String value) => run(() async {
    await r.join(value);
  });
  Future<void> scan() async {
    final value = await Navigator.of(context)
        .push<String>(MaterialPageRoute(builder: (_) => const LocalScanner()));
    if (value != null && mounted) await join(value);
  }

  Future<void> importCode() => run(() async {
    final file = await FilePicker.pickFile();
    if (file == null) return;
    final value = utf8.decode(
      await readLocalPick(file, 18000, overflow: 'LOCAL_CODE_INVALID'),
    );
    await r.join(value);
  });

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: r,
    builder: (context, _) {
      final pair = r.pairing, phase = pair['phase'] as String? ?? 'idle';
      if (['show', 'reply', 'verify', 'waiting'].contains(phase)) {
        return LocalConnect(controller: r, legacy: true);
      }
      if (nearby &&
          !['preparing', 'online-request', 'connected'].contains(phase)) {
        return Column(
          children: [
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: () => setState(() => nearby = false),
                icon: const Icon(Icons.arrow_back),
                label: Text(lc(context, 'Back', '返回')),
              ),
            ),
            LocalConnect(controller: r, legacy: true),
          ],
        );
      }
      Widget body;
      if (phase == 'preparing') {
        body = const SizedBox(
          height: 240,
          child: Center(child: MotionSpinner(size: 24)),
        );
      } else if (phase == 'online-request') {
        body = Column(
          children: [
            const SizedBox(height: 24),
            const Icon(Icons.devices_outlined, size: 48),
            const SizedBox(height: 24),
            Text(
              pair['peer'] ?? lc(context, 'Device', '设备'),
              style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 32),
            SizedBox(
              width: double.infinity,
              child: MotionButton.primary(
                onPressed: busy ? null : () => run(r.confirm),
                busy: busy,
                child: Text(lc(context, 'Connect', '连接')),
              ),
            ),
            const SizedBox(height: 12),
            TextButton(
              onPressed: busy ? null : () => run(r.declinePairing),
              child: Text(lc(context, 'Decline', '拒绝')),
            ),
          ],
        );
      } else if (phase == 'online-waiting') {
        body = Padding(
          padding: const EdgeInsets.symmetric(vertical: 80),
          child: Column(
            children: [
              const MotionSpinner(size: 24),
              const SizedBox(height: 24),
              Text(lc(context, 'Waiting for confirmation', '等待对方确认')),
            ],
          ),
        );
      } else if (phase == 'connected') {
        body = Column(
          children: [
            const SizedBox(height: 32),
            const Icon(
              Icons.check_circle_outline,
              color: Color(0xff0866ff),
              size: 56,
            ),
            const SizedBox(height: 32),
            SizedBox(
              width: double.infinity,
              child: MotionButton.primary(
                onPressed: () => Navigator.pop(context),
                child: Text(lc(context, 'Done', '完成')),
              ),
            ),
          ],
        );
      } else {
        final link = pair['value'] as String?, code = pair['code'] as String?;
        body = Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                for (final tab in ['show', 'enter'])
                  Expanded(
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        border: Border(
                          bottom: BorderSide(
                            color: view == tab
                                ? const Color(0xff101010)
                                : const Color(0xffe4e4e7),
                            width: view == tab ? 2 : 1,
                          ),
                        ),
                      ),
                      child: TextButton(
                        style: TextButton.styleFrom(
                          foregroundColor: view == tab
                              ? const Color(0xff101010)
                              : const Color(0xff737373),
                        ),
                        onPressed: () => setState(() => view = tab),
                        child: Text(
                          tab == 'show'
                              ? lc(context, 'Show code', '显示连接码')
                              : lc(context, 'Enter code', '输入连接码'),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 32),
            if (view == 'show' &&
                phase == 'online-show' &&
                link != null &&
                code != null) ...[
              Center(
                child: SizedBox(
                  width: 200,
                  height: 200,
                  child: QrImageView(
                    data: link,
                    padding: const EdgeInsets.all(8),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Center(
                child: Text(
                  code.length == 8
                      ? '${code.substring(0, 4)} ${code.substring(4)}'
                      : code,
                  style: const TextStyle(
                    fontSize: 24,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(height: 8),
              Center(
                child: Text(
                  lc(context, 'Expires within 10 minutes', '10 分钟内有效'),
                  style: const TextStyle(
                    color: Color(0xff737373),
                    fontSize: 12,
                  ),
                ),
              ),
              const SizedBox(height: 32),
              MotionButton(
                onPressed: () => Clipboard.setData(ClipboardData(text: link)),
                child: Text(lc(context, 'Copy link', '复制链接')),
              ),
              const SizedBox(height: 16),
              TextButton(
                style: TextButton.styleFrom(
                  foregroundColor: const Color(0xff52525b),
                ),
                onPressed: busy
                    ? null
                    : () => run(() async {
                        await r.inviteOffline();
                      }),
                child: Text(lc(context, 'Connect without internet', '离线连接')),
              ),
              TextButton(
                style: TextButton.styleFrom(
                  foregroundColor: const Color(0xff52525b),
                ),
                onPressed: () => setState(() => nearby = true),
                child: Text(lc(context, 'Nearby devices', '附近设备')),
              ),
            ] else if (view == 'show') ...[
              MotionButton.primary(
                onPressed: busy
                    ? null
                    : () => run(() async {
                        await r.invite();
                      }),
                busy: busy,
                child: Text(lc(context, 'Show code', '显示连接码')),
              ),
              TextButton(
                style: TextButton.styleFrom(
                  foregroundColor: const Color(0xff52525b),
                ),
                onPressed: () => setState(() => nearby = true),
                child: Text(lc(context, 'Nearby devices', '附近设备')),
              ),
            ] else ...[
              TextField(
                controller: input,
                maxLength: 18000,
                minLines: 2,
                maxLines: 4,
                autocorrect: false,
                enableSuggestions: false,
                decoration: InputDecoration(
                  labelText: lc(context, 'Connection code or link', '连接码或链接'),
                  counterText: '',
                ),
                onSubmitted: (_) => join(input.text),
              ),
              const SizedBox(height: 16),
              MotionButton.primary(
                onPressed: busy ? null : () => join(input.text),
                busy: busy,
                child: Text(lc(context, 'Connect', '连接')),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  TextButton(
                    onPressed: busy ? null : scan,
                    child: Text(lc(context, 'Scan', '扫描')),
                  ),
                  TextButton(
                    onPressed: busy ? null : importCode,
                    child: Text(lc(context, 'Import', '导入')),
                  ),
                ],
              ),
            ],
          ],
        );
      }
      return Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          body,
          if (error.isNotEmpty) ...[
            const SizedBox(height: 16),
            Semantics(
              liveRegion: true,
              child: Text(
                error,
                style: const TextStyle(color: Color(0xff737373)),
              ),
            ),
          ],
        ],
      );
    },
  );
}
