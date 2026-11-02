import 'dart:convert';

import 'package:drift/drift.dart';
// Drift's local-isolate error wrapper is public; its wire protocol is experimental.
// ignore: experimental_member_use
import 'package:drift/remote.dart' show DriftRemoteException;
import 'package:drift_flutter/drift_flutter.dart';
import 'package:flutter/foundation.dart';
import 'package:sqlite3/common.dart' show SqliteException;

part 'database.g.dart';

class ClientMetadata extends Table {
  TextColumn get key => text()();
  TextColumn get body => text()();
  @override
  Set<Column> get primaryKey => {key};
}

@DataClassName('PeerRow')
class ConfirmedPeers extends Table {
  TextColumn get root => text()();
  TextColumn get signingKey => text()();
  TextColumn get encryptionKey => text()();
  BoolColumn get revoked => boolean().withDefault(const Constant(false))();
  @override
  Set<Column> get primaryKey => {root};
}

@DataClassName('InboxRow')
class ReceivedHistory extends Table {
  TextColumn get id => text()();
  TextColumn get hash => text()();
  TextColumn get body => text()();
  TextColumn get status => text()();
  @override
  Set<Column> get primaryKey => {id};
}

@DataClassName('HistoryRow')
class HistoryProjection extends Table {
  TextColumn get id => text()();
  TextColumn get aggregateId => text()();
  TextColumn get authority => text()();
  TextColumn get source => text()();
  TextColumn get kind => text()();
  IntColumn get version => integer()();
  TextColumn get body => text()();
  @override
  Set<Column> get primaryKey => {id};
}

@DataClassName('ResponseRow')
class ResponseCache extends Table {
  TextColumn get path => text()();
  TextColumn get body => text()();
  IntColumn get updated => integer()();
  @override
  Set<Column> get primaryKey => {path};
}

class PreparedHistory {
  PreparedHistory(this.id, this.hash, this.event, this.fact, this.trustedKey);
  final String id, hash;
  final Map<String, dynamic> event;
  final Map<String, dynamic>? fact;
  final String? trustedKey;
}

@DriftDatabase(
  tables: [
    ClientMetadata,
    ConfirmedPeers,
    ReceivedHistory,
    HistoryProjection,
    ResponseCache,
  ],
)
class ClientDatabase extends _$ClientDatabase {
  ClientDatabase(String name)
    : super(
        driftDatabase(
          name: name,
          native: const DriftNativeOptions(shareAcrossIsolates: true),
          web: DriftWebOptions(
            sqlite3Wasm: Uri.parse('sqlite3.wasm'),
            driftWorker: Uri.parse('drift_worker.js'),
          ),
        ),
      );
  ClientDatabase.testing(super.executor);
  @override
  Future<T> transaction<T>(
    Future<T> Function() action, {
    bool requireNew = false,
  }) async {
    final elapsed = Stopwatch()..start();
    for (var attempt = 0; ; attempt++) {
      var entered = false;
      try {
        return await super.transaction(() {
          entered = true;
          return action();
        }, requireNew: requireNew);
      } catch (error) {
        final cause = error is DriftRemoteException ? error.remoteCause : error;
        // Another process may hold the writer past SQLite's busy timeout.
        // Yield before trying BEGIN again; never replay an action or commit.
        if (entered ||
            cause is! SqliteException ||
            cause.resultCode != 5 ||
            cause.causingStatement != 'BEGIN IMMEDIATE' ||
            attempt >= 5 ||
            elapsed.elapsed >= const Duration(seconds: 10)) {
          rethrow;
        }
        await Future<void>.delayed(
          Duration(milliseconds: 20 << (attempt > 3 ? 3 : attempt)),
        );
      }
    }
  }

