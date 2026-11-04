import 'dart:async';
import 'dart:typed_data';

import '../auth/controller.dart';
import '../local/controller.dart';
import 'account.dart';
import 'connection_input.dart';
import 'coordinator.dart';

/// LAN, visitor and account slots keep separate keys, stores and authority.
class CompositeController extends LocalController {
  CompositeController(
    this.local,
    this.guest,
    this.server, {
    this.auth,
    this.accountOpener = openNativeAccountCoordinator,
  }) {
    local.addListener(_changed);
    guest.addListener(_changed);
    auth?.addListener(_authChanged);
    _authChanged();
    _changed();
  }
  final LocalController local;
  final NativeGuestCoordinator guest;
  final String server;
  final AccountController? auth;
  final Future<NativeGuestCoordinator> Function(AccountController)
  accountOpener;
  static const _peerPrefix = 'guest:';
  static const _jobPrefix = 'guest-job:';
  static const _accountPeerPrefix = 'account:';
  static const _accountJobPrefix = 'account-job:';
  static const _accountCandidatePrefix = 'account-candidate:';
  NativeGuestCoordinator? _account, _pairingSource;
  Future<void>? _accountOpening;
  String? _accountKey;
  int _accountEpoch = 0;
  bool _foreground = true;
  LocalMap? _pairing;
  int _epoch = 0;
  final Set<String> _dismissedCandidates = {};
  Future<void>? _closing;
  bool _closed = false;

  @override
  bool get smartConnection => true;
  @override
  bool get accountActive => _account != null;

  LocalMap _peer(LocalMap value, {bool account = false}) => {
    ...value,
    'root': '${account ? _accountPeerPrefix : _peerPrefix}${value['pair_id']}',
    'name': value['device_name'] ?? 'Device',
    'authority': account ? 'ACCOUNT_SERVICE' : 'GUEST_SERVICE',
  };

