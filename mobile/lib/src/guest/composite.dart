import 'dart:async';
import 'dart:typed_data';

import '../local/controller.dart';
import 'connection_input.dart';
import 'coordinator.dart';

/// The LAN and visitor slots have separate keys, storage and authority.
class CompositeController extends LocalController {
  CompositeController(this.local, this.guest, this.server) {
    local.addListener(_changed);
    guest.addListener(_changed);
    _changed();
  }
  final LocalController local;
  final NativeGuestCoordinator guest;
  final String server;
  static const _peerPrefix = 'guest:';
  static const _jobPrefix = 'guest-job:';
  LocalMap? _pairing;
  int _epoch = 0;
  final Set<String> _dismissedCandidates = {};
  Future<void>? _closing;
  bool _closed = false;

  @override
  bool get smartConnection => true;

  LocalMap _peer(LocalMap value) => {
    ...value,
    'root': '$_peerPrefix${value['pair_id']}',
    'name': value['device_name'] ?? 'Device',
    'authority': 'GUEST_SERVICE',
  };

  LocalMap _job(LocalMap value) {
    final progress = value['progress'] as Map?,
        delivery = progress?['delivery'] as Map?,
        transfer = value['transfer'] as Map?;
    final size =
        value['cipherSize'] as int? ?? transfer?['size_bytes'] as int? ?? 0;
    return {
      ...value,
      'id': '$_jobPrefix${value['id']}',
      'authority': 'GUEST_SERVICE',
      'peer': _peer(Map<String, dynamic>.from(value['peer'] as Map)),
      'cipherSize': size,
      'textManifest':
          '${value['state']}:${value['manifest'] ?? value['senderManifest'] ?? ''}',
      'state': {
        'state': value['state'],
        'size': size,
        'received': delivery?['acknowledged_bytes'] as int? ?? 0,
        'paused': value['paused'] == true,
      },
    };
  }

  @override
  List<LocalMap> get peers => [...local.peers, ...guest.peers.map(_peer)];
  @override
  List<LocalMap> get jobs => [...local.jobs, ...guest.jobs.map(_job)];
  @override
  List<LocalMap> get nearby => local.nearby;
  @override
  List<LocalMap> get pairRequests => guest.candidates
      .where((item) => item['incoming'] == true && item['status'] == 'WAITING')
      .toList();

  @override
  void showPairRequest(String id) {
    final candidate = pairRequests
        .where((item) => item['id'] == id)
        .firstOrNull;
    if (candidate == null || _closed) return;
    _dismissedCandidates.remove(id);
    _pairing = {
      'phase': 'online-request',
      'candidate': id,
      'peer': candidate['device_name'],
    };
    _changed();
  }

  @override
  LocalMap get pairing => _pairing ?? local.pairing;
  @override
  String? get discoveryError => local.discoveryError;
  @override
  bool get discovering => local.discovering;
  @override
  bool reachable(String root) =>
      root.startsWith(_peerPrefix) ? guest.online : local.reachable(root);
  @override
  bool online(String root) => root.startsWith(_peerPrefix)
      ? guest.peers.any(
          (p) => '$_peerPrefix${p['pair_id']}' == root && p['online'] == true,
        )
      : local.online(root);
  @override
  bool supportsText(String root) =>
      root.startsWith(_peerPrefix) ? guest.online : local.supportsText(root);

  void _changed() {
    if (_closed) return;
    final phase = _pairing?['phase'];
    if (phase == 'online-waiting') {
      final invite = _pairing?['invite'];
      if (guest.peers.any((peer) => peer['invite_id'] == invite)) {
        _pairing = {'phase': 'connected'};
      } else if (guest.candidates.any(
        (candidate) =>
            candidate['id'] == _pairing?['candidate'] &&
            candidate['status'] == 'DECLINED',
      )) {
        _pairing = {'phase': 'expired', 'error': 'CONNECTION_DECLINED'};
      }
    }
    final candidate = guest.candidates
        .where(
          (item) =>
              item['incoming'] == true &&
              item['status'] == 'WAITING' &&
              !_dismissedCandidates.contains(item['id']),
        )
        .firstOrNull;
    if (candidate != null &&
        (local.pairing['phase'] == 'idle' || _pairing != null) &&
        (_pairing == null || _pairing?['phase'] == 'online-show')) {
      _pairing = {
        'phase': 'online-request',
        'candidate': candidate['id'],
        'peer': candidate['device_name'],
      };
    }
    notifyListeners();
  }

  @override
  Future<void> discover() => local.discover();
  @override
  Future<void> openSettings() => local.openSettings();
  @override
  Future<void> connectNearby(String id) async {
    _pairing = null;
    await local.connectNearby(id);
  }

