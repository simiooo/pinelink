import 'dart:convert';
import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/data/runtime.dart';
import 'package:pinelink_mobile/src/data/wire.dart';

class MemoryVault implements IdentityVault {
  final _values = <String, String>{};
  @override
  Future<String?> read(String key) async => _values[key];
  @override
  Future<void> write(String key, String value) async {
    _values[key] = value;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final binary = File(
    '../.artifacts/historyfixture${Platform.isWindows ? '.exe' : ''}',
  ).absolute.path;
  setUpAll(() async {
    await Directory('../.artifacts').create(recursive: true);
    final result = await Process.run('go', [
      'build',
      '-o',
      binary,
      './cmd/historyfixture',
    ], workingDirectory: '../cli');
    if (result.exitCode != 0) {
      throw StateError('fixture build failed: ${result.stderr}');
    }
  });
  Future<Map<String, dynamic>> fixture(LocalIdentity identity) async {
    final child = await Process.start(binary, []);
    child.stdin.write(
      jsonEncode({
        'root': identity.root,
        'signing_key': identity.signingKey,
        'encryption_key': identity.encryptionKey,
      }),
    );
    await child.stdin.close();
    final output = child.stdout.transform(utf8.decoder).join();
    final errors = child.stderr.transform(utf8.decoder).join();
    final code = await child.exitCode;
    if (code != 0) {
      await errors;
      throw StateError('fixture generation failed');
    }
    return jsonDecode(await output) as Map<String, dynamic>;
  }

  Future<void> trust(ClientDatabase db, Map<String, dynamic> peer) => db
      .into(db.confirmedPeers)
      .insertOnConflictUpdate(
        ConfirmedPeersCompanion.insert(
          root: peer['root'],
          signingKey: peer['signing_key'],
          encryptionKey: peer['encryption_key'],
        ),
      );

  test('real SQLite reopen preserves identity, verified Go age facts and cursor', () async {
    final directory = await Directory.systemTemp.createTemp('pinelink-db-');
    addTearDown(() {
      final target = directory.absolute.path;
      if (!target.startsWith(
        '${Directory.systemTemp.absolute.path}${Platform.pathSeparator}pinelink-db-',
      )) {
        throw StateError('unexpected test directory');
      }
      return directory.delete(recursive: true);
    });
    final file = File('${directory.path}/history.sqlite');
    var db = ClientDatabase.testing(NativeDatabase(file));
    final vault = MemoryVault();
    final identity = await LocalIdentity.load(db, vault, 'test', '');
    final vector = await fixture(identity);
    await trust(db, vector['peer']);
    final page = vector['page'] as Map<String, dynamic>;
    final prepared = await preparePage(
      identity,
      page,
      vector['pin'],
      await db.peers(),
    );
    expect(prepared.first.fact?['file_name'], '本地 <>&.fig');
    await db.apply(identity.root, page, prepared);
    await db.apply(identity.root, page, prepared);
    final stale = vector['older_snapshot'] as Map<String, dynamic>;
    await db.apply(
      identity.root,
      stale,
      await preparePage(identity, stale, vector['pin'], await db.peers()),
    );
    expect((await db.target())['cursor'], 2);
    await expectLater(
      preparePage(
        identity,
        vector['bad_receipt_page'],
        vector['pin'],
        await db.peers(),
      ),
      throwsStateError,
    );
    final completion = vector['completion_page'] as Map<String, dynamic>;
    await db.apply(
      identity.root,
      completion,
      await preparePage(identity, completion, vector['pin'], await db.peers()),
    );
    await db.close();
    db = ClientDatabase.testing(NativeDatabase(file));
    final reopened = await LocalIdentity.load(db, vault, 'test', '');
    expect(reopened.root, identity.root);
    expect(reopened.stream, identity.stream);
    expect((await db.select(db.historyProjection).get()).length, 2);
    expect((await db.target())['cursor'], 3);
    final journal = await db.customSelect('PRAGMA journal_mode').getSingle();
    expect(journal.data.values.first, 'wal');
    await expectLater(
      LocalIdentity.load(db, MemoryVault(), 'test', ''),
      throwsStateError,
    );
    await db.close();
  });
  test('cursor failure rolls back all data; revoked peers cannot apply prepared plaintext', () async {
    final db = ClientDatabase.testing(NativeDatabase.memory());
    addTearDown(db.close);
    final identity = await LocalIdentity.load(db, MemoryVault(), 'test', '');
    final vector = await fixture(identity);
    await trust(db, vector['peer']);
    final page = vector['page'] as Map<String, dynamic>;
    final rows = await preparePage(
      identity,
      page,
      vector['pin'],
      await db.peers(),
    );
    await db.customStatement(
      "CREATE TRIGGER fail_cursor BEFORE INSERT ON client_metadata WHEN NEW.key='cloud' BEGIN SELECT RAISE(ABORT,'injected cursor failure'); END;",
    );
    await expectLater(db.apply(identity.root, page, rows), throwsA(anything));
    expect(await db.select(db.receivedHistory).get(), isEmpty);
    expect(await db.select(db.historyProjection).get(), isEmpty);
    await db.customStatement('DROP TRIGGER fail_cursor');
    await db.customStatement('UPDATE confirmed_peers SET revoked=1');
    await db.apply(identity.root, page, rows);
    expect((await db.select(db.historyProjection).get()).length, 1);
    await db.customStatement('UPDATE confirmed_peers SET revoked=0');
    await db.apply(identity.root, page, rows);
    expect((await db.select(db.historyProjection).get()).length, 2);
    await expectLater(
      preparePage(
        identity,
        vector['unsafe_page'],
        vector['pin'],
        await db.peers(),
      ),
      throwsStateError,
    );
  });
  test('offline cache survives restart and explicit authorization errors still fail', () async {
    final db = ClientDatabase.testing(NativeDatabase.memory());
    final vault = MemoryVault();
    var offline = true;
    final runtime = await ClientRuntime.open(
      server: 'https://test.invalid',
      owner: 'owner',
      database: db,
      vault: vault,
      autoStart: false,
      client: MockClient((request) async {
        if (offline) throw http.ClientException('offline');
        return http.Response(jsonEncode({'code': 'UNAUTHORIZED'}), 401);
      }),
    );
    await db.patchTarget({'bound': 'API_ONLY'});
    final path =
        '/v1/tasks?owner_id=owner&device_id=${runtime.identity.device}&include_deleted=true';
    await db.cacheResponse(path, {
      'tasks': [
        {'id': 'task', 'version': 1, 'state': 'RUNNING'},
      ],
    }, 1);
    final cached = await runtime.request('GET', path);
    expect((cached['tasks'] as List).length, 1);
    await db.cacheMutation({'id': 'task', 'version': 2, 'state': 'PAUSED'});
    await db.cacheResponse(path, {
      'tasks': [
        {'id': 'task', 'version': 1, 'state': 'RUNNING'},
      ],
    }, 2);
    expect((await db.cached(path))?['tasks'][0]['state'], 'PAUSED');
    offline = false;
    await expectLater(
      runtime.request('GET', path),
      throwsA(isA<DataException>().having((e) => e.status, 'status', 401)),
    );
    await expectLater(
      runtime.request('POST', '/v1/devices/revoke', {
        'device_id': 'another-device',
      }),
      throwsA(isA<DataException>().having((e) => e.status, 'status', 403)),
    );
    await runtime.close();
  });
}
