import 'package:http/http.dart' as http;

import 'dart:typed_data';

import '../data/database.dart';
import '../data/identity.dart';
import 'connections.dart';
import '../local/file_crypto.dart';
import 'session.dart';
import 'store.dart';
import 'relay.dart';

/// Native visitor storage and keys are separate from account and LAN stores.
class NativeGuestRuntime {
  NativeGuestRuntime._(this.database, this.identity, this.session, this.store)
    : connections = GuestConnections(session);
  final ClientDatabase database;
  final LocalIdentity identity;
  final GuestSession session;
  final GuestConnections connections;
  final NativeGuestStore store;
  late final GuestRelay relay = GuestRelay(session);
  final Set<Future<void>> _transfers = {};
  bool _closed = false;
  Future<void>? _closing;

  Future<void> _transfer(Future<void> Function() work) {
    if (_closed) return Future.error(StateError('GUEST_CLOSED'));
    late Future<void> operation;
    operation = Future<void>.sync(work)
        .whenComplete(() => _transfers.remove(operation));
    _transfers.add(operation);
    return operation;
  }

  static Future<NativeGuestRuntime> open({
    required String server,
    IdentityVault? vault,
    ClientDatabase? database,
    http.Client? client,
  }) async {
    final address = Uri.base
        .resolve(server)
        .toString()
        .replaceAll(RegExp(r'/$'), '');
    final namespace = await GuestSession.scopeFor(address);
    final db = database ?? ClientDatabase(namespace),
        secureVault = vault ?? PlatformVault();
    try {
      await db.transaction(() async {
        final marker = await db.metadata('guest-service');
        if (marker != null && marker['scope'] != namespace ||
            marker == null && await db.metadata('profile') != null) {
          throw StateError('GUEST_STORE_MISMATCH');
        }
        if (marker == null) {
          await db.putMetadata('guest-service', {'scope': namespace});
        }
      });
      final identity = await LocalIdentity.load(db, secureVault, namespace, '');
      final session = await GuestSession.open(
        server: address,
        identity: identity,
        vault: secureVault,
        client: client,
      );
      final store = NativeGuestStore(db);
      await store.initialize();
      return NativeGuestRuntime._(db, identity, session, store);
    } catch (_) {
      await db.close();
      rethrow;
    }
  }

  /// Persist the encrypted attempt before creating its idempotent server offer.
  /// This prepares the control plane only; data is sent by a separate worker.
  Future<String> offer(
    Uint8List bytes,
    String name,
    GuestMap peer, {
    String? sensitivity,
  }) async {
    if (bytes.length > 64 * 1024 * 1024) throw StateError('FILE_TOO_LARGE');
    if (peer['pair_id'] is! String ||
        peer['owner_id'] is! String ||
        peer['device_id'] is! String ||
        peer['encryption_key'] is! String) {
      throw StateError('GUEST_PEER_INVALID');
    }
    await session.ensureSession();
    final prepared = await prepareLocalFile(
      bytes,
      name,
      peer['encryption_key'],
      senderRecipient: identity.encryptionKey,
      sensitivity: sensitivity,
    );
    final id = randomUuid();
    final job = <String, dynamic>{
      'id': id,
      'created': DateTime.now().millisecondsSinceEpoch,
      'direction': 'send',
      'state': 'PREPARING',
      'peer': peer,
      'name': prepared['name'],
      'plainSize': bytes.length,
      'cipherSize': (prepared['cipher'] as Uint8List).length,
      'cipherRoot': prepared['root'],
      'plainRoot': prepared['plainRoot'],
      'recipientManifest': prepared['manifest'],
      'senderManifest': prepared['senderManifest'],
      if (sensitivity != null) 'contentKind': 'TEXT',
    };
    await store.create(job, prepared['cipher']);
    await resumeOffer(id);
    return id;
  }

  Future<void> resumeOffer(String id) async {
    await session.ensureSession();
    final job = await store.job(id);
    if (job == null ||
        job['direction'] != 'send' ||
        job['state'] != 'PREPARING') {
      throw StateError('GUEST_JOB_CHANGED');
    }
    final peer = job['peer'] as GuestMap;
    final transfer = await session.request(
      'POST',
      job['contentKind'] == null ? '/v1/p2p' : '/v1/p2p/text',
      {
        'sender_owner_id': session.owner,
        'sender_device_id': identity.device,
        'recipient_owner_id': peer['owner_id'],
        'recipient_device_id': peer['device_id'],
        'guest_pair_id': peer['pair_id'],
        'client_request_id': id,
        'file_name': job['name'],
        'size_bytes': job['cipherSize'],
        'content_root': job['cipherRoot'],
        'encryption_suite': fileSuite,
        'encrypted_manifest': job['recipientManifest'],
        'sender_encrypted_manifest': job['senderManifest'],
      },
    );
    if (transfer['content_kind'] != job['contentKind'] ||
        transfer['content_root'] != job['cipherRoot'] ||
        transfer['size_bytes'] != job['cipherSize']) {
      throw StateError('GUEST_JOB_CHANGED');
    }
    await store.update(id, job['localRevision'], {
      'state': transfer['state'],
      'transfer': transfer,
    });
  }

