import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart' show compute;
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import 'dart:math';

import '../data/database.dart';
import '../data/identity.dart';
import '../data/wire.dart' as wire;
import 'controller.dart';
import 'discovery.dart';
import 'file_crypto.dart';
import 'native_crypto.dart';
import 'protocol.dart';
import 'session.dart';
import 'store.dart';

Future<LocalController?> openLocalController() async {
  await initializeLocalCrypto();
  final db = ClientDatabase('pinelink-native-local-v1');
  try {
    final identity = await LocalIdentity.load(
      db,
      PlatformVault(),
      'pinelink-native-local-v1',
      '',
    );
    final store = LocalStore(db);
    await store.initialize();
    String? name;
    if (Platform.isAndroid) {
      try {
        name = await const MethodChannel('pinelink/local-device')
            .invokeMethod<String>('name');
        name = name?.replaceAll(RegExp(r'[\x00-\x1f\x7f]'), '').trim();
        if (name != null && name.length > 64) name = name.substring(0, 64);
        if (name?.isEmpty == true) name = null;
      } catch (_) {
        name = null;
      }
    }
    return await NativeLocalRuntime.open(store, identity, name: name);
  } catch (_) {
    await db.close();
    rethrow;
  }
}

class NativeLocalRuntime extends LocalController {
  NativeLocalRuntime(this.store, this.identity, this.deviceName);
  final LocalStore store;
  final LocalIdentity identity;
  final String deviceName;
  late final LocalDiscovery discovery = LocalDiscovery(
    identity.root,
    deviceName,
    _incoming,
    _notify,
  );
  @override
  List<Json> peers = [], jobs = [];
  @override
  Json pairing = {'phase': 'idle'};
  final Map<String, LocalSession> _links = {}, _sessions = {};
  final Map<String, Future<void>> _serial = {};
  final Map<
    String,
    ({
      int version,
      FrameAssembler assembler,
      List<({int index, Uint8List bytes})> pending,
    })
  >
  _assemblers = {};
  final Map<String, Object> _workers = {};
  final Set<String> _manifests = {}, _verifying = {};
  final Set<String> _removing = {};
  LocalSession? _pending;
  String? _expected;
  Timer? _timer;
  bool _closed = false, _pulseRunning = false, _pairBusy = false;
  static Future<NativeLocalRuntime> open(
    LocalStore store,
    LocalIdentity identity, {
    String? name,
  }) async {
    final r = NativeLocalRuntime(
      store,
      identity,
      name ??
          (Platform.isAndroid
              ? 'Android'
              : Platform.isIOS
              ? 'iPhone'
              : Platform.localHostname),
    );
    await r._refresh();
    r._timer = Timer.periodic(const Duration(seconds: 2), (_) {
      if (!r._pulseRunning) {
        r._pulseRunning = true;
        unawaited(
          r._pulse().catchError((Object _) {}).whenComplete(() {
            r._pulseRunning = false;
          }),
        );
      }
    });
    unawaited(r.discover());
    return r;
  }

  int get now => DateTime.now().millisecondsSinceEpoch;
  void _check() {
    if (_closed) bad('LOCAL_CONNECTION_LOST');
  }

  void _notify() {
    if (!_closed) notifyListeners();
  }

  Future<void> _refresh() async {
    peers = await store.peers();
    jobs = await store.jobs();
    _notify();
  }

  void _update(Json changes) {
    pairing = {...pairing, ...changes};
    _notify();
  }

  @override
  List<Json> get nearby => discovery.nearby;
  @override
  String? get discoveryError => discovery.error;
  @override
  bool get discovering => discovery.running;
  @override
  bool reachable(String root) => _links[root]?.ready == true;
  @override
  Future<void> discover() => discovery.start();
  Future<Map<String, Json>> inspectRoutes() async {
    final result = <String, Json>{};
    for (final entry in _links.entries.toList()) {
      if (!entry.value.ready) continue;
      final proof = await entry.value.proof();
      if (proof != null) result[entry.key] = proof;
    }
    return result;
  }

  @override
  Future<void> openSettings() async {
    if (Platform.isAndroid) {
      await const MethodChannel('pinelink/local-device')
          .invokeMethod<void>('settings');
    } else if (Platform.isIOS) {
      await launchUrl(
        Uri.parse('app-settings:'),
        mode: LaunchMode.externalApplication,
      );
    }
  }

  Future<void> _task(String id, Future<void> Function() fn) {
    final future = (_serial[id] ?? Future<void>.value())
        .catchError((Object _) {})
        .then((_) {
          _check();
          return fn();
        });
    _serial[id] = future;
    unawaited(
      future
          .whenComplete(() {
            if (_serial[id] == future) _serial.remove(id);
          })
          .catchError((Object _) {}),
    );
    return future;
  }

