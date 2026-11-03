import 'dart:async';
import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import 'browser.dart';

import '../data/identity.dart';
import '../data/runtime.dart';
import 'transport.dart';

class AccountException implements Exception {
  const AccountException(this.code);
  final String code;
  @override
  String toString() => code;
}

class AccountController extends ChangeNotifier with WidgetsBindingObserver {
  AccountController({
    required this.server,
    IdentityVault? vault,
    http.Client? client,
  }) : vault = vault ?? PlatformVault(),
       client = client ?? accountClient() {
    WidgetsBinding.instance.addObserver(this);
  }
  final String server;
  final IdentityVault vault;
  final http.Client client;
  Map<String, dynamic> capabilities = {}, session = {}, account = {};
  Map<String, dynamic>? pending;
  String _namespace = '', deviceCode = '';
  bool managed = true, _closed = false, _active = true, _cancelGoogle = false;
  int profileRevision = 0;
  bool _mutatingAccount = false;
  int _emailRevision = 0;
  int _emailAppliedRevision = 0;
  Future<void> _pendingWrites = Future.value();
  Future<T> _accountOperation<T>(Future<T> Function() work) async {
    if (_mutatingAccount) throw const AccountException('AUTH_BUSY');
    _mutatingAccount = true;
    try {
      return await work();
    } finally {
      _mutatingAccount = false;
    }
  }

  Future<void> login(String email, String password) =>
      _accountOperation(() => _login(email, password));
  Future<void> startEmail(String purpose, String email, String password) =>
      _accountOperation(() => _startEmail(purpose, email, password));
  Future<String> verifyCode(String code) =>
      _accountOperation(() => _verifyCode(code));
  Future<void> resetPassword(String password) =>
      _accountOperation(() => _resetPassword(password));
  Future<void> googleLogin() => _accountOperation(_googleLogin);
  Future<bool> signOut() => _accountOperation(_signOut);

