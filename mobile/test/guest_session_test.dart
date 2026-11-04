import 'dart:async';
import 'dart:convert';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/data/wire.dart' as wire;
import 'package:pinelink_mobile/src/guest/session.dart';
import 'package:pinelink_mobile/src/guest/runtime.dart';

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
  test('native visitor keys are separate per service and cannot reuse an account database', () async {
    final vault = _Vault(),
        aDb = ClientDatabase.testing(NativeDatabase.memory()),
        bDb = ClientDatabase.testing(NativeDatabase.memory());
    final a = await NativeGuestRuntime.open(
      server: 'https://service-a.example/api',
      vault: vault,
      database: aDb,
    );
    final b = await NativeGuestRuntime.open(
      server: 'https://service-b.example/api',
      vault: vault,
      database: bDb,
    );
    expect(a.identity.root, isNot(b.identity.root));
    expect(a.session.namespace, isNot(b.session.namespace));
    await a.close();
    await b.close();
    final accountDb = ClientDatabase.testing(NativeDatabase.memory());
    await LocalIdentity.load(accountDb, vault, 'account', '');
    await expectLater(
      NativeGuestRuntime.open(
        server: 'https://service-a.example/api',
        vault: vault,
        database: accountDb,
      ),
      throwsStateError,
    );
  });
  test('native enrollment and requests sign exact bytes and keep the token in a service-scoped vault', () async {
    final db = ClientDatabase.testing(NativeDatabase.memory()),
        vault = _Vault();
    final identity = await LocalIdentity.load(
      db,
      vault,
      'guest:service-a',
      'test-device',
    );
    final seen = <String>[];
    final client = MockClient((request) async {
      seen.add(request.url.path);
      expect(request.headers['X-Pinelink-Surface'], 'app');
      expect(request.followRedirects, isFalse);
      if (request.url.path.endsWith('/challenge')) {
        return http.Response(
          jsonEncode({
            'challenge': 'one-use',
            'unix_time': DateTime.now().millisecondsSinceEpoch ~/ 1000,
          }),
          201,
        );
      }
      if (request.url.path.endsWith('/enroll')) {
        final payload = jsonDecode(request.body);
        expect(payload['device_id'], identity.device);
        final message = utf8.encode(
          'pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n${await digest(utf8.encode(request.body))}',
        );
        expect(
          await wire.verify(
            identity.signingKey,
            message,
            request.headers['X-Pinelink-Signature']!,
          ),
          isTrue,
        );
        return http.Response(
          jsonEncode({
            'guest': {
              'owner_id': 'guest-a',
              'device_id': identity.device,
              'client_kind': 'native',
              'surface': 'app',
            },
            'session_token': 'fixture-only-token',
          }),
          200,
        );
      }
      expect(request.headers['X-Pinelink-Guest-Session'], 'fixture-only-token');
      final timestamp = request.headers['X-Pinelink-Timestamp'];
      final message = utf8.encode(
        'GET\n/v1/guest/connections\n$timestamp\n${await digest([])}',
      );
      expect(
        await wire.verify(
          identity.signingKey,
          message,
          request.headers['X-Pinelink-Signature']!,
        ),
        isTrue,
      );
      return http.Response(jsonEncode({'peers': []}), 200);
    });
    final session = await GuestSession.open(
      server: 'https://service-a.example/api',
      identity: identity,
      vault: vault,
      client: client,
    );
    await Future.wait([session.ensureSession(), session.ensureSession()]);
    expect(seen.where((v) => v.endsWith('/enroll')), hasLength(1));
    expect(await session.request('GET', '/v1/guest/connections'), {
      'peers': [],
    });
    expect(
      vault
          .data['${await GuestSession.scopeFor('https://service-a.example/api')}.guest-session'],
      contains('fixture-only-token'),
    );
    expect(await db.metadata('guest-session'), isNull);
    expect(vault.data['guest:service-b.guest-session'], isNull);
    session.close();
    await db.close();
  });

  test('account conversations reuse account keys, isolate ciphertext and stop on session change', () async {
    final vault = _Vault();
    final identityDb = ClientDatabase.testing(NativeDatabase.memory());
    final accountIdentity = await LocalIdentity.load(
      identityDb,
      vault,
      'account-profile',
      'account-device',
    );
    final visitor = await NativeGuestRuntime.open(
      server: 'https://service-a.example/api',
      vault: vault,
      database: ClientDatabase.testing(NativeDatabase.memory()),
    );
    var current = true;
    var requests = 0;
    var holdResponse = false;
    final started = Completer<void>(), response = Completer<http.Response>();
    final account = await NativeGuestRuntime.openAccount(
      server: 'https://service-a.example/api',
      owner: 'acct-fixture',
      identity: accountIdentity,
      vault: vault,
      database: ClientDatabase.testing(NativeDatabase.memory()),
      current: () => current,
      headers: {
        'X-Pinelink-Session': 'account-token',
        'X-Pinelink-CSRF': 'csrf',
      },
      client: MockClient((request) async {
        requests++;
        expect(request.url.path, '/api/v1/guest/connections');
        expect(request.headers['X-Pinelink-Session'], 'account-token');
        expect(request.headers['X-Pinelink-CSRF'], 'csrf');
        expect(request.headers['X-Pinelink-Guest-Session'], isNull);
        expect(request.headers['X-Pinelink-Owner'], 'acct-fixture');
        final message = utf8.encode(
          'GET\n/v1/guest/connections\n${request.headers['X-Pinelink-Timestamp']}\n${await digest([])}',
        );
        expect(
          await wire.verify(
            accountIdentity.signingKey,
            message,
            request.headers['X-Pinelink-Signature']!,
          ),
          isTrue,
        );
        if (holdResponse) {
          started.complete();
          return response.future;
        }
        return http.Response(jsonEncode({'peers': []}), 200);
      }),
    );
    expect(account.identity.root, accountIdentity.root);
    expect(account.identity.root, isNot(visitor.identity.root));
    expect(account.session.namespace, isNot(visitor.session.namespace));
    expect(await account.database.metadata('profile'), isNull);
    expect(
      vault.data.keys.where((key) => key.startsWith(account.session.namespace)),
      isEmpty,
    );
    await account.session.ensureSession();
    expect(await account.session.request('GET', '/v1/guest/connections'), {
      'peers': [],
    });
    holdResponse = true;
    final pending = account.session.request('GET', '/v1/guest/connections');
    await started.future;
    final refused = expectLater(pending, throwsStateError);
    current = false;
    response.complete(http.Response(jsonEncode({'peers': []}), 200));
    await refused;
    await expectLater(account.session.ensureSession(), throwsStateError);
    await expectLater(
      account.session.request('GET', '/v1/guest/connections'),
      throwsStateError,
    );
    await expectLater(
      account.session.signedHeaders('GET', '/v1/guest/connections'),
      throwsStateError,
    );
    expect(requests, 2);
    expect(await visitor.database.metadata('guest-service'), isNotNull);
    await account.close();
    expect(await visitor.database.metadata('guest-service'), isNotNull);
    await visitor.close();
    await identityDb.close();
  });

  test('account conversation cannot open a visitor database', () async {
    final vault = _Vault(),
        identityDb = ClientDatabase.testing(NativeDatabase.memory());
    final identity = await LocalIdentity.load(
      identityDb,
      vault,
      'account-profile',
      'account-device',
    );
    final visitorDb = ClientDatabase.testing(NativeDatabase.memory());
    await visitorDb.putMetadata('guest-service', {'scope': 'visitor'});
    await expectLater(
      NativeGuestRuntime.openAccount(
        server: 'https://service-a.example/api',
        owner: 'acct-fixture',
        identity: identity,
        headers: const {},
        current: () => false,
        database: visitorDb,
        vault: vault,
      ),
      throwsStateError,
    );
    await identityDb.close();
  });
}