  @override
  int get schemaVersion => 1;
  @override
  MigrationStrategy get migration => MigrationStrategy(
    onCreate: (m) => m.createAll(),
    beforeOpen: (_) async {
      await customStatement('PRAGMA foreign_keys=ON');
      if (!kIsWeb) {
        await customStatement('PRAGMA journal_mode=WAL');
        await customStatement('PRAGMA synchronous=FULL');
        await customStatement('PRAGMA busy_timeout=5000');
      }
    },
  );
  Future<Map<String, dynamic>?> metadata(String key) async {
    final row = await (select(
      clientMetadata,
    )..where((r) => r.key.equals(key))).getSingleOrNull();
    return row == null ? null : jsonDecode(row.body) as Map<String, dynamic>;
  }

  Future<void> putMetadata(String key, Map<String, dynamic> value) =>
      into(clientMetadata).insertOnConflictUpdate(
        ClientMetadataCompanion.insert(key: key, body: jsonEncode(value)),
      );
  Future<Map<String, dynamic>> target() async =>
      await metadata('cloud') ??
      {
        'epoch': '',
        'cursor': 0,
        'bootstrapped': false,
        'upper': -1,
        'key': '',
        'bound': '',
        'rosterVersion': 0,
        'rosterHash': '',
        'failures': 0,
        'retryAt': 0,
        'error': '',
        'offset': 0,
      };
  Future<void> patchTarget(Map<String, dynamic> fields) =>
      transaction(() async {
        await _reserveWriter();
        await putMetadata('cloud', {...await target(), ...fields});
      });
  // Obtain the SQLite writer before reading a value that will be changed. This
  // also avoids deferred-read -> write upgrade races between connections.
  Future<void> _reserveWriter() => customStatement(
    "INSERT INTO client_metadata (key, body) VALUES ('cache_order', '{}') "
    'ON CONFLICT(key) DO UPDATE SET body = client_metadata.body',
  );
  Future<int> _advanceCacheClock({bool mutation = false}) async {
    final clock = await metadata('cache_order') ?? {};
    final latest =
        await (select(responseCache)
              ..orderBy([(r) => OrderingTerm.desc(r.updated)])
              ..limit(1))
            .getSingleOrNull();
    final revision =
        [
          clock['next'] as int? ?? 0,
          latest?.updated ?? 0,
          DateTime.now().millisecondsSinceEpoch,
        ].reduce((a, b) => a > b ? a : b) +
        1;
    if (revision > 9007199254740991) throw StateError('CACHE_CLOCK_EXHAUSTED');
    await putMetadata('cache_order', {
      ...clock,
      'next': revision,
      if (mutation) 'barrier': revision,
    });
    return revision;
  }