  Future<LocalSession> _create(bool incoming, [String? id]) async {
    if (id != null && _sessions.values.any((s) => !s.closed && s.id == id)) {
      bad('LOCAL_CODE_USED');
    }
    late final LocalSession link;
    link = await LocalSession.create(
      identity,
      deviceName,
      incoming,
      id: id,
      opened: (l) async {
        if (_removing.contains(l.peer!['root'])) bad('CONNECTION_BUSY');
        if (_expected != null && _expected != l.peer!['root']) {
          bad('LOCAL_IDENTITY_CHANGED');
        }
        final known = (await store.peers())
            .where((p) => p['root'] == l.peer!['root'])
            .firstOrNull;
        l.check();
        _update({'phase': 'verify', 'code': l.code, 'peer': l.peer!['name']});
        if (known != null) {
          if (known['signing_key'] != l.peer!['signing_key'] ||
              known['encryption_key'] != l.peer!['encryption_key']) {
            bad('LOCAL_IDENTITY_CHANGED');
          }
          await l.confirm();
        }
      },
      control: _control,
      data: _data,
      changed: () {
        if (identical(_pending, link) && link.code.isNotEmpty) {
          _update({
            'phase': link.ready
                ? 'connected'
                : link.selfConfirmed
                ? 'waiting'
                : 'verify',
            'code': link.code,
            'peer': link.peer?['name'],
          });
        }
      },
      closed: _disconnected,
    );
    _check();
    _sessions[link.id] = link;
    _pending = link;
    return link;
  }

  Future<T> _pair<T>(Future<T> Function() fn) async {
    _check();
    if (_pairBusy) bad('LOCAL_DEVICE_BUSY');
    _pairBusy = true;
    try {
      return await fn();
    } finally {
      _pairBusy = false;
    }
  }

  @override
  Future<String?> invite([String? expected]) => _pair(() async {
    if (expected != null && reachable(expected)) bad('LOCAL_DEVICE_BUSY');
    if (_pending != null && !_pending!.ready) await _pending!.close();
    _expected = expected;
    pairing = {'phase': 'preparing'};
    _notify();
    final link = await _create(false);
    try {
      final packet = encodePackage(await link.offer());
      link.check();
      _update({'phase': 'show', 'value': packet});
      return packet;
    } catch (_) {
      await link.close();
      rethrow;
    }
  });
  @override
  Future<String?> join(String code, [String? expected]) =>
      _pair(() => _join(code, expected));
  Future<String?> _join(String code, String? expected) async {
    final packet = await decodePackage(code);
    _check();
    if (packet['peer']['root'] == identity.root) bad('LOCAL_SELF');
    if (_removing.contains(packet['peer']['root'])) bad('CONNECTION_BUSY');
    if (reachable(packet['peer']['root'])) bad('LOCAL_DEVICE_BUSY');
    if (expected != null && expected != packet['peer']['root']) {
      bad('LOCAL_IDENTITY_CHANGED');
    }
    if (packet['type'] == 'answer') {
      final link = _pending;
      if (link == null ||
          link.incoming ||
          link.id != packet['id'] ||
          link.closed ||
          link.peer != null) {
        bad('LOCAL_CODE_EXPIRED');
      }
      await store.reserve(packet['peer']['root'], link.id);
      try {
        link.check();
        await link.setAnswer(packet);
        _update({'phase': 'preparing'});
        return null;
      } catch (_) {
        await store.release(packet['peer']['root'], link.id);
        await link.close();
        rethrow;
      }
    }
    if (_pending != null && !_pending!.ready) await _pending!.close();
    _expected = expected;
    pairing = {'phase': 'preparing'};
    _notify();
    final link = await _create(true, packet['id']);
    try {
      await store.reserve(packet['peer']['root'], link.id);
      link.check();
      await store.consume('${packet['id']}:${packet['nonce']}');
      final reply = encodePackage(await link.answer(packet));
      link.check();
      _update({'phase': 'reply', 'value': reply});
      return reply;
    } catch (_) {
      await store.release(packet['peer']['root'], link.id);
      await link.close();
      rethrow;
    }
  }

  Future<String?> _incoming(String code) async {
    if (_pairBusy ||
        _pending != null &&
            !_pending!.ready &&
            !['idle', 'expired'].contains(pairing['phase'])) {
      bad('LOCAL_DEVICE_BUSY');
    }
    return join(code);
  }

