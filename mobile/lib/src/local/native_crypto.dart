import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';
import 'package:flutter/foundation.dart'
    show LicenseRegistry, LicenseEntryWithLineBreaks;
import 'package:flutter/services.dart' show rootBundle;
import 'package:sodium/sodium.dart' as sodium;

// Bundled native assets: initialization never contacts a server. Keep the
// existing 32-byte Ed25519 seed and public identity when changing providers.
Future<void>? _initialization;
Future<void> initializeLocalCrypto() => _initialization ??= () async {
  final library = await sodium.SodiumInit.init();
  Cryptography.instance = _NativeCryptography(library);
  LicenseRegistry.addLicense(() async* {
    yield LicenseEntryWithLineBreaks([
      'libsodium',
    ], await rootBundle.loadString('assets/licenses/libsodium-ISC.txt'));
    yield LicenseEntryWithLineBreaks([
      'WebRTC and bundled dependencies',
    ], await rootBundle.loadString('assets/licenses/WebRTC-NOTICES.txt'));
  });
}();

class _NativeCryptography extends DartCryptography {
  _NativeCryptography(sodium.Sodium library) : _sign = _NativeEd25519(library);
  final Ed25519 _sign;
  @override
  Ed25519 ed25519() => _sign;
}

class _NativeEd25519 extends Ed25519 {
  _NativeEd25519(this.library) : super.constructor();
  final sodium.Sodium library;

  @override
  Future<SimpleKeyPair> newKeyPairFromSeed(List<int> seed) async {
    if (seed.length != 32) throw ArgumentError('Ed25519 seed must be 32 bytes');
    final secret = library.secureCopy(Uint8List.fromList(seed));
    try {
      final pair = library.crypto.sign.seedKeyPair(secret);
      try {
        return SimpleKeyPairData(
          List<int>.from(seed),
          publicKey: SimplePublicKey(pair.publicKey, type: KeyPairType.ed25519),
          type: KeyPairType.ed25519,
        );
      } finally {
        pair.dispose();
      }
    } finally {
      secret.dispose();
    }
  }

  @override
  Future<Signature> sign(List<int> message, {required KeyPair keyPair}) async {
    if (keyPair is! SimpleKeyPair) {
      throw ArgumentError('Expected an Ed25519 key pair');
    }
    final data = await keyPair.extract();
    if (data.type != KeyPairType.ed25519) {
      throw ArgumentError('Expected Ed25519');
    }
    if (data.bytes.length != 32) throw ArgumentError('Invalid Ed25519 seed');
    final seed = library.secureCopy(Uint8List.fromList(data.bytes));
    try {
      final pair = library.crypto.sign.seedKeyPair(seed);
      try {
        if (data.publicKey !=
            SimplePublicKey(pair.publicKey, type: KeyPairType.ed25519)) {
          throw ArgumentError('Ed25519 public key does not match its seed');
        }
        return Signature(
          library.crypto.sign.detached(
            message: Uint8List.fromList(message),
            secretKey: pair.secretKey,
          ),
          publicKey: data.publicKey,
        );
      } finally {
        pair.dispose();
      }
    } finally {
      seed.dispose();
    }
  }

  @override
  Future<bool> verify(List<int> message, {required Signature signature}) async {
    final key = signature.publicKey;
    if (key is! SimplePublicKey ||
        key.type != KeyPairType.ed25519 ||
        key.bytes.length != 32 ||
        signature.bytes.length != 64) {
      return false;
    }
    return library.crypto.sign.verifyDetached(
      message: Uint8List.fromList(message),
      signature: Uint8List.fromList(signature.bytes),
      publicKey: Uint8List.fromList(key.bytes),
    );
  }
}
