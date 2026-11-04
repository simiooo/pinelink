import 'dart:async';
import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../motion/motion.dart';
import 'controller.dart';
import 'copy.dart';
import 'picker.dart';
import 'smart_connect.dart';

class LocalConnect extends StatefulWidget {
  const LocalConnect({
    super.key,
    required this.controller,
    this.legacy = false,
  });
  final LocalController controller;
  final bool legacy;
  @override
  State<LocalConnect> createState() => _LocalConnectState();
}

class _LocalConnectState extends State<LocalConnect> {
  LocalController get r => widget.controller;
  final input = TextEditingController();
  String view = 'nearby', error = '';
  bool busy = false;
  @override
  void dispose() {
    input.dispose();
    super.dispose();
  }

  Future<void> run(Future<void> Function() fn) async {
    if (busy) return;
    setState(() {
      busy = true;
      error = '';
    });
    try {
      await fn();
    } catch (e) {
      if (mounted) {
        setState(() {
          error = localError(context, e);
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          busy = false;
        });
      }
    }
  }

  Future<void> join(String text) => run(() async {
    await r.join(text);
    if (mounted) {
      setState(() {
        view = 'code';
      });
    }
  });
  Future<void> importCode() async {
    final picked = await FilePicker.pickFile();
    if (picked == null) return;
    await r.join(
      utf8.decode(
        await readLocalPick(picked, 18000, overflow: 'LOCAL_CODE_INVALID'),
      ),
    );
    if (mounted) {
      setState(() {
        view = 'code';
      });
    }
  }

  Future<void> scan() async {
    final value = await Navigator.of(context)
        .push<String>(MaterialPageRoute(builder: (_) => const LocalScanner()));
    if (value != null) await join(value);
  }

