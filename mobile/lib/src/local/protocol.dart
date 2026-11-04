import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:dartage/dartage.dart';

import '../data/identity.dart';
import '../data/wire.dart' as wire;

typedef Json = Map<String, dynamic>;
const _alphabet = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ \$%*+-./:';
const localChannel = 'pinelink-local-v1';
const chunkSize = 65536;
bool localId(dynamic id) =>
    id is String && id.startsWith('local:') && wire.validId(id.substring(6));
Never bad([String reason = 'LOCAL_MESSAGE_INVALID']) =>
    throw StateError(reason);
List<int> localProof(String domain, Json value) => utf8.encode(
  'pinelink-local-browser-$domain-v1\n${wire.canonical({...value, 'signature': ''})}',
);
Future<Json> signed(LocalIdentity identity, String domain, Json value) async =>
    {...value, 'signature': await identity.sign(localProof(domain, value))};
Future<bool> verified(String key, String domain, Json value) async =>
    value['signature'] is String &&
    await wire.verify(key, localProof(domain, value), value['signature']);
Future<String> packageHash(Json packet) =>
    digest(utf8.encode(wire.canonical(packet)));
Json publicIdentity(LocalIdentity identity, String name) =>
    {...identity.public, 'name': name}..remove('stream');
Future<void> validateIdentity(Json p) async {
  if (p['name'] is! String ||
      (p['name'] as String).trim().isEmpty ||
      p['name'].length > 64 ||
      RegExp(r'[\x00-\x1f\x7f]').hasMatch(p['name']) ||
      p['device'] is! String ||
      p['device'].length > 96 ||
      !wire.validRoot(p['root']) ||
      p['signing_key'] is! String ||
      base64Decode(p['signing_key']).length != 32 ||
      await digest(base64Decode(p['signing_key'])) != p['root'] ||
      p['encryption_key'] is! String ||
      p['encryption_key'].length > 128) {
    bad('LOCAL_CODE_INVALID');
  }
  if (p.containsKey('capabilities')) {
    final capabilities = p['capabilities'];
    if (capabilities is! List ||
        capabilities.length > 16 ||
        capabilities.any(
          (value) =>
              value is! String || !RegExp(r'^[a-z0-9-]{1,32}$').hasMatch(value),
        ) ||
        capabilities.toSet().length != capabilities.length) {
      bad('LOCAL_CODE_INVALID');
    }
  }
  X25519Recipient.parse(p['encryption_key']);
}

Future<Json> validatePackage(Json p) async {
  final sdp = p['sdp'];
  if (p['v'] != 1 ||
      !['offer', 'answer'].contains(p['type']) ||
      !wire.validId(p['id']) ||
      p['nonce'] is! String ||
      !RegExp(r'^[A-Za-z0-9_-]{43}$').hasMatch(p['nonce']) ||
      p['reply'] is! String ||
      (p['type'] == 'answer'
          ? !wire.validRoot(p['reply'])
          : p['reply'] != '') ||
      sdp is! String ||
      sdp.length > 16384 ||
      sdp.split(RegExp(r'\r?\n')).where((s) => s.startsWith('m=')).length !=
          1 ||
      !RegExp(
        r'^m=application \d+ UDP/DTLS/SCTP webrtc-datachannel\r?$',
        multiLine: true,
      ).hasMatch(sdp) ||
      !RegExp(
        r'^a=fingerprint:sha-256 [0-9A-F:]+\r?$',
        multiLine: true,
        caseSensitive: false,
      ).hasMatch(sdp) ||
      !RegExp(r'^a=ice-ufrag:.+$', multiLine: true).hasMatch(sdp) ||
      !RegExp(r'^a=ice-pwd:.+$', multiLine: true).hasMatch(sdp) ||
      p['peer'] is! Json) {
    bad('LOCAL_CODE_INVALID');
  }
  await validateIdentity(p['peer']);
  if (!await verified(p['peer']['signing_key'], 'connection', p)) {
    bad('LOCAL_CODE_INVALID');
  }
  return p;
}

String base45(List<int> bytes) {
  final result = StringBuffer();
  for (var i = 0; i < bytes.length; i += 2) {
    final pair = i + 1 < bytes.length;
    final n = pair ? bytes[i] * 256 + bytes[i + 1] : bytes[i];
    result.write(_alphabet[n % 45]);
    result.write(_alphabet[(n ~/ 45) % 45]);
    if (pair) result.write(_alphabet[n ~/ 2025]);
  }
  return result.toString();
}

Uint8List unbase45(String value) {
  if (value.length % 3 == 1) bad('LOCAL_CODE_INVALID');
  final result = BytesBuilder();
  for (var i = 0; i < value.length; i += 3) {
    final count = value.length - i < 3 ? value.length - i : 3;
    final digits = List.generate(count, (n) => _alphabet.indexOf(value[i + n]));
    if (digits.any((n) => n < 0)) bad('LOCAL_CODE_INVALID');
    final n = digits[0] + digits[1] * 45 + (count == 3 ? digits[2] * 2025 : 0);
    if (n > (count == 3 ? 65535 : 255)) bad('LOCAL_CODE_INVALID');
    if (count == 3) result.addByte(n >> 8);
    result.addByte(n & 255);
  }
  return result.takeBytes();
}

String encodePackage(Json value) {
  final bytes = zlib.encode(utf8.encode(jsonEncode(value)));
  if (bytes.length > 11996) bad('LOCAL_CODE_INVALID');
  return 'PL2:${base45(bytes)}:';
}

class _BoundedSink implements Sink<List<int>> {
  final buffer = BytesBuilder();
  @override
  void add(List<int> value) {
    if (buffer.length + value.length > 24000) bad('LOCAL_CODE_INVALID');
    buffer.add(value);
  }

  @override
  void close() {}
}

Future<Json> decodePackage(String code) async {
  try {
    if (code.length > 18000) bad();
    final v = code.trim();
    final bytes = v.startsWith('PL2:') && v.endsWith(':')
        ? unbase45(v.substring(4, v.length - 1))
        : v.startsWith('PL1.')
        ? base64Url.decode(base64Url.normalize(v.substring(4)))
        : throw StateError('LOCAL_CODE_INVALID');
    final target = _BoundedSink();
    // Feed a bounded compressed input into a bounded output sink (zip bombs are
    // rejected while expanding, before JSON allocation).
    final decoder = zlib.decoder.startChunkedConversion(target);
    decoder.add(bytes);
    decoder.close();
    return await validatePackage(
      jsonDecode(utf8.decode(target.buffer.takeBytes())) as Json,
    );
  } catch (_) {
    bad('LOCAL_CODE_INVALID');
  }
}

Future<String> comparison(Json offer, Json answer) async {
  final hash = await digest(
    utf8.encode(
      'pinelink-local-compare-v1\n${await packageHash(offer)}\n${await packageHash(answer)}',
    ),
  );
  final code = (int.parse(hash.substring(0, 8), radix: 16) % 1000000)
      .toString()
      .padLeft(6, '0');
  return '${code.substring(0, 3)} ${code.substring(3)}';
}
