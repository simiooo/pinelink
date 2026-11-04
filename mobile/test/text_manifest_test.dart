import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/local/file_crypto.dart';

void main() {
  Map<String, dynamic> base() => {
    'suite': fileSuite,
    'data_key': base64Encode(Uint8List(32)),
    'plaintext_root': 'a' * 64,
    'plaintext_size': 1,
    'file_name': 'Text',
  };

  test('accepts legacy files and recognized typed text metadata', () {
    expect(() => validateManifest(base()), returnsNormally);
    expect(
      () => validateManifest({
        ...base(),
        'content_version': 1,
        'content_kind': 'TEXT',
        'sensitivity': 'SENSITIVE',
      }),
      returnsNormally,
    );
    for (final invalid in [
      {'content_kind': 'TEXT'},
      {'content_kind': 'TEXT', 'content_version': 2, 'sensitivity': 'NORMAL'},
      {'content_kind': 'TEXT', 'content_version': 1, 'sensitivity': 'OTHER'},
      {
        'content_kind': 'FILE',
        'content_version': 1,
        'sensitivity': 'SENSITIVE',
      },
      {'content_kind': 'OTHER', 'content_version': 1},
      {'content_kind': null},
    ]) {
      expect(() => validateManifest({...base(), ...invalid}), throwsStateError);
    }
    expect(
      () => validateManifest({
        ...base(),
        'plaintext_size': textByteLimit + 1,
        'content_version': 1,
        'content_kind': 'TEXT',
        'sensitivity': 'NORMAL',
      }),
      throwsStateError,
    );
  });
}
