import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/drift.dart';

import '../data/database.dart';
import '../data/identity.dart';

typedef GuestMap = Map<String, dynamic>;
bool _hasPlaintext(GuestMap value) =>
    ['plain', 'text', 'value', 'message'].any(value.containsKey);

/// Durable service-scoped visitor jobs. Progress advances in the same SQLite
/// transaction that stores the corresponding authenticated ciphertext chunk.
class NativeGuestStore {
  NativeGuestStore(this.db);
  final ClientDatabase db;

  Future<void> initialize() async {
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS guest_jobs (id TEXT PRIMARY KEY, revision INTEGER NOT NULL, body TEXT NOT NULL)',
    );
    await db.customStatement(
      'CREATE TABLE IF NOT EXISTS guest_chunks (job TEXT NOT NULL, kind TEXT NOT NULL, idx INTEGER NOT NULL, hash TEXT NOT NULL, bytes BLOB NOT NULL, PRIMARY KEY(job,kind,idx))',
    );
  }

  Future<GuestMap?> job(String id) async {
    final rows = await db
        .customSelect(
          'SELECT revision,body FROM guest_jobs WHERE id=?',
          variables: [Variable.withString(id)],
        )
        .get();
    if (rows.isEmpty) return null;
    return {
      ...jsonDecode(rows.first.read<String>('body')) as GuestMap,
      'localRevision': rows.first.read<int>('revision'),
    };
  }

  Future<List<GuestMap>> jobs() async {
    final rows = await db
        .customSelect('SELECT revision,body FROM guest_jobs')
        .get();
    return rows
        .map(
          (row) => {
            ...jsonDecode(row.read<String>('body')) as GuestMap,
            'localRevision': row.read<int>('revision'),
          },
        )
        .toList();
  }

  Future<void> create(GuestMap job, [Uint8List? ciphertext]) => db.transaction(
    () async {
      if (job['id'] is! String ||
          _hasPlaintext(job) ||
          job.containsKey('localRevision')) {
        throw StateError('GUEST_JOB_INVALID');
      }
      await db.customStatement(
        'INSERT INTO guest_jobs(id,revision,body) VALUES(?,0,?)',
        [job['id'], jsonEncode(job)],
      );
      if (ciphertext != null) {
        await db.customStatement(
          'INSERT INTO guest_chunks(job,kind,idx,hash,bytes) VALUES(?,?,?,?,?)',
          [job['id'], 'source', 0, await digest(ciphertext), ciphertext],
        );
      }
    },
  );

  Future<void> update(
    String id,
    int expected,
    GuestMap changes, {
    int? index,
    Uint8List? chunk,
  }) => db.transaction(() async {
    final current = await job(id);
    if (current == null ||
        current['localRevision'] != expected ||
        changes.containsKey('id') ||
        changes.containsKey('localRevision') ||
        _hasPlaintext(changes)) {
      throw StateError('GUEST_JOB_CHANGED');
    }
    if ((index == null) != (chunk == null) || (index != null && index < 0)) {
      throw StateError('GUEST_CHUNK_INVALID');
    }
    if (chunk != null) {
      if (current['direction'] != 'receive' ||
          current['state'] != 'ACCEPTED' ||
          current['paused'] == true) {
        throw StateError('GUEST_CONSENT_REQUIRED');
      }
      final hash = await digest(chunk);
      final rows = await db
          .customSelect(
            'SELECT hash FROM guest_chunks WHERE job=? AND kind=? AND idx=?',
            variables: [
              Variable.withString(id),
              Variable.withString('received'),
              Variable.withInt(index!),
            ],
          )
          .get();
      if (rows.isNotEmpty && rows.first.read<String>('hash') != hash) {
        throw StateError('GUEST_CHUNK_CHANGED');
      }
      if (rows.isEmpty) {
        await db.customStatement(
          'INSERT INTO guest_chunks(job,kind,idx,hash,bytes) VALUES(?,?,?,?,?)',
          [id, 'received', index, hash, chunk],
        );
      }
    }
    current.remove('localRevision');
    await db.customStatement(
      'UPDATE guest_jobs SET revision=?,body=? WHERE id=? AND revision=?',
      [
        expected + 1,
        jsonEncode({...current, ...changes}),
        id,
        expected,
      ],
    );
  });

  Future<Uint8List?> bytes(String id, String kind) async {
    if (!['source', 'received'].contains(kind)) {
      throw StateError('GUEST_CHUNK_INVALID');
    }
    final rows = await db
        .customSelect(
          'SELECT bytes FROM guest_chunks WHERE job=? AND kind=? ORDER BY idx',
          variables: [Variable.withString(id), Variable.withString(kind)],
        )
        .get();
    if (rows.isEmpty) return null;
    final builder = BytesBuilder(copy: false);
    for (final row in rows) {
      builder.add(row.read<Uint8List>('bytes'));
    }
    return builder.takeBytes();
  }
}
