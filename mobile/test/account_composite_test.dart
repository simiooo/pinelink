import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/auth/controller.dart';
import 'package:pinelink_mobile/src/guest/composite.dart';
import 'package:pinelink_mobile/src/guest/coordinator.dart';
import 'package:pinelink_mobile/src/local/controller.dart';

class _Auth extends AccountController {
  _Auth() : super(server: 'https://service.example/api');
  void signedInAs(String owner) {
    account = {'id': owner};
    session = {
      'device_id': 'account-device',
      'session_token': 'account-token',
      'csrf': 'csrf',
    };
    notifyListeners();
  }

  void signedOut() {
    account = {};
    session = {};
    notifyListeners();
  }
}

class _Local extends LocalController {
  @override
  List<LocalMap> get peers => [];
  @override
  List<LocalMap> get jobs => [];
  @override
  List<LocalMap> get nearby => [];
  @override
  LocalMap get pairing => {'phase': 'idle'};
  @override
  String? get discoveryError => null;
  @override
  bool get discovering => false;
  @override
  bool reachable(String root) => false;
  @override
  Future<void> discover() async {}
  @override
  Future<void> connectNearby(String id) async {}
  @override
  Future<String?> invite([String? expected]) async => null;
  @override
  Future<String?> join(String code, [String? expected]) async => null;
  @override
  Future<void> confirm() async {}
  @override
  Future<void> cancelPairing() async {}
  @override
  Future<String> send(Uint8List bytes, String name, LocalMap peer) async =>
      'local';
  @override
  Future<void> action(String id, String action) async {}
  @override
  Future<Uint8List?> plaintext(String id) async => null;
  @override
  Future<void> rename(String root, String name) async {}
  @override
  Future<void> remove(String root) async {}
  @override
  Future<void> foreground(bool active) async {}
  @override
  Future<void> close() async {
    dispose();
  }
}

class _Coordinator extends ChangeNotifier implements NativeGuestCoordinator {
  _Coordinator(this.label);
  final String label;
  bool closed = false;
  int sends = 0, invites = 0;
  @override
  bool online = true;
  @override
  List<LocalMap> peers = [];
  @override
  List<LocalMap> jobs = [];
  @override
  List<LocalMap> candidates = [];
  @override
  Future<void> poll() async {}
  @override
  Future<void> foreground(bool active) async {}
  @override
  Future<LocalMap> invite() async {
    invites++;
    return {'id': label, 'token': 'token', 'code': 'ABCD1234'};
  }

  @override
  Future<String> send(
    Uint8List bytes,
    String name,
    LocalMap peer, {
    String? sensitivity,
  }) async {
    sends++;
    return label;
  }

  @override
  Future<void> close() async {
    closed = true;
    dispose();
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Future<void> _wait(bool Function() condition) async {
  for (var i = 0; i < 100 && !condition(); i++) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
  expect(condition(), isTrue);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('account and visitor peers route independently while login and logout preserve visitor work', () async {
    final auth = _Auth(),
        local = _Local(),
        guest = _Coordinator('guest'),
        account = _Coordinator('account');
    guest.peers = [
      {
        'pair_id': 'g',
        'owner_id': 'guest-remote',
        'device_name': 'Guest',
        'online': true,
      },
    ];
    account.peers = [
      {
        'pair_id': 'a',
        'owner_id': 'acct-remote',
        'device_name': 'Account',
        'online': true,
      },
    ];
    final workspace = CompositeController(
      local,
      guest,
      auth.server,
      auth: auth,
      accountOpener: (_) async => account,
    );
    await workspace.invite();
    expect(workspace.pairing['invite'], 'guest');
    auth.signedInAs('acct-owner');
    await _wait(() => workspace.accountActive);
    expect(workspace.pairing['invite'], 'guest');
    expect(workspace.peers.map((peer) => peer['root']), [
      'guest:g',
      'account:a',
    ]);
    final accountPeer = workspace.peers.last, guestPeer = workspace.peers.first;
    expect(
      await workspace.send(Uint8List(1), 'file', guestPeer),
      'guest-job:guest',
    );
    expect(
      await workspace.sendText(Uint8List(1), accountPeer, 'SENSITIVE'),
      'account-job:account',
    );
    expect(guest.sends, 1);
    expect(account.sends, 1);
    await workspace.invite();
    expect(account.invites, 1);
    auth.signedOut();
    expect(workspace.accountActive, isFalse);
    expect(workspace.peers.map((peer) => peer['root']), ['guest:g']);
    await expectLater(
      workspace.send(Uint8List(1), 'file', accountPeer),
      throwsStateError,
    );
    await _wait(() => account.closed);
    expect(guest.closed, isFalse);
    await workspace.close();
    auth.dispose();
  });

  test(
    'account opening that finishes after sign-out is closed and never exposed',
    () async {
      final auth = _Auth(),
          local = _Local(),
          guest = _Coordinator('guest'),
          lateAccount = _Coordinator('late');
      final opening = Completer<NativeGuestCoordinator>();
      final workspace = CompositeController(
        local,
        guest,
        auth.server,
        auth: auth,
        accountOpener: (_) => opening.future,
      );
      auth.signedInAs('acct-owner');
      await Future<void>.delayed(Duration.zero);
      auth.signedOut();
      opening.complete(lateAccount);
      await _wait(() => lateAccount.closed);
      expect(workspace.accountActive, isFalse);
      expect(guest.closed, isFalse);
      await workspace.close();
      auth.dispose();
    },
  );
}
