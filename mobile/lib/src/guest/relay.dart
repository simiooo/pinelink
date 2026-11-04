import 'dart:async';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import '../local/file_crypto.dart';
import '../data/identity.dart';
import 'session.dart';
import 'store.dart';

/// Sends authenticated ciphertext through the existing cloud relay ledger.
/// A cloud-stored result is not a receiver delivery receipt.
class GuestRelay {
  GuestRelay(this.session);
  final GuestSession session;

  /// The caller has already recorded the recipient's consent. A chunk is
  /// committed to local storage before the corresponding server acknowledgement.
  Future<Map<String, dynamic>> receive(
    NativeGuestStore store,
    String jobId,
  ) async {
    var job = await store.job(jobId);
    if (job == null ||
        job['direction'] != 'receive' ||
        job['state'] != 'ACCEPTED') {
      throw StateError('GUEST_CONSENT_REQUIRED');
    }
    final transfer = job['transfer'] as Map<String, dynamic>;
    final id = transfer['id'] as String;
    final size = transfer['size_bytes'] as int;
    if (size < 0 || size > (65 << 20)) throw StateError('FILE_SIZE_INVALID');
    final current = await session.request('GET', '/v1/transfers/$id');
    if (current['content_root'] != transfer['content_root'] ||
        current['size_bytes'] != size ||
        current['content_kind'] != transfer['content_kind'] ||
        current['encrypted_manifest'] is! String ||
        current['state'] != 'ACCEPTED') {
      throw StateError('FILE_JOB_CHANGED');
    }
    final manifest = await session.identity.decrypt(
      current['encrypted_manifest'],
    );
    validateManifest(manifest);
    if ((manifest['content_kind'] == 'TEXT') !=
        (current['content_kind'] == 'TEXT')) {
      throw StateError('FILE_MANIFEST_INVALID');
    }
    final snapshot = await session.request('GET', '/v1/p2p/$id/progress');
    final route = snapshot['route'] as Map?;
    final epoch = max(
      1,
      (route?['epoch'] as int? ?? 0) + (route?['kind'] == 'RELAY' ? 0 : 1),
    );
    final acknowledged =
        (snapshot['delivery'] as Map)['acknowledged_bytes'] as int;
    final local = await store.bytes(jobId, 'received');
    if (acknowledged > (local?.length ?? 0)) {
      throw StateError('FILE_CHECKPOINT_MISSING');
    }
    Future<void> report(int bytes) async =>
        session.request('POST', '/v1/p2p/$id/progress', {
          'owner_id': session.owner,
          'device_id': session.identity.device,
          'received_bytes': bytes,
          'route': 'RELAY',
          'route_epoch': epoch,
        });
    await report(acknowledged);
    final path = '/v1/p2p/$id/relay?role=receiver';
    final url = Uri.parse('${session.server}$path');
    final socket = await WebSocket.connect(
      url.replace(scheme: url.scheme == 'https' ? 'wss' : 'ws').toString(),
      protocols: ['pinelink-relay-v1'],
      headers: await session.signedHeaders('GET', path),
    ).timeout(const Duration(seconds: 15));
    try {
      var expected = min(
        acknowledged ~/ wireChunkSize,
        max(0, (size + wireChunkSize - 1) ~/ wireChunkSize - 1),
      );
      await for (final event in socket.timeout(const Duration(minutes: 2))) {
        if (event is! List<int> || event.length < 36) {
          throw StateError('FILE_FRAME_INVALID');
        }
        final frame = Uint8List.fromList(event);
        final index = ByteData.sublistView(frame).getUint32(0);
        final bytes = Uint8List.sublistView(frame, 36);
        final length = min(wireChunkSize, size - index * wireChunkSize);
        if (index != expected ||
            length < 0 ||
            bytes.length != length ||
            await digest(bytes) !=
                frame
                    .sublist(4, 36)
                    .map((b) => b.toRadixString(16).padLeft(2, '0'))
                    .join()) {
          throw StateError('FILE_FRAME_INVALID');
        }
        job = await store.job(jobId);
        if (job == null) throw StateError('GUEST_JOB_CHANGED');
        final received = min(size, (index + 1) * wireChunkSize);
        await store.update(
          jobId,
          job['localRevision'],
          {'received': received},
          index: index,
          chunk: bytes,
        );
        await report(received);
        expected++;
        if (received == size) {
          final cipher = await store.bytes(jobId, 'received');
          if (cipher == null ||
              cipher.length != size ||
              await digest(cipher) != current['content_root']) {
            throw StateError('FILE_CHECKSUM_INVALID');
          }
          final plain = await decryptDare(cipher, manifest);
          if (current['content_kind'] == 'TEXT') decodeText(plain);
          final fresh = await session.request('GET', '/v1/transfers/$id');
          final completed = await session.request(
            'POST',
            '/v1/p2p/$id/complete',
            {
              'owner_id': session.owner,
              'device_id': session.identity.device,
              'expected_version': fresh['version'],
            },
          );
          job = await store.job(jobId);
          if (job == null) throw StateError('GUEST_JOB_CHANGED');
          await store.update(jobId, job['localRevision'], {
            'state': 'COMPLETED',
            'transfer': completed,
            'manifest': current['encrypted_manifest'],
          });
          return completed;
        }
      }
      throw StateError('TRANSFER_INTERRUPTED');
    } finally {
      await socket.close();
    }
  }