  /// Synchronize incoming offers only from currently authorized peers.
  Future<void> syncIncoming() async {
    await session.ensureSession();
    final peers = (await connections.list())['peers'] as List? ?? [];
    final transfers =
        (await session.request('GET', '/v1/transfers'))['transfers'] as List? ??
        [];
    final jobs = await store.jobs();
    for (final item in transfers) {
      if (item is! Map<String, dynamic>) continue;
      final existing = jobs
          .where((j) => (j['transfer'] as Map?)?['id'] == item['id'])
          .firstOrNull;
      if (existing != null) {
        final original = existing['transfer'] as Map;
        for (final key in [
          'content_root',
          'size_bytes',
          'content_kind',
          'file_name',
          'sender_owner_id',
          'sender_device_id',
          'recipient_owner_id',
          'recipient_device_id',
        ]) {
          if (item[key] != original[key]) throw StateError('FILE_JOB_CHANGED');
        }
        if ((item['version'] as int) >= (original['version'] as int)) {
          final terminal = [
            'REJECTED',
            'CANCELLED',
            'EXPIRED',
            'FAILED',
          ].contains(item['state']);
          final progress = await session.request(
            'GET',
            '/v1/p2p/${item['id']}/progress',
          );
          final latest = await store.job(existing['id']);
          if (latest != null) {
            final previousTransfer = latest['transfer'] as Map;
            final previousProgress = latest['progress'] as Map?;
            if ((item['version'] as int) <
                    (previousTransfer['version'] as int) ||
                (progress['control_revision'] as int? ?? 0) <
                    ((latest['controlRevision'] as int?) ??
                        (previousProgress?['control_revision'] as int?) ??
                        0) ||
                (progress['progress_revision'] as int? ?? 0) <
                    (previousProgress?['progress_revision'] as int? ?? 0)) {
              continue;
            }
            final previousState = latest['state'];
            if ([
                  'COMPLETED',
                  'REJECTED',
                  'CANCELLED',
                  'EXPIRED',
                  'FAILED',
                ].contains(previousState) &&
                item['state'] != previousState) {
              continue;
            }
            final delivery = progress['delivery'] as Map?;
            await store.update(existing['id'], latest['localRevision'], {
              'transfer': item,
              'progress': progress,
              'paused': delivery?['state'] == 'PAUSED',
              if (terminal ||
                  existing['direction'] == 'send' ||
                  item['state'] == 'ACCEPTED' && previousState == 'PENDING')
                'state': item['state'],
            });
          }
        }
        continue;
      }
      if (item['recipient_owner_id'] != session.owner ||
          item['recipient_device_id'] != identity.device ||
          !['PENDING', 'ACCEPTED'].contains(item['state'])) {
        continue;
      }
      final matching = peers.where(
        (p) =>
            p is Map &&
            p['owner_id'] == item['sender_owner_id'] &&
            p['device_id'] == item['sender_device_id'],
      );
      if (matching.isEmpty ||
          item['id'] is! String ||
          item['size_bytes'] is! int ||
          item['file_name'] is! String) {
        continue;
      }
      final peer = Map<String, dynamic>.from(matching.first as Map);
      await store.create({
        'id': item['id'],
        'created': DateTime.now().millisecondsSinceEpoch,
        'direction': 'receive',
        'state': item['state'],
        'peer': peer,
        'name': safeFileName(item['file_name']),
        'plainSize': item['size_bytes'],
        'received': 0,
        'transfer': item,
        if (item['content_kind'] == 'TEXT') 'contentKind': 'TEXT',
      });
    }
  }

  Future<void> respond(String id, bool accept) async {
    await session.ensureSession();
    final job = await store.job(id);
    if (job == null ||
        job['direction'] != 'receive' ||
        job['state'] != 'PENDING') {
      throw StateError('GUEST_JOB_CHANGED');
    }
    final original = job['transfer'] as GuestMap;
    final current = await session.request(
      'GET',
      '/v1/transfers/${original['id']}',
    );
    for (final key in [
      'content_root',
      'size_bytes',
      'content_kind',
      'file_name',
      'sender_owner_id',
      'sender_device_id',
      'recipient_owner_id',
      'recipient_device_id',
    ]) {
      if (current[key] != original[key]) throw StateError('FILE_JOB_CHANGED');
    }
    final transfer = await session.request(
      'POST',
      '/v1/transfers/${original['id']}/${accept ? 'accept' : 'reject'}',
      {
        'owner_id': session.owner,
        'device_id': identity.device,
        'expected_version': current['version'],
        if (accept && current['content_kind'] == 'TEXT')
          'accept_content_kind': 'TEXT',
      },
    );
    await store.update(id, job['localRevision'], {
      'state': transfer['state'],
      'transfer': transfer,
    });
  }

