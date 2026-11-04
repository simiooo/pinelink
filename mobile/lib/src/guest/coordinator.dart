import 'dart:async';

import 'package:flutter/foundation.dart';

import 'runtime.dart';
import 'store.dart';

/// Controls the service-scoped visitor slot without blocking local startup.
/// Network and roster mutations are serialized; file workers are independently
/// cancellable and resumed only from durable consent and checkpoints.
class NativeGuestCoordinator extends ChangeNotifier {
  NativeGuestCoordinator._(this.runtime);
  final NativeGuestRuntime runtime;
  List<GuestMap> peers = [], jobs = [], candidates = [];
  bool online = false;
  bool _foreground = true, _closed = false;
  Future<void> _serial = Future.value();
  Future<void>? _polling, _closing;
  int _refreshEpoch = 0;
  final Map<String, Future<void>> _workers = {};
  Timer? _timer;

  static Future<NativeGuestCoordinator> open(
    NativeGuestRuntime runtime, {
    bool autoPoll = true,
  }) async {
    final result = NativeGuestCoordinator._(runtime);
    await result._refresh();
    if (autoPoll) {
      result._timer = Timer.periodic(const Duration(seconds: 3), (_) {
        unawaited(result.poll());
      });
      unawaited(result.poll());
    }
    return result;
  }

  void _notify() {
    if (!_closed) notifyListeners();
  }

  Future<T> _enqueue<T>(Future<T> Function() work) {
    if (_closed) return Future.error(StateError('GUEST_CLOSED'));
    final done = Completer<T>();
    _serial = _serial.then((_) async {
      if (_closed) {
        done.completeError(StateError('GUEST_CLOSED'));
        return;
      }
      try {
        done.complete(await work());
      } catch (error, stack) {
        done.completeError(error, stack);
      }
    });
    return done.future;
  }

  Future<void> _refresh() async {
    final epoch = ++_refreshEpoch;
    final cached = await runtime.store.peers();
    final stored = await runtime.store.jobs();
    if (_closed || epoch != _refreshEpoch) return;
    peers = cached
        .map((peer) => {...peer, 'online': online && peer['online'] == true})
        .toList();
    jobs = stored;
    _notify();
  }

  /// A failed network poll never deletes the last available offline history.
  Future<void> poll() {
    if (_closed || !_foreground) return Future.value();
    if (_polling != null) return _polling!;
    late Future<void> current;
    current =
        _enqueue(() async {
          try {
            await runtime.session.ensureSession();
            final list = await runtime.connections.list();
            final roster = (list['peers'] as List? ?? [])
                .map((item) => Map<String, dynamic>.from(item as Map))
                .toList();
            await runtime.store.replacePeers(roster);
            candidates = (list['candidates'] as List? ?? [])
                .map((item) => Map<String, dynamic>.from(item as Map))
                .toList();
            online = true;
          } catch (_) {
            online = false;
            candidates = [];
            await _refresh();
            return;
          }
          try {
            await runtime.syncIncoming();
          } catch (_) {
            // A concurrent durable chunk or control can invalidate a snapshot.
            // Keep the last verified state and retry on the next poll.
          }
          await _refresh();
          _startEligibleWorkers();
        }).catchError((Object _) {}).whenComplete(() {
          if (identical(_polling, current)) _polling = null;
        });
    _polling = current;
    return current;
  }

  void _startEligibleWorkers() {
    if (_closed || !_foreground || !online) return;
    for (final job in jobs) {
      final id = job['id'];
      if (id is! String || _workers.containsKey(id)) continue;
      final state = job['state'];
      Future<void> Function()? operation;
      if (job['direction'] == 'send' && state == 'PREPARING') {
        operation = () => runtime.resumeOffer(id);
      } else if (job['direction'] == 'send' &&
          ['PENDING', 'ACCEPTED'].contains(state) &&
          (job['progress'] as Map?)?['completion']?['stored'] != true) {
        operation = () => runtime.upload(id);
      } else if (job['direction'] == 'receive' &&
          state == 'ACCEPTED' &&
          job['paused'] != true) {
        operation = () => runtime.receive(id);
      }
      if (operation == null) continue;
      late Future<void> worker;
      worker = Future<void>.sync(operation)
          .catchError((Object _) {})
          .whenComplete(() async {
            try {
              if (!_closed) await _refresh();
            } finally {
              if (identical(_workers[id], worker)) _workers.remove(id);
            }
          })
          .catchError((Object _) {});
      _workers[id] = worker;
    }
  }