  Future<Map<String, dynamic>> upload(String id, Uint8List ciphertext) async {
    final signals = await session.request('GET', '/v1/p2p/$id/signals?after=0');
    final items = signals['signals'] as List? ?? [];
    if (!items.any(
      (item) =>
          item is Map &&
          [
            'web-direct-failed',
            'web-direct-failed-sender',
          ].contains(item['channel']),
    )) {
      await session.request('POST', '/v1/p2p/$id/signals', {
        'owner_id': session.owner,
        'device_id': session.identity.device,
        'role': 'SENDER',
        'channel': 'web-direct-failed-sender',
        'payload': 'unavailable',
      });
    }
    final path = '/v1/p2p/$id/relay?role=sender';
    final url = Uri.parse('${session.server}$path');
    final socket = await WebSocket.connect(
      url.replace(scheme: url.scheme == 'https' ? 'wss' : 'ws').toString(),
      protocols: ['pinelink-relay-v1'],
      headers: await session.signedHeaders('GET', path),
    ).timeout(const Duration(seconds: 15));
    try {
      final count = max(
        1,
        (ciphertext.length + wireChunkSize - 1) ~/ wireChunkSize,
      );
      for (var index = 0; index < count; index++) {
        if (socket.readyState != WebSocket.open) {
          throw StateError('TRANSFER_INTERRUPTED');
        }
        final start = index * wireChunkSize,
            end = min(ciphertext.length, start + wireChunkSize);
        socket.add(
          await fileFrame(index, Uint8List.sublistView(ciphertext, start, end)),
        );
        await Future<void>.delayed(const Duration(milliseconds: 2));
      }
      final deadline = DateTime.now().add(const Duration(minutes: 2));
      while (DateTime.now().isBefore(deadline)) {
        final snapshot = await session.request('GET', '/v1/p2p/$id/progress');
        final completion = snapshot['completion'];
        if (completion is Map &&
            (completion['stored'] == true || completion['delivered'] == true)) {
          return snapshot;
        }
        if ([
              'REJECTED',
              'CANCELLED',
              'EXPIRED',
              'FAILED',
            ].contains(snapshot['state']) ||
            socket.readyState != WebSocket.open) {
          throw StateError('TRANSFER_INTERRUPTED');
        }
        await Future<void>.delayed(const Duration(milliseconds: 500));
      }
      throw StateError('TRANSFER_INTERRUPTED');
    } finally {
      await socket.close();
    }
  }
}
