import 'dart:io';
import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/guest/connections.dart';
import 'package:pinelink_mobile/src/guest/runtime.dart';
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
  Future<void> slowBlobs(int milliseconds) async {
    final client = HttpClient();
    try {
      final uri = Uri.parse(server!).replace(
        port: 8085,
        path: '/delay',
        queryParameters: {'ms': '$milliseconds'},
      );
      final response = await (await client.postUrl(uri)).close();
      if (response.statusCode != 204) throw StateError('FIXTURE_UNAVAILABLE');
      await response.drain<void>();
    } finally {
      client.close(force: true);
    }
  }

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
        await runtimeA.upload(jobId);
        final payload = Uint8List.fromList(
          List.generate(2 * 1024 * 1024, (index) => index % 251),
        );
        final fileId = await runtimeA.offer(
          payload,
          'sample.bin',
          Map<String, dynamic>.from(peer),
        );
        await runtimeA.upload(fileId);
        await runtimeB.syncIncoming();
        final file = (await runtimeB.store.jobs()).firstWhere(
          (j) => j['name'] == 'sample.bin',
        );
        await runtimeB.respond(file['id'], true);
        await slowBlobs(40);
        final operation = runtimeB
            .receive(file['id'])
            .then((_) => true, onError: (Object _) => false);
        final deadline = DateTime.now().add(const Duration(seconds: 15));
        while (((await runtimeB.store.job(file['id']))?['received'] as int? ??
                    0) ==
                0 &&
            DateTime.now().isBefore(deadline)) {
          await Future<void>.delayed(const Duration(milliseconds: 20));
        }
        expect(
          (await runtimeB.store.job(file['id']))!['received'],
          greaterThan(0),
        );
        await runtimeB.control(file['id'], 'pause');
        expect(await operation, isFalse);
        final paused = (await runtimeB.store.job(file['id']))!;
        expect(paused['state'], 'ACCEPTED');
        expect(paused['paused'], isTrue);
        await runtimeB.syncIncoming();
        expect((await runtimeB.store.job(file['id']))!['paused'], isTrue);
        await slowBlobs(0);
        await runtimeB.control(file['id'], 'resume');
        await runtimeB.receive(file['id']);
        expect(await runtimeB.plaintext(file['id']), payload);
        final emptyId = await runtimeA.offer(
          Uint8List(0),
          'empty.bin',
          Map<String, dynamic>.from(peer),
        );
        await runtimeA.upload(emptyId);
        await runtimeB.syncIncoming();
        final empty = (await runtimeB.store.jobs()).firstWhere(
          (j) => j['name'] == 'empty.bin',
        );
        await runtimeB.respond(empty['id'], true);
        await runtimeB.receive(empty['id']);
        expect(await runtimeB.plaintext(empty['id']), isEmpty);
        final stored = (await runtimeA.store.job(jobId))!['progress'];
        expect(stored['completion']['stored'], isTrue);
        expect(stored['completion']['delivered'], isFalse);
        await runtimeB.syncIncoming();
        final incoming = (await runtimeB.store.jobs()).firstWhere(
          (j) => j['contentKind'] == 'TEXT',
        );
        expect(incoming['state'], 'PENDING');
        await expectLater(runtimeB.receive(incoming['id']), throwsStateError);
        await runtimeB.respond(incoming['id'], true);
        await runtimeB.control(incoming['id'], 'pause');
        await expectLater(runtimeB.receive(incoming['id']), throwsStateError);
        await runtimeB.control(incoming['id'], 'resume');
        await runtimeB.receive(incoming['id']);
        final received = (await runtimeB.store.job(incoming['id']))!;
        expect(received['state'], 'COMPLETED');
        expect((await runtimeB.text(incoming['id']))?.value, 'private text');
        expect((await runtimeB.text(incoming['id']))?.sensitive, isTrue);
        expect((await runtimeA.text(jobId))?.value, 'private text');
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
        await runtimeA.syncIncoming();
        expect((await runtimeA.store.job(jobId))!['state'], 'COMPLETED');
        // Simulate a crash after the server receipt and before the local
        // completion transaction. Recovery must re-verify local ciphertext.
        await runtimeB.store.update(incoming['id'], received['localRevision'], {
          'state': 'ACCEPTED',
        });
        await runtimeB.receive(incoming['id']);
        expect(
          (await runtimeB.store.job(incoming['id']))!['state'],
          'COMPLETED',
        );
        await runtimeA.upload(jobId);
        final closingId = await runtimeA.offer(
          payload,
          'closing.bin',
          Map<String, dynamic>.from(peer),
        );
        await runtimeA.upload(closingId);
        await runtimeB.syncIncoming();
        final closing = (await runtimeB.store.jobs()).firstWhere(
          (j) => j['name'] == 'closing.bin',
        );
        await runtimeB.respond(closing['id'], true);
        await slowBlobs(40);
        final interrupted = runtimeB
            .receive(closing['id'])
            .then((_) => true, onError: (Object _) => false);
        final closeDeadline = DateTime.now().add(const Duration(seconds: 15));
        while (((await runtimeB.store.job(closing['id']))?['received']
                        as int? ??
                    0) ==
                0 &&
            DateTime.now().isBefore(closeDeadline)) {
          await Future<void>.delayed(const Duration(milliseconds: 20));
        }
        expect(
          (await runtimeB.store.job(closing['id']))!['received'],
          greaterThan(0),
        );
        await Future.wait([runtimeB.close(), runtimeB.close()]);
        expect(await interrupted, isFalse);
        await slowBlobs(0);
        await runtimeA.control(closingId, 'cancel');
        await a.remove(peer['pair_id']);
        expect((await a.list())['peers'], isEmpty);
      } finally {
        await runtimeA.close();
        await runtimeB.close();
      }
    },
    skip: server == null ? 'requires isolated native guest test API' : false,
  );

  test(
    'recipient can retrieve a sealed cloud copy after the sender exits',
    () async {
      final sender = await NativeGuestRuntime.open(
        server: server!,
        vault: _Vault(),
        database: ClientDatabase.testing(NativeDatabase.memory()),
      );
      final recipient = await NativeGuestRuntime.open(
        server: server,
        vault: _Vault(),
        database: ClientDatabase.testing(NativeDatabase.memory()),
      );
      try {
        await sender.session.ensureSession();
        await recipient.session.ensureSession();
        final invite = await sender.connections.invite();
        final candidate = await recipient.connections.join(invite['code']);
        await sender.connections.respond(candidate['id'], true);
        final peer =
            ((await sender.connections.list())['peers'] as List).single
                as Map<String, dynamic>;
        final payload = Uint8List.fromList(
          List.generate(65537, (index) => index % 251),
        );
        final id = await sender.offer(payload, 'sender-exits.bin', peer);
        await sender.upload(id);
        final stored = await sender.store.job(id);
        expect((stored?['progress'] as Map?)?['completion']?['stored'], isTrue);
        expect(
          (stored?['progress'] as Map?)?['completion']?['delivered'],
          isFalse,
        );
        await sender.close();
        await recipient.syncIncoming();
        final incoming = (await recipient.store.jobs()).single;
        expect(incoming['state'], 'PENDING');
        await recipient.respond(incoming['id'], true);
        await recipient.receive(incoming['id']);
        expect(await recipient.plaintext(incoming['id']), payload);
      } finally {
        await sender.close();
        await recipient.close();
      }
    },
    skip: server == null ? 'requires isolated native guest test API' : false,
  );
}
