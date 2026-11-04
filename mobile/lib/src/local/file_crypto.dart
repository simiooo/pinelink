import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:dartage/dartage.dart';

import '../data/identity.dart';

const wireChunkSize = 65536;
const fileSuite = 'DARE20-AGE-X25519-v1';
const textByteLimit = 256 * 1024;
Future<Map<String, dynamic>> prepareFileWorker(
  (Uint8List, String, String) input,
) => prepareLocalFile(input.$1, input.$2, input.$3);
Future<Uint8List> decryptFileWorker((Uint8List, Map<String, dynamic>) input) =>
    decryptDare(input.$1, input.$2);
int cipherSize(int n) => n + ((n + wireChunkSize - 1) ~/ wireChunkSize) * 32;
String safeFileName(String name) {
  var s = name
      .replaceAll(RegExp(r'[\\/<>:"|?*\x00-\x1f\x7f]'), '_')
      .replaceAll(RegExp(r'^\.+$'), '_');
  if (s.length > 180) s = s.substring(0, 180);
  return s.isEmpty ? 'File' : s;
}

Uint8List joinBytes(Iterable<List<int>> parts) =>
    (BytesBuilder(copy: false)..addAllParts(parts)).takeBytes();

extension on BytesBuilder {
  void addAllParts(Iterable<List<int>> parts) {
    for (final p in parts) {
      add(p);
    }
  }
}

Cipher _algorithm(int suite) =>
    suite == 0 ? AesGcm.with256bits() : Chacha20.poly1305Aead();
Future<Uint8List> encryptDare(
  Uint8List input,
  List<int> key, {
  List<int>? nonce,
  int suite = 0,
}) async {
  if (key.length != 32 || ![0, 1].contains(suite)) {
    throw StateError('FILE_KEY_INVALID');
  }
  nonce ??= randomBytes(12);
  if (nonce.length != 12) throw StateError('FILE_KEY_INVALID');
  final out = BytesBuilder(copy: false),
      cipher = _algorithm(suite),
      secret = SecretKey(key);
  for (
    var offset = 0, sequence = 0;
    offset < input.length;
    offset += wireChunkSize, sequence++
  ) {
    final plain = Uint8List.sublistView(
      input,
      offset,
      min(input.length, offset + wireChunkSize),
    );
    final header = Uint8List(16), view = ByteData.sublistView(header);
    header[0] = 0x20;
    header[1] = suite;
    view.setUint16(2, plain.length - 1, Endian.little);
    header.setAll(4, nonce);
    header[4] =
        (header[4] & 0x7f) | (offset + plain.length == input.length ? 0x80 : 0);
    final iv = Uint8List.fromList(header.sublist(4)),
        ivView = ByteData.sublistView(iv);
    ivView.setUint32(
      8,
      ivView.getUint32(8, Endian.little) ^ sequence,
      Endian.little,
    );
    final box = await cipher.encrypt(
      plain,
      secretKey: secret,
      nonce: iv,
      aad: header.sublist(0, 4),
    );
    out.add(header);
    out.add(box.cipherText);
    out.add(box.mac.bytes);
  }
  return out.takeBytes();
}

void validateManifest(Map<String, dynamic> m) {
  if (m['suite'] != fileSuite ||
      m['data_key'] is! String ||
      base64Decode(m['data_key']).length != 32 ||
      m['plaintext_root'] is! String ||
      !RegExp(r'^[a-f0-9]{64}$').hasMatch(m['plaintext_root']) ||
      m['plaintext_size'] is! int ||
      m['plaintext_size'] < 0 ||
      m['plaintext_size'] > (64 << 20) ||
      m['file_name'] is! String ||
      m['file_name'].length > 1024) {
    throw StateError('FILE_MANIFEST_INVALID');
  }
  final kind = m['content_kind'];
  if (kind == null) {
    if (m.containsKey('content_kind') ||
        m.containsKey('content_version') ||
        m.containsKey('sensitivity')) {
      throw StateError('FILE_MANIFEST_INVALID');
    }
  } else if (m['content_version'] != 1 ||
      !['FILE', 'TEXT'].contains(kind) ||
      (kind == 'TEXT'
          ? !['NORMAL', 'SENSITIVE'].contains(m['sensitivity']) ||
                m['plaintext_size'] == 0 ||
                m['plaintext_size'] > textByteLimit
          : m.containsKey('sensitivity'))) {
    throw StateError('FILE_MANIFEST_INVALID');
  }
}

