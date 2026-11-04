import '../data/identity.dart';
import 'session.dart';

/// Online pairing is a service grant; it never grants receipt of a transfer.
class GuestConnections {
  GuestConnections(this.session);
  final GuestSession session;

  Future<Map<String, dynamic>> list() =>
      session.request('GET', '/v1/guest/connections');
  Future<Map<String, dynamic>> invite() => session.request(
    'POST',
    '/v1/guest/invites',
    {'request_id': randomUuid()},
  );
  Future<Map<String, dynamic>> join(String value) {
    final input = value.trim();
    if (RegExp(r'^[A-Za-z0-9_-]{22}$').hasMatch(input)) {
      return session.request('POST', '/v1/guest/join', {'token': input});
    }
    final code = input.toUpperCase().replaceAll(RegExp(r'[ -]'), '');
    if (!RegExp(r'^[0-9A-Z]{8}$').hasMatch(code)) {
      throw StateError('GUEST_CODE_INVALID');
    }
    return session.request('POST', '/v1/guest/join', {'code': code});
  }

  Future<Map<String, dynamic>> respond(
    String candidate,
    bool approve,
  ) => session.request(
    'POST',
    '/v1/guest/candidates/${Uri.encodeComponent(candidate)}/${approve ? 'approve' : 'decline'}',
    {},
  );
  Future<Map<String, dynamic>> remove(String pair) => session.request(
    'DELETE',
    '/v1/guest/connections/${Uri.encodeComponent(pair)}',
  );
}