  Future<int> beginRead() => transaction(() async {
    await _reserveWriter();
    return _advanceCacheClock();
  });
  Future<void> reset(String epoch) => patchTarget({
    'epoch': epoch,
    'cursor': 0,
    'bootstrapped': false,
    'upper': -1,
    'key': '',
  });
  Future<Map<String, PeerRow>> peers() async => {
    for (final p in await select(confirmedPeers).get()) p.root: p,
  };
  Future<void> apply(
    String ownRoot,
    Map<String, dynamic> page,
    List<PreparedHistory> rows,
  ) => transaction(() async {
    await _reserveWriter();
    if ((await metadata('profile'))?['root'] != ownRoot) {
      throw StateError('PROFILE_CHANGED');
    }
    final current = await target();
    if (current['epoch'] != '' && current['epoch'] != page['epoch']) {
      throw StateError('EPOCH_CHANGED');
    }
    final confirmed = await peers();
    for (final row in rows) {
      final old = await (select(
        receivedHistory,
      )..where((r) => r.id.equals(row.id))).getSingleOrNull();
      if (old != null && old.hash != row.hash) {
        throw StateError('EVENT_CONFLICT');
      }
      if (old?.status == 'APPLIED') continue;
      final e = row.event;
      final p = confirmed[e['source_root']];
      final trusted =
          e['authority'] == 'SERVER' ||
          e['source_root'] == ownRoot ||
          (p != null && !p.revoked && p.signingKey == row.trustedKey);
      final applied = trusted && row.fact != null;
      await into(receivedHistory).insertOnConflictUpdate(
        ReceivedHistoryCompanion.insert(
          id: row.id,
          hash: row.hash,
          body: jsonEncode(e),
          status: applied ? 'APPLIED' : 'IGNORED_UNTRUSTED',
        ),
      );
      if (applied) {
        final id = [
          e['authority_root'],
          e['aggregate_id'],
          e['source_root'],
          e['kind'],
        ].join('|');
        final previous = await (select(
          historyProjection,
        )..where((r) => r.id.equals(id))).getSingleOrNull();
        if (previous == null ||
            (e['aggregate_version'] as int) > previous.version) {
          await into(historyProjection).insertOnConflictUpdate(
            HistoryProjectionCompanion.insert(
              id: id,
              aggregateId: e['aggregate_id'],
              authority: e['authority_root'],
              source: e['source_root'],
              kind: e['kind'],
              version: e['aggregate_version'],
              body: jsonEncode(row.fact),
            ),
          );
        }
      }
    }
    if (page['snapshot'] == true &&
        current['bootstrapped'] == true &&
        current['epoch'] == page['epoch']) {
      return;
    }
    final next = {...current, 'epoch': page['epoch']};
    if (page['snapshot'] == true && page['more'] == true) {
      next['upper'] = page['upper'];
      next['key'] = page['next_key'];
    } else {
      next['cursor'] = (current['cursor'] as int) > (page['through'] as int)
          ? current['cursor']
          : page['through'];
      next['bootstrapped'] = true;
      next['upper'] = -1;
      next['key'] = '';
    }
    await putMetadata('cloud', next);
  });
  Future<bool> cacheResponse(
    String path,
    Map<String, dynamic> value,
    int started,
  ) => transaction(() async {
    await _reserveWriter();
    final clock = await metadata('cache_order') ?? {};
    final old = await (select(
      responseCache,
    )..where((r) => r.path.equals(path))).getSingleOrNull();
    if (started < (clock['barrier'] as int? ?? 0) ||
        (old != null && old.updated > started)) {
      return false;
    }
    await into(responseCache).insertOnConflictUpdate(
      ResponseCacheCompanion.insert(
        path: path,
        body: jsonEncode(value),
        updated: started,
      ),
    );
    return true;
  });
  Future<Map<String, dynamic>?> cached(String path) async {
    final row = await (select(
      responseCache,
    )..where((r) => r.path.equals(path))).getSingleOrNull();
    return row == null ? null : jsonDecode(row.body) as Map<String, dynamic>;
  }

  Stream<String> watchResponses() =>
      (select(responseCache)..orderBy([(r) => OrderingTerm.asc(r.path)]))
          .watch()
          .map((rows) => rows.map((r) => '${r.path}\n${r.body}').join('\n'))
          .distinct();
  Future<void> cacheMutation(Map<String, dynamic> value) => transaction(
    () async {
      await _reserveWriter();
      final revision = await _advanceCacheClock(mutation: true);
      if (value['id'] == null) return;
      for (final row in await select(responseCache).get()) {
        final next = jsonDecode(row.body) as Map<String, dynamic>;
        var changed = false;
        for (final key in ['tasks', 'transfers', 'devices']) {
          if (next[key] is! List) continue;
          next[key] = (next[key] as List).expand((dynamic item) {
            if (item['id'] != value['id'] ||
                (item['version'] as num? ?? 0) >
                    (value['version'] as num? ?? 0)) {
              return [item];
            }
            changed = true;
            return value['state'] == 'DELETED' &&
                    row.path.contains('include_deleted=false')
                ? <dynamic>[]
                : [value];
          }).toList();
        }
        if (next['id'] == value['id'] &&
            (next['version'] as num? ?? 0) <= (value['version'] as num? ?? 0)) {
          next.addAll(value);
          changed = true;
        }
        if (changed) {
          await into(responseCache).insertOnConflictUpdate(
            ResponseCacheCompanion.insert(
              path: row.path,
              body: jsonEncode(next),
              updated: revision,
            ),
          );
        }
      }
    },
  );
}