  Future<void> receive(String id) => _transfer(() async {
    await relay.receive(store, id);
  });

  Future<void> control(String id, String action) async {
    if (!['pause', 'resume', 'cancel'].contains(action)) {
      throw StateError('GUEST_ACTION_INVALID');
    }
    await session.ensureSession();
    final job = await store.job(id);
    if (job == null || job['transfer'] is! Map) {
      throw StateError('GUEST_JOB_CHANGED');
    }
    final transfer = job['transfer'] as Map;
    if (action == 'cancel' && job['direction'] == 'send') {
      final fresh = await session.request(
        'GET',
        '/v1/transfers/${transfer['id']}',
      );
      final cancelled = await session.request(
        'POST',
        '/v1/transfers/${transfer['id']}/cancel',
        {
          'owner_id': session.owner,
          'device_id': identity.device,
          'expected_version': fresh['version'],
        },
      );
      await relay.stop(transfer['id']);
      final latest = await store.job(id);
      if (latest != null) {
        await store.update(id, latest['localRevision'], {
          'state': cancelled['state'],
          'transfer': cancelled,
        });
      }
      return;
    }
    final tasks =
        (await session.request('GET', '/v1/tasks'))['tasks'] as List? ?? [];
    final matches = tasks.where(
      (t) =>
          t is Map &&
          t['kind'] == 'DOWNLOAD' &&
          t['reference_id'] == transfer['id'],
    );
    if (matches.isEmpty) throw StateError('TRANSFER_INTERRUPTED');
    final task = matches.first as Map;
    final changedTask = await session.request(
      'POST',
      '/v1/tasks/${task['id']}/$action',
      {
        'owner_id': session.owner,
        'device_id': identity.device,
        'expected_version': task['version'],
      },
    );
    if (action != 'resume') await relay.stop(transfer['id']);
    final latest = await store.job(id);
    if (latest != null) {
      await store.update(id, latest['localRevision'], {
        'paused': action == 'pause',
        'controlRevision': changedTask['version'],
        if (action == 'cancel') 'state': 'CANCELLED',
      });
    }
  }

  Future<void> upload(String id) => _transfer(() => _upload(id));

  Future<void> _upload(String id) async {
    await session.ensureSession();
    final job = await store.job(id);
    if (job == null || job['direction'] != 'send' || job['transfer'] is! Map) {
      throw StateError('GUEST_JOB_CHANGED');
    }
    final cipher = await store.bytes(id, 'source');
    if (cipher == null ||
        cipher.length != job['cipherSize'] ||
        await digest(cipher) != job['cipherRoot']) {
      throw StateError('FILE_SOURCE_MISSING');
    }
    final result = await relay.upload((job['transfer'] as Map)['id'], cipher);
    final latest = await store.job(id);
    if (latest == null) throw StateError('GUEST_JOB_CHANGED');
    await store.update(id, latest['localRevision'], {'progress': result});
  }

  Future<({String value, bool sensitive})?> text(String id) async {
    final job = await store.job(id);
    if (job == null || job['contentKind'] != 'TEXT') return null;
    final sent = job['direction'] == 'send';
    if (!sent && job['state'] != 'COMPLETED') return null;
    final envelope = sent ? job['senderManifest'] : job['manifest'];
    if (envelope is! String) return null;
    final cipher = await store.bytes(id, sent ? 'source' : 'received');
    if (cipher == null) return null;
    final manifest = await identity.decrypt(envelope);
    validateManifest(manifest);
    if (manifest['content_kind'] != 'TEXT') {
      throw StateError('FILE_MANIFEST_INVALID');
    }
    return (
      value: decodeText(await decryptDare(cipher, manifest)),
      sensitive: manifest['sensitivity'] == 'SENSITIVE',
    );
  }

  Future<Uint8List?> plaintext(String id) async {
    final job = await store.job(id);
    if (job == null ||
        job['direction'] != 'receive' ||
        job['state'] != 'COMPLETED' ||
        job['contentKind'] == 'TEXT' ||
        job['manifest'] is! String) {
      return null;
    }
    final cipher = await store.bytes(id, 'received');
    if (cipher == null) return null;
    return decryptDare(cipher, await identity.decrypt(job['manifest']));
  }

  Future<void> close() => _closing ??= _finishClose();

  Future<void> _finishClose() async {
    _closed = true;
    await relay.close();
    await Future.wait(
      _transfers.toList().map((task) => task.catchError((Object _) {})),
    );
    session.close();
    await database.close();
  }
}
