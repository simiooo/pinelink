import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/local/file_crypto.dart';
import 'package:pinelink_mobile/src/local/protocol.dart';
import 'package:pinelink_mobile/src/local/store.dart';
import 'package:pinelink_mobile/src/local/discovery.dart';

class _Vault implements IdentityVault {
  final data = <String, String>{};
  @override
  Future<String?> read(String key) async => data[key];
  @override
  Future<void> write(String key, String value) async {
    data[key] = value;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late ClientDatabase db;
  late LocalIdentity identity;
  setUp(() async {
    db = ClientDatabase.testing(NativeDatabase.memory());
    identity = await LocalIdentity.load(db, _Vault(), 'test', 'test-device');
  });
  tearDown(() => db.close());
  test('bounds optional capabilities on signed peer identities', () async {
    final peer = publicIdentity(identity, 'Phone');
    await validateIdentity(peer);
    await validateIdentity({
      ...peer,
      'capabilities': ['text-v1'],
    });
    await expectLater(
      validateIdentity({
        ...peer,
        'capabilities': ['text-v1', 'text-v1'],
      }),
      throwsStateError,
    );
    await expectLater(
      validateIdentity({...peer, 'capabilities': null}),
      throwsStateError,
    );
  });
  test(
    'an empty source remains an empty blob when reading the wire chunk',
    () async {
      final store = LocalStore(db);
      await store.initialize();
      await store.bytes('empty', 'source', 0, Uint8List(0));
      expect(
        await store.readBytes(
          'empty',
          'source',
          start: 0,
          length: wireChunkSize,
        ),
        isEmpty,
      );
      expect(await store.readBytes('empty', 'source'), isEmpty);
      expect(
        await store.readBytes(
          'missing',
          'source',
          start: 0,
          length: wireChunkSize,
        ),
        isNull,
      );
    },
  );
  test('a failed batch cannot leave chunks, an advanced ACK or a journal entry behind', () async {
    final store = LocalStore(db);
    await store.initialize();
    final root = 'b' * 64;
    await store.reserve(root, 'test-batch');
    final state = <String, dynamic>{
      'id': 'local:${randomUuid()}',
      'recipient': identity.root,
      'state': 'ACCEPTED',
      'version': 2,
      'revision': 0,
      'received': 0,
      'paused': false,
    };
    final job = <String, dynamic>{
      'id': state['id'],
      'created': 1,
      'peer': {'root': root},
      'state': state,
    };
    await store.put(job);
    await db.customStatement(
      "CREATE TEMP TRIGGER fail_batch BEFORE INSERT ON local_bytes WHEN NEW.idx=1 BEGIN SELECT RAISE(ABORT,'simulated write failure'); END",
    );
    final batch = [
          (index: 0, bytes: Uint8List.fromList([1])),
          (index: 1, bytes: Uint8List.fromList([2])),
        ],
        next = {...state, 'revision': 1, 'received': 2};
    await expectLater(
      store.saveState(job, next, chunks: batch),
      throwsA(anything),
    );
    expect((await store.job(job['id']))!['state']['received'], 0);
    expect(await store.hasBytes(job['id'], 'chunk'), isFalse);
    expect(
      (await db
              .customSelect('SELECT COUNT(*) AS n FROM local_journal')
              .getSingle())
          .read<int>('n'),
      0,
    );
    await db.customStatement('DROP TRIGGER fail_batch');
    await store.saveState(job, next, chunks: batch);
    expect((await store.job(job['id']))!['state']['received'], 2);
    expect(await store.readBytes(job['id'], 'chunk'), [1, 2]);
  });
  test('LAN signal sockets allow private IPv4, IPv6 and mapped peers but reject public hosts', () {
    for (final host in [
      '192.168.1.5',
      '10.0.2.16',
      '172.20.0.2',
      '127.0.0.1',
      '::1',
      'fd58:958::2',
      'fe80::1',
      '::ffff:10.0.2.16',
    ]) {
      expect(privateAddress(InternetAddress(host)), isTrue, reason: host);
    }
    for (final host in [
      '8.8.8.8',
      '1.1.1.1',
      '172.32.0.1',
      '0.0.0.0',
      '::',
      '2001:4860:4860::8888',
      '::ffff:8.8.8.8',
    ]) {
      expect(privateAddress(InternetAddress(host)), isFalse, reason: host);
    }
  });
  test(
    'Go DARE vectors, age envelope and native-to-Go encryption agree',
    () async {
      await Directory('../.artifacts').create(recursive: true);
      final binary = File('../.artifacts/localhistoryfixture').absolute.path;
      final build = await Process.run('go', [
        'build',
        '-o',
        binary,
        './cmd/historyfixture',
      ], workingDirectory: '../cli');
      expect(build.exitCode, 0, reason: '${build.stderr}');
      final child = await Process.start(binary, []);
      addTearDown(() => child.kill());
      final lines = StreamIterator(
        child.stdout.transform(utf8.decoder).transform(const LineSplitter()),
      );
      final errors = child.stderr.transform(utf8.decoder).join();
      child.stdin.writeln(
        jsonEncode({
          'root': identity.root,
          'signing_key': identity.signingKey,
          'encryption_key': identity.encryptionKey,
        }),
      );
      expect(await lines.moveNext(), isTrue);
      final fixture = jsonDecode(lines.current) as Json;
      for (final vector in fixture['file_vectors']) {
        final plain = Uint8List.fromList(
          List.generate(vector['size'], (i) => (i * 29 + 7) & 255),
        );
        final cipher = await encryptDare(
          plain,
          List.generate(32, (i) => i),
          nonce: List.generate(12, (i) => i),
          suite: vector['cipher'],
        );
        expect(await digest(cipher), vector['root']);
        expect(
          await decryptDare(cipher, {
            'suite': fileSuite,
            'data_key': base64Encode(List.generate(32, (i) => i)),
            'plaintext_size': plain.length,
            'plaintext_root': await digest(plain),
            'file_name': 'fixture.bin',
          }),
          plain,
        );
      }
      final m = await identity.decrypt(fixture['file_manifest']);
      expect(
        utf8.decode(await decryptDare(base64Decode(fixture['file_cipher']), m)),
        'Pinelink browser / Go interoperability\n',
      );
      final source = Uint8List.fromList(List.filled(131073, 19)),
          prepared = await prepareLocalFile(
            source,
            '中文 <>&.bin',
            fixture['peer']['encryption_key'],
          );
      child.stdin.writeln(
        jsonEncode({
          'manifest': prepared['manifest'],
          'cipher': base64Encode(prepared['cipher']),
        }),
      );
      await child.stdin.close();
      expect(await lines.moveNext(), isTrue);
      final result = jsonDecode(lines.current);
      expect(result['verified'], isTrue);
      expect(result['root'], await digest(source));
      expect(result['size'], source.length);
      expect(await child.exitCode, 0, reason: await errors);
    },
    timeout: const Timeout(Duration(minutes: 2)),
  );
  test('signed connection package survives QR encoding and rejects key substitution', () async {
    const sdp =
        'v=0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=fingerprint:sha-256 00:AA\r\na=ice-ufrag:test\r\na=ice-pwd:test\r\n';
    final offer = await signed(identity, 'connection', {
      'v': 1,
      'type': 'offer',
      'id': randomUuid(),
      'nonce': base64UrlEncode(randomBytes(32)).replaceAll('=', ''),
      'peer': publicIdentity(identity, '设备 <>&'),
      'sdp': sdp,
      'reply': '',
    });
    expect(await decodePackage(encodePackage(offer)), offer);
    final changed = {
      ...offer,
      'peer': {...offer['peer'], 'encryption_key': 'age1invalid'},
    };
    await expectLater(decodePackage(encodePackage(changed)), throwsStateError);
    await expectLater(
      decodePackage('PL2:${base45(zlib.encode(List.filled(25000, 65)))}:'),
      throwsStateError,
    );
    for (var n = 0; n < 180; n++) {
      final b = Uint8List.fromList(randomBytes(n));
      expect(unbase45(base45(b)), b);
    }
  });
  test('frame assembler bounds queues and fences overlapping or replayed fragments', () async {
    final payload = Uint8List.fromList(List.filled(65536, 41)),
        frame = await fileFrame(0, payload),
        assembler = FrameAssembler(payload.length, 0),
        parts = fragments(frame).toList();
    Uint8List? full;
    for (final part in parts) {
      full = assembler.push(part);
    }
    expect(full, frame);
    expect(() => assembler.push(parts.first), throwsStateError);
    final next = FrameAssembler(payload.length, 0);
    next.push(parts.first);
    expect(() => next.push(parts.first), throwsStateError);
    final empty = await fileFrame(0, Uint8List(0));
    expect(FrameAssembler(0, 0).push(fragments(empty).single), empty);
  });
  test('durable ACK, journal and chunks commit atomically; cancellation fences late verification', () async {
    final store = LocalStore(db), contender = LocalStore(db);
    await store.initialize();
    final root = 'a' * 64;
    await store.reserve(root, 'session');
    await expectLater(contender.reserve(root, 'other'), throwsStateError);
    await store.release(root, 'stale-session');
    await store.lease(root, 'session');
    final state = <String, dynamic>{
      'id': 'local:${randomUuid()}',
      'recipient': identity.root,
      'state': 'ACCEPTED',
      'version': 2,
      'revision': 0,
      'received': 0,
      'paused': false,
    };
    final job = <String, dynamic>{
      'id': state['id'],
      'created': 1,
      'direction': 'receive',
      'peer': {'root': root},
      'state': state,
    };
    await store.put(job);
    final committed = {...state, 'revision': 1, 'received': 7};
    await store.saveState(
      job,
      committed,
      chunk: Uint8List.fromList([1, 2, 3]),
      index: 0,
    );
    expect((await store.job(job['id']))!['state']['received'], 7);
    expect(await store.readBytes(job['id'], 'chunk'), [1, 2, 3]);
    final current = (await store.job(job['id']))!;
    await store.saveState(current, {
      ...committed,
      'version': 3,
      'state': 'CANCELLED',
    });
    await expectLater(
      store.saveState(current, {
        ...committed,
        'version': 3,
        'state': 'COMPLETED',
      }, plain: Uint8List.fromList([9])),
      throwsStateError,
    );
    expect(await store.readBytes(job['id'], 'chunk'), isNull);
    expect(await store.readBytes(job['id'], 'plain'), isNull);
    expect(
      (await db
              .customSelect('SELECT COUNT(*) AS n FROM local_journal')
              .getSingle())
          .read<int>('n'),
      2,
    );
    await store.consume('nonce');
    await expectLater(store.consume('nonce'), throwsStateError);
    await store.release(root, 'session');
    await contender.reserve(root, 'new');
    await expectLater(store.lease(root), throwsStateError);
  });
}
