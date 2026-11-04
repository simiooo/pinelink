import 'package:http/http.dart' as http;

import '../data/database.dart';
import '../data/identity.dart';
import 'connections.dart';
import 'session.dart';

/// Native visitor storage and keys are separate from account and LAN stores.
class NativeGuestRuntime {
  NativeGuestRuntime._(this.database, this.identity, this.session)
    : connections = GuestConnections(session);
  final ClientDatabase database;
  final LocalIdentity identity;
  final GuestSession session;
  final GuestConnections connections;

  static Future<NativeGuestRuntime> open({
    required String server,
    IdentityVault? vault,
    ClientDatabase? database,
    http.Client? client,
  }) async {
    final address = Uri.base
        .resolve(server)
        .toString()
        .replaceAll(RegExp(r'/$'), '');
    final namespace = await GuestSession.scopeFor(address);
    final db = database ?? ClientDatabase(namespace),
        secureVault = vault ?? PlatformVault();
    try {
      await db.transaction(() async {
        final marker = await db.metadata('guest-service');
        if (marker != null && marker['scope'] != namespace ||
            marker == null && await db.metadata('profile') != null) {
          throw StateError('GUEST_STORE_MISMATCH');
        }
        if (marker == null) {
          await db.putMetadata('guest-service', {'scope': namespace});
        }
      });
      final identity = await LocalIdentity.load(db, secureVault, namespace, '');
      final session = await GuestSession.open(
        server: address,
        identity: identity,
        vault: secureVault,
        client: client,
      );
      return NativeGuestRuntime._(db, identity, session);
    } catch (_) {
      await db.close();
      rethrow;
    }
  }

  Future<void> close() async {
    session.close();
    await database.close();
  }
}