  @override
  Future<void> connectNearby(String id) async {
    final hint = nearby.where((p) => p['id'] == id).firstOrNull;
    if (hint == null) bad('LOCAL_DEVICE_GONE');
    final offer = await invite(hint['root']);
    try {
      final answer = await discovery.exchange(id, offer!);
      await join(answer, hint['root']);
    } catch (_) {
      await _pending?.close();
      rethrow;
    }
  }

  @override
  Future<void> confirm() async {
    if (_pending == null) bad('LOCAL_CODE_EXPIRED');
    await _pending!.confirm();
  }

  @override
  Future<void> cancelPairing() async {
    if (_pending != null && !_pending!.ready) await _pending!.close();
    _pending = null;
    pairing = {'phase': 'idle'};
    _notify();
  }

  Future<void> _paired(LocalSession link) async {
    final peer = link.peer!, root = peer['root'] as String, old = _links[root];
    if (_removing.contains(root)) bad('CONNECTION_BUSY');
    link.check();
    await store.transaction(() async {
      await store.lease(root, link.id);
      await store.savePeer(peer);
    });
    link.check();
    _links[root] = link;
    if (old != null && old != link) await old.close();
    await _refresh();
    _update({'phase': 'connected', 'value': null, 'peer': peer['name']});
    for (final job in jobs.where((j) => j['peer']['root'] == root).take(100)) {
      if (job['direction'] == 'receive') {
        await link.send({'type': 'state', 'state': job['state']});
      } else if (!localTerminal(job['state']?['state']) &&
          job['expires'] > now) {
        await _offer(job, link);
      }
    }
  }

  Future<void> _disconnected(LocalSession link) async {
    _manifests.removeWhere((key) => key.startsWith('${link.id}:'));
    if (identical(_sessions[link.id], link)) _sessions.remove(link.id);
    final root = link.peer?['root'] as String?;
    if (root != null && identical(_links[root], link)) {
      _links.remove(root);
      for (final job in (await store.jobs()).where(
        (j) => j['peer']['root'] == root && localActive(j),
      )) {
        _workers.remove(job['id']);
        _assemblers.remove(job['id']);
        if (!_closed) {
          await _task(job['id'], () async {
            final current = await store.job(job['id']);
            if (current == null || !localActive(current) || reachable(root)) {
              return;
            }
            if (current['direction'] == 'receive' &&
                current['state']['state'] == 'ACCEPTED' &&
                current['state']['paused'] != true) {
              await _change(current, {
                'paused': true,
                'version': current['state']['version'] + 1,
              });
            }
            final latest = (await store.job(job['id']))!;
            if (localActive(latest)) {
              await store.put({
                ...latest,
                'disconnected': true,
                'error': link.error,
              });
            }
          }).catchError((Object _) {});
        }
      }
    }
    if (root != null) await store.release(root, link.id);
    if (!_closed) {
      await _refresh();
      if (identical(_pending, link)) {
        if (pairing['phase'] == 'connected') {
          _pending = null;
          pairing = {'phase': 'idle'};
          _notify();
        } else {
          _update({'phase': 'expired', 'value': null, 'error': link.error});
        }
      }
    }
  }

  Future<void> _offer(Json job, LocalSession link) => link.send({
    'type': 'offer',
    'id': job['id'],
    'root': job['cipherRoot'],
    'size': job['cipherSize'],
    'name': job['name'],
    'plain_size': job['plainSize'],
    'remaining': max(0, job['expires'] - now),
  });
  @override
  Future<String> send(Uint8List bytes, String name, Json peer) async {
    _check();
    final link = _links[peer['root']];
    if (link?.ready != true) bad('LOCAL_RECONNECT_REQUIRED');
    if (bytes.length > (64 << 20)) bad('FILE_TOO_LARGE');
    final prepared = await compute(prepareFileWorker, (
      bytes,
      name,
      peer['encryption_key'] as String,
    ));
    link!.check();
    final id = 'local:${randomUuid()}';
    final job = <String, dynamic>{
      'id': id,
      'created': now,
      'direction': 'send',
      'peer': peer,
      'name': prepared['name'],
      'plainSize': bytes.length,
      'cipherSize': (prepared['cipher'] as Uint8List).length,
      'cipherRoot': prepared['root'],
      'plainRoot': prepared['plainRoot'],
      'recipientManifest': prepared['manifest'],
      'expires': now + 600000,
    };
    await store.transaction(() async {
      await store.lease(peer['root']);
      if ((await store.jobs()).where(localActive).length >= 2) {
        bad('LOCAL_LIMIT_REACHED');
      }
      if (await store.storedBytes() + job['cipherSize'] > (256 << 20)) {
        bad('LOCAL_STORAGE_FULL');
      }
      await store.put(job);
      await store.bytes(id, 'source', 0, prepared['cipher']);
    });
    await _refresh();
    await _offer(job, link);
    return id;
  }