  Future<void> Function()? beforeProfileChange;
  bool get signedIn => session.isNotEmpty;
  bool get localOnly => managed && !signedIn;
  String get owner =>
      account['id'] as String? ??
      const String.fromEnvironment('PINELINK_OWNER', defaultValue: 'dev-owner');
  String get token => kIsWeb ? '' : session['session_token'] as String? ?? '';
  String get csrf => session['csrf'] as String? ?? '';
  Map<String, String> get headers => {
    'X-Pinelink-Surface': 'app',
    if (token.isNotEmpty) 'X-Pinelink-Session': token,
    if (csrf.isNotEmpty) 'X-Pinelink-CSRF': csrf,
  };
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _active = state == AppLifecycleState.resumed;
  }

  void _check() {
    if (_closed) throw StateError('CLIENT_CLOSED');
  }

  Future<Map<String, dynamic>> _request(
    String path, [
    Map<String, dynamic>? body,
    Map<String, String> extra = const {},
  ]) async {
    _check();
    final response =
        await (body == null
                ? client.get(
                    Uri.parse('$server$path'),
                    headers: {...headers, ...extra},
                  )
                : client.post(
                    Uri.parse('$server$path'),
                    headers: {
                      'Content-Type': 'application/json',
                      ...headers,
                      ...extra,
                    },
                    body: jsonEncode(body),
                  ))
            .timeout(const Duration(seconds: 12));
    _check();
    Map<String, dynamic> value;
    try {
      value = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const AccountException('AUTH_UNAVAILABLE');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw AccountException(value['code'] as String? ?? 'AUTH_UNAVAILABLE');
    }
    return value;
  }

  Future<void> load() async {
    _namespace = 'pinelink-auth-${await digest(utf8.encode(server))}';
    final prefs = await SharedPreferences.getInstance();
    try {
      account = jsonDecode(
        prefs.getString('$_namespace.account') ?? '{}',
      ) as Map<String, dynamic>;
    } catch (_) {
      account = {};
    }
    var logoutPending = prefs.getBool('$_namespace.logout-pending') ?? false;
    try {
      final raw = await vault.read('$_namespace.session');
      if (!kIsWeb && raw != null && raw.isNotEmpty) {
        session = jsonDecode(raw) as Map<String, dynamic>;
      }
      capabilities = await _request('/v1/auth/capabilities');
      managed = capabilities['enabled'] == true;
      if (managed) {
        final savedToken = token;
        session = await _request('/v1/auth/session');
        if (!kIsWeb) session['session_token'] = savedToken;
        if (logoutPending) {
          await _request('/v1/auth/logout', {});
          await vault.write('$_namespace.session', '');
          await prefs.remove('$_namespace.logout-pending');
          session = {};
          account = {};
          logoutPending = false;
        } else {
          account = session['account'] as Map<String, dynamic>;
          await prefs.setString('$_namespace.account', jsonEncode(account));
        }
      }
    } catch (error) {
      session = {};
      if (error is AccountException && error.code == 'AUTH_INVALID') {
        await vault.write('$_namespace.session', '');
        if (logoutPending) {
          await prefs.remove('$_namespace.logout-pending');
          logoutPending = false;
        }
      }
    }
    if (logoutPending) {
      session = {};
      account = {};
    }
    final raw = await vault.read('$_namespace.pending');
    if (raw != null && raw.isNotEmpty) {
      try {
        final p = jsonDecode(raw) as Map<String, dynamic>;
        if ((p['expires'] as int) > DateTime.now().millisecondsSinceEpoch) {
          pending = p;
        } else {
          await vault.write('$_namespace.pending', '');
        }
      } catch (_) {
        await vault.write('$_namespace.pending', '');
      }
    }
    notifyListeners();
  }

  Future<Map<String, dynamic>> createPending() async {
    await _completePendingLogout();
    final pair = await Ed25519().newKeyPair(),
        verifier = base64UrlEncode(randomBytes(32)).replaceAll('=', '');
    final challenge = base64UrlEncode(
      (await Sha256().hash(utf8.encode(verifier))).bytes,
    ).replaceAll('=', '');
    final value = await _request('/v1/auth/requests', {
      'proof_key': base64Encode((await pair.extractPublicKey()).bytes),
      'challenge': challenge,
      'client_kind': kIsWeb ? 'web' : 'native',
      'surface': 'app',
      'device_name': kIsWeb
          ? 'Pinelink Web'
          : 'Pinelink ${defaultTargetPlatform.name}',
      'continuation': 'home',
    });
    final r = value['request'] as Map<String, dynamic>;
    pending = {
      'id': r['id'],
      'seed': base64Encode(await pair.extractPrivateKeyBytes()),
      'verifier': verifier,
      'expires': DateTime.parse(r['expires_at']).millisecondsSinceEpoch,
    };
    await _savePending();
    return pending!;
  }

  Future<void> _savePending() {
    final value = pending == null ? '' : jsonEncode(pending);
    final write = _pendingWrites
        .catchError((_) {})
        .then((_) => vault.write('$_namespace.pending', value));
    _pendingWrites = write.catchError((_) {});
    return write;
  }

  Future<Map<String, dynamic>> action(
    String name, [
    Map<String, dynamic> input = const {},
  ]) {
    final p = pending;
    if (p == null) throw const AccountException('AUTH_EXPIRED');
    return _actionFor(p, name, input);
  }

  Future<Map<String, dynamic>> _actionFor(
    Map<String, dynamic> p,
    String name, [
    Map<String, dynamic> input = const {},
  ]) async {
    if (p['expires'] < DateTime.now().millisecondsSinceEpoch) {
      throw const AccountException('AUTH_EXPIRED');
    }
    final path = '/v1/auth/requests/${p['id']}/$name',
        body = {...input, 'verifier': p['verifier']};
    final key = await Ed25519().newKeyPairFromSeed(base64Decode(p['seed']));
    final signature = await Ed25519().sign(
      utf8.encode(
        'pinelink-auth-v1\nPOST\n$path\n${await digest(utf8.encode(jsonEncode(body)))}',
      ),
      keyPair: key,
    );
    return _request(path, body, {
      'X-Pinelink-Auth-Signature': base64Encode(signature.bytes),
    });
  }

  Future<void> _login(String email, String password) async {
    await createPending();
    await action('password-login', {'email': email, 'password': password});
    await finish();
  }

  Future<void> _startEmail(
    String purpose,
    String email,
    String password,
  ) async {
    await createPending();
    final c = await action(
      purpose == 'signup' ? 'password-signup' : 'reset-start',
      {'email': email, 'password': password},
    );
    pending!['challenge'] = c;
    pending!['email'] = email;
    await _savePending();
    notifyListeners();
  }

  Future<void> refreshEmail({bool resend = false}) async {
    if (resend) {
      await _accountOperation(() => _refreshEmail(true));
    } else {
      await _refreshEmail(false);
    }
  }

  Future<void> _refreshEmail(bool resend) async {
    final p = pending, c = pending?['challenge'] as Map<String, dynamic>?;
    if (p == null || c == null) throw const AccountException('AUTH_EXPIRED');
    final revision = ++_emailRevision;
    final updated = await _actionFor(
      p,
      resend ? 'email-resend' : 'email-status',
      {'challenge_id': c['id'], 'generation': c['generation']},
    );
    if (!identical(p, pending)) return;
    final current = pending!['challenge'] as Map;
    final generation = updated['generation'] as int,
        previous = current['generation'] as int;
    if (generation < previous ||
        (generation == previous && revision < _emailAppliedRevision)) {
      return;
    }
    if (generation == previous &&
        ['sent', 'failed'].contains(current['delivery_state']) &&
        ['pending', 'retry'].contains(updated['delivery_state'])) {
      return;
    }
    _emailAppliedRevision = revision;
    pending!['challenge'] = updated;
    await _savePending();
    if (!_closed) notifyListeners();
  }

  Future<String> _verifyCode(String code) async {
    final c = pending!['challenge'] as Map<String, dynamic>;
    final v = await action('email-verify', {
      'challenge_id': c['id'],
      'generation': c['generation'],
      'code': code,
    });
    if (v['state'] == 'ready') await finish();
    if (v['state'] == 'reset') {
      pending!['reset_grant'] = v['reset_grant'];
      await _savePending();
    }
    return v['state'] as String;
  }

  Future<void> _resetPassword(String password) async {
    await action('reset-complete', {
      'reset_grant': pending?['reset_grant'],
      'password': password,
    });
    pending = null;
    await _savePending();
    await _localSignOut();
  }

  Future<void> _googleLogin() async {
    final browser = AuthBrowser();
    try {
      await createPending();
      _cancelGoogle = false;
      final p = pending;
      final value = await action('google-start');
      final url = Uri.parse(value['url']);
      final service = Uri.parse(server);
      if (url.scheme != service.scheme ||
          url.host != service.host ||
          url.port != service.port) {
        throw const AccountException('AUTH_UNAVAILABLE');
      }
      final state = await action('state');
      deviceCode = state['device_code'] as String? ?? '';
      notifyListeners();
      if (!await browser.open(url)) {
        throw const AccountException('GOOGLE_UNAVAILABLE');
      }
      final deadline = DateTime.now().add(const Duration(minutes: 10));
      while (!_closed &&
          !_cancelGoogle &&
          identical(p, pending) &&
          DateTime.now().isBefore(deadline)) {
        await Future<void>.delayed(const Duration(seconds: 2));
        if (!_active && !kIsWeb) continue;
        if (_closed || _cancelGoogle) break;
        final result = await action('state'),
            status = (result['request'] as Map)['status'];
        if (status == 'READY' || status == 'CLAIMED') {
          await finish();
          deviceCode = '';
          notifyListeners();
          return;
        }
        if (status == 'FAILED') {
          throw const AccountException('GOOGLE_UNAVAILABLE');
        }
        if (status == 'CANCELLED') {
          throw const AccountException('GOOGLE_CANCELLED');
        }
      }
      deviceCode = '';
      if (!_closed) notifyListeners();
      throw const AccountException('GOOGLE_CANCELLED');
    } finally {
      browser.close();
    }
  }

  Future<void> cancelGoogle() async {
    _cancelGoogle = true;
    deviceCode = '';
    if (pending != null) {
      try {
        await action('cancel');
      } catch (_) {}
    }
    notifyListeners();
  }

  Future<void> finish() async {
    final state = await action('state');
    final resolved = state['account'] as Map<String, dynamic>?;
    if (resolved == null) throw const AccountException('AUTH_NOT_READY');
    await beforeProfileChange?.call();
    final profile = await ClientRuntime.open(
      server: server,
      owner: resolved['id'],
      autoStart: false,
      localOnly: true,
      client: accountClient(),
    );
    try {
      final id = profile.identity;
      final proof =
          'pinelink-enrollment-v1\n${pending!['id']}\n${resolved['id']}\n${id.device}\n${id.signingKey}\n${id.encryptionKey}';
      final result = await action('claim', {
        'device_id': id.device,
        'signing_key': id.signingKey,
        'encryption_key': id.encryptionKey,
        'device_signature': await id.sign(utf8.encode(proof)),
      });
      session = result;
      account = result['account'] as Map<String, dynamic>;
      if (!kIsWeb) {
        await vault.write('$_namespace.session', jsonEncode(session));
      }
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString('$_namespace.account', jsonEncode(account));
      await prefs.remove('$_namespace.logout-pending');
      pending = null;
      await _savePending();
      profileRevision++;
      notifyListeners();
    } finally {
      await profile.close();
    }
  }

  Future<ClientRuntime> openRuntime() async {
    final guard = '$token|$csrf';
    return ClientRuntime.open(
      server: server,
      owner: owner,
      device: account.isEmpty
          ? const String.fromEnvironment('PINELINK_DEVICE')
          : '',
      pin: const String.fromEnvironment('PINELINK_HISTORY_KEY'),
      localOnly: localOnly,
      accountHeaders: headers,
      client: accountClient(),
      onUnauthorized: () {
        unawaited(_expire(guard));
      },
    );
  }

  Future<void> _expire(String guard) async {
    if (!signedIn || guard != '$token|$csrf') return;
    session = {};
    await beforeProfileChange?.call();
    await vault.write('$_namespace.session', '');
    profileRevision++;
    if (!_closed) notifyListeners();
  }

  Future<void> _completePendingLogout() async {
    final prefs = await SharedPreferences.getInstance();
    if (prefs.getBool('$_namespace.logout-pending') != true) return;
    final stored = await vault.read('$_namespace.session');
    final old = stored == null || stored.isEmpty
        ? <String, dynamic>{}
        : jsonDecode(stored) as Map<String, dynamic>;
    final proofHeaders = <String, String>{
      if (!kIsWeb && old['session_token'] != null)
        'X-Pinelink-Session': old['session_token'],
    };
    try {
      final current = await _request('/v1/auth/session', null, proofHeaders);
      await _request('/v1/auth/logout', {}, {
        ...proofHeaders,
        if (kIsWeb) 'X-Pinelink-CSRF': current['csrf'] as String,
      });
    } on AccountException catch (error) {
      if (error.code != 'AUTH_INVALID') rethrow;
    }
    await prefs.remove('$_namespace.logout-pending');
    await vault.write('$_namespace.session', '');
  }

  Future<void> _localSignOut() async {
    await beforeProfileChange?.call();
    session = {};
    account = {};
    await vault.write('$_namespace.session', '');
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('$_namespace.account');
    profileRevision++;
    notifyListeners();
  }

  Future<bool> _signOut() async {
    final oldHeaders = {...headers};
    await _localSignOut();
    var revoked = false;
    try {
      final response = await client
          .post(Uri.parse('$server/v1/auth/logout'), headers: oldHeaders)
          .timeout(const Duration(seconds: 8));
      revoked = response.statusCode == 200;
    } catch (_) {}
    if (!revoked) {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setBool('$_namespace.logout-pending', true);
      if (!kIsWeb) {
        await vault.write(
          '$_namespace.session',
          jsonEncode({'session_token': oldHeaders['X-Pinelink-Session']}),
        );
      }
    }
    return revoked;
  }

  @override
  void dispose() {
    _closed = true;
    _cancelGoogle = true;
    client.close();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }
}

class AccountScope extends InheritedNotifier<AccountController> {
  const AccountScope({
    super.key,
    required AccountController controller,
    required super.child,
  }) : super(notifier: controller);
  static AccountController? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AccountScope>()?.notifier;
}
