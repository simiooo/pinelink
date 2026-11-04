import 'dart:async';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../auth/controller.dart';
import '../i18n/controller.dart';
import '../motion/motion.dart';
import '../motion/surface.dart';
import '../motion/cloud_transfer_graph.dart';
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
  bool _showHome = false;
  final GlobalKey _graphKey = GlobalKey();
  final Map<String, GlobalKey> _nodeKeys = {};
  LocalMap? _openDevice;
  Offset _deviceAnchor = Offset.zero;
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
          'online-request',
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
        _showHome = false;
        _openDevice = null;
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
              title: Text(deviceDisplayName(context, peer)),
              onTap: () {
                Navigator.pop(context);
                unawaited(_send(peer));
              },
            ),
        ],
      ),
    );
  }

  void _device(LocalMap peer) {
    final root = peer['root'] as String;
    final node = _nodeKeys[root]?.currentContext?.findRenderObject();
    final graph = _graphKey.currentContext?.findRenderObject();
    if (node is! RenderBox || graph is! RenderBox) return;
    final center = node.localToGlobal(
      node.size.center(Offset.zero),
      ancestor: graph,
    );
    setState(() {
      _openDevice = peer;
      _deviceAnchor = center;
    });
  }

  void _deviceAction(LocalMap peer, String action) {
    setState(() => _openDevice = null);
    switch (action) {
      case 'conversation':
        _files(peer['root']);
      case 'rename':
        unawaited(_rename(peer));
      case 'remove':
        unawaited(_remove(peer));
      case 'reconnect':
        unawaited(
          _run(() async {
            await r.invite(peer['root']);
            await _pair();
          }),
        );
    }
  }

  Future<void> _rename(LocalMap peer) async {
    final input = TextEditingController(text: deviceDisplayName(context, peer));
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
          Text(deviceDisplayName(context, peer)),
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
        if (waiting.isEmpty && r.pairRequests.isEmpty) {
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
            for (final request in r.pairRequests)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.devices_outlined),
                title: Text(
                  request['device_name'] ?? lc(context, 'Device', '设备'),
                ),
                subtitle: Text(lc(context, 'Connection request', '连接请求')),
                onTap: () {
                  Navigator.pop(context);
                  r.showPairRequest(request['id']);
                },
              ),
            for (final job in waiting)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.insert_drive_file_outlined),
                title: Text(
                  job['name'],
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                subtitle: Text(deviceDisplayName(context, job['peer'])),
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
          selected = _showHome
              ? null
              : r.jobs.where((j) => j['id'] == _selected).firstOrNull ??
                    active.firstOrNull;
      final count =
          r.jobs
              .where(
                (j) =>
                    j['direction'] == 'receive' &&
                    j['state']?['state'] == 'PENDING' &&
                    localActive(j),
              )
              .length +
          r.pairRequests.length;
      return Scaffold(
        appBar: AppBar(
          toolbarHeight: 72,
          titleSpacing: 20,
          title: TextButton(
            onPressed: () => setState(() {
              _selected = null;
              _showHome = true;
              _openDevice = null;
            }),
            style: TextButton.styleFrom(
              padding: EdgeInsets.zero,
              alignment: Alignment.centerLeft,
              foregroundColor: const Color(0xff101010),
            ),
            child: const Text(
              'Pinelink',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w800),
            ),
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
                unawaited(PinelinkI18n.of(context)?.setPreference(value));
              },
              itemBuilder: (_) => [
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
                              final progress = selected['progress'] as Map?,
                                  spool = progress?['spool'] as Map?,
                                  delivery = progress?['delivery'] as Map?;
                              double fraction(Map? ledger, bool confirmed) {
                                if (confirmed) return 1;
                                final count =
                                    ledger?['acknowledged_bytes'] as int? ?? 0;
                                final maximum =
                                    ledger?['total_bytes'] as int? ?? total;
                                return maximum > 0
                                    ? (count / maximum).clamp(0.0, 1.0)
                                    : (confirmed ? 1 : 0);
                              }

                              final graph = serviceAuthority(selected)
                                  ? CloudTransferGraph(
                                      key: const ValueKey(
                                        'native-cloud-transfer-graph',
                                      ),
                                      spoolValue: fraction(
                                        spool,
                                        (progress?['completion']
                                                as Map?)?['stored'] ==
                                            true,
                                      ),
                                      deliveryValue: fraction(delivery, done),
                                      spoolRunning:
                                          connected && localActive(selected),
                                      deliveryRunning:
                                          connected &&
                                          localActive(selected) &&
                                          s?['paused'] != true,
                                      completed: done,
                                      incoming:
                                          selected['direction'] == 'receive',
                                      localLabel: lc(
                                        context,
                                        'This device',
                                        '本机',
                                      ),
                                      peerLabel: deviceDisplayName(
                                        context,
                                        r.peers
                                                .where(
                                                  (p) =>
                                                      p['root'] ==
                                                      selected['peer']['root'],
                                                )
                                                .firstOrNull ??
                                            selected['peer'],
                                      ),
                                      cloudLabel: lc(context, 'Cloud', '云端'),
                                      spoolLabel: lc(
                                        context,
                                        'Cloud copy',
                                        '云端暂存',
                                      ),
                                      deliveryLabel: lc(
                                        context,
                                        selected['direction'] == 'receive'
                                            ? 'Received'
                                            : 'Delivered',
                                        selected['direction'] == 'receive'
                                            ? '本机接收'
                                            : '对方接收',
                                      ),
                                      detailsLabel: lc(
                                        context,
                                        'Transfer details',
                                        '传输详情',
                                      ),
                                      localIcon: Icons.smartphone_outlined,
                                      peerIcon: peerIcon(selected['peer']),
                                      peerDisconnected:
                                          !r.online(selected['peer']['root']) &&
                                          !done,
                                      onDetails: () => showLocalJob(
                                        context,
                                        r,
                                        selected['id'],
                                      ),
                                    )
                                  : TransferGraph(
                                      key: const ValueKey(
                                        'native-transfer-graph',
                                      ),
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
                                      incoming:
                                          selected['direction'] == 'receive',
                                      localIcon: Icons.smartphone_outlined,
                                      peerIcon: peerIcon(selected['peer']),
                                      peerDisconnected:
                                          !r.reachable(
                                            selected['peer']['root'],
                                          ) &&
                                          !done,
                                      localLabel: lc(
                                        context,
                                        'This device',
                                        '本机',
                                      ),
                                      peerLabel: deviceDisplayName(
                                        context,
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
                                      onDetails: () => showLocalJob(
                                        context,
                                        r,
                                        selected['id'],
                                      ),
                                    );
                              return Center(
                                child: SingleChildScrollView(child: graph),
                              );
                            },
                          )
                        : LayoutBuilder(
                            builder: (context, box) => Stack(
                              key: _graphKey,
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
                                                  iconKey: _nodeKeys
                                                      .putIfAbsent(
                                                        peer['root'],
                                                        GlobalKey.new,
                                                      ),
                                                  online: r.online(
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
                                      Stack(
                                        clipBehavior: Clip.none,
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
                                          if (r.accountActive)
                                            Positioned(
                                              right: -4,
                                              top: -4,
                                              child: Tooltip(
                                                message: lc(
                                                  context,
                                                  'Account device',
                                                  '账号设备',
                                                ),
                                                child: Container(
                                                  padding: const EdgeInsets.all(
                                                    3,
                                                  ),
                                                  decoration:
                                                      const BoxDecoration(
                                                        color: Colors.white,
                                                        shape: BoxShape.circle,
                                                      ),
                                                  child: const Icon(
                                                    Icons.how_to_reg_outlined,
                                                    size: 14,
                                                    color: Color(0xff0866ff),
                                                  ),
                                                ),
                                              ),
                                            ),
                                        ],
                                      ),
                                      const SizedBox(height: 12),
                                      Text(
                                        lc(context, 'This device', '本机'),
                                        style: const TextStyle(fontSize: 13),
                                      ),
                                    ],
                                  ),
                                ),
                                if (_openDevice != null &&
                                    r.peers.any(
                                      (peer) =>
                                          peer['root'] == _openDevice!['root'],
                                    ))
                                  Positioned.fill(
                                    child: _DeviceActions(
                                      key: ValueKey(_openDevice!['root']),
                                      peer: _openDevice!,
                                      anchor: _deviceAnchor,
                                      reconnect:
                                          !serviceAuthority(_openDevice!) &&
                                          !r.reachable(_openDevice!['root']),
                                      onClose: () =>
                                          setState(() => _openDevice = null),
                                      onAction: (action) =>
                                          _deviceAction(_openDevice!, action),
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
      r'Android|iPhone|phone|Pixel|flutter-',
      caseSensitive: false,
    ).hasMatch(peer['name'] ?? '')
    ? Icons.smartphone_outlined
    : Icons.computer_outlined;

class _DeviceActions extends StatelessWidget {
  const _DeviceActions({
    super.key,
    required this.peer,
    required this.anchor,
    required this.reconnect,
    required this.onClose,
    required this.onAction,
  });
  final LocalMap peer;
  final Offset anchor;
  final bool reconnect;
  final VoidCallback onClose;
  final void Function(String) onAction;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final down = anchor.dy + 120 < box.maxHeight ? 92.0 : -92.0;
      final side = anchor.dx < 80 ? 92.0 : -92.0;
      Offset position(double dx, double dy) => Offset(
        (anchor.dx + dx).clamp(26.0, box.maxWidth - 26),
        (anchor.dy + dy).clamp(26.0, box.maxHeight - 26),
      );
      final actions =
          <({String id, String label, IconData icon, Offset target})>[
            (
              id: 'conversation',
              label: lc(context, 'Conversation', '聊天'),
              icon: Icons.chat_bubble_outline,
              target: position(side, 0),
            ),
            (
              id: 'rename',
              label: lc(context, 'Rename', '修改备注'),
              icon: Icons.edit_outlined,
              target: position(-48, down),
            ),
            (
              id: 'remove',
              label: lc(context, 'Remove device', '移除设备'),
              icon: Icons.delete_outline,
              target: position(48, down),
            ),
            if (reconnect)
              (
                id: 'reconnect',
                label: lc(context, 'Reconnect', '重新连接'),
                icon: Icons.link,
                target: position(-side, 0),
              ),
          ];
      return Stack(
        children: [
          Positioned.fill(
            child: Semantics(
              label: lc(context, 'Close device actions', '关闭设备操作'),
              button: true,
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: onClose,
              ),
            ),
          ),
          TweenAnimationBuilder<double>(
            tween: Tween(begin: 0, end: 1),
            duration: PinelinkMotion.reduced(context)
                ? Duration.zero
                : PinelinkMotion.quick,
            curve: Curves.easeOutCubic,
            builder: (context, value, _) => Stack(
              children: [
                for (final action in actions)
                  Positioned(
                    left:
                        anchor.dx + (action.target.dx - anchor.dx) * value - 24,
                    top:
                        anchor.dy + (action.target.dy - anchor.dy) * value - 24,
                    child: Opacity(
                      opacity: value.clamp(0, 1),
                      child: IconButton(
                        tooltip: action.label,
                        style: IconButton.styleFrom(
                          backgroundColor: Colors.white,
                          side: const BorderSide(color: Color(0xffe4e4e7)),
                          minimumSize: const Size(48, 48),
                        ),
                        onPressed: value < .5
                            ? null
                            : () => onAction(action.id),
                        icon: Icon(action.icon, size: 21),
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ],
      );
    },
  );
}

class _DeviceNode extends StatelessWidget {
  const _DeviceNode({
    required this.peer,
    required this.iconKey,
    required this.online,
    required this.onTap,
  });
  final LocalMap peer;
  final GlobalKey iconKey;
  final bool online;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => Semantics(
    button: true,
    label:
        '${deviceDisplayName(context, peer)}, ${online ? lc(context, 'Connected', '已连接') : lc(context, 'Disconnected', '未连接')}',
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
                    key: iconKey,
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
                    message: serviceAuthority(peer)
                        ? ((peer['owner_id'] as String?)?.startsWith('acct-') ==
                                  true
                              ? lc(context, 'Account device', '账号设备')
                              : lc(context, 'Visitor device', '访客设备'))
                        : lc(context, 'Paired locally', '已在本地配对'),
                    child: Container(
                      padding: const EdgeInsets.all(3),
                      decoration: const BoxDecoration(
                        color: Colors.white,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        serviceAuthority(peer)
                            ? ((peer['owner_id'] as String?)?.startsWith(
                                        'acct-',
                                      ) ==
                                      true
                                  ? Icons.how_to_reg_outlined
                                  : Icons.person_outline)
                            : Icons.verified_user_outlined,
                        size: 12,
                        color: online
                            ? ((peer['owner_id'] as String?)?.startsWith(
                                        'acct-',
                                      ) ==
                                      true
                                  ? const Color(0xff0866ff)
                                  : const Color(0xFF111111))
                            : const Color(0xFFA3A3A3),
                      ),
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              deviceDisplayName(context, peer),
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
                : deviceDisplayName(context, job['peer']),
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
                    if (job['plainSize'] is int)
                      Text(
                        localBytes(job['plainSize'] as int),
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
            if (serviceAuthority(job)) ...[
              Row(
                children: [
                  Expanded(child: Text(lc(context, 'Cloud copy', '云端暂存'))),
                  Text(
                    '${localBytes(((job['progress'] as Map?)?['spool'] as Map?)?['acknowledged_bytes'] as int? ?? 0)} / ${localBytes(job['cipherSize'] as int)}',
                    style: const TextStyle(
                      color: Color(0xFF737373),
                      fontSize: 13,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
            ],
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
                  if (s?['state'] == 'ACCEPTED' &&
                      (!serviceAuthority(job) || job['direction'] == 'receive'))
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
            peer == null
                ? lc(context, 'Files', '文件')
                : deviceDisplayName(context, peer),
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
