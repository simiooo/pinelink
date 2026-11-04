import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/guest/connections.dart';
import 'package:pinelink_mobile/src/guest/session.dart';

class _Vault implements IdentityVault {
  final values = <String, String>{};
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  // This integration case intentionally targets the isolated loopback fixture.
  setUpAll(() => HttpOverrides.global = null);
  final server = Platform.environment['PINELINK_NATIVE_GUEST_TEST_API'];
  test(
    'two native visitor identities pair only after approval on the real API',
    () async {
      final dbA = ClientDatabase.testing(NativeDatabase.memory()),
          dbB = ClientDatabase.testing(NativeDatabase.memory()),
          vaultA = _Vault(),
          vaultB = _Vault();
      final identityA = await LocalIdentity.load(
            dbA,
            vaultA,
            'native-guest-a',
            '',
          ),
          identityB = await LocalIdentity.load(
            dbB,
            vaultB,
            'native-guest-b',
            '',
          );
      final sessionA = await GuestSession.open(
            server: server!,
            identity: identityA,
            vault: vaultA,
          ),
          sessionB = await GuestSession.open(
            server: server,
            identity: identityB,
            vault: vaultB,
          );
      try {
        await sessionA.ensureSession(deviceName: 'Android A');
        await sessionB.ensureSession(deviceName: 'Android B');
        final a = GuestConnections(sessionA), b = GuestConnections(sessionB);
        final invite = await a.invite();
        final candidate = await b.join(invite['code']);
        expect((await a.list())['peers'], isEmpty);
        expect((await b.list())['peers'], isEmpty);
        await a.respond(candidate['id'], true);
        expect((await a.list())['peers'], hasLength(1));
        expect((await b.list())['peers'], hasLength(1));
        final peer = ((await a.list())['peers'] as List).single;
        expect(peer['device_id'], identityB.device);
        await a.remove(peer['pair_id']);
        expect((await a.list())['peers'], isEmpty);
        expect((await b.list())['peers'], isEmpty);
      } finally {
        sessionA.close();
        sessionB.close();
        await dbA.close();
        await dbB.close();
      }
    },
    skip: server == null ? 'requires isolated native guest test API' : false,
  );
}
