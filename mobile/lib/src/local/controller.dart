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
  List<LocalMap> get peers;
  List<LocalMap> get jobs;
  List<LocalMap> get nearby;
  LocalMap get pairing;
  String? get discoveryError;
  bool get discovering;
  bool reachable(String root);
  Future<void> discover();
  Future<void> openSettings() async {}
  Future<void> connectNearby(String id);
  Future<String?> invite([String? expected]);
  Future<String?> join(String code, [String? expected]);
  Future<void> confirm();
  Future<void> cancelPairing();
  Future<String> send(Uint8List bytes, String name, LocalMap peer);
  Future<void> action(String id, String action);
  Future<Uint8List?> plaintext(String id);
  Future<void> rename(String root, String name);
  Future<void> remove(String root);
  Future<void> foreground(bool active);
  Future<void> close();
}