  @override
  Future<String?> invite([String? expected]) async {
    if (expected != null && !expected.startsWith(_peerPrefix)) {
      return local.invite(expected);
    }
    final epoch = ++_epoch;
    _pairing = {'phase': 'preparing'};
    _changed();
    await guest.poll();
    if (_closed || epoch != _epoch) return null;
    if (!guest.online) {
      _pairing = null;
      return local.invite();
    }
    LocalMap value;
    try {
      value = await guest.invite();
    } catch (_) {
      await guest.poll();
      if (_closed || epoch != _epoch) return null;
      if (!guest.online) return inviteOffline();
      rethrow;
    }
    if (_closed || epoch != _epoch) return null;
    final token = value['token'], code = value['code'];
    if (token is! String || code is! String) {
      throw StateError('GUEST_RESPONSE_INVALID');
    }
    final origin = Uri.parse(server).origin;
    final link = '$origin/connect#$token';
    _pairing = {
      'phase': 'online-show',
      'value': link,
      'code': code,
      'invite': value['id'],
    };
    _changed();
    return link;
  }

  @override
  Future<String?> inviteOffline() async {
    ++_epoch;
    _pairing = null;
    return local.invite();
  }

  @override
  Future<String?> join(String code, [String? expected]) async {
    final parsed = parseConnectionInput(code, server);
    if (parsed.kind == ConnectionKind.local) {
      ++_epoch;
      _pairing = null;
      return local.join(parsed.value, expected);
    }
    if (expected != null) throw StateError('GUEST_CODE_INVALID');
    final epoch = ++_epoch;
    _pairing = {'phase': 'preparing'};
    _changed();
    final candidate = await guest.join(parsed.value);
    if (_closed || epoch != _epoch) return null;
    _pairing = {
      'phase': 'online-waiting',
      'candidate': candidate['id'],
      'invite': candidate['invite_id'],
    };
    _changed();
    await guest.poll();
    return null;
  }

  @override
  Future<void> confirm() async {
    if (_pairing?['phase'] != 'online-request') return local.confirm();
    final id = _pairing!['candidate'] as String;
    await guest.respondCandidate(id, true);
    _dismissedCandidates.add(id);
    _pairing = {'phase': 'connected'};
    _changed();
    await guest.poll();
  }

  @override
  Future<void> declinePairing() async {
    if (_pairing?['phase'] != 'online-request') return cancelPairing();
    final id = _pairing!['candidate'] as String;
    await guest.respondCandidate(id, false);
    _dismissedCandidates.add(id);
    _pairing = {'phase': 'expired', 'error': 'CONNECTION_DECLINED'};
    _changed();
    await guest.poll();
  }

  @override
  Future<void> cancelPairing() async {
    ++_epoch;
    if (_pairing?['phase'] == 'online-request') {
      _dismissedCandidates.add(_pairing!['candidate'] as String);
    }
    _pairing = null;
    await local.cancelPairing();
    _changed();
  }

  String _jobId(String id) => id.substring(_jobPrefix.length);
  @override
  Future<String> send(Uint8List bytes, String name, LocalMap peer) async =>
      peer['authority'] == 'GUEST_SERVICE'
      ? '$_jobPrefix${await guest.send(bytes, name, peer)}'
      : local.send(bytes, name, peer);
  @override
  Future<String> sendText(
    Uint8List bytes,
    LocalMap peer,
    String sensitivity,
  ) async => peer['authority'] == 'GUEST_SERVICE'
      ? '$_jobPrefix${await guest.send(bytes, 'Text', peer, sensitivity: sensitivity)}'
      : local.sendText(bytes, peer, sensitivity);
  @override
  Future<({String value, bool sensitive})?> text(String id) =>
      id.startsWith(_jobPrefix)
      ? guest.runtime.text(_jobId(id))
      : local.text(id);
  @override
  Future<Uint8List?> plaintext(String id) => id.startsWith(_jobPrefix)
      ? guest.runtime.plaintext(_jobId(id))
      : local.plaintext(id);
  @override
  Future<void> action(String id, String action) => id.startsWith(_jobPrefix)
      ? guest.action(_jobId(id), action)
      : local.action(id, action);
  @override
  Future<void> rename(String root, String name) => root.startsWith(_peerPrefix)
      ? guest.rename(root.substring(_peerPrefix.length), name)
      : local.rename(root, name);
  @override
  Future<void> remove(String root) => root.startsWith(_peerPrefix)
      ? guest.remove(root.substring(_peerPrefix.length))
      : local.remove(root);
  @override
  Future<void> foreground(bool active) async {
    await Future.wait([local.foreground(active), guest.foreground(active)]);
  }

  @override
  Future<void> close() => _closing ??= _finishClose();
  Future<void> _finishClose() async {
    _closed = true;
    ++_epoch;
    local.removeListener(_changed);
    guest.removeListener(_changed);
    await Future.wait([local.close(), guest.close()]);
    dispose();
  }
}
