import 'dart:convert';

import 'package:cryptography/cryptography.dart';

import 'database.dart';
import 'identity.dart';

String canonical(dynamic value) {
  if (value is num && (value is! int || value.abs() > 9007199254740991)) {
    throw StateError('UNSAFE_PROTOCOL_NUMBER');
  }
  if (value is List) return '[${value.map(canonical).join(',')}]';
  if (value is Map) {
    final keys = value.keys.cast<String>().toList()..sort();
    return '{${keys.map((k) => '${canonical(k)}:${canonical(value[k])}').join(',')}}';
  }
  return jsonEncode(value).replaceAllMapped(
    RegExp('[<>&\u2028\u2029]'),
    (m) => '\\u${m[0]!.codeUnitAt(0).toRadixString(16).padLeft(4, '0')}',
  );
}

List<int> proof(String domain, Map<String, dynamic> value) => utf8.encode(
  'pinelink-replication-$domain-v1\n${canonical({...value, 'signature': ''})}',
);
Future<bool> verify(String key, List<int> bytes, String signature) async {
  try {
    return await Ed25519().verify(
      bytes,
      signature: Signature(
        base64Decode(signature),
        publicKey: SimplePublicKey(
          base64Decode(key),
          type: KeyPairType.ed25519,
        ),
      ),
    );
  } catch (_) {
    return false;
  }
}

Never invalid() => throw StateError('INVALID_SIGNED_HISTORY');
bool validRoot(dynamic s) =>
    s is String && RegExp(r'^[a-f0-9]{64}$').hasMatch(s);
bool validId(dynamic s) =>
    s is String &&
    RegExp(r'^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$')
        .hasMatch(s);
bool integer(dynamic n, [int minimum = 0]) =>
    n is int && n >= minimum && n <= 9007199254740991;

Future<void> verifyFact(
  LocalIdentity identity,
  Map<String, dynamic> e,
  Map<String, dynamic> f,
  Map<String, PeerRow> peers,
) async {
  const states = {
    'PREPARING',
    'PENDING',
    'ACCEPTED',
    'PAUSED',
    'VERIFYING',
    'COMPLETED',
    'REJECTED',
    'CANCELLED',
    'EXPIRED',
    'FAILED',
  };
  if (f['transfer_id'] != e['aggregate_id'] ||
      f['state_version'] != e['aggregate_version'] ||
      f['authority'] != e['authority'] ||
      f['authority_device_id'] != e['authority_root'] ||
      f['peer_id'] != e['peer_root'] ||
      !integer(f['plaintext_size']) ||
      f['file_name'] is! String ||
      utf8.encode(f['file_name']).length > 512 ||
      !states.contains(f['state']) ||
      f['direction'] != (e['kind'] == 'SENT_CHANGED' ? 'SEND' : 'RECEIVE')) {
    invalid();
  }
  if (f['state'] != 'COMPLETED') {
    if (f['receipt'] != null) invalid();
    return;
  }
  final r = f['receipt'];
  if (r is! Map<String, dynamic>) invalid();
  final key = e['authority_root'] == identity.root
      ? identity.signingKey
      : peers[e['authority_root']]?.signingKey;
  final sender = e['kind'] == 'SENT_CHANGED'
      ? e['source_root']
      : e['peer_root'];
  if (key == null ||
      r['version'] != 1 ||
      r['transfer_id'] != e['aggregate_id'] ||
      r['recipient_id'] != e['authority_root'] ||
      r['sender_id'] != sender ||
      r['cipher_root'] != f['cipher_root'] ||
      r['plaintext_size'] != f['plaintext_size'] ||
      !validRoot(r['cipher_root']) ||
      !validRoot(r['plaintext_root']) ||
      r['delivered_at'] is! String ||
      !RegExp(r'^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$')
          .hasMatch(r['delivered_at'])) {
    invalid();
  }
  final at = (r['delivered_at'] as String)
      .replaceAllMapped(RegExp(r'(\.\d*?)0+Z$'), (m) => '${m[1]}Z')
      .replaceAll('.Z', 'Z');
  final text = [
    'pinelink-receipt-v1',
    r['version'],
    r['transfer_id'],
    r['sender_id'],
    r['recipient_id'],
    r['cipher_root'],
    r['plaintext_root'],
    r['plaintext_size'],
    at,
  ].join('\n');
  if (!await verify(key, utf8.encode(text), r['signature'])) invalid();
}

