import 'dart:async';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../auth/controller.dart';
import '../i18n/controller.dart';
import '../motion/motion.dart';
import '../motion/surface.dart';
import '../motion/transfer_graph.dart';
import '../pages/account_page.dart';
import 'connect.dart';
import 'controller.dart';
import 'conversation.dart';
import 'copy.dart';
import 'picker.dart';

class LocalShell extends StatefulWidget {
  const LocalShell({super.key, required this.controller, this.auth});
  final LocalController controller;
  final AccountController? auth;
  @override
  State<LocalShell> createState() => _LocalShellState();
}

class _LocalShellState extends State<LocalShell> with WidgetsBindingObserver {
  LocalController get r => widget.controller;
  bool _pairOpen = false, _foreground = true;
  String? _selected;
  @override
  void initState() {
    super.initState();
    r.addListener(_changed);
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    r.removeListener(_changed);
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final active = state == AppLifecycleState.resumed;
    if (_foreground == active) return;
    setState(() {
      _foreground = active;
    });
    unawaited(r.foreground(active).catchError((Object _) {}));
  }

  void _changed() {
    if (!_pairOpen &&
        [
          'preparing',
          'reply',
          'verify',
          'waiting',
        ].contains(r.pairing['phase'])) {
      _pairOpen = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) unawaited(_pair(already: true));
      });
    }
  }

  Future<void> _run(Future<void> Function() fn) async {
    try {
      await fn();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(localError(context, e))));
      }
    }
  }

  Future<void> _pair({bool already = false}) async {
    if (_pairOpen && !already) return;
    _pairOpen = true;
    try {
      await showMotionSurface<void>(
        context,
        title: '',
        horizontalPadding: 20,
        titleBuilder: (context) => AnimatedBuilder(
          animation: r,
          builder: (context, _) => Text(
            r.pairing['peer'] ?? lc(context, 'Add device', '添加设备'),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
          ),
        ),
        builder: (_) => LocalConnect(controller: r),
      );
    } finally {
      await r.cancelPairing();
      _pairOpen = false;
    }
  }

  Future<void> _send(LocalMap peer) => _run(() async {
    final file = await FilePicker.pickFile();
    if (file == null) return;
    final bytes = await readLocalPick(file, 64 << 20);
    final id = await r.send(bytes, file.name, peer);
    if (mounted) {
      setState(() {
        _selected = id;
      });
    }
  });
  Future<void> _choosePeer() async {
    final peers = r.peers.where((p) => r.reachable(p['root'])).toList();
    if (peers.length == 1) {
      await _send(peers.single);
      return;
    }
    if (peers.isEmpty) {
      await _pair();
      return;
    }
    await showMotionSurface<void>(
      context,
      title: lc(context, 'Send to', '发送到'),
      horizontalPadding: 20,
      builder: (context) => Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final peer in peers)
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: const Icon(Icons.devices_outlined),
              title: Text(localPeerName(peer)),
              onTap: () {
                Navigator.pop(context);
                unawaited(_send(peer));
              },
            ),
        ],
      ),
    );
  }

  Future<void> _device(LocalMap peer) async {
    await showMotionSurface<void>(
      context,
      title: localPeerName(peer),
      horizontalPadding: 20,
      builder: (context) => Wrap(
        spacing: 24,
        runSpacing: 24,
        children: [
          IconButton(
            tooltip: lc(context, 'Send file', '发送文件'),
            onPressed: r.reachable(peer['root'])
                ? () {
                    Navigator.pop(context);
                    unawaited(_send(peer));
                  }
                : null,
            icon: const Icon(Icons.arrow_upward),
          ),
          IconButton(
            tooltip: lc(context, 'Files', '文件'),
            onPressed: () {
              Navigator.pop(context);
              _files(peer['root']);
            },
            icon: const Icon(Icons.history),
          ),
          IconButton(
            tooltip: lc(context, 'Reconnect', '重新连接'),
            onPressed: () {
              Navigator.pop(context);
              unawaited(
                _run(() async {
                  await r.invite(peer['root']);
                  await _pair();
                }),
              );
            },
            icon: const Icon(Icons.link),
          ),
          IconButton(
            tooltip: lc(context, 'Rename', '修改备注'),
            onPressed: () {
              Navigator.pop(context);
              unawaited(_rename(peer));
            },
            icon: const Icon(Icons.edit_outlined),
          ),
          IconButton(
            tooltip: lc(context, 'Remove device', '移除设备'),
            onPressed: () {
              Navigator.pop(context);
              unawaited(_remove(peer));
            },
            icon: const Icon(Icons.delete_outline),
          ),
        ],
      ),
    );
  }

  Future<void> _rename(LocalMap peer) async {
    final input = TextEditingController(text: localPeerName(peer));
    try {
      final name = await showMotionSurface<String>(
        context,
        title: lc(context, 'Rename', '修改备注'),
        horizontalPadding: 20,
        builder: (context) => Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: input,
              autofocus: true,
              maxLength: 64,
              decoration: InputDecoration(
                labelText: lc(context, 'Device name', '设备备注'),
              ),
            ),
            const SizedBox(height: 24),
            MotionButton.primary(
              onPressed: () => Navigator.pop(context, input.text),
              child: Text(lc(context, 'Save', '保存')),
            ),
          ],
        ),
      );
      if (name != null) await _run(() => r.rename(peer['root'], name));
    } finally {
      input.dispose();
    }
  }

  Future<void> _remove(LocalMap peer) async {
    final yes = await showMotionSurface<bool>(
      context,
      title: lc(context, 'Remove device?', '移除设备？'),
      horizontalPadding: 20,
      builder: (context) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(localPeerName(peer)),
          const SizedBox(height: 24),
          MotionButton.primary(
            onPressed: () => Navigator.pop(context, true),
            child: Text(lc(context, 'Remove', '移除')),
          ),
        ],
      ),
    );
    if (yes == true) await _run(() => r.remove(peer['root']));
  }

  void _files([String? root]) {
    Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => root == null
            ? LocalFiles(controller: r)
            : LocalConversation(
                controller: r,
                root: root,
                onJob: (context, id) => unawaited(showLocalJob(context, r, id)),
              ),
      ),
    );
  }

  Future<void> _notifications() => showMotionSurface<void>(
    context,
    title: lc(context, 'Notifications', '通知'),
    horizontalPadding: 20,
    builder: (context) => AnimatedBuilder(
      animation: r,
      builder: (context, _) {
        final waiting = r.jobs
            .where(
              (j) =>
                  j['direction'] == 'receive' &&
                  j['state']?['state'] == 'PENDING' &&
                  localActive(j),
            )
            .toList();
        if (waiting.isEmpty) {
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 32),
            child: Text(
              lc(context, 'All caught up', '暂无新通知'),
              style: const TextStyle(color: Color(0xFF737373)),
            ),
          );
        }
        return Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final job in waiting)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.insert_drive_file_outlined),
                title: Text(
                  job['name'],
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                subtitle: Text(localPeerName(job['peer'])),
                onTap: () {
                  Navigator.pop(context);
                  unawaited(showLocalJob(this.context, r, job['id']));
                },
              ),
          ],
        );
      },
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: r,
    builder: (context, _) {
      final active = r.jobs.where(localActive).toList(),
          selected =
              r.jobs.where((j) => j['id'] == _selected).firstOrNull ??
              active.firstOrNull;
      final count = r.jobs
          .where(
            (j) =>
                j['direction'] == 'receive' &&
                j['state']?['state'] == 'PENDING' &&
                localActive(j),
          )
          .length;
      return Scaffold(
        appBar: AppBar(
          toolbarHeight: 72,
          titleSpacing: 20,
          title: const Text(
            'Pinelink',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.w800),
          ),
          actions: [
            IconButton(
              tooltip: lc(context, 'Notifications', '通知'),
              onPressed: _notifications,
              icon: Badge(
                isLabelVisible: count > 0,
                smallSize: 6,
                child: const Icon(Icons.notifications_none),
              ),
            ),
            if (widget.auth != null)
              IconButton(
                tooltip: lc(context, 'Account', '账号'),
                onPressed: () => Navigator.of(context).push<void>(
                  MaterialPageRoute(
                    builder: (_) => AccountPage(controller: widget.auth!),
                  ),
                ),
                icon: const Icon(Icons.person_outline),
              ),
            PopupMenuButton<String>(
              tooltip: lc(context, 'More', '更多'),
              icon: const Icon(Icons.more_horiz),
              onSelected: (value) {
                if (value == 'files') {
                  _files();
                } else {
                  unawaited(PinelinkI18n.of(context)?.setPreference(value));
                }
              },
              itemBuilder: (_) => [
                PopupMenuItem(
                  value: 'files',
                  child: Text(lc(context, 'Files', '文件')),
                ),
                const PopupMenuItem(value: 'en-US', child: Text('English')),
                const PopupMenuItem(value: 'zh-CN', child: Text('简体中文')),
              ],
            ),
            const SizedBox(width: 12),
          ],
        ),
        body: SafeArea(
          top: false,
          child: Column(
            children: [
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: TickerMode(
                    enabled: _foreground,
                    child: selected != null
                        ? LayoutBuilder(
                            builder: (context, box) {
                              final s = selected['state'] as Map?,
                                  connected = r.reachable(
                                    selected['peer']['root'],
                                  );
                              final total = selected['cipherSize'] as int,
                                  received = s?['received'] as int? ?? 0,
                                  done = s?['state'] == 'COMPLETED';
                              final graph = TransferGraph(
                                key: const ValueKey('native-transfer-graph'),
                                value: total == 0
                                    ? (done ? 1 : 0)
                                    : received / total,
                                running:
                                    connected &&
                                    localActive(selected) &&
                                    s?['state'] == 'ACCEPTED' &&
                                    s?['paused'] != true &&
                                    selected['intent'] == null &&
                                    selected['disconnected'] != true,
                                completed: done,
                                cloud: false,
                                incoming: selected['direction'] == 'receive',
                                localIcon: Icons.smartphone_outlined,
                                peerIcon: peerIcon(selected['peer']),
                                peerDisconnected:
                                    !r.reachable(selected['peer']['root']) &&
                                    !done,
                                localLabel: lc(context, 'This device', '本机'),
                                peerLabel: localPeerName(
                                  r.peers
                                          .where(
                                            (p) =>
                                                p['root'] ==
                                                selected['peer']['root'],
                                          )
                                          .firstOrNull ??
                                      selected['peer'],
                                ),
                                statusLabel: jobStatus(
                                  context,
                                  selected,
                                  connected,
                                ),
                                detailsLabel: lc(
                                  context,
                                  'Transfer details',
                                  '传输详情',
                                ),
                                onDetails: () =>
                                    showLocalJob(context, r, selected['id']),
                              );
                              return Center(
                                child: SingleChildScrollView(child: graph),
                              );
                            },
                          )
                        : LayoutBuilder(
                            builder: (context, box) => Stack(
                              children: [
                                if (r.peers.isNotEmpty)
                                  Positioned.fill(
                                    bottom: 144,
                                    child: SingleChildScrollView(
                                      child: Padding(
                                        padding: const EdgeInsets.only(top: 32),
                                        child: Wrap(
                                          alignment: WrapAlignment.spaceAround,
                                          spacing: 24,
                                          runSpacing: 32,
                                          children: [
                                            for (final peer in r.peers)
                                              SizedBox(
                                                width: (box.maxWidth - 24) / 2,
                                                child: _DeviceNode(
                                                  peer: peer,
                                                  online: r.reachable(
                                                    peer['root'],
                                                  ),
                                                  onTap: () => _device(peer),
                                                ),
                                              ),
                                          ],
                                        ),
                                      ),
                                    ),
                                  )
                                else
                                  Positioned(
                                    top: 56,
                                    left: 0,
                                    right: 0,
                                    child: Center(
                                      child: IconButton(
                                        tooltip: lc(
                                          context,
                                          'Add device',
                                          '添加设备',
                                        ),
                                        style: IconButton.styleFrom(
                                          minimumSize: const Size(56, 56),
                                          backgroundColor: const Color(
                                            0xFFF5F5F5,
                                          ),
                                        ),
                                        onPressed: _pair,
                                        icon: const Icon(Icons.add),
                                      ),
                                    ),
                                  ),
                                Positioned(
                                  bottom: 24,
                                  left: 0,
                                  right: 0,
                                  child: Column(
                                    children: [
                                      Container(
                                        width: 80,
                                        height: 80,
                                        decoration: const BoxDecoration(
                                          shape: BoxShape.circle,
                                          color: Color(0xFFF5F5F5),
                                        ),
                                        child: const Icon(
                                          Icons.smartphone_outlined,
                                          size: 40,
                                        ),
                                      ),
                                      const SizedBox(height: 12),
                                      Text(
                                        lc(context, 'This device', '本机'),
                                        style: const TextStyle(fontSize: 13),
                                      ),
                                    ],
                                  ),
                                ),
                              ],
                            ),
                          ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
                child: Row(
                  children: [
                    IconButton(
                      tooltip: lc(context, 'Files', '文件'),
                      onPressed: _files,
                      icon: const Icon(Icons.folder_open_outlined),
                    ),
                    const Spacer(),
                    MotionButton.primary(
                      onPressed: _choosePeer,
                      child: Text(lc(context, 'Choose file', '选择文件')),
                    ),
                    const Spacer(),
                    IconButton(
                      tooltip: lc(context, 'Add device', '添加设备'),
                      onPressed: _pair,
                      icon: const Icon(Icons.add),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    },
  );
}

IconData peerIcon(LocalMap peer) =>
    RegExp(
      r'Android|iPhone|phone|Pixel',
      caseSensitive: false,
    ).hasMatch(peer['name'] ?? '')
    ? Icons.smartphone_outlined
    : Icons.computer_outlined;

class _DeviceNode extends StatelessWidget {
  const _DeviceNode({
    required this.peer,
    required this.online,
    required this.onTap,
  });
  final LocalMap peer;
  final bool online;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => Semantics(
    button: true,
    label:
        '${localPeerName(peer)}, ${online ? lc(context, 'Connected', '已连接') : lc(context, 'Disconnected', '未连接')}',
    child: InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(24),
      child: Padding(
        padding: const EdgeInsets.all(8),
        child: Column(
          children: [
            Stack(
              clipBehavior: Clip.none,
              children: [
                Opacity(
                  opacity: online ? 1 : .42,
                  child: Container(
                    width: 56,
                    height: 56,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: Color(0xFFF5F5F5),
                    ),
                    child: Icon(peerIcon(peer), size: 28),
                  ),
                ),
                Positioned(
                  right: -2,
                  bottom: -2,
                  child: Tooltip(
                    message: lc(context, 'Paired locally', '已在本地配对'),
                    child: Container(
                      padding: const EdgeInsets.all(3),
                      decoration: const BoxDecoration(
                        color: Colors.white,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        Icons.verified_user_outlined,
                        size: 12,
                        color: online
                            ? const Color(0xFF111111)
                            : const Color(0xFFA3A3A3),
                      ),
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              localPeerName(peer),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontSize: 12),
            ),
          ],
        ),
      ),
    ),
  );
}

Future<void> showLocalJob(BuildContext context, LocalController r, String id) =>
    showMotionSurface<void>(
      context,
      title: '',
      horizontalPadding: 20,
      titleBuilder: (context) => AnimatedBuilder(
        animation: r,
        builder: (context, _) {
          final job = r.jobs.where((j) => j['id'] == id).firstOrNull;
          return Text(
            job == null
                ? lc(context, 'Transfer', '传输')
                : localPeerName(job['peer']),
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
          );
        },
      ),
      builder: (_) => _LocalJob(controller: r, id: id),
    );

class _LocalJob extends StatefulWidget {
  const _LocalJob({required this.controller, required this.id});
  final LocalController controller;
  final String id;
  @override
  State<_LocalJob> createState() => _LocalJobState();
}

class _LocalJobState extends State<_LocalJob> {
  bool busy = false;
  String error = '';
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

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) {
      final r = widget.controller,
          job = r.jobs.where((j) => j['id'] == widget.id).firstOrNull;
      if (job == null) return const SizedBox.shrink();
      final s = job['state'] as Map?,
          pending = s?['state'] == 'PENDING' && job['direction'] == 'receive',
          done = s?['state'] == 'COMPLETED',
          connected = r.reachable(job['peer']['root']);
      final paused =
          s?['paused'] == true ||
          !connected ||
          job['disconnected'] == true ||
          job['intent'] == 'pause';
      return Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              LocalFileGlyph(job: job, connected: connected, size: 40),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      job['contentKind'] == 'TEXT'
                          ? lc(context, 'Text', '文本')
                          : job['name'],
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      localBytes(job['plainSize']),
                      style: const TextStyle(
                        color: Color(0xFF737373),
                        fontSize: 13,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 32),
          if (pending) ...[
            MotionButton.primary(
              onPressed: busy || !connected
                  ? null
                  : () => run(() => r.action(job['id'], 'accept')),
              busy: busy,
              child: Text(lc(context, 'Receive', '接收')),
            ),
            const SizedBox(height: 12),
            MotionButton(
              onPressed: busy
                  ? null
                  : () => run(() => r.action(job['id'], 'reject')),
              child: Text(lc(context, 'Decline', '拒绝')),
            ),
          ] else ...[
            Row(
              children: [
                Expanded(child: Text(jobStatus(context, job, connected))),
                Text(
                  '${localBytes(s?['received'] ?? 0)} / ${localBytes(job['cipherSize'])}',
                  style: const TextStyle(
                    color: Color(0xFF737373),
                    fontSize: 13,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 24),
            if (done &&
                job['direction'] == 'receive' &&
                job['contentKind'] != 'TEXT')
              MotionButton.primary(
                onPressed: busy
                    ? null
                    : () => run(() async {
                        final plain = await r.plaintext(job['id']);
                        if (plain == null) {
                          throw StateError('FILE_SOURCE_MISSING');
                        }
                        await FilePicker.saveFile(
                          fileName: job['name'],
                          bytes: plain,
                        );
                      }),
                child: Text(lc(context, 'Save file', '保存文件')),
              ),
            if (localActive(job))
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  IconButton(
                    tooltip: lc(context, 'Cancel transfer', '取消传输'),
                    onPressed: busy
                        ? null
                        : () => run(() => r.action(job['id'], 'cancel')),
                    icon: const Icon(Icons.close),
                  ),
                  const SizedBox(width: 16),
                  if (s?['state'] == 'ACCEPTED')
                    IconButton(
                      tooltip: paused
                          ? lc(context, 'Resume', '继续')
                          : lc(context, 'Pause', '暂停'),
                      onPressed: busy || !connected && paused
                          ? null
                          : () => run(
                              () => r.action(
                                job['id'],
                                paused ? 'resume' : 'pause',
                              ),
                            ),
                      icon: Icon(
                        paused ? Icons.play_arrow_outlined : Icons.pause,
                      ),
                    ),
                ],
              ),
          ],
          if (error.isNotEmpty) ...[
            const SizedBox(height: 16),
            Semantics(liveRegion: true, child: Text(error)),
          ],
        ],
      );
    },
  );
}

class LocalFileGlyph extends StatelessWidget {
  const LocalFileGlyph({
    super.key,
    required this.job,
    required this.connected,
    this.size = 44,
  });
  final LocalMap job;
  final bool connected;
  final double size;
  @override
  Widget build(BuildContext context) {
    final state = job['state']?['state'],
        done = state == 'COMPLETED',
        stopped =
            localTerminal(state) && !done ||
            job['expired'] == true ||
            job['intent'] == 'cancel',
        paused =
            !connected ||
            job['state']?['paused'] == true ||
            job['intent'] == 'pause';
    return Tooltip(
      message: jobStatus(context, job, connected),
      child: Opacity(
        opacity: stopped ? .42 : 1,
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Icon(
              Icons.insert_drive_file_outlined,
              size: size,
              color: const Color(0xFF8C8C8C),
            ),
            Positioned(
              bottom: -1,
              right: -4,
              child: Container(
                padding: const EdgeInsets.all(2),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: Colors.white,
                  border: Border.all(color: const Color(0xFFDBDBDB)),
                ),
                child: Icon(
                  done
                      ? Icons.check
                      : stopped
                      ? Icons.access_time
                      : paused
                      ? Icons.pause
                      : Icons.sync,
                  size: 10,
                  color: done
                      ? const Color(0xFF0866FF)
                      : const Color(0xFF737373),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class LocalFiles extends StatefulWidget {
  const LocalFiles({super.key, required this.controller, this.root});
  final LocalController controller;
  final String? root;
  @override
  State<LocalFiles> createState() => _LocalFilesState();
}

class _LocalFilesState extends State<LocalFiles> {
  int filter = 0;
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) {
      final r = widget.controller,
          jobs = r.jobs
              .where(
                (j) =>
                    (widget.root == null || j['peer']['root'] == widget.root) &&
                    (filter == 0 ||
                        filter == 1 && localActive(j) ||
                        filter == 2 && j['state']?['state'] == 'COMPLETED' ||
                        filter == 3 &&
                            (!localActive(j) &&
                                j['state']?['state'] != 'COMPLETED')),
              )
              .toList();
      final peer = r.peers.where((p) => p['root'] == widget.root).firstOrNull;
      return Scaffold(
        appBar: AppBar(
          title: Text(
            peer == null ? lc(context, 'Files', '文件') : localPeerName(peer),
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
          ),
        ),
        body: SafeArea(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SizedBox(height: 16),
                Wrap(
                  spacing: 24,
                  children: [
                    for (var i = 0; i < 4; i++)
                      DecoratedBox(
                        decoration: BoxDecoration(
                          border: Border(
                            bottom: BorderSide(
                              color: filter == i
                                  ? const Color(0xFF111111)
                                  : Colors.transparent,
                              width: 2,
                            ),
                          ),
                        ),
                        child: TextButton(
                          style: TextButton.styleFrom(
                            foregroundColor: filter == i
                                ? const Color(0xFF111111)
                                : const Color(0xFF737373),
                            padding: const EdgeInsets.symmetric(
                              horizontal: 4,
                              vertical: 12,
                            ),
                            shape: const RoundedRectangleBorder(),
                          ),
                          onPressed: () => setState(() {
                            filter = i;
                          }),
                          child: Text(
                            [
                              lc(context, 'All', '全部'),
                              lc(context, 'Active', '进行中'),
                              lc(context, 'Saved', '已完成'),
                              lc(context, 'Issues', '异常'),
                            ][i],
                          ),
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 32),
                Expanded(
                  child: jobs.isEmpty
                      ? Center(
                          child: Text(
                            lc(context, 'No files yet', '暂无文件'),
                            style: const TextStyle(color: Color(0xFF737373)),
                          ),
                        )
                      : GridView.builder(
                          gridDelegate:
                              const SliverGridDelegateWithMaxCrossAxisExtent(
                                maxCrossAxisExtent: 140,
                                mainAxisSpacing: 24,
                                crossAxisSpacing: 16,
                                childAspectRatio: 1.05,
                              ),
                          itemCount: jobs.length,
                          itemBuilder: (context, i) {
                            final j = jobs[i];
                            return InkWell(
                              borderRadius: BorderRadius.circular(12),
                              onTap: () => showLocalJob(context, r, j['id']),
                              child: Padding(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 4,
                                  vertical: 8,
                                ),
                                child: Column(
                                  children: [
                                    LocalFileGlyph(
                                      job: j,
                                      connected: r.reachable(j['peer']['root']),
                                    ),
                                    const SizedBox(height: 12),
                                    Text(
                                      j['name'],
                                      maxLines: 1,
                                      overflow: TextOverflow.ellipsis,
                                      style: const TextStyle(fontSize: 12),
                                    ),
                                  ],
                                ),
                              ),
                            );
                          },
                        ),
                ),
              ],
            ),
          ),
        ),
      );
    },
  );
}
