import 'package:flutter/foundation.dart';

typedef LocalMap = Map<String, dynamic>;
bool localTerminal(String? state) => const [
  'COMPLETED',
  'REJECTED',
  'CANCELLED',
  'EXPIRED',
  'FAILED',
].contains(state);
bool localActive(LocalMap job) =>
    !localTerminal(job['state']?['state']) &&
    job['expired'] != true &&
    job['intent'] != 'cancel';
String localPeerName(LocalMap peer) =>
    peer['local_name'] ?? peer['name'] ?? 'Device';

abstract class LocalController extends ChangeNotifier {
  bool get smartConnection => false;
  List<LocalMap> get peers;
  List<LocalMap> get jobs;
  List<LocalMap> get nearby;
  List<LocalMap> get pairRequests => const [];
  LocalMap get pairing;
  String? get discoveryError;
  bool get discovering;
  bool reachable(String root);
  bool online(String root) => reachable(root);
  bool supportsText(String root) => false;
  Future<void> discover();
  Future<void> openSettings() async {}
  Future<void> connectNearby(String id);
  Future<String?> invite([String? expected]);
  Future<String?> inviteOffline() => invite();
  Future<String?> join(String code, [String? expected]);
  Future<void> confirm();
  Future<void> declinePairing() => cancelPairing();
  void showPairRequest(String id) {}
  Future<void> cancelPairing();
  Future<String> send(Uint8List bytes, String name, LocalMap peer);
  Future<String> sendText(Uint8List bytes, LocalMap peer, String sensitivity) =>
      Future.error(StateError('TEXT_UNSUPPORTED'));
  Future<({String value, bool sensitive})?> text(String id) async => null;
  Future<void> action(String id, String action);
  Future<Uint8List?> plaintext(String id);
  Future<void> rename(String root, String name);
  Future<void> remove(String root);
  Future<void> foreground(bool active);
  Future<void> close();
}
