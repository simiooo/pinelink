import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/guest/store.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'receiver consent, ciphertext and checkpoint are transactional and fenced',
    () async {
      final db = ClientDatabase.testing(NativeDatabase.memory()),
          store = NativeGuestStore(db);
      await store.initialize();
      await store.create({
        'id': 'one',
        'direction': 'receive',
        'state': 'PENDING',
        'received': 0,
      });
      final chunk = Uint8List.fromList([1, 2, 3]);
      await expectLater(
        store.update('one', 0, {'received': 3}, index: 0, chunk: chunk),
        throwsStateError,
      );
      expect(await store.bytes('one', 'received'), isNull);
      await store.update('one', 0, {'state': 'ACCEPTED'});
      await db.customStatement(
        "CREATE TEMP TRIGGER reject_guest_chunk BEFORE INSERT ON guest_chunks BEGIN SELECT RAISE(ABORT, 'test failure'); END",
      );
      await expectLater(
        store.update('one', 1, {'received': 3}, index: 0, chunk: chunk),
        throwsA(anything),
      );
      expect((await store.job('one'))!['received'], 0);
      expect(await store.bytes('one', 'received'), isNull);
      await db.customStatement('DROP TRIGGER reject_guest_chunk');
      await store.update('one', 1, {'received': 3}, index: 0, chunk: chunk);
      expect(await store.bytes('one', 'received'), [1, 2, 3]);
      await expectLater(
        store.update('one', 1, {'received': 4}),
        throwsStateError,
      );
      await expectLater(
        store.update(
          'one',
          2,
          {'received': 3},
          index: 0,
          chunk: Uint8List.fromList([4]),
        ),
        throwsStateError,
      );
      expect((await store.job('one'))!['localRevision'], 2);
      await db.close();
    },
  );
}
