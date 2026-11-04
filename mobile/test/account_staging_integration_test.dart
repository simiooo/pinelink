import 'dart:convert';
import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter/services.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/auth/controller.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/guest/account.dart';
import 'package:pinelink_mobile/src/guest/coordinator.dart';
import 'package:pinelink_mobile/src/guest/runtime.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _Vault implements IdentityVault {
  final values = <String, String>{};
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

Future<void> _until(Future<bool> Function() condition) async {
  final end = DateTime.now().add(const Duration(seconds: 90));
  while (!await condition()) {
    if (DateTime.now().isAfter(end)) throw StateError('TEST_TIMEOUT');
    await Future<void>.delayed(const Duration(milliseconds: 300));
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final secretFile = Platform.environment['PINELINK_ACCOUNT_SMOKE_FILE'];
  test(
    'real native account and visitor pair, exchange encrypted text and files, then revoke',
    () async {
      HttpOverrides.global = null;
      final secrets = jsonDecode(
        await File(secretFile!).readAsString(),
      ) as Map<String, dynamic>;
      final directory = await Directory.systemTemp.createTemp(
        'pinelink-account-smoke-',
      );
      SharedPreferences.setMockInitialValues({});
      FlutterSecureStorage.setMockInitialValues({});
      const pathChannel = MethodChannel('plugins.flutter.io/path_provider');
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(pathChannel, (_) async => directory.path);
      final auth = AccountController(
        server: 'https://staging-pinelink.sqmem.top/api',
        vault: _Vault(),
      );
      NativeGuestCoordinator? account, visitor;
      try {
        await auth.load();
        await auth.login(
          secrets['email'] as String,
          secrets['password'] as String,
        );
        expect(auth.signedIn, isTrue);
        account = await openNativeAccountCoordinator(auth);
        visitor = await NativeGuestCoordinator.open(
          await NativeGuestRuntime.open(
            server: auth.server,
            vault: _Vault(),
            database: ClientDatabase.testing(NativeDatabase.memory()),
          ),
          autoPoll: false,
        );
        await account.poll();
        await visitor.poll();
        expect(account.online, isTrue);
        expect(visitor.online, isTrue);
        final invite = await account.invite();
        final candidate = await visitor.join(invite['code'] as String);
        await account.poll();
        expect(account.peers, isEmpty);
        expect(visitor.peers, isEmpty);
        await account.respondCandidate(candidate['id'] as String, true);
        await account.poll();
        await visitor.poll();
        expect(account.peers, hasLength(1));
        expect(visitor.peers, hasLength(1));
        final message = Uint8List.fromList(
          utf8.encode('# Account\n\n**sensitive**'),
        );
        final sent = await account.send(
          message,
          'Text',
          account.peers.single,
          sensitivity: 'SENSITIVE',
        );
        await _until(() async {
          await visitor!.poll();
          return visitor!.jobs.any(
            (j) => j['direction'] == 'receive' && j['state'] == 'PENDING',
          );
        });
        final incoming = visitor.jobs.singleWhere(
          (j) => j['direction'] == 'receive',
        );
        expect(
          await visitor.runtime.store.bytes(
            incoming['id'] as String,
            'received',
          ),
          isNull,
        );
        await visitor.action(incoming['id'] as String, 'accept');
        await _until(() async {
          await account!.poll();
          await visitor!.poll();
          return visitor!.jobs.any(
            (j) => j['id'] == incoming['id'] && j['state'] == 'COMPLETED',
          );
        });
        expect(
          (await visitor.runtime.text(incoming['id'] as String))?.value,
          utf8.decode(message),
        );
        expect(
          (await visitor.runtime.text(incoming['id'] as String))?.sensitive,
          isTrue,
        );
        await _until(() async {
          await account!.poll();
          return account!.jobs.any(
            (j) => j['id'] == sent && j['state'] == 'COMPLETED',
          );
        });
        final bytes = Uint8List.fromList(List.generate(4096, (i) => i % 251));
        await visitor.send(bytes, 'visitor-file.bin', visitor.peers.single);
        await _until(() async {
          await account!.poll();
          return account!.jobs.any(
            (j) => j['direction'] == 'receive' && j['state'] == 'PENDING',
          );
        });
        final received = account.jobs.singleWhere(
          (j) => j['direction'] == 'receive',
        );
        expect(
          await account.runtime.store.bytes(
            received['id'] as String,
            'received',
          ),
          isNull,
        );
        await account.action(received['id'] as String, 'accept');
        await _until(() async {
          await account!.poll();
          await visitor!.poll();
          return account!.jobs.any(
            (j) => j['id'] == received['id'] && j['state'] == 'COMPLETED',
          );
        });
        expect(
          await account.runtime.plaintext(received['id'] as String),
          bytes,
        );
        await account.remove(account.peers.single['pair_id'] as String);
        await account.poll();
        await visitor.poll();
        expect(account.peers, isEmpty);
        expect(visitor.peers, isEmpty);
        await auth.signOut();
        await expectLater(
          account.runtime.session.ensureSession(),
          throwsStateError,
        );
      } finally {
        await account?.close();
        await visitor?.close();
        auth.dispose();
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
            .setMockMethodCallHandler(pathChannel, null);
        await directory.delete(recursive: true);
      }
    },
    skip: secretFile == null
        ? 'requires private staging test credentials'
        : false,
    timeout: const Timeout(Duration(minutes: 8)),
  );
}
