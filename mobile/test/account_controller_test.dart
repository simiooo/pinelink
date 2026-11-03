import 'dart:convert';
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pinelink_mobile/src/auth/controller.dart';
import 'package:pinelink_mobile/src/data/identity.dart';

class MemoryAuthVault implements IdentityVault {
  final values = <String, String>{};
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() => SharedPreferences.setMockInitialValues({}));
  test('a saved native token is revalidated and an outage enables only local access', () async {
    const server = 'https://unit.invalid/api';
    final prefix = 'pinelink-auth-${await digest(utf8.encode(server))}',
        vault = MemoryAuthVault();
    vault.values['$prefix.session'] = jsonEncode({
      'session_token': 'saved-native-token',
    });
    final preferences = await SharedPreferences.getInstance();
    await preferences.setString(
      '$prefix.account',
      jsonEncode({'id': 'acct-saved', 'email': 'person@example.com'}),
    );
    final auth = AccountController(
      server: server,
      vault: vault,
      client: MockClient((request) async {
        expect(request.headers['X-Pinelink-Session'], 'saved-native-token');
        throw http.ClientException('offline');
      }),
    );
    await auth.load();
    expect(auth.signedIn, false);
    expect(auth.localOnly, true);
    expect(auth.owner, 'acct-saved');
    auth.dispose();
  });
  test(
    'offline sign-out stops the profile and retains a bounded revocation retry',
    () async {
      const server = 'https://unit.invalid/api';
      final vault = MemoryAuthVault();
      var stopped = false;
      final auth = AccountController(
        server: server,
        vault: vault,
        client: MockClient(
          (_) async => http.Response('{"code":"AUTH_UNAVAILABLE"}', 503),
        ),
      );
      await auth.load();
      auth.session = {'session_token': 'native-current'};
      auth.account = {'id': 'acct-saved'};
      auth.beforeProfileChange = () async {
        stopped = true;
      };
      final revoked = await auth.signOut();
      expect(stopped, true);
      expect(revoked, false);
      expect(auth.signedIn, false);
      expect(auth.localOnly, true);
      expect(auth.account, isEmpty);
      final prefix = 'pinelink-auth-${await digest(utf8.encode(server))}';
      final preferences = await SharedPreferences.getInstance();
      expect(preferences.getBool('$prefix.logout-pending'), true);
      expect(vault.values['$prefix.session'], contains('native-current'));
      auth.dispose();
    },
  );
  test(
    'two account pages cannot replace one in-flight login request',
    () async {
      final started = Completer<void>(), release = Completer<http.Response>();
      var writes = 0;
      final auth = AccountController(
        server: 'https://unit.invalid/api',
        vault: MemoryAuthVault(),
        client: MockClient((request) async {
          writes++;
          if (!started.isCompleted) started.complete();
          return release.future;
        }),
      );
      final first = expectLater(
        auth.login(
          'first@example.com',
          'a lengthy password for the first person',
        ),
        throwsA(isA<AccountException>()),
      );
      await started.future;
      await expectLater(
        auth.login(
          'second@example.com',
          'a different password for a second person',
        ),
        throwsA(
          isA<AccountException>().having((e) => e.code, 'code', 'AUTH_BUSY'),
        ),
      );
      expect(writes, 1);
      release.complete(http.Response('{"code":"AUTH_UNAVAILABLE"}', 503));
      await first;
      auth.dispose();
    },
  );
}
