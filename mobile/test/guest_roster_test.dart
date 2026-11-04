import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/guest/store.dart';

void main() {
  test('authoritative refresh preserves local names and rejects identity changes atomically', () async {
    final db = ClientDatabase.testing(NativeDatabase.memory());
    final store = NativeGuestStore(db);
    await store.initialize();
    final peer = <String, dynamic>{
      'pair_id': 'pair',
      'owner_id': 'guest-a',
      'device_id': 'a',
      'device_name': 'Phone',
      'encryption_key': 'enc',
      'signing_key': 'sig',
      'online': true,
    };
    await store.replacePeers([peer]);
    await store.renamePeer('pair', 'My phone');
    await store.replacePeers([
      {...peer, 'device_name': 'New name', 'online': false},
    ]);
    expect((await store.peers()).single['local_name'], 'My phone');
    await expectLater(
      store.replacePeers([
        {...peer, 'signing_key': 'changed'},
      ]),
      throwsStateError,
    );
    final retained = (await store.peers()).single;
    expect(retained['signing_key'], 'sig');
    expect(retained['local_name'], 'My phone');
    await store.replacePeers([]);
    expect(await store.peers(), isEmpty);
    await db.close();
  });
}
