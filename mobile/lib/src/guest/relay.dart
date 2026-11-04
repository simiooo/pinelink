import 'dart:async';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import '../local/file_crypto.dart';
import 'session.dart';

/// Sends authenticated ciphertext through the existing cloud relay ledger.
/// A cloud-stored result is not a receiver delivery receipt.
class GuestRelay {
  GuestRelay(this.session);
  final GuestSession session;

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