  Future<Json> _change(
    Json job,
    Json changes, [
    LocalSession? link,
    Uint8List? chunk,
    int? index,
    Uint8List? plain,
    List<({int index, Uint8List bytes})>? chunks,
  ]) async {
    final state = await signed(identity, 'state', {
      ...job['state'] as Json,
      ...changes,
      'signature': '',
    });
    await store.saveState(
      job,
      state,
      chunk: chunk,
      index: index,
      plain: plain,
      chunks: chunks,
    );
    if (link?.ready == true) {
      await link!.send({'type': 'state', 'state': state});
    }
    await _refresh();
    return state;
  }

  Future<void> _offered(Json v, LocalSession link) async {
    if (!localId(v['id']) ||
        !wire.validRoot(v['root']) ||
        !wire.integer(v['size']) ||
        !wire.integer(v['plain_size']) ||
        v['plain_size'] > (64 << 20) ||
        v['size'] != cipherSize(v['plain_size']) ||
        v['name'] is! String ||
        safeFileName(v['name']) != v['name'] ||
        !wire.integer(v['remaining'], 1) ||
        v['remaining'] > 600000) {
      bad();
    }
    await _task(v['id'], () async {
      final old = await store.job(v['id']);
      if (old != null) {
        if (old['direction'] != 'receive' ||
            old['peer']['root'] != link.peer!['root'] ||
            old['cipherRoot'] != v['root'] ||
            old['cipherSize'] != v['size'] ||
            old['name'] != v['name'] ||
            old['plainSize'] != v['plain_size']) {
          bad();
        }
        await link.send({'type': 'state', 'state': old['state']});
        return;
      }
      final state = await signed(identity, 'state', {
        'id': v['id'],
        'sender': link.peer!['root'],
        'recipient': identity.root,
        'root': v['root'],
        'size': v['size'],
        'name': v['name'],
        'plain_size': v['plain_size'],
        'state': 'PENDING',
        'version': 1,
        'revision': 0,
        'received': 0,
        'paused': false,
      });
      final job = <String, dynamic>{
        'id': v['id'],
        'created': now,
        'direction': 'receive',
        'peer': link.peer,
        'name': v['name'],
        'plainSize': v['plain_size'],
        'cipherRoot': v['root'],
        'cipherSize': v['size'],
        'expires': now + v['remaining'],
        'state': state,
      };
      await store.transaction(() async {
        await store.lease(link.peer!['root']);
        if ((await store.jobs()).where(localActive).length >= 2) {
          bad('LOCAL_LIMIT_REACHED');
        }
        if (await store.storedBytes() + (v['size'] as int) * 2 > (256 << 20)) {
          bad('LOCAL_STORAGE_FULL');
        }
        await store.put(job);
        await store.journal(state);
      });
      await link.send({'type': 'state', 'state': state});
      await _refresh();
    });
  }

  Future<void> _state(Json s, LocalSession link) async {
    if (!localId(s['id']) ||
        s['recipient'] != link.peer!['root'] ||
        s['sender'] != identity.root ||
        !wire.integer(s['version'], 1) ||
        !wire.integer(s['revision']) ||
        !wire.integer(s['received']) ||
        !wire.integer(s['size']) ||
        s['received'] > s['size'] ||
        s['paused'] is! bool ||
        ![
          'PENDING',
          'ACCEPTED',
          'COMPLETED',
          'REJECTED',
          'CANCELLED',
          'EXPIRED',
          'FAILED',
        ].contains(s['state']) ||
        !await verified(link.peer!['signing_key'], 'state', s)) {
      bad();
    }
    await _task(s['id'], () async {
      final job = await store.job(s['id']);
      if (job == null ||
          job['direction'] != 'send' ||
          job['peer']['root'] != link.peer!['root']) {
        return;
      }
      if (job['cipherRoot'] != s['root'] ||
          job['cipherSize'] != s['size'] ||
          job['plainSize'] != s['plain_size'] ||
          job['name'] != s['name']) {
        bad();
      }
      final old = job['state'] as Json?;
      if (old != null &&
          (s['version'] < old['version'] ||
              s['revision'] < old['revision'] ||
              s['received'] < old['received'] ||
              localTerminal(old['state']))) {
        return;
      }
      if (s['state'] == 'COMPLETED') {
        final r = s['receipt'];
        if (r is! Json ||
            r['v'] != 1 ||
            r['id'] != s['id'] ||
            r['sender'] != identity.root ||
            r['recipient'] != s['recipient'] ||
            r['root'] != s['root'] ||
            r['plaintext_root'] != job['plainRoot'] ||
            r['plaintext_size'] != job['plainSize'] ||
            s['received'] != s['size'] ||
            !await verified(link.peer!['signing_key'], 'receipt', r)) {
          bad('FILE_CHECKSUM_INVALID');
        }
      }
      final intent =
          localTerminal(s['state']) ||
              job['intent'] == 'pause' && s['paused'] == true
          ? null
          : job['intent'];
      await store.transaction(() async {
        await store.lease(link.peer!['root']);
        await store.put({
          ...job,
          'state': s,
          'intent': intent,
          'disconnected': false,
        });
        await store.journal(s);
        if (localTerminal(s['state'])) {
          await store.deleteBytes(job['id'], 'source');
        }
      });
      if (s['paused'] == true || localTerminal(s['state'])) {
        _workers.remove(job['id']);
      }
      await _refresh();
      if (intent != null && !localTerminal(s['state'])) {
        await link.send({
          'type': 'action',
          'id': job['id'],
          'action': intent,
          'expected': s['version'],
        });
        return;
      }
      final key = '${link.id}:${job['id']}:${s['version']}';
      if (s['state'] == 'ACCEPTED' &&
          s['paused'] != true &&
          job['recipientManifest'] != null &&
          !_manifests.contains(key)) {
        _manifests.add(key);
        await link.send({
          'type': 'manifest',
          'id': job['id'],
          'version': s['version'],
          'value': job['recipientManifest'],
        });
      }
    });
  }