  LocalMap _job(LocalMap value, {bool account = false}) {
    final progress = value['progress'] as Map?,
        delivery = progress?['delivery'] as Map?,
        transfer = value['transfer'] as Map?;
    final size =
        value['cipherSize'] as int? ?? transfer?['size_bytes'] as int? ?? 0;
    return {
      ...value,
      'id': '${account ? _accountJobPrefix : _jobPrefix}${value['id']}',
      'authority': account ? 'ACCOUNT_SERVICE' : 'GUEST_SERVICE',
      'peer': _peer(
        Map<String, dynamic>.from(value['peer'] as Map),
        account: account,
      ),
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
  List<LocalMap> get peers => [
    ...local.peers,
    ...guest.peers.map(_peer),
    ...?_account?.peers.map((peer) => _peer(peer, account: true)),
  ];
  @override
  List<LocalMap> get jobs => [
    ...local.jobs,
    ...guest.jobs.map(_job),
    ...?_account?.jobs.map((job) => _job(job, account: true)),
  ];
  @override
  List<LocalMap> get nearby => local.nearby;
  @override
  List<LocalMap> get pairRequests => [
    ...guest.candidates.where(
      (item) => item['incoming'] == true && item['status'] == 'WAITING',
    ),
    ...?_account?.candidates
        .where(
          (item) => item['incoming'] == true && item['status'] == 'WAITING',
        )
        .map(
          (item) => {...item, 'id': '$_accountCandidatePrefix${item['id']}'},
        ),
  ];

  String? _currentAccountKey() {
    final value = auth;
    final owner = value?.account['id'];
    if (value == null || owner is! String || !owner.startsWith('acct-')) {
      return null;
    }
    return '$owner\n${value.session['device_id'] ?? ''}\n${value.token}\n${value.csrf}';
  }

  void _authChanged() {
    if (_closed) return;
    final key = _currentAccountKey();
    if (key == _accountKey) return;
    final epoch = ++_accountEpoch,
        previous = _account,
        opening = _accountOpening;
    _accountKey = key;
    _account = null;
    previous?.removeListener(_changed);
    if (previous != null && identical(_pairingSource, previous)) {
      _pairingSource = null;
      _pairing = null;
      ++_epoch;
    }
    _changed();
    _accountOpening =
        () async {
          if (opening != null) await opening;
          if (previous != null) await previous.close();
          if (_closed ||
              epoch != _accountEpoch ||
              key == null ||
              auth == null) {
            return;
          }
          final next = await accountOpener(auth!);
          if (_closed || epoch != _accountEpoch) {
            await next.close();
            return;
          }
          _account = next;
          next.addListener(_changed);
          await next.foreground(_foreground);
          _changed();
        }().catchError((Object _) {
          if (!_closed && epoch == _accountEpoch) _changed();
        });
  }

  Future<NativeGuestCoordinator> _connectionSource() async {
    if (auth?.signedIn != true) return guest;
    await _accountOpening;
    if (_account == null) throw StateError('ACCOUNT_UNAVAILABLE');
    return _account!;
  }

  NativeGuestCoordinator? _candidateSource(String id) =>
      id.startsWith(_accountCandidatePrefix) ? _account : guest;
  String _candidateId(String id) => id.startsWith(_accountCandidatePrefix)
      ? id.substring(_accountCandidatePrefix.length)
      : id;
  String _candidateKey(NativeGuestCoordinator source, String id) =>
      identical(source, _account) ? '$_accountCandidatePrefix$id' : id;

  @override
  void showPairRequest(String id) {
    final candidate = pairRequests
        .where((item) => item['id'] == id)
        .firstOrNull;
    if (candidate == null || _closed) return;
    final source = _candidateSource(id);
    if (source == null) return;
    _dismissedCandidates.remove(id);
    _pairingSource = source;
    _pairing = {
      'phase': 'online-request',
      'candidate': _candidateId(id),
      'candidateKey': id,
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
  bool reachable(String root) => root.startsWith(_accountPeerPrefix)
      ? _account?.online ?? false
      : root.startsWith(_peerPrefix)
      ? guest.online
      : local.reachable(root);
  @override
  bool online(String root) => root.startsWith(_accountPeerPrefix)
      ? _account?.peers.any(
              (p) =>
                  '$_accountPeerPrefix${p['pair_id']}' == root &&
                  p['online'] == true,
            ) ??
            false
      : root.startsWith(_peerPrefix)
      ? guest.peers.any(
          (p) => '$_peerPrefix${p['pair_id']}' == root && p['online'] == true,
        )
      : local.online(root);
  @override
  bool supportsText(String root) => root.startsWith(_accountPeerPrefix)
      ? _account?.online ?? false
      : root.startsWith(_peerPrefix)
      ? guest.online
      : local.supportsText(root);

  void _changed() {
    if (_closed) return;
    final phase = _pairing?['phase'];
    final source = _pairingSource;
    if (phase == 'online-waiting' && source != null) {
      final invite = _pairing?['invite'];
      if (source.peers.any((peer) => peer['invite_id'] == invite)) {
        _pairing = {'phase': 'connected'};
      } else if (source.candidates.any(
        (candidate) =>
            candidate['id'] == _pairing?['candidate'] &&
            candidate['status'] == 'DECLINED',
      )) {
        _pairing = {'phase': 'expired', 'error': 'CONNECTION_DECLINED'};
      }
    }
    final candidate = pairRequests
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
      final key = candidate['id'] as String;
      _pairingSource = _candidateSource(key);
      _pairing = {
        'phase': 'online-request',
        'candidate': _candidateId(key),
        'candidateKey': key,
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
    if (expected != null &&
        !expected.startsWith(_peerPrefix) &&
        !expected.startsWith(_accountPeerPrefix)) {
      return local.invite(expected);
    }
    final epoch = ++_epoch;
    _pairing = {'phase': 'preparing'};
    _changed();
    final source = await _connectionSource();
    if (_closed || epoch != _epoch) return null;
    _pairingSource = source;
    await source.poll();
    if (_closed || epoch != _epoch) return null;
    if (!source.online) {
      _pairing = null;
      return local.invite();
    }
    LocalMap value;
    try {
      value = await source.invite();
    } catch (_) {
      await source.poll();
      if (_closed || epoch != _epoch) return null;
      if (!source.online) return inviteOffline();
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
    _pairingSource = null;
    return local.invite();
  }

  @override
  Future<String?> join(String code, [String? expected]) async {
    final parsed = parseConnectionInput(code, server);
    if (parsed.kind == ConnectionKind.local) {
      ++_epoch;
      _pairing = null;
      _pairingSource = null;
      return local.join(parsed.value, expected);
    }
    if (expected != null) throw StateError('GUEST_CODE_INVALID');
    final epoch = ++_epoch;
    _pairing = {'phase': 'preparing'};
    _changed();
    final source = await _connectionSource();
    if (_closed || epoch != _epoch) return null;
    _pairingSource = source;
    final candidate = await source.join(parsed.value);
    if (_closed || epoch != _epoch) return null;
    _pairing = {
      'phase': 'online-waiting',
      'candidate': candidate['id'],
      'invite': candidate['invite_id'],
    };
    _changed();
    await source.poll();
    return null;
  }

  @override
  Future<void> confirm() async {
    if (_pairing?['phase'] != 'online-request') return local.confirm();
    final id = _pairing!['candidate'] as String;
    final source = _pairingSource;
    if (source == null) throw StateError('CONNECTION_GONE');
    final epoch = _epoch,
        key = _pairing?['candidateKey'] as String? ?? _candidateKey(source, id);
    await source.respondCandidate(id, true);
    if (_closed || epoch != _epoch || !identical(source, _pairingSource)) {
      return;
    }
    _dismissedCandidates.add(key);
    _pairing = {'phase': 'connected'};
    _changed();
    await source.poll();
  }

  @override
  Future<void> declinePairing() async {
    if (_pairing?['phase'] != 'online-request') return cancelPairing();
    final id = _pairing!['candidate'] as String;
    final source = _pairingSource;
    if (source == null) throw StateError('CONNECTION_GONE');
    final epoch = _epoch,
        key = _pairing?['candidateKey'] as String? ?? _candidateKey(source, id);
    await source.respondCandidate(id, false);
    if (_closed || epoch != _epoch || !identical(source, _pairingSource)) {
      return;
    }
    _dismissedCandidates.add(key);
    _pairing = {'phase': 'expired', 'error': 'CONNECTION_DECLINED'};
    _changed();
    await source.poll();
  }

  @override
  Future<void> cancelPairing() async {
    ++_epoch;
    if (_pairing?['phase'] == 'online-request') {
      _dismissedCandidates.add(
        _pairing?['candidateKey'] as String? ??
            _pairing!['candidate'] as String,
      );
    }
    _pairing = null;
    _pairingSource = null;
    await local.cancelPairing();
    _changed();
  }

  NativeGuestCoordinator get _requiredAccount =>
      _account ?? (throw StateError('ACCOUNT_UNAVAILABLE'));
  String _jobId(String id) => id.substring(
    id.startsWith(_accountJobPrefix)
        ? _accountJobPrefix.length
        : _jobPrefix.length,
  );
  NativeGuestCoordinator? _jobSource(String id) =>
      id.startsWith(_accountJobPrefix)
      ? _requiredAccount
      : id.startsWith(_jobPrefix)
      ? guest
      : null;
  @override
  Future<String> send(Uint8List bytes, String name, LocalMap peer) async =>
      peer['authority'] == 'ACCOUNT_SERVICE'
      ? '$_accountJobPrefix${await _requiredAccount.send(bytes, name, peer)}'
      : peer['authority'] == 'GUEST_SERVICE'
      ? '$_jobPrefix${await guest.send(bytes, name, peer)}'
      : local.send(bytes, name, peer);
  @override
  Future<String> sendText(
    Uint8List bytes,
    LocalMap peer,
    String sensitivity,
  ) async => peer['authority'] == 'ACCOUNT_SERVICE'
      ? '$_accountJobPrefix${await _requiredAccount.send(bytes, 'Text', peer, sensitivity: sensitivity)}'
      : peer['authority'] == 'GUEST_SERVICE'
      ? '$_jobPrefix${await guest.send(bytes, 'Text', peer, sensitivity: sensitivity)}'
      : local.sendText(bytes, peer, sensitivity);
  @override
  Future<({String value, bool sensitive})?> text(String id) {
    final source = _jobSource(id);
    return source != null ? source.runtime.text(_jobId(id)) : local.text(id);
  }

  @override
  Future<Uint8List?> plaintext(String id) {
    final source = _jobSource(id);
    return source != null
        ? source.runtime.plaintext(_jobId(id))
        : local.plaintext(id);
  }

  @override
  Future<void> action(String id, String action) {
    final source = _jobSource(id);
    return source != null
        ? source.action(_jobId(id), action)
        : local.action(id, action);
  }

  @override
  Future<void> rename(String root, String name) =>
      root.startsWith(_accountPeerPrefix)
      ? _requiredAccount.rename(root.substring(_accountPeerPrefix.length), name)
      : root.startsWith(_peerPrefix)
      ? guest.rename(root.substring(_peerPrefix.length), name)
      : local.rename(root, name);
  @override
  Future<void> remove(String root) => root.startsWith(_accountPeerPrefix)
      ? _requiredAccount.remove(root.substring(_accountPeerPrefix.length))
      : root.startsWith(_peerPrefix)
      ? guest.remove(root.substring(_peerPrefix.length))
      : local.remove(root);
  @override
  Future<void> foreground(bool active) async {
    _foreground = active;
    await Future.wait([
      local.foreground(active),
      guest.foreground(active),
      if (_account != null) _account!.foreground(active),
    ]);
  }

  @override
  Future<void> close() => _closing ??= _finishClose();
  Future<void> _finishClose() async {
    _closed = true;
    ++_epoch;
    ++_accountEpoch;
    auth?.removeListener(_authChanged);
    local.removeListener(_changed);
    guest.removeListener(_changed);
    final account = _account;
    _account = null;
    account?.removeListener(_changed);
    await Future.wait([
      local.close(),
      guest.close(),
      if (account != null) account.close(),
      if (_accountOpening != null) _accountOpening!,
    ]);
    dispose();
  }
}
