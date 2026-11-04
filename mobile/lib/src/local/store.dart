import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/drift.dart';

import '../data/database.dart';
import '../data/identity.dart';
import 'controller.dart';
import 'protocol.dart';

class LocalStore {
  LocalStore(this.db, {String? owner}) : owner = owner ?? randomUuid();
  final ClientDatabase db;
  final String owner;
  int get now => DateTime.now().millisecondsSinceEpoch;
  Future<void> initialize() async {
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS local_jobs (id TEXT PRIMARY KEY, body TEXT NOT NULL)',
    );
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS local_bytes (job TEXT NOT NULL, kind TEXT NOT NULL, idx INTEGER NOT NULL, bytes BLOB NOT NULL, PRIMARY KEY(job,kind,idx))',
    );
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS local_journal (id TEXT PRIMARY KEY, body TEXT NOT NULL, uploaded INTEGER NOT NULL DEFAULT 0)',
    );
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS local_leases (root TEXT PRIMARY KEY, owner TEXT NOT NULL, session TEXT NOT NULL, until_ms INTEGER NOT NULL)',
    );
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS local_used (id TEXT PRIMARY KEY, until_ms INTEGER NOT NULL)',
    );
    await db.customStatement('DELETE FROM local_used WHERE until_ms < ?', [
      now,
    ]);
  }

  Future<T> transaction<T>(Future<T> Function() fn) => db.transaction(() async {
    await db.putMetadata('local-write-reservation', {'schema': 1});
    return fn();
  });
  Future<Json?> job(String id) async {
    final rows = await db
        .customSelect(
          'SELECT body FROM local_jobs WHERE id = ?',
          variables: [Variable.withString(id)],
        )
        .get();
    return rows.isEmpty
        ? null
        : jsonDecode(rows.first.read<String>('body')) as Json;
  }

  Future<List<Json>> jobs() async =>
      (await db.customSelect('SELECT body FROM local_jobs').get())
          .map((r) => jsonDecode(r.read<String>('body')) as Json)
          .toList()
        ..sort((a, b) => (b['created'] as int).compareTo(a['created'] as int));
  Future<void> put(Json job) => db.customStatement(
    'INSERT INTO local_jobs (id,body) VALUES (?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body',
    [job['id'], jsonEncode(job)],
  );
  Future<void> journal(Json state) => db.customStatement(
    'INSERT OR IGNORE INTO local_journal (id,body) VALUES (?,?)',
    [
      '${state['recipient']}:${state['id']}:${state['version']}:${state['revision']}',
      jsonEncode({
        'authority': 'LOCAL_DEVICE',
        'authority_device_id': state['recipient'],
        'transfer': state['id'],
        'snapshot': state,
      }),
    ],
  );
  Future<List<Json>> peers() async =>
      List<Json>.from((await db.metadata('local-peers'))?['items'] ?? []);
  Future<void> savePeer(Json peer) => transaction(() async {
    final list = await peers(),
        old = list.where((p) => p['root'] == peer['root']).firstOrNull;
    if (old != null &&
        (old['signing_key'] != peer['signing_key'] ||
            old['encryption_key'] != peer['encryption_key'])) {
      bad('LOCAL_IDENTITY_CHANGED');
    }
    list.removeWhere((p) => p['root'] == peer['root']);
    list.add({
      ...peer,
      if (old?['local_name'] != null) 'local_name': old!['local_name'],
    });
    await db.putMetadata('local-peers', {'items': list});
  });
  Future<void> rename(String root, String name) => transaction(() async {
    final list = await peers(),
        peer = list.where((p) => p['root'] == root).firstOrNull;
    if (peer == null) bad('CONNECTION_GONE');
    peer['local_name'] = name;
    await db.putMetadata('local-peers', {'items': list});
  });
  Future<void> remove(String root) => transaction(() async {
    final list = (await peers())..removeWhere((p) => p['root'] == root);
    await db.putMetadata('local-peers', {'items': list});
  });
  Future<void> reserve(String root, String session) => transaction(() async {
    final rows = await db
        .customSelect(
          'SELECT * FROM local_leases WHERE root=?',
          variables: [Variable.withString(root)],
        )
        .get();
    if (rows.isNotEmpty &&
        rows.first.read<String>('owner') != owner &&
        rows.first.read<int>('until_ms') > now) {
      bad('LOCAL_OTHER_WINDOW');
    }
    await db.customStatement(
      'INSERT INTO local_leases(root,owner,session,until_ms) VALUES (?,?,?,?) ON CONFLICT(root) DO UPDATE SET owner=excluded.owner,session=excluded.session,until_ms=excluded.until_ms',
      [root, owner, session, now + 15000],
    );
  });
  Future<void> lease(String root, [String? session]) async {
    final rows = await db
        .customSelect(
          'SELECT * FROM local_leases WHERE root=?',
          variables: [Variable.withString(root)],
        )
        .get();
    if (rows.isEmpty ||
        rows.first.read<String>('owner') != owner ||
        rows.first.read<int>('until_ms') <= now ||
        session != null && rows.first.read<String>('session') != session) {
      bad('LOCAL_OTHER_WINDOW');
    }
    await db.customStatement(
      'UPDATE local_leases SET until_ms=? WHERE root=? AND owner=?',
      [now + 15000, root, owner],
    );
  }

  Future<void> release(String root, String session) => transaction(
    () => db.customStatement(
      'DELETE FROM local_leases WHERE root=? AND owner=? AND session=?',
      [root, owner, session],
    ),
  );
  Future<T> control<T>(String root, Future<T> Function() fn) async {
    // Serial callers on this runtime already share the live session lease.
    final row = await db
        .customSelect(
          'SELECT owner,until_ms FROM local_leases WHERE root=?',
          variables: [Variable.withString(root)],
        )
        .get();
    if (row.isNotEmpty &&
        row.first.read<String>('owner') == owner &&
        row.first.read<int>('until_ms') > now) {
      return fn();
    }
    final stamp = 'control:${randomUuid()}';
    await reserve(root, stamp);
    try {
      return await fn();
    } finally {
      await release(root, stamp);
    }
  }

  Future<void> consume(String id) => transaction(() async {
    final row = await db
        .customSelect(
          'SELECT id FROM local_used WHERE id=?',
          variables: [Variable.withString(id)],
        )
        .get();
    if (row.isNotEmpty) bad('LOCAL_CODE_USED');
    await db.customStatement(
      'INSERT INTO local_used(id,until_ms) VALUES (?,?)',
      [id, now + 86400000],
    );
  });
  Future<void> bytes(String id, String kind, int index, Uint8List bytes) =>
      db.customStatement(
        'INSERT INTO local_bytes(job,kind,idx,bytes) VALUES (?,?,?,?) ON CONFLICT(job,kind,idx) DO UPDATE SET bytes=excluded.bytes',
        [id, kind, index, bytes],
      );
  Future<Uint8List?> readBytes(
    String id,
    String kind, {
    int? start,
    int? length,
  }) async {
    final sql = start == null
        ? 'SELECT bytes AS value FROM local_bytes WHERE job=? AND kind=? ORDER BY idx'
        : "SELECT COALESCE(substr(bytes,?,?),X'') AS value FROM local_bytes WHERE job=? AND kind=? AND idx=0";
    final rows = await db
        .customSelect(
          sql,
          variables: [
            if (start != null) Variable.withInt(start + 1),
            if (start != null) Variable.withInt(length!),
            Variable.withString(id),
            Variable.withString(kind),
          ],
        )
        .get();
    if (rows.isEmpty) return null;
    final out = BytesBuilder(copy: false);
    for (final row in rows) {
      out.add(row.read<Uint8List>('value'));
    }
    return out.takeBytes();
  }

  Future<int> storedBytes() async =>
      (await db
              .customSelect(
                'SELECT COALESCE(SUM(length(bytes)),0) AS size FROM local_bytes',
              )
              .getSingle())
          .read<int>('size');
  Future<void> deleteBytes(String id, [String? kind]) => db.customStatement(
    'DELETE FROM local_bytes WHERE job=?${kind == null ? '' : ' AND kind=?'}',
    [id, if (kind != null) kind],
  );
  Future<bool> hasBytes(String id, String kind) async =>
      (await db
              .customSelect(
                'SELECT 1 FROM local_bytes WHERE job=? AND kind=? LIMIT 1',
                variables: [Variable.withString(id), Variable.withString(kind)],
              )
              .get())
          .isNotEmpty;
  Future<void> saveState(
    Json oldJob,
    Json state, {
    Uint8List? chunk,
    int? index,
    Uint8List? plain,
    List<({int index, Uint8List bytes})>? chunks,
  }) => transaction(() async {
    await lease(oldJob['peer']['root']);
    final current = await job(oldJob['id']), old = oldJob['state'] as Json;
    if (current == null ||
        current['state']['version'] != old['version'] ||
        current['state']['revision'] != old['revision'] ||
        localTerminal(current['state']['state'])) {
      bad('FILE_JOB_CHANGED');
    }
    if (chunk != null || chunks != null) {
      if (old['paused'] == true || old['state'] != 'ACCEPTED') {
        bad('FILE_JOB_CHANGED');
      }
      if (chunk != null) await bytes(oldJob['id'], 'chunk', index!, chunk);
      for (final item in chunks ?? <({int index, Uint8List bytes})>[]) {
        await bytes(oldJob['id'], 'chunk', item.index, item.bytes);
      }
    }
    final next = {
      ...current,
      'state': state,
      'disconnected': false,
      'error': null,
    };
    if (localTerminal(state['state'])) {
      next.remove('manifest');
      next.remove('recipientManifest');
      if (state['state'] != 'COMPLETED') {
        await deleteBytes(oldJob['id']);
      } else {
        await deleteBytes(oldJob['id'], 'chunk');
      }
    }
    if (plain != null) await bytes(oldJob['id'], 'plain', 0, plain);
    await put(next);
    await journal(state);
  });
}
