import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:drift/drift.dart' show driftRuntimeOptions;
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/data/runtime.dart';
import 'package:pinelink_mobile/src/pages/history_page.dart';

class TestVault implements IdentityVault {
  final values = <String, String>{};
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

class PageApi extends PinelinkApi {
  int reads = 0;
  void emit() => notifyListeners();
  bool get listening => hasListeners;
  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async {
    reads++;
    return [];
  }

  @override
  Future<List<TransferRequest>> getTransfers(
    String owner,
    String device,
  ) async => [];
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const path = '/v1/tasks?include_deleted=true';
  final running = {
    'tasks': [
      {'id': 'task', 'version': 1, 'state': 'RUNNING'},
    ],
  };
  final paused = {'id': 'task', 'version': 2, 'state': 'PAUSED'};
  Future<ClientRuntime> open(http.Client client) async {
    final db = ClientDatabase.testing(NativeDatabase.memory());
    final runtime = await ClientRuntime.open(
      server: 'https://unit.invalid',
      owner: 'owner',
      database: db,
      vault: TestVault(),
      client: client,
      autoStart: false,
    );
    await db.patchTarget({'bound': 'API_ONLY'});
    addTearDown(runtime.close);
    return runtime;
  }

  test(
    'a completed control fences a late read, including the first uncached read',
    () async {
      final entered = Completer<void>(), response = Completer<http.Response>();
      var calls = 0;
      final runtime = await open(
        MockClient((_) {
          if (++calls == 1) {
            entered.complete();
            return response.future;
          }
          return Future.value(
            http.Response(
              jsonEncode({
                'tasks': [paused],
              }),
              200,
            ),
          );
        }),
      );
      final read = runtime.request('GET', path);
      await entered.future;
      await runtime.database.cacheMutation(paused);
      response.complete(http.Response(jsonEncode(running), 200));
      expect(await read, {
        'tasks': [paused],
      });
      expect(calls, 2);
    },
  );

  test('close drains foreground work and rejects late success and subsequent reuse', () async {
    final entered = Completer<void>(), response = Completer<http.Response>();
    final runtime = await open(
      MockClient((_) {
        entered.complete();
        return response.future;
      }),
    );
    final read = runtime.request('GET', path);
    final rejected = expectLater(
      read,
      throwsA(
        isA<DataException>().having((e) => e.code, 'code', 'CLIENT_CLOSED'),
      ),
    );
    await entered.future;
    var finished = false;
    final closing = runtime.close().then((_) {
      finished = true;
    });
    await Future<void>.delayed(Duration.zero);
    expect(finished, isFalse);
    response.complete(http.Response(jsonEncode(running), 200));
    await rejected;
    await closing;
    await expectLater(
      runtime.request('GET', path),
      throwsA(isA<DataException>()),
    );
    expect(runtime.start, throwsA(isA<DataException>()));
    await runtime.close();
  });

  test(
    'a started transaction is rolled back without replaying its action',
    () async {
      final db = ClientDatabase.testing(NativeDatabase.memory());
      addTearDown(db.close);
      var calls = 0;
      await expectLater(
        db.transaction(() async {
          calls++;
          await db.putMetadata('must-rollback', {});
          throw SqliteException(
            extendedResultCode: 5,
            message: 'busy after BEGIN',
          );
        }),
        throwsA(isA<SqliteException>()),
      );
      expect(calls, 1);
      expect(await db.metadata('must-rollback'), isNull);
    },
  );