  @override
  Widget build(BuildContext context) => r.smartConnection && !widget.legacy
      ? SmartConnect(controller: r)
      : AnimatedBuilder(
          animation: r,
          builder: (context, _) {
            final p = r.pairing,
                phase = p['phase'] as String? ?? 'idle',
                packet = p['value'] as String?;
            Widget body;
            if (['verify', 'waiting'].contains(phase)) {
              body = Column(
                children: [
                  const SizedBox(height: 8),
                  const Icon(Icons.phonelink_lock_outlined, size: 48),
                  const SizedBox(height: 24),
                  Text(
                    p['code'] ?? '',
                    key: const ValueKey('local-comparison'),
                    style: const TextStyle(
                      fontSize: 36,
                      fontWeight: FontWeight.w700,
                      letterSpacing: 2,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    lc(context, 'Compare on both devices', '请核对两台设备上的数字'),
                    style: const TextStyle(color: Color(0xFF737373)),
                  ),
                  const SizedBox(height: 32),
                  SizedBox(
                    width: double.infinity,
                    child: MotionButton.primary(
                      onPressed: phase == 'waiting'
                          ? null
                          : () => run(r.confirm),
                      busy: busy,
                      child: phase == 'waiting'
                          ? Text(
                              lc(context, 'Waiting for confirmation', '等待对方确认'),
                            )
                          : Text(lc(context, 'Numbers match', '数字相同')),
                    ),
                  ),
                ],
              );
            } else if (phase == 'connected') {
              body = Column(
                children: [
                  const Icon(
                    Icons.check_circle_outline,
                    color: Color(0xFF0866FF),
                    size: 56,
                  ),
                  const SizedBox(height: 24),
                  SizedBox(
                    width: double.infinity,
                    child: MotionButton.primary(
                      onPressed: () => Navigator.pop(context),
                      child: Text(lc(context, 'Done', '完成')),
                    ),
                  ),
                ],
              );
            } else if (phase == 'preparing') {
              body = const SizedBox(
                height: 240,
                child: Center(child: MotionSpinner(size: 24)),
              );
            } else if (['show', 'reply'].contains(phase) && packet != null) {
              final fitsQr = QrValidator.validate(data: packet).isValid;
              body = Column(
                children: [
                  SizedBox(
                    width: 240,
                    height: 240,
                    child: QrImageView(
                      data: packet,
                      version: QrVersions.auto,
                      size: 240,
                      padding: const EdgeInsets.all(8),
                      errorCorrectionLevel: QrErrorCorrectLevel.L,
                      errorStateBuilder: (_, _) => const Icon(
                        Icons.insert_drive_file_outlined,
                        size: 64,
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  Text(
                    lc(
                      context,
                      !fitsQr
                          ? 'Save and open the connection package'
                          : phase == 'reply'
                          ? 'Scan this reply on the other device'
                          : 'Scan on the other device',
                      !fitsQr
                          ? '保存并打开连接包'
                          : phase == 'reply'
                          ? '用另一台设备扫描此回复'
                          : '用另一台设备扫描',
                    ),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 24),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      IconButton(
                        tooltip: lc(context, 'Copy connection code', '复制连接码'),
                        onPressed: () =>
                            Clipboard.setData(ClipboardData(text: packet)),
                        icon: const Icon(Icons.copy_outlined),
                      ),
                      const SizedBox(width: 16),
                      IconButton(
                        tooltip: lc(
                          context,
                          'Save connection package',
                          '保存连接包',
                        ),
                        onPressed: () => run(() async {
                          await FilePicker.saveFile(
                            fileName: 'Pinelink-connection.txt',
                            bytes: Uint8List.fromList(utf8.encode(packet)),
                          );
                        }),
                        icon: const Icon(Icons.download_outlined),
                      ),
                    ],
                  ),
                  if (phase == 'show') ...[
                    const SizedBox(height: 16),
                    MotionButton(
                      onPressed: scan,
                      child: Text(lc(context, 'Scan reply', '扫描回复')),
                    ),
                    const SizedBox(height: 12),
                    TextButton(
                      onPressed: () => setState(() {
                        view = 'input';
                      }),
                      child: Text(lc(context, 'Enter reply', '输入回复')),
                    ),
                  ],
                  if (phase == 'show' && view == 'input') ...[
                    const SizedBox(height: 16),
                    ..._input(),
                  ],
                ],
              );
            } else {
              body = Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (phase == 'expired') ...[
                    Text(
                      localError(
                        context,
                        StateError(p['error'] ?? 'LOCAL_CODE_EXPIRED'),
                      ),
                      style: const TextStyle(color: Color(0xFF737373)),
                    ),
                    const SizedBox(height: 24),
                  ],
                  Row(
                    children: [
                      for (final tab in ['nearby', 'code'])
                        Padding(
                          padding: const EdgeInsets.only(right: 24),
                          child: DecoratedBox(
                            decoration: BoxDecoration(
                              border: Border(
                                bottom: BorderSide(
                                  color: view == tab
                                      ? const Color(0xFF111111)
                                      : Colors.transparent,
                                  width: 2,
                                ),
                              ),
                            ),
                            child: TextButton(
                              style: TextButton.styleFrom(
                                foregroundColor: view == tab
                                    ? const Color(0xFF111111)
                                    : const Color(0xFF737373),
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 4,
                                  vertical: 12,
                                ),
                                shape: const RoundedRectangleBorder(),
                              ),
                              onPressed: () => setState(() {
                                view = tab;
                              }),
                              child: Text(
                                tab == 'nearby'
                                    ? lc(context, 'Nearby', '附近')
                                    : lc(context, 'Code', '连接码'),
                              ),
                            ),
                          ),
                        ),
                    ],
                  ),
                  const SizedBox(height: 24),
                  if (view == 'nearby') ...[
                    if (r.nearby.isEmpty)
                      Padding(
                        padding: const EdgeInsets.symmetric(vertical: 32),
                        child: Row(
                          children: [
                            r.discovering
                                ? const MotionSpinner(size: 18)
                                : const Icon(Icons.wifi_off_outlined, size: 24),
                            const SizedBox(width: 16),
                            Expanded(
                              child: Text(
                                r.discoveryError == null
                                    ? lc(
                                        context,
                                        'Looking for devices',
                                        '正在寻找设备',
                                      )
                                    : localError(
                                        context,
                                        StateError(r.discoveryError!),
                                      ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    if (r.discoveryError == 'LOCAL_PERMISSION_DENIED')
                      Padding(
                        padding: const EdgeInsets.only(bottom: 24),
                        child: TextButton(
                          onPressed: () => run(r.openSettings),
                          child: Text(lc(context, 'Open settings', '打开系统设置')),
                        ),
                      ),
                    for (final device in r.nearby)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 16),
                        child: Row(
                          children: [
                            const Icon(Icons.devices_outlined, size: 28),
                            const SizedBox(width: 16),
                            Expanded(
                              child: Text(
                                device['name'],
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(
                                  fontSize: 16,
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                            const SizedBox(width: 16),
                            MotionButton(
                              onPressed: busy
                                  ? null
                                  : () => run(
                                      () => r.connectNearby(device['id']),
                                    ),
                              child: Text(lc(context, 'Connect', '连接')),
                            ),
                          ],
                        ),
                      ),
                    const SizedBox(height: 24),
                    Row(
                      children: [
                        TextButton(
                          onPressed: () => setState(() {
                            view = 'code';
                          }),
                          child: Text(lc(context, 'Use a code', '使用连接码')),
                        ),
                        const Spacer(),
                        IconButton(
                          tooltip: lc(context, 'Search again', '重新搜索'),
                          onPressed: busy ? null : () => run(r.discover),
                          icon: const Icon(Icons.refresh),
                        ),
                      ],
                    ),
                  ] else ...[
                    SizedBox(
                      width: double.infinity,
                      child: MotionButton.primary(
                        onPressed: busy
                            ? null
                            : () => run(() async {
                                await r.invite();
                              }),
                        busy: busy,
                        child: Text(lc(context, 'Show code', '显示连接码')),
                      ),
                    ),
                    const SizedBox(height: 16),
                    SizedBox(
                      width: double.infinity,
                      child: MotionButton(
                        onPressed: scan,
                        child: Text(lc(context, 'Scan code', '扫描连接码')),
                      ),
                    ),
                    const SizedBox(height: 24),
                    ..._input(),
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
                      style: const TextStyle(color: Color(0xFF737373)),
                    ),
                  ),
                ],
              ],
            );
          },
        );
  List<Widget> _input() => [
    TextField(
      controller: input,
      maxLength: 18000,
      minLines: 2,
      maxLines: 4,
      autocorrect: false,
      enableSuggestions: false,
      decoration: InputDecoration(
        labelText: lc(context, 'Connection code', '连接码'),
        counterText: '',
      ),
      onSubmitted: (_) => join(input.text),
    ),
    const SizedBox(height: 16),
    Row(
      children: [
        TextButton(
          onPressed: busy ? null : () => run(importCode),
          child: Text(lc(context, 'Import package', '导入连接包')),
        ),
        const Spacer(),
        MotionButton(
          onPressed: busy ? null : () => join(input.text),
          child: Text(lc(context, 'Connect', '连接')),
        ),
      ],
    ),
  ];
}

class LocalScanner extends StatefulWidget {
  const LocalScanner({super.key});
  @override
  State<LocalScanner> createState() => _LocalScannerState();
}

class _LocalScannerState extends State<LocalScanner>
    with WidgetsBindingObserver {
  final camera = MobileScannerController(formats: const [BarcodeFormat.qrCode]);
  bool used = false;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The OS permission dialog changes lifecycle before the camera is ready.
    if (!camera.value.hasCameraPermission) return;
    if (state == AppLifecycleState.resumed && !used) {
      unawaited(camera.start().catchError((Object _) {}));
    } else if (state == AppLifecycleState.inactive) {
      unawaited(camera.stop().catchError((Object _) {}));
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    unawaited(camera.dispose().catchError((Object _) {}));
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(lc(context, 'Scan code', '扫描连接码'))),
    body: MobileScanner(
      controller: camera,
      errorBuilder: (context, error) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.no_photography_outlined, size: 48),
            const SizedBox(height: 24),
            Text(lc(context, 'Camera unavailable', '相机不可用')),
            const SizedBox(height: 24),
            MotionButton(
              onPressed: () => Navigator.pop(context),
              child: Text(lc(context, 'Enter a code', '输入连接码')),
            ),
          ],
        ),
      ),
      onDetect: (capture) {
        final code = capture.barcodes
            .map((b) => b.rawValue)
            .whereType<String>()
            .firstOrNull;
        if (!used && code != null) {
          used = true;
          unawaited(camera.stop().catchError((Object _) {}));
          Navigator.pop(context, code);
        }
      },
    ),
  );
}