  @override
  Future<void> action(String id, String action) async {
    final original = await store.job(id);
    if (original == null || !localActive(original)) return;
    final link = _links[original['peer']['root']];
    if (original['direction'] == 'send') {
      return _task(id, () async {
        final job = (await store.job(id))!;
        if (!localActive(job)) return;
        if (!['pause', 'resume', 'cancel'].contains(action)) bad();
        _workers.remove(id);
        if (action == 'resume' && link?.ready != true) {
          bad('LOCAL_RECONNECT_REQUIRED');
        }
        await store.control(
          job['peer']['root'],
          () => store.transaction(() async {
            await store.lease(job['peer']['root']);
            await store.put({
              ...job,
              'intent': action == 'resume' ? null : action,
            });
            if (action == 'cancel') await store.deleteBytes(id, 'source');
          }),
        );
        if (link?.ready == true) {
          await link!.send({
            'type': 'action',
            'id': id,
            'action': action,
            'expected': job['state']?['version'] ?? 0,
          });
        }
        await _refresh();
      });
    }
    if (['accept', 'resume'].contains(action) && link?.ready != true) {
      bad('LOCAL_RECONNECT_REQUIRED');
    }
    return _task(
      id,
      () => store.control(original['peer']['root'], () async {
        final job = (await store.job(id))!, s = job['state'] as Json;
        if (localTerminal(s['state'])) return;
        if (job['expires'] <= now) {
          await _change(job, {
            'state': 'EXPIRED',
            'version': s['version'] + 1,
          }, link);
          return;
        }
        if (![
              'accept',
              'reject',
              'pause',
              'resume',
              'cancel',
            ].contains(action) ||
            ['accept', 'reject'].contains(action) && s['state'] != 'PENDING' ||
            ['pause', 'resume'].contains(action) && s['state'] != 'ACCEPTED') {
          bad('FILE_JOB_CHANGED');
        }
        if (action == 'pause' && s['paused'] == true ||
            action == 'resume' && s['paused'] == false) {
          if (link?.ready == true) {
            await link!.send({'type': 'state', 'state': s});
          }
          return;
        }
        _workers.remove(id);
        _assemblers.remove(id);
        await _change(job, {
          'version': s['version'] + 1,
          'state': action == 'accept'
              ? 'ACCEPTED'
              : action == 'reject'
              ? 'REJECTED'
              : action == 'cancel'
              ? 'CANCELLED'
              : s['state'],
          'paused': action == 'pause'
              ? true
              : ['accept', 'resume'].contains(action)
              ? false
              : s['paused'],
        }, link);
      }),
    );
  }