  Future<GuestMap> invite() => _enqueue(() async {
    await runtime.session.ensureSession();
    final list = await runtime.connections.list();
    final live = (list['invites'] as List? ?? []).whereType<Map>().where((
      invite,
    ) {
      final expiry = DateTime.tryParse(invite['expires_at']?.toString() ?? '');
      return invite['consumed'] != true &&
          expiry != null &&
          expiry.isAfter(DateTime.now());
    }).firstOrNull;
    return runtime.connections.invite(live?['id'] as String?);
  });

  Future<GuestMap> join(String value) => _enqueue(() async {
    await runtime.session.ensureSession();
    return runtime.connections.join(value);
  });

  Future<void> respondCandidate(String id, bool approve) => _enqueue(() async {
    await runtime.session.ensureSession();
    await runtime.connections.respond(id, approve);
  });

  Future<void> rename(String pair, String name) => _enqueue(() async {
    final value = name.trim();
    if (value.isEmpty ||
        value.runes.length > 64 ||
        RegExp(r'[\x00-\x1f\x7f]').hasMatch(value)) {
      throw StateError('DEVICE_NAME_INVALID');
    }
    await runtime.store.renamePeer(pair, value);
    await _refresh();
  });

  Future<void> remove(String pair) => _enqueue(() async {
    await runtime.session.ensureSession();
    await runtime.connections.remove(pair);
    await runtime.store.forgetPeer(pair);
    await _refresh();
  });

  Future<String> send(
    Uint8List bytes,
    String name,
    GuestMap peer, {
    String? sensitivity,
  }) => _enqueue(() async {
    if (!online) throw StateError('GUEST_UNAVAILABLE');
    await runtime.session.ensureSession();
    if (jobs
            .where(
              (j) => ![
                'COMPLETED',
                'REJECTED',
                'CANCELLED',
                'EXPIRED',
                'FAILED',
              ].contains(j['state']),
            )
            .length >=
        2) {
      throw StateError('GUEST_LIMIT_REACHED');
    }
    final actual = (await runtime.store.peers())
        .where((p) => p['pair_id'] == peer['pair_id'])
        .firstOrNull;
    if (actual == null ||
        actual['device_id'] != peer['device_id'] ||
        actual['encryption_key'] != peer['encryption_key']) {
      throw StateError('GUEST_PEER_INVALID');
    }
    late String id;
    try {
      id = await runtime.offer(bytes, name, actual, sensitivity: sensitivity);
    } on GuestOfferPending catch (pending) {
      id = pending.id;
    }
    await _refresh();
    _startEligibleWorkers();
    return id;
  });

  Future<void> action(String id, String action) => _enqueue(() async {
    final job = await runtime.store.job(id);
    if (job == null) throw StateError('GUEST_JOB_CHANGED');
    if (action == 'accept' || action == 'reject') {
      await runtime.respond(id, action == 'accept');
    } else {
      if (job['state'] == 'PREPARING') await runtime.resumeOffer(id);
      await runtime.control(id, action);
    }
    await _refresh();
    _startEligibleWorkers();
  });

  Future<void> foreground(bool active) async {
    if (_closed || _foreground == active) return;
    _foreground = active;
    if (!active) {
      final transfers = jobs
          .where((job) => _workers.containsKey(job['id']))
          .map((job) => (job['transfer'] as Map?)?['id'])
          .whereType<String>()
          .toList();
      await Future.wait(transfers.map(runtime.relay.stop));
      return;
    }
    await poll();
  }

  Future<void> close() => _closing ??= _finishClose();

  Future<void> _finishClose() async {
    _closed = true;
    _timer?.cancel();
    await runtime.relay.close();
    await _serial;
    await Future.wait(_workers.values.toList());
    await runtime.close();
    dispose();
  }
}
