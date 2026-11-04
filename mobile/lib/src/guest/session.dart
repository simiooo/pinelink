import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import '../data/identity.dart';

/// A service-scoped native visitor session. Account and LAN identities must be
/// opened separately; the session token is stored only in the platform vault.
class GuestSession {
  GuestSession._({
    required this.server,
    required this.identity,
    required this.vault,
    required this.namespace,
    http.Client? client,
  }) : client = client ?? http.Client() {
    final uri = Uri.parse(server);
    if (!uri.hasScheme ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        !['https', 'http'].contains(uri.scheme) ||
        (uri.scheme == 'http' &&
            !['localhost', '127.0.0.1', '::1'].contains(uri.host)) ||
        server.endsWith('/')) {
      throw StateError('GUEST_SERVICE_INVALID');
    }
  }
  static Future<String> scopeFor(String server) async =>
      'pinelink-guest-${await digest(utf8.encode(server))}';
  static Future<GuestSession> open({
    required String server,
    required LocalIdentity identity,
    required IdentityVault vault,
    http.Client? client,
  }) async => GuestSession._(
    server: server,
    identity: identity,
    vault: vault,
    namespace: await scopeFor(server),
    client: client,
  );
  final String server, namespace;
  final LocalIdentity identity;
  final IdentityVault vault;
  final http.Client client;
  String? _token, _owner;
  int _offset = 0;
  Future<void>? _enrollment;
  bool _closed = false;
  String get owner => _owner ?? (throw StateError('GUEST_EXPIRED'));

  Future<Map<String, dynamic>> _send(
    String method,
    String path,
    String body,
    Map<String, String> headers,
  ) async {
    if (_closed) throw StateError('GUEST_CLOSED');
    if (!path.startsWith('/v1/') || path.contains('#') || path.contains('\n')) {
      throw StateError('GUEST_PATH_INVALID');
    }
    final request = http.Request(method, Uri.parse('$server$path'))
      ..followRedirects = false
      ..headers.addAll({
        'Content-Type': 'application/json',
        'X-Pinelink-Surface': 'app',
        ...headers,
      });
    if (method != 'GET') request.body = body;
    final response = await client
        .send(request)
        .timeout(const Duration(seconds: 15));
    if (response.contentLength != null && response.contentLength! > 1048576) {
      throw StateError('GUEST_RESPONSE_INVALID');
    }
    final bytes = BytesBuilder(copy: false);
    var count = 0;
    await for (final chunk in response.stream.timeout(
      const Duration(seconds: 15),
    )) {
      count += chunk.length;
      if (count > 1048576) throw StateError('GUEST_RESPONSE_INVALID');
      bytes.add(chunk);
    }
    if (_closed) throw StateError('GUEST_CLOSED');
    final decoded = jsonDecode(utf8.decode(bytes.takeBytes()));
    if (decoded is! Map<String, dynamic>) {
      throw StateError('GUEST_RESPONSE_INVALID');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw StateError(
        decoded['code'] is String ? decoded['code'] : 'GUEST_UNAVAILABLE',
      );
    }
    return decoded;
  }

  Future<void> ensureSession({String deviceName = 'Android'}) {
    if (_closed) return Future.error(StateError('GUEST_CLOSED'));
    if (_token != null) return Future.value();
    return _enrollment ??= _enroll(deviceName)
        .whenComplete(() => _enrollment = null);
  }

  Future<void> _enroll(String deviceName) async {
    final stored = await vault.read('$namespace.guest-session');
    final previous = stored == null
        ? null
        : jsonDecode(stored) as Map<String, dynamic>;
    final challenge = await _send(
      'POST',
      '/v1/guest/challenge',
      jsonEncode({
        'signing_key': identity.signingKey,
        'client_kind': 'native',
        'surface': 'app',
      }),
      {},
    );
    if (challenge['challenge'] is! String || challenge['unix_time'] is! int) {
      throw StateError('GUEST_RESPONSE_INVALID');
    }
    final body = jsonEncode({
      'challenge': challenge['challenge'],
      'device_id': identity.device,
      'encryption_key': identity.encryptionKey,
      'device_name': deviceName,
    });
    final signature = await identity.sign(
      utf8.encode(
        'pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n${await digest(utf8.encode(body))}',
      ),
    );
    final response = await _send('POST', '/v1/guest/enroll', body, {
      'X-Pinelink-Signature': signature,
    });
    final guest = response['guest'];
    if (guest is! Map<String, dynamic> ||
        guest['owner_id'] is! String ||
        !(guest['owner_id'] as String).startsWith('guest-') ||
        guest['device_id'] != identity.device ||
        guest['client_kind'] != 'native' ||
        guest['surface'] != 'app' ||
        response['session_token'] is! String ||
        (response['session_token'] as String).isEmpty) {
      throw StateError('GUEST_RESPONSE_INVALID');
    }
    if (previous != null &&
        (previous['owner'] != guest['owner_id'] ||
            previous['device'] != identity.device)) {
      throw StateError('GUEST_IDENTITY_CHANGED');
    }
    final token = response['session_token'] as String,
        owner = guest['owner_id'] as String;
    await vault.write(
      '$namespace.guest-session',
      jsonEncode({'owner': owner, 'device': identity.device, 'token': token}),
    );
    if (_closed) throw StateError('GUEST_CLOSED');
    _owner = owner;
    _token = token;
    _offset =
        challenge['unix_time'] * 1000 - DateTime.now().millisecondsSinceEpoch;
  }

  Future<Map<String, dynamic>> request(
    String method,
    String path, [
    Map<String, dynamic>? value,
  ]) async {
    if (_token == null || _owner == null) throw StateError('GUEST_EXPIRED');
    if (!['GET', 'POST', 'DELETE'].contains(method)) {
      throw StateError('GUEST_METHOD_INVALID');
    }
    final body = value == null ? '' : jsonEncode(value),
        timestamp = ((DateTime.now().millisecondsSinceEpoch + _offset) ~/ 1000)
            .toString();
    final signature = await identity.sign(
      utf8.encode(
        '$method\n$path\n$timestamp\n${await digest(utf8.encode(body))}',
      ),
    );
    return _send(method, path, body, {
      'X-Pinelink-Owner': _owner!,
      'X-Pinelink-Device': identity.device,
      'X-Pinelink-Timestamp': timestamp,
      'X-Pinelink-Signature': signature,
      'X-Pinelink-Guest-Session': _token!,
    });
  }

  void close() {
    _closed = true;
    client.close();
  }
}