  Future<void> _manifest(Json v, LocalSession link) async {
    if (v['id'] is! String ||
        v['value'] is! String ||
        v['value'].length > 8192) {
      bad();
    }
    await _task(v['id'], () async {
      final job = await store.job(v['id']), s = job?['state'] as Json?;
      if (job == null ||
          job['direction'] != 'receive' ||
          job['peer']['root'] != link.peer!['root'] ||
          s == null ||
          s['state'] != 'ACCEPTED' ||
          s['paused'] == true ||
          s['version'] != v['version']) {
        return;
      }
      final m = await identity.decrypt(v['value']);
      validateManifest(m);
      if (m['plaintext_size'] != job['plainSize'] ||
          cipherSize(m['plaintext_size']) != s['size'] ||
          safeFileName(m['file_name']) != job['name']) {
        bad('FILE_MANIFEST_INVALID');
      }
      link.check();
      if (!identical(_links[job['peer']['root']], link)) return;
      await store.transaction(() async {
        await store.lease(job['peer']['root'], link.id);
        await store.put({
          ...job,
          'manifest': v['value'],
          'disconnected': false,
        });
      });
      _assemblers[job['id']] = (
        version: s['version'] as int,
        assembler: FrameAssembler(s['size'], s['received'] ~/ wireChunkSize),
        pending: [],
      );
      await link.send({
        'type': 'ready',
        'id': job['id'],
        'version': s['version'],
        'from': s['received'],
      });
      await _refresh();
      if (s['received'] == s['size'] &&
          await store.hasBytes(job['id'], 'chunk')) {
        _verifyFile(job['id'], link);
      }
    });
  }

  Future<void> _ready(Json v, LocalSession link) async {
    if (v['id'] is! String || !wire.integer(v['from'])) bad();
    final job = await store.job(v['id']), s = job?['state'] as Json?;
    if (job == null ||
        job['direction'] != 'send' ||
        job['peer']['root'] != link.peer!['root'] ||
        s == null ||
        s['state'] != 'ACCEPTED' ||
        s['paused'] == true ||
        s['version'] != v['version'] ||
        s['received'] != v['from'] ||
        job['intent'] != null ||
        v['from'] != s['size'] && v['from'] % wireChunkSize != 0) {
      return;
    }
    final marker = Object();
    _workers[job['id']] = marker;
    unawaited(
      _sendBytes(job, s, link, marker)
          .catchError((Object e) {
            if (identical(_workers[job['id']], marker)) {
              link.error = e is StateError
                  ? e.message.toString()
                  : 'LOCAL_CONNECTION_LOST';
              unawaited(link.close());
            }
          })
          .whenComplete(() {
            if (identical(_workers[job['id']], marker)) {
              _workers.remove(job['id']);
            }
          }),
    );
  }

  Future<void> _sendBytes(
    Json job,
    Json state,
    LocalSession link,
    Object marker,
  ) async {
    final id = job['id'] as String,
        count = max(
          1,
          ((state['size'] as int) + wireChunkSize - 1) ~/ wireChunkSize,
        ),
        hex = id.substring(6).replaceAll('-', '');
    final prefix = Uint8List(20);
    for (var i = 0; i < 16; i++) {
      prefix[i] = int.parse(hex.substring(i * 2, i * 2 + 2), radix: 16);
    }
    ByteData.sublistView(prefix).setUint32(16, state['version']);
    var sent = state['received'] as int;
    final first = state['size'] > 0 && sent == state['size']
        ? count
        : sent ~/ wireChunkSize;
    for (var i = first; i < count; i++) {
      final deadline = now + 15000;
      while (true) {
        _check();
        link.check();
        if (!identical(_workers[id], marker)) return;
        final fresh = await store.job(id);
        if (fresh == null ||
            fresh['state']['version'] != state['version'] ||
            fresh['state']['paused'] == true ||
            fresh['intent'] != null ||
            !localActive(fresh)) {
          return;
        }
        if (sent - (fresh['state']['received'] as int) < 4 * wireChunkSize) {
          break;
        }
        if (now > deadline) bad('LOCAL_CONNECTION_LOST');
        await Future<void>.delayed(const Duration(milliseconds: 15));
      }
      final bytes = await store.readBytes(
        id,
        'source',
        start: i * wireChunkSize,
        length: wireChunkSize,
      );
      if (bytes == null) bad('FILE_SOURCE_MISSING');
      final frame = await fileFrame(i, bytes);
      for (final part in fragments(frame)) {
        if (!identical(_workers[id], marker)) return;
        await link.sendData(joinBytes([prefix, part]));
      }
      sent = min(state['size'], (i + 1) * wireChunkSize);
    }
  }