  test('cache order is persistent and serial across separate SQLite connections', () async {
    final directory = await Directory.systemTemp.createTemp('pinelink-race-');
    addTearDown(() async {
      if (!directory.absolute.path.startsWith(
        '${Directory.systemTemp.absolute.path}${Platform.pathSeparator}pinelink-race-',
      )) {
        throw StateError('unexpected test directory');
      }
      await directory.delete(recursive: true);
    });
    final file = File('${directory.path}/cache.sqlite');
    // Deliberately use independent background executors, never share an executor
    // between database instances. Drift's debug warning counts classes only.
    final warn = driftRuntimeOptions.dontWarnAboutMultipleDatabases;
    driftRuntimeOptions.dontWarnAboutMultipleDatabases = true;
    final a = ClientDatabase.testing(NativeDatabase.createInBackground(file));
    final b = ClientDatabase.testing(NativeDatabase.createInBackground(file));
    driftRuntimeOptions.dontWarnAboutMultipleDatabases = warn;
    try {
      await a.target();
      await b.target();
      // Force a real SQLITE_BUSY at BEGIN on the independent connection. The
      // operation must wait asynchronously and acquire a fresh transaction.
      await b.customStatement('PRAGMA busy_timeout=1');
      final locked = Completer<void>(), release = Completer<void>();
      final holder = a.transaction(() async {
        await a.putMetadata('held-writer', {});
        locked.complete();
        await release.future;
      });
      await locked.future;
      final waiting = b.beginRead();
      final timer = Timer(const Duration(milliseconds: 80), release.complete);
      try {
        await waiting;
      } finally {
        if (!release.isCompleted) release.complete();
        timer.cancel();
        await holder;
      }
      await b.customStatement('PRAGMA busy_timeout=5000');
      // Seed a legacy entry ahead of wall time to model clock rollback.
      final future = DateTime.now().millisecondsSinceEpoch + 100000;
      await a.cacheResponse(path, running, future);
      final tokens = await Future.wait(
        List.generate(16, (i) => (i.isEven ? a : b).beginRead()),
      );
      expect(tokens.toSet().length, 16);
      expect(tokens.every((t) => t > future), isTrue);
      tokens.sort();
      await b.cacheResponse(path, {
        'tasks': [paused],
      }, tokens.last);
      expect(await a.cacheResponse(path, running, tokens.first), isFalse);
      await a.cacheMutation(paused);
      expect(await b.cacheResponse(path, running, tokens.last), isFalse);
      expect(await a.cached(path), {
        'tasks': [paused],
      });
    } finally {
      await a.close();
      await b.close();
    }
  });

  test(
    'API initialization is shared, retryable, and cannot start after close',
    () async {
      final runtime = await open(
        MockClient((_) async => http.Response('{}', 200)),
      );
      var calls = 0;
      final gate = Completer<ClientRuntime>();
      final api = PinelinkApi(
        openRuntime: () {
          calls++;
          return gate.future;
        },
      );
      final first = api.runtime, second = api.runtime;
      expect(identical(first, second), isTrue);
      final rejected = expectLater(first, throwsStateError);
      final closing = api.close();
      gate.complete(runtime);
      await rejected;
      await closing;
      await expectLater(api.runtime, throwsStateError);
      expect(calls, 1);
      api.dispose();
      final cold = PinelinkApi(
        openRuntime: () {
          calls++;
          return Future.value(runtime);
        },
      );
      await cold.close();
      await expectLater(cold.runtime, throwsStateError);
      expect(calls, 1);
      cold.dispose();
    },
  );

  test('a failed API open can be retried', () async {
    final runtime = await open(
      MockClient((_) async => http.Response('{}', 200)),
    );
    var calls = 0;
    final api = PinelinkApi(
      openRuntime: () async {
        if (++calls == 1) throw StateError('unavailable');
        return runtime;
      },
    );
    await expectLater(api.runtime, throwsStateError);
    expect(await api.runtime, runtime);
    await api.close();
    api.dispose();
  });

  testWidgets(
    'replacing a page API releases its old listener and ignores stale loads',
    (tester) async {
      final a = PageApi(), b = PageApi();
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: HistoryPage(api: a)),
        ),
      );
      await tester.pumpAndSettle();
      expect(a.listening, isTrue);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: HistoryPage(api: b)),
        ),
      );
      await tester.pumpAndSettle();
      expect(a.listening, isFalse);
      expect(b.listening, isTrue);
      final reads = b.reads;
      a.emit();
      await tester.pumpAndSettle();
      expect(b.reads, reads);
      b.emit();
      await tester.pumpAndSettle();
      expect(b.reads, reads + 1);
      await tester.pumpWidget(const SizedBox());
      expect(b.listening, isFalse);
      a.dispose();
      b.dispose();
    },
  );
}
