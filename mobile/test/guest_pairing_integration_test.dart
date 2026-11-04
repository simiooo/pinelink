import 'dart:io';
import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/guest/connections.dart';
import 'package:pinelink_mobile/src/guest/runtime.dart';
import 'package:pinelink_mobile/src/guest/relay.dart';
import 'package:pinelink_mobile/src/local/file_crypto.dart';

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
      final runtimeA = await NativeGuestRuntime.open(
            server: server!,
            vault: vaultA,
            database: dbA,
          ),
          runtimeB = await NativeGuestRuntime.open(
            server: server,
            vault: vaultB,
            database: dbB,
          );
      final sessionA = runtimeA.session, sessionB = runtimeB.session;
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
        expect(peer['device_id'], runtimeB.identity.device);
        final raw = Uint8List.fromList(utf8.encode('private text'));
        final jobId = await runtimeA.offer(
          raw,
          'secret.txt',
          Map<String, dynamic>.from(peer),
          sensitivity: 'SENSITIVE',
        );
        final job = (await runtimeA.store.job(jobId))!;
        expect(job['name'], 'Text');
        expect(job['state'], 'PENDING');
        expect(await runtimeA.store.bytes(jobId, 'source'), isNot(raw));
        final transfer = job['transfer'] as Map<String, dynamic>;
        final pending = await sessionB.request(
          'GET',
          '/v1/transfers/${transfer['id']}',
        );
        expect(pending['content_kind'], 'TEXT');
        expect(pending['encrypted_manifest'], isNull);
        final stored = await GuestRelay(sessionA).upload(
          transfer['id'] as String,
          (await runtimeA.store.bytes(jobId, 'source'))!,
        );
        expect(stored['completion']['stored'], isTrue);
        expect(stored['completion']['delivered'], isFalse);
        await runtimeB.syncIncoming();
        final incoming = (await runtimeB.store.jobs()).single;
        expect(incoming['state'], 'PENDING');
        await expectLater(runtimeB.receive(incoming['id']), throwsStateError);
        await runtimeB.respond(incoming['id'], true);
        await runtimeB.receive(incoming['id']);
        final received = (await runtimeB.store.job(incoming['id']))!;
        expect(received['state'], 'COMPLETED');
        final manifest = await runtimeB.identity.decrypt(received['manifest']);
        final cipher = (await runtimeB.store.bytes(
          incoming['id'],
          'received',
        ))!;
        expect(await decryptDare(cipher, manifest), raw);
        final delivered = await sessionA.request(
          'GET',
          '/v1/p2p/${transfer['id']}/progress',
        );
        expect(delivered['completion']['delivered'], isTrue);
        await a.remove(peer['pair_id']);
        expect((await a.list())['peers'], isEmpty);
        expect((await b.list())['peers'], isEmpty);
      } finally {
        await runtimeA.close();
        await runtimeB.close();
      }
    },
    skip: server == null ? 'requires isolated native guest test API' : false,
  );
}