  Future<void> _data(Uint8List bytes, LocalSession link) async {
    if (bytes.length < 29 || bytes.length > 16384) bad('FILE_FRAME_INVALID');
    final hex = hexBytes(bytes.sublist(0, 16)),
        id =
            'local:${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}',
        version = ByteData.sublistView(bytes).getUint32(16);
    await _task(id, () async {
      final job = await store.job(id), s = job?['state'] as Json?;
      if (job == null ||
          job['direction'] != 'receive' ||
          job['peer']['root'] != link.peer!['root'] ||
          s == null) {
        bad('FILE_CONSENT_REQUIRED');
      }
      if (!link.ready || !identical(_links[job['peer']['root']], link)) return;
      if (localTerminal(s['state']) ||
          s['paused'] == true ||
          version < s['version']) {
        return;
      }
      if (s['state'] != 'ACCEPTED' ||
          version != s['version'] ||
          job['manifest'] == null) {
        bad('FILE_CONSENT_REQUIRED');
      }
      if (job['expires'] <= now) {
        await _change(job, {
          'state': 'EXPIRED',
          'version': s['version'] + 1,
        }, link);
        return;
      }
      final assembler = _assemblers[id];
      if (assembler == null || assembler.version != version) {
        bad('FILE_FRAME_INVALID');
      }
      final frame = assembler.assembler.push(Uint8List.sublistView(bytes, 20));
      if (frame == null) return;
      final index = ByteData.sublistView(frame).getUint32(0),
          payload = Uint8List.sublistView(frame, 36);
      if (index != s['received'] ~/ wireChunkSize + assembler.pending.length ||
          payload.length !=
              min(wireChunkSize, s['size'] - index * wireChunkSize) ||
          hexBytes(frame.sublist(4, 36)) != await digest(payload)) {
        bad('FILE_FRAME_INVALID');
      }
      final received = min(s['size'], (index + 1) * wireChunkSize);
      assembler.pending.add((index: index, bytes: payload));
      // Match the sender's four-chunk window. Sign and commit together so the
      // acknowledged prefix always covers durable data, with <=256 KiB replay.
      if (assembler.pending.length < 4 && received != s['size']) return;
      await _change(
        job,
        {'received': received, 'revision': s['revision'] + 1},
        link,
        null,
        null,
        null,
        assembler.pending.toList(),
      );
      assembler.pending.clear();
      if (received == s['size']) _verifyFile(id, link);
    });
  }

  void _verifyFile(String id, [LocalSession? link]) {
    if (!_verifying.add(id)) return;
    int? verifyingVersion;
    unawaited(
      (() async {
            final job = (await store.job(id))!, s = job['state'] as Json;
            verifyingVersion = s['version'];
            final manifest = await identity.decrypt(job['manifest']);
            final cipher = await store.readBytes(id, 'chunk');
            if (cipher == null || await digest(cipher) != job['cipherRoot']) {
              bad('FILE_CHECKSUM_INVALID');
            }
            final plain = await compute(decryptFileWorker, (cipher, manifest));
            final receipt = await signed(identity, 'receipt', {
              'v': 1,
              'id': id,
              'sender': s['sender'],
              'recipient': s['recipient'],
              'root': s['root'],
              'plaintext_root': manifest['plaintext_root'],
              'plaintext_size': manifest['plaintext_size'],
              'delivered_at': DateTime.now().toUtc().toIso8601String(),
            });
            await _task(
              id,
              () => store.control(job['peer']['root'], () async {
                final current = await store.job(id), state = current?['state'];
                if (current == null ||
                    state['version'] != s['version'] ||
                    state['state'] != 'ACCEPTED' ||
                    state['paused'] == true ||
                    current['expires'] <= now) {
                  return;
                }
                await _change(
                  current,
                  {
                    'state': 'COMPLETED',
                    'version': state['version'] + 1,
                    'receipt': receipt,
                  },
                  link,
                  null,
                  null,
                  plain,
                );
              }),
            );
          })()
          .catchError((Object e) async {
            // An expired/changed writer lease is retryable. In particular, a
            // restart at the final ACK must not turn valid bytes into FAILED
            // if the previous owner's lease expires between the two attempts.
            if (_closed ||
                e is StateError &&
                    const {
                      'LOCAL_OTHER_WINDOW',
                      'LOCAL_CONNECTION_LOST',
                      'FILE_JOB_CHANGED',
                    }.contains(e.message)) {
              return;
            }
            await _task(id, () async {
              final job = await store.job(id);
              if (job == null ||
                  job['state']?['version'] != verifyingVersion ||
                  job['state']?['paused'] == true ||
                  localTerminal(job['state']?['state'])) {
                return;
              }
              await store.control(
                job['peer']['root'],
                () => _change(job, {
                  'state': 'FAILED',
                  'version': job['state']['version'] + 1,
                }, link),
              );
            }).catchError((Object _) {});
          })
          .whenComplete(() {
            _verifying.remove(id);
          }),
    );
  }

