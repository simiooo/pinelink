import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:dartage/dartage.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'database.dart';

String hexBytes(List<int> bytes) =>
    bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
Future<String> digest(List<int> bytes) async =>
    hexBytes((await Sha256().hash(bytes)).bytes);
List<int> randomBytes(int n) {
  final r = Random.secure();
  return List.generate(n, (_) => r.nextInt(256));
}

String randomUuid() {
  final b = randomBytes(16);
  b[6] = (b[6] & 15) | 64;
  b[8] = (b[8] & 63) | 128;
  final h = hexBytes(b);
  return '${h.substring(0, 8)}-${h.substring(8, 12)}-${h.substring(12, 16)}-${h.substring(16, 20)}-${h.substring(20)}';
}

abstract interface class IdentityVault {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
}

class PlatformVault implements IdentityVault {
  final _storage = const FlutterSecureStorage();
  @override
  Future<String?> read(String key) => _storage.read(key: key);
  @override
  Future<void> write(String key, String value) =>
      _storage.write(key: key, value: value);
}

class LocalIdentity {
  LocalIdentity(this.public, this._signing, this._age);
  final Map<String, dynamic> public;
  final SimpleKeyPair _signing;
  final X25519Identity _age;
  String get root => public['root'];
  String get device => public['device'];
  String get signingKey => public['signing_key'];
  String get encryptionKey => public['encryption_key'];
  String get stream => public['stream'];
  static Future<LocalIdentity> load(
    ClientDatabase db,
    IdentityVault vault,
    String namespace,
    String configuredDevice,
  ) async {
    // A write reserves initialization before touching the platform vault. A
    // second isolate/process reuses the winner; a crash after vault publication
    // can recover without inventing a replacement identity.
    return db.transaction(() async {
      await db.putMetadata('identity-initialization', {'schema': 1});
      final existing = await db.metadata('profile');
      final stored = await vault.read('$namespace.identity');
      if (stored == null && existing != null) {
        throw StateError('IDENTITY_UNAVAILABLE');
      }
      Map<String, dynamic> secret;
      if (stored != null) {
        secret = jsonDecode(stored) as Map<String, dynamic>;
      } else {
        final sign = await Ed25519().newKeyPair();
        final age = X25519Identity.generate();
        secret = {
          'seed': base64Encode(await sign.extractPrivateKeyBytes()),
          'age': age.encoded,
          'device': configuredDevice.isEmpty
              ? 'flutter-${randomUuid()}'
              : configuredDevice,
        };
        await vault.write('$namespace.identity', jsonEncode(secret));
      }
      final sign = await Ed25519().newKeyPairFromSeed(
        base64Decode(secret['seed']),
      );
      final publicSign = base64Encode((await sign.extractPublicKey()).bytes);
      final age = X25519Identity.parse(secret['age']);
      final root = await digest(base64Decode(publicSign));
      if (existing != null && existing['root'] != root) {
        throw StateError('IDENTITY_MISMATCH');
      }
      final public =
          existing ??
          {
            'root': root,
            'device': secret['device'],
            'stream': randomUuid(),
            'signing_key': publicSign,
            'encryption_key': (await age.recipient()).encoded,
          };
      await db.putMetadata('profile', public);
      return LocalIdentity(public, sign, age);
    });
  }

  Future<String> sign(List<int> bytes) async =>
      base64Encode((await Ed25519().sign(bytes, keyPair: _signing)).bytes);
  Future<Map<String, dynamic>> decrypt(String value) async {
    final bytes = base64Decode(value);
    if (bytes.length > 65536) throw StateError('HISTORY_TOO_LARGE');
    final plaintext = await AgeDecrypter(identities: [_age])
        .decrypt(Uint8List.fromList(bytes));
    if (plaintext.length > 32768) throw StateError('HISTORY_TOO_LARGE');
    return jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;
  }
}