Future<Uint8List> decryptDare(Uint8List input, Map<String, dynamic> m) async {
  validateManifest(m);
  if (input.length != cipherSize(m['plaintext_size'])) {
    throw StateError('FILE_TRUNCATED');
  }
  final key = SecretKey(base64Decode(m['data_key'])),
      out = BytesBuilder(copy: false);
  var offset = 0, sequence = 0, finalized = input.isEmpty, suite = -1;
  List<int>? reference;
  while (offset < input.length) {
    if (finalized || input.length - offset <= 32) {
      throw StateError('FILE_FRAMING_INVALID');
    }
    final header = Uint8List.sublistView(input, offset, offset + 16);
    final length = ByteData.sublistView(header).getUint16(2, Endian.little) + 1,
        last = header[4] & 0x80 != 0;
    if (header[0] != 0x20 ||
        ![0, 1].contains(header[1]) ||
        (!last && length != wireChunkSize) ||
        offset + length + 32 > input.length) {
      throw StateError('FILE_FRAMING_INVALID');
    }
    final normalized = Uint8List.fromList(header.sublist(4));
    normalized[0] &= 0x7f;
    reference ??= normalized;
    if (suite < 0) suite = header[1];
    if (suite != header[1] ||
        base64Encode(reference) != base64Encode(normalized)) {
      throw StateError('FILE_NONCE_CHANGED');
    }
    final iv = Uint8List.fromList(header.sublist(4)),
        view = ByteData.sublistView(iv);
    view.setUint32(
      8,
      view.getUint32(8, Endian.little) ^ sequence,
      Endian.little,
    );
    final box = SecretBox(
      Uint8List.sublistView(input, offset + 16, offset + 16 + length),
      nonce: iv,
      mac: Mac(
        Uint8List.sublistView(
          input,
          offset + 16 + length,
          offset + length + 32,
        ),
      ),
    );
    out.add(
      await _algorithm(suite)
          .decrypt(box, secretKey: key, aad: header.sublist(0, 4)),
    );
    offset += length + 32;
    sequence++;
    finalized = last;
  }
  final plain = out.takeBytes();
  if (!finalized ||
      plain.length != m['plaintext_size'] ||
      await digest(plain) != m['plaintext_root']) {
    throw StateError('FILE_CHECKSUM_INVALID');
  }
  return plain;
}

Future<Map<String, dynamic>> prepareLocalFile(
  Uint8List bytes,
  String name,
  String recipient,
) async {
  final key = randomBytes(32),
      manifest = <String, dynamic>{
        'suite': fileSuite,
        'data_key': base64Encode(key),
        'plaintext_root': await digest(bytes),
        'plaintext_size': bytes.length,
        'file_name': safeFileName(name),
      };
  final cipher = await encryptDare(bytes, key);
  final encrypted = await AgeEncrypter(
    recipients: [X25519Recipient.parse(recipient)],
  ).encrypt(Uint8List.fromList(utf8.encode(jsonEncode(manifest))));
  return {
    'cipher': cipher,
    'root': await digest(cipher),
    'plainRoot': manifest['plaintext_root'],
    'manifest': base64Encode(encrypted),
    'name': manifest['file_name'],
  };
}

Future<Uint8List> fileFrame(int index, Uint8List bytes) async {
  final out = Uint8List(36 + bytes.length);
  ByteData.sublistView(out).setUint32(0, index);
  out.setAll(4, (await Sha256().hash(bytes)).bytes);
  out.setAll(36, bytes);
  return out;
}

Iterable<Uint8List> fragments(Uint8List frame) sync* {
  for (var off = 0; off < frame.length; off += 16352) {
    final n = min(16352, frame.length - off),
        packet = Uint8List(8 + n),
        view = ByteData.sublistView(packet);
    view.setUint32(0, ByteData.sublistView(frame).getUint32(0));
    view.setUint32(4, off);
    packet.setRange(8, 8 + n, frame, off);
    yield packet;
  }
}

class FrameAssembler {
  FrameAssembler(this.size, this.next);
  final int size;
  int next, offset = 0;
  Uint8List? pending;
  Uint8List? push(Uint8List packet) {
    if (packet.length < 9 || packet.length > 16384) {
      throw StateError('FILE_FRAME_INVALID');
    }
    final view = ByteData.sublistView(packet),
        index = view.getUint32(0),
        off = view.getUint32(4),
        length = min(wireChunkSize, size - index * wireChunkSize) + 36;
    if (index != next ||
        index >= max(1, (size + wireChunkSize - 1) ~/ wireChunkSize) ||
        off != offset ||
        off + packet.length - 8 > length) {
      throw StateError('FILE_FRAME_INVALID');
    }
    pending ??= Uint8List(length);
    pending!.setRange(off, off + packet.length - 8, packet, 8);
    offset += packet.length - 8;
    if (offset != length) return null;
    final frame = pending!;
    if (ByteData.sublistView(frame).getUint32(0) != index) {
      throw StateError('FILE_FRAME_INVALID');
    }
    pending = null;
    offset = 0;
    next++;
    return frame;
  }
}