  Future<void> _control(Json v, LocalSession link) async {
    if (v['type'] != 'paired' && !identical(_links[link.peer?['root']], link)) {
      return;
    }
    switch (v['type']) {
      case 'paired':
        return _paired(link);
      case 'forget':
        await store.remove(link.peer!['root']);
        await link.close();
        await _refresh();
        return;
      case 'offer':
        return _offered(v, link);
      case 'state':
        if (v['state'] is! Json) bad();
        return _state(v['state'], link);
      case 'manifest':
        return _manifest(v, link);
      case 'ready':
        return _ready(v, link);
      case 'action':
        if (v['id'] is! String ||
            !['pause', 'resume', 'cancel'].contains(v['action'])) {
          bad();
        }
        final job = await store.job(v['id']);
        if (job == null ||
            job['direction'] != 'receive' ||
            job['peer']['root'] != link.peer!['root']) {
          bad();
        }
        if (job['state']['version'] != v['expected'] ||
            localTerminal(job['state']['state'])) {
          await link.send({'type': 'state', 'state': job['state']});
          return;
        }
        return action(v['id'], v['action']);
      default:
        bad();
    }
  }

  Future<void> _pulse() async {
    if (_closed) return;
    for (final link in _sessions.values.toList()) {
      if (link.peer != null) {
        try {
          await store.transaction(
            () => store.lease(link.peer!['root'], link.id),
          );
        } catch (_) {
          await link.close();
        }
      }
    }
    for (final job in (await store.jobs()).where(localActive)) {
      if (job['expires'] <= now) {
        if (job['direction'] == 'receive') {
          await _task(
            job['id'],
            () => store.control(job['peer']['root'], () async {
              final fresh = (await store.job(job['id']))!;
              if (!localActive(fresh)) return;
              await _change(fresh, {
                'state': 'EXPIRED',
                'version': fresh['state']['version'] + 1,
              }, _links[job['peer']['root']]);
            }),
          ).catchError((Object _) {});
        } else {
          await _task(job['id'], () async {
            final fresh = (await store.job(job['id']))!;
            if (!localActive(fresh)) return;
            await store.put({...fresh, 'expired': true});
            await store.deleteBytes(job['id'], 'source');
          });
        }
      } else if (job['direction'] == 'receive' &&
          job['state']['state'] == 'ACCEPTED' &&
          job['state']['paused'] != true &&
          !reachable(job['peer']['root'])) {
        if (job['state']['received'] == job['cipherSize'] &&
            job['manifest'] != null &&
            await store.hasBytes(job['id'], 'chunk')) {
          _verifyFile(job['id']);
        } else {
          await _task(
            job['id'],
            () => store.control(job['peer']['root'], () async {
              final fresh = (await store.job(job['id']))!;
              if (!localActive(fresh) || reachable(fresh['peer']['root'])) {
                return;
              }
              await _change(fresh, {
                'paused': true,
                'version': fresh['state']['version'] + 1,
              });
            }),
          ).catchError((Object _) {});
        }
      }
    }
    await _refresh();
  }

  @override
  Future<Uint8List?> plaintext(String id) => store.readBytes(id, 'plain');
  @override
  Future<void> rename(String root, String name) async {
    final n = name.trim();
    if (n.isEmpty || n.length > 64 || RegExp(r'[\x00-\x1f\x7f]').hasMatch(n)) {
      bad('DEVICE_NAME_INVALID');
    }
    await store.rename(root, n);
    await _refresh();
  }

  @override
  Future<void> remove(String root) async {
    if (!_removing.add(root)) bad('CONNECTION_BUSY');
    bool hasActive(List<Json> jobs) => jobs.any(
      (j) =>
          j['peer']['root'] == root &&
          (localActive(j) ||
              j['intent'] == 'cancel' &&
                  !localTerminal(j['state']?['state']) &&
                  j['expires'] > now),
    );
    try {
      if (hasActive(await store.jobs())) bad('CONNECTION_BUSY');
      final links = _sessions.values
          .where(
            (l) =>
                l.peer?['root'] == root ||
                identical(l, _pending) && _expected == root,
          )
          .toList();
      for (final link in links) {
        if (link.ready) {
          try {
            await link.send({'type': 'forget'});
          } catch (_) {
            /* Best effort before local revocation. */
          }
        }
        await link.close();
        await link.drain();
      }
      // A confirmation already inside its transaction has now drained. Delete
      // last, so it cannot silently restore trust after the user removes it.
      await store.transaction(() async {
        if (hasActive(await store.jobs())) bad('CONNECTION_BUSY');
        await store.remove(root);
      });
      await _refresh();
    } finally {
      _removing.remove(root);
    }
  }

  @override
  Future<void> foreground(bool active) async {
    if (active) {
      await discover();
      await _pulse();
    } else {
      await discovery.pause();
    }
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    _timer?.cancel();
    _workers.clear();
    await discovery.close();
    for (final link in _sessions.values.toList()) {
      await link.close();
      await link.drain();
    }
    await Future.wait(
      _serial.values.toList().map((f) => f.catchError((Object _) {})),
    );
    await store.db.close();
  }
}
