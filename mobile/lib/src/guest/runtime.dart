import 'package:http/http.dart' as http;

import 'dart:typed_data';

import '../data/database.dart';
import '../data/identity.dart';
import 'connections.dart';
import '../local/file_crypto.dart';
import 'session.dart';
import 'store.dart';

/// Native visitor storage and keys are separate from account and LAN stores.
class NativeGuestRuntime {
  NativeGuestRuntime._(this.database, this.identity, this.session, this.store)
    : connections = GuestConnections(session);
  final ClientDatabase database;
  final LocalIdentity identity;
  final GuestSession session;
  final GuestConnections connections;
  final NativeGuestStore store;

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

  Future<void> close() async {
    session.close();
    await database.close();
  }
}
