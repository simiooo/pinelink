import '../auth/controller.dart';
import '../data/identity.dart';
import '../data/runtime.dart';
import 'coordinator.dart';
import 'runtime.dart';

/// Open the already-enrolled account key separately from the visitor store.
/// The controller can retain a local account for offline history; network
/// requests require the exact session and device captured at opening time.
Future<NativeGuestCoordinator> openNativeAccountCoordinator(
  AccountController auth,
) async {
  final owner = auth.account['id'];
  if (owner is! String || !owner.startsWith('acct-')) {
    throw StateError('ACCOUNT_SIGN_IN_REQUIRED');
  }
  final headers = Map<String, String>.from(auth.headers);
  final token = auth.token, csrf = auth.csrf;
  final device = auth.session['device_id'] as String?;
  final profile = await ClientRuntime.open(
    server: auth.server,
    owner: owner,
    autoStart: false,
    localOnly: true,
  );
  late final LocalIdentity identity;
  late final int offset;
  try {
    identity = profile.identity;
    offset = (await profile.database.target())['offset'] as int? ?? 0;
    if (device != null && device != identity.device) {
      throw StateError('ACCOUNT_IDENTITY_INVALID');
    }
  } finally {
    await profile.close();
  }
  bool current() =>
      auth.signedIn &&
      auth.account['id'] == owner &&
      auth.session['device_id'] == identity.device &&
      auth.token == token &&
      auth.csrf == csrf;
  final runtime = await NativeGuestRuntime.openAccount(
    server: auth.server,
    owner: owner,
    identity: identity,
    headers: headers,
    current: current,
    offset: offset,
  );
  try {
    return await NativeGuestCoordinator.open(runtime);
  } catch (_) {
    await runtime.close();
    rethrow;
  }
}
