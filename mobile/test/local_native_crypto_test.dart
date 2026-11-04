import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/local/native_crypto.dart';

void main() {
  test(
    'bundled native Ed25519 preserves seeded identity and Dart wire signatures',
    () async {
      await initializeLocalCrypto();
      final native = Ed25519(), pure = DartEd25519();
      final seed = List.generate(32, (i) => i),
          message = utf8.encode('pinelink-local-browser-control-v1\n{}');
      final key = await native.newKeyPairFromSeed(seed),
          dartKey = await pure.newKeyPairFromSeed(seed);
      expect(await key.extractPrivateKeyBytes(), seed);
      expect(await key.extractPublicKey(), await dartKey.extractPublicKey());
      final a = await native.sign(message, keyPair: key),
          b = await pure.sign(message, keyPair: dartKey);
      expect(a.bytes, b.bytes);
      expect(await native.verify(message, signature: b), isTrue);
      expect(await pure.verify(message, signature: a), isTrue);
      expect(await native.verify([...message, 1], signature: a), isFalse);
      final wrongKey = await pure.newKeyPairFromSeed(List.filled(32, 255));
      expect(
        await native.verify(
          message,
          signature: Signature(
            a.bytes,
            publicKey: await wrongKey.extractPublicKey(),
          ),
        ),
        isFalse,
      );
      expect(
        await native.verify(
          message,
          signature: Signature([0], publicKey: a.publicKey),
        ),
        isFalse,
      );
      final mismatched = SimpleKeyPairData(
        seed,
        publicKey: await wrongKey.extractPublicKey(),
        type: KeyPairType.ed25519,
      );
      await expectLater(
        native.sign(message, keyPair: mismatched),
        throwsArgumentError,
      );
    },
  );
}
