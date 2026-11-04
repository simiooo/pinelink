import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/io_client.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/guest/coordinator.dart';
import 'package:pinelink_mobile/src/guest/runtime.dart';

class _Vault implements IdentityVault {
  final values = <String, String>{};
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

class _Network extends http.BaseClient {
  final client = IOClient();
  bool available = true;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) => available
      ? client.send(request)
      : Future.error(http.ClientException('unavailable'));
  @override
  void close() => client.close();
}

Future<void> until(Future<bool> Function() check) async {
  final deadline = DateTime.now().add(const Duration(seconds: 30));
  while (!await check()) {
    if (DateTime.now().isAfter(deadline)) throw StateError('TEST_TIMEOUT');
    await Future<void>.delayed(const Duration(milliseconds: 100));
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() => HttpOverrides.global = null);
  final server = Platform.environment['PINELINK_NATIVE_GUEST_TEST_API'];

  test('coordinator pairs, preserves offline history, and transfers only after consent', () async {
    final directory = await Directory.systemTemp.createTemp(
      'pinelink-guest-test-',
    );
    final databaseFile = File('${directory.path}/guest.sqlite');
    final vaultA = _Vault();
    final networkA = _Network(), networkB = _Network();
    final runtimeA = await NativeGuestRuntime.open(
      server: server!,
      vault: vaultA,
      database: ClientDatabase.testing(NativeDatabase(databaseFile)),
      client: networkA,
    );
    final runtimeB = await NativeGuestRuntime.open(
      server: server,
      vault: _Vault(),
      database: ClientDatabase.testing(NativeDatabase.memory()),
      client: networkB,
    );
    final a = await NativeGuestCoordinator.open(runtimeA, autoPoll: false);
    final b = await NativeGuestCoordinator.open(runtimeB, autoPoll: false);
    try {
      await a.poll();
      await b.poll();
      final invite = await a.invite();
      expect((await a.invite())['id'], invite['id']);
      final candidate = await b.join(invite['code']);
      await a.poll();
      expect(a.peers, isEmpty);
      expect(a.candidates.single['id'], candidate['id']);
      await a.respondCandidate(candidate['id'], true);
      await a.poll();
      await b.poll();
      expect(a.peers, hasLength(1));
      expect(b.peers, hasLength(1));
      final peerA = a.peers.single;
      await a.rename(peerA['pair_id'], 'My phone');
      await a.poll();
      expect(a.peers.single['local_name'], 'My phone');

      final payload = Uint8List.fromList(List.generate(131072, (i) => i % 251));
      final outgoing = await a.send(payload, 'sample.bin', peerA);
      await until(() async {
        await a.poll();
        final job = await runtimeA.store.job(outgoing);
        return (job?['progress'] as Map?)?['completion']?['stored'] == true;
      });
      await until(() async {
        await b.poll();
        return b.jobs.any((job) => job['name'] == 'sample.bin');
      });
      final incoming = b.jobs.firstWhere((job) => job['name'] == 'sample.bin');
      expect(incoming['state'], 'PENDING');
      expect(incoming['plainSize'], isNull);
      expect(await runtimeB.store.bytes(incoming['id'], 'received'), isNull);
      await b.action(incoming['id'], 'accept');
      await until(() async {
        await b.poll();
        return (await runtimeB.store.job(incoming['id']))?['state'] ==
            'COMPLETED';
      });
      expect(await runtimeB.plaintext(incoming['id']), payload);
      expect(
        (await runtimeB.store.job(incoming['id']))?['plainSize'],
        payload.length,
      );
      await until(() async {
        await a.poll();
        return (await runtimeA.store.job(outgoing))?['state'] == 'COMPLETED';
      });

      final sentText = await b.send(
        Uint8List.fromList(utf8.encode('private **text**')),
        'Text',
        b.peers.single,
        sensitivity: 'SENSITIVE',
      );
      await until(() async {
        await a.poll();
        return a.jobs.any((job) => job['contentKind'] == 'TEXT');
      });
      final receivedText = a.jobs.firstWhere(
        (job) => job['contentKind'] == 'TEXT',
      );
      expect(receivedText['state'], 'PENDING');
      await a.action(receivedText['id'], 'accept');
      await until(() async {
        await a.poll();
        return (await runtimeA.store.job(receivedText['id']))?['state'] ==
            'COMPLETED';
      });
      expect(
        (await runtimeA.text(receivedText['id']))?.value,
        'private **text**',
      );
      expect((await runtimeA.text(receivedText['id']))?.sensitive, isTrue);
      expect((await runtimeB.text(sentText))?.value, 'private **text**');

      networkA.available = false;
      await a.poll();
      expect(a.online, isFalse);
      expect(a.peers.single['local_name'], 'My phone');
      expect(a.peers.single['online'], isFalse);
      expect(a.jobs, isNotEmpty);
      expect(
        (await runtimeA.text(receivedText['id']))?.value,
        'private **text**',
      );
      await expectLater(
        a.send(Uint8List(0), 'offline.bin', peerA),
        throwsStateError,
      );
      await a.close();
      expect(
        latin1
            .decode(await databaseFile.readAsBytes())
            .contains('private **text**'),
        isFalse,
      );
      final offlineNetwork = _Network()..available = false;
      final reopenedRuntime = await NativeGuestRuntime.open(
        server: server,
        vault: vaultA,
        database: ClientDatabase.testing(NativeDatabase(databaseFile)),
        client: offlineNetwork,
      );
      final reopened = await NativeGuestCoordinator.open(
        reopenedRuntime,
        autoPoll: false,
      );
      try {
        expect(reopened.online, isFalse);
        expect(reopened.peers.single['local_name'], 'My phone');
        expect(
          (await reopenedRuntime.text(receivedText['id']))?.value,
          'private **text**',
        );
      } finally {
        await reopened.close();
      }
    } finally {
      await a.close();
      await b.close();
      await directory.delete(recursive: true);
    }
  }, skip: server == null ? 'requires isolated native guest test API' : false);
}