Future<List<PreparedHistory>> preparePage(
  LocalIdentity identity,
  Map<String, dynamic> page,
  String pin,
  Map<String, PeerRow> peers,
) async {
  if (page['consumer_root'] != identity.root ||
      !validId(page['epoch']) ||
      !integer(page['after']) ||
      !integer(page['through'], page['after']) ||
      !integer(page['upper'], page['through']) ||
      page['items'] is! List ||
      (page['items'] as List).length > 100 ||
      !await verify(pin, proof('page', page), page['signature'])) {
    invalid();
  }
  if (page['more'] == true &&
      ((page['snapshot'] != true && page['through'] == page['after']) ||
          (page['snapshot'] == true &&
              (page['next_key'] as String? ?? '').isEmpty))) {
    invalid();
  }
  var last = page['after'] as int;
  final result = <PreparedHistory>[];
  for (final dynamic item in page['items']) {
    final e = item['event'] as Map<String, dynamic>;
    if (!integer(item['sequence'], 1) ||
        item['sequence'] > page['upper'] ||
        (page['snapshot'] != true &&
            (item['sequence'] <= last || item['sequence'] > page['through']))) {
      invalid();
    }
    last = item['sequence'];
    final recipients = e['recipients'];
    if (e['version'] != 1 ||
        !validId(e['event_id']) ||
        !validId(e['stream_id']) ||
        !validId(e['aggregate_id']) ||
        !integer(e['source_sequence'], 1) ||
        !integer(e['aggregate_version']) ||
        !validRoot(e['authority_root']) ||
        await digest(base64Decode(e['signing_key'])) != e['source_root'] ||
        recipients is! List ||
        recipients.isEmpty ||
        recipients.length > 2 ||
        !recipients.contains(identity.root) ||
        recipients.toSet().length != recipients.length ||
        !recipients.every(validRoot) ||
        !await verify(e['signing_key'], proof('envelope', e), e['signature'])) {
      invalid();
    }
    final sorted = recipients.cast<String>().toList()..sort();
    if (canonical(recipients) != canonical(sorted)) invalid();
    Map<String, dynamic>? fact;
    String? trustedKey;
    if (e['authority'] == 'SERVER') {
      if (e['signing_key'] != pin ||
          e['authority_root'] != e['source_root'] ||
          !(e['kind'] as String).startsWith('SERVER_') ||
          e['ciphertext'] != null ||
          e['server_payload'] is! Map<String, dynamic> ||
          utf8.encode(canonical(e['server_payload'])).length > 65536) {
        invalid();
      }
      fact = e['server_payload'];
    } else {
      if (e['authority'] != 'LOCAL_DEVICE' ||
          !validRoot(e['peer_root']) ||
          !recipients.contains(e['source_root']) ||
          recipients.any((r) => r != e['source_root'] && r != e['peer_root']) ||
          e['ciphertext'] is! String ||
          base64Decode(e['ciphertext']).length > 65536 ||
          e['server_payload'] != null ||
          !{'SENT_CHANGED', 'TRANSFER_CHANGED'}.contains(e['kind']) ||
          e['authority_root'] !=
              (e['kind'] == 'SENT_CHANGED'
                  ? e['peer_root']
                  : e['source_root'])) {
        invalid();
      }
      final peer = peers[e['source_root']];
      final key = e['source_root'] == identity.root
          ? identity.signingKey
          : peer?.signingKey;
      if (key == e['signing_key'] &&
          (e['source_root'] == identity.root ||
              peer != null && !peer.revoked)) {
        trustedKey = key;
        fact = await identity.decrypt(e['ciphertext']);
        await verifyFact(identity, e, fact, peers);
      }
    }
    result.add(
      PreparedHistory(
        [e['source_root'], e['stream_id'], e['event_id']].join('|'),
        await digest(utf8.encode(canonical(e))),
        e,
        fact,
        trustedKey,
      ),
    );
  }
  return result;
}
