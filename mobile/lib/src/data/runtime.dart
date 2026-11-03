import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:drift/drift.dart';
import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;

import 'database.dart';
import 'identity.dart';
import 'wire.dart';

class DataException implements Exception {
  const DataException(this.status, this.code);
  final int status;
  final String code;
}

class ClientRuntime with WidgetsBindingObserver {
  final _networkSnapshots = <String>{};
  bool isNetworkFresh(String path) =>
      !_closed && _networkSnapshots.contains(_canonicalPath(path));
  String _canonicalPath(String path) {
    final uri = Uri.parse(path), query = {...uri.queryParameters};
    if (query.containsKey('owner_id')) query['owner_id'] = owner;
    if (query.containsKey('device_id')) query['device_id'] = identity.device;
    return uri
        .replace(queryParameters: query.isEmpty ? null : query)
        .toString();
  }

  ClientRuntime._(
    this.server,
    this.owner,
    this.pin,
    this.database,
    this.identity,
    this._client,
    this.localOnly,
    this.accountHeaders,
    this.onUnauthorized,
  );
  final String server, owner, pin;
  final bool localOnly;
  final Map<String, String> accountHeaders;
  final VoidCallback? onUnauthorized;
  final ClientDatabase database;
  final LocalIdentity identity;
  final http.Client _client;
  Timer? _timer;
  Future<void>? _active;
  bool _closed = false;
  Future<void>? _closing;
  int _generation = 0;
  final _operations = <Future<void>>{};
  final _aborts = <Completer<void>>{};
  bool _observing = false;
  bool _started = false;
  static Future<ClientRuntime> open({
    required String server,
    required String owner,
    String device = '',
    String pin = '',
    ClientDatabase? database,
    IdentityVault? vault,
    http.Client? client,
    bool autoStart = true,
    bool localOnly = false,
    Map<String, String> accountHeaders = const {},
    VoidCallback? onUnauthorized,
  }) async {
    final address = Uri.base
        .resolve(server)
        .toString()
        .replaceAll(RegExp(r'/$'), '');
    final namespace =
        '${owner.startsWith('acct-') ? 'pinelink-flutter-account' : 'pinelink'}-${await digest(utf8.encode('$address\n$owner\n$device'))}';
    final db = database ?? ClientDatabase(namespace);
    late LocalIdentity identity;
    try {
      identity = await LocalIdentity.load(
        db,
        vault ?? PlatformVault(),
        namespace,
        device,
      );
    } catch (_) {
      await db.close();
      rethrow;
    }
    final runtime = ClientRuntime._(
      address,
      owner,
      pin,
      db,
      identity,
      client ?? http.Client(),
      localOnly,
      Map.unmodifiable(accountHeaders),
      onUnauthorized,
    );
    if (autoStart) runtime.start();
    return runtime;
  }

  void _checkOpen() {
    if (_closed) throw const DataException(0, 'CLIENT_CLOSED');
  }

  Future<T> _track<T>(Future<T> Function() work) {
    if (_closed) return Future.error(const DataException(0, 'CLIENT_CLOSED'));
    final operation = Future<T>.sync(work);
    final settled = operation.then<void>(
      (_) {},
      onError: (Object _, StackTrace __) {},
    );
    _operations.add(settled);
    unawaited(
      settled.then((_) {
        _operations.remove(settled);
      }),
    );
    return operation;
  }

  Future<Map<String, dynamic>> _wire(
    String method,
    String path, [
    Map<String, dynamic>? input,
    bool signed = true,
  ]) async {
    _checkOpen();
    final abort = Completer<void>();
    _aborts.add(abort);
    try {
      return await _exchange(method, path, input, signed, abort.future).timeout(
        const Duration(seconds: 6),
        onTimeout: () {
          if (!abort.isCompleted) abort.complete();
          throw TimeoutException('REQUEST_TIMEOUT');
        },
      );
    } finally {
      _aborts.remove(abort);
    }
  }

  Future<Map<String, dynamic>> _exchange(
    String method,
    String path,
    Map<String, dynamic>? input,
    bool signed,
    Future<void> abort,
  ) async {
    final body = input == null ? '' : jsonEncode(input);
    final request = http.AbortableRequest(
      method,
      Uri.parse('$server$path'),
      abortTrigger: abort,
    );
    request.headers['Content-Type'] = 'application/json';
    request.headers.addAll(accountHeaders);
    if (signed) {
      final target = await database.target();
      final timestamp =
          ((DateTime.now().millisecondsSinceEpoch +
                      (target['offset'] as int)) ~/
                  1000)
              .toString();
      request.headers.addAll({
        'X-Pinelink-Owner': owner,
        'X-Pinelink-Device': identity.device,
        'X-Pinelink-Timestamp': timestamp,
        'X-Pinelink-Signature': await identity.sign(
          utf8.encode(
            '$method\n$path\n$timestamp\n${await digest(utf8.encode(body))}',
          ),
        ),
      });
    }
    if (method != 'GET') request.body = body;
    request.followRedirects = false;
    _checkOpen();
    final response = await _client.send(request);
    final bytes = <int>[];
    await for (final part in response.stream) {
      bytes.addAll(part);
      if (bytes.length > 12 * 1024 * 1024) {
        throw const DataException(0, 'RESPONSE_TOO_LARGE');
      }
    }
    _checkOpen();
    Map<String, dynamic> value;
    try {
      value = bytes.isEmpty
          ? {}
          : jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
    } catch (_) {
      throw DataException(response.statusCode, 'INVALID_SERVER_RESPONSE');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      if (response.statusCode == 401 &&
          value['code'] == 'ACCOUNT_SIGN_IN_REQUIRED') {
        onUnauthorized?.call();
      }
      throw DataException(
        response.statusCode,
        value['code']?.toString() ?? 'REQUEST_FAILED',
      );
    }
    return value;
  }

  bool _cacheable(String path) =>
      RegExp(
        r'^/v1/(tasks|transfers|notifications|devices|quota|files|pulls)(/|\?|$)',
      ).hasMatch(path) &&
      !RegExp(r'/(content|manifest|envelope|chunks)(/|\?|$)').hasMatch(path);
  Future<Map<String, dynamic>> request(
    String method,
    String path, [
    Map<String, dynamic>? input,
  ]) => _track(() => _request(method, path, input));

  Future<Map<String, dynamic>> _request(
    String method,
    String path,
    Map<String, dynamic>? input,
  ) async {
    _checkOpen();
    path = _canonicalPath(path);
    if (localOnly) {
      if (method != 'GET') throw const DataException(401, 'SIGNIN_REQUIRED');
      final saved = await database.cached(path);
      if (saved != null) return saved;
      if (path.startsWith('/v1/tasks?')) return {'tasks': []};
      if (path.startsWith('/v1/transfers?') ||
          path.startsWith('/v1/notifications?')) {
        return {'transfers': []};
      }
      if (path.startsWith('/v1/devices?')) return {'devices': []};
      if (path.startsWith('/v1/quota?')) {
        return {'scope_id': owner, 'quota_bytes': 0, 'used_bytes': 0};
      }
      throw const DataException(0, 'OFFLINE_CACHE_MISS');
    }
    if (input != null) {
      if (path == '/v1/devices/revoke' &&
          input['device_id'] != identity.device) {
        throw const DataException(403, 'DEVICE_REVOCATION_FORBIDDEN');
      }
      input = {...input};
      for (final field in ['owner_id', 'sender_owner_id']) {
        if (input.containsKey(field)) input[field] = owner;
      }
      for (final field in ['device_id', 'sender_device_id']) {
        if (input.containsKey(field)) input[field] = identity.device;
      }
    }
    try {
      if ((await database.target())['bound'] == '') {
        try {
          await sync(force: true);
        } catch (_) {
          if ((await database.target())['bound'] == '') rethrow;
        }
      }
      for (var attempt = 0; attempt < 2; attempt++) {
        final cached = method == 'GET' && _cacheable(path);
        final started = cached ? await database.beginRead() : 0;
        final value = await _wire(method, path, input);
        _checkOpen();
        if (cached) {
          await database.cacheResponse(path, value, started);
          final saved = await database.cached(path);
          _checkOpen();
          if (saved != null) {
            _networkSnapshots.add(path);
            return saved;
          }
          continue;
        }
        if (method != 'GET') await database.cacheMutation(value);
        _checkOpen();
        return value;
      }
      throw const DataException(409, 'RESPONSE_SUPERSEDED');
    } catch (error) {
      _checkOpen();
      _networkSnapshots.remove(path);
      final retryable =
          error is! DataException ||
          error.status >= 500 ||
          error.status == 408 ||
          error.status == 429;
      if (method == 'GET' && _cacheable(path) && retryable) {
        final stored = await database.cached(path);
        _checkOpen();
        if (stored != null) return stored;
      }
      rethrow;
    }
  }

  Future<Map<String, dynamic>> _profile(Map<String, dynamic> p) async {
    if (p['root'] != identity.root ||
        !validId(p['epoch']) ||
        !integer(p['roster_version']) ||
        !await verify(pin, proof('profile', p), p['signature'])) {
      invalid();
    }
    return p;
  }

  Future<void> _pass() async {
    var target = await database.target();
    var epoch = 'API_ONLY';
    if (pin.isNotEmpty) {
      final nonce = base64UrlEncode(randomBytes(32)).replaceAll('=', '');
      final info = await _wire(
        'GET',
        '/v1/replication/info?nonce=$nonce',
        null,
        false,
      );
      if (info['version'] != 1 ||
          info['public_key'] != pin ||
          info['nonce'] != nonce ||
          !validId(info['epoch']) ||
          !integer(info['unix_time'], 1) ||
          !await verify(pin, proof('info', info), info['signature'])) {
        invalid();
      }
      epoch = info['epoch'];
      await database.patchTarget({
        'offset':
            (info['unix_time'] as int) * 1000 -
            DateTime.now().millisecondsSinceEpoch,
      });
    }
    if (target['bound'] != epoch) {
      await _wire('POST', '/v1/devices/register', {
        'owner_id': owner,
        'device_id': identity.device,
        'public_key': identity.encryptionKey,
        'signing_public_key': identity.signingKey,
      });
      if (pin.isNotEmpty) {
        final p = await _profile(
          await _wire('POST', '/v1/replication/bind', {
            'root': identity.root,
            'signing_key': identity.signingKey,
            'encryption_key': identity.encryptionKey,
          }),
        );
        if (p['epoch'] != epoch) invalid();
        await database.patchTarget({
          'bound': epoch,
          'rosterVersion': p['roster_version'],
          'rosterHash': '',
          'pendingRoster': null,
        });
        await database.reset(epoch);
      } else {
        await database.patchTarget({'bound': epoch});
      }
    }
    if (pin.isNotEmpty) {
      final p = await _profile(
        await _wire('GET', '/v1/replication/profile?stream=${identity.stream}'),
      );
      if (p['epoch'] != epoch) invalid();
      Future<List<Map<String, dynamic>>> roster() async {
        final peers =
            (await database.peers()).values.where((p) => !p.revoked).toList()
              ..sort((a, b) => a.root.compareTo(b.root));
        return peers
            .map(
              (p) => {
                'root': p.root,
                'signing_key': p.signingKey,
                'encryption_key': p.encryptionKey,
              },
            )
            .toList();
      }

      final peers = await roster();
      final hash = await digest(utf8.encode(canonical(peers)));
      target = await database.target();
      if (target['rosterHash'] != hash || target['pendingRoster'] != null) {
        final pending =
            target['pendingRoster'] as Map<String, dynamic>? ??
            {
              'operation_id': randomUuid(),
              'expected_version': p['roster_version'],
              'peers': peers,
            };
        await database.patchTarget({'pendingRoster': pending});
        try {
          final ack = await _profile(
            await _wire('PUT', '/v1/replication/peers', pending),
          );
          await database.patchTarget({
            'rosterVersion': ack['roster_version'],
            'rosterHash': await digest(
              utf8.encode(canonical(pending['peers'])),
            ),
            'pendingRoster': null,
          });
        } catch (error) {
          if (error is DataException && error.status == 409) {
            await database.patchTarget({'pendingRoster': null});
          }
          rethrow;
        }
        if (await digest(utf8.encode(canonical(await roster()))) != hash) {
          return;
        }
      }
      for (var n = 0; n < 4; n++) {
        target = await database.target();
        if (target['epoch'] != epoch) {
          await database.reset(epoch);
          target = await database.target();
        }
        final endpoint = target['bootstrapped'] == true ? 'events' : 'snapshot';
        final query = Uri(
          queryParameters: {
            'epoch': epoch,
            'after': '${target['cursor']}',
            'upper': '${target['upper']}',
            'key': target['key'],
            'limit': '100',
          },
        ).query;
        Map<String, dynamic> page;
        try {
          page = await _wire('GET', '/v1/replication/$endpoint?$query');
        } catch (error) {
          if (error is DataException && error.status == 409) {
            await database.reset(epoch);
          }
          rethrow;
        }
        if (page['epoch'] != epoch ||
            page['after'] != target['cursor'] ||
            (page['snapshot'] == true) == (target['bootstrapped'] == true) ||
            (target['bootstrapped'] != true &&
                target['upper'] >= 0 &&
                page['upper'] != target['upper'])) {
          invalid();
        }
        await database.apply(
          identity.root,
          page,
          await preparePage(identity, page, pin, await database.peers()),
        );
        if (page['more'] != true) break;
      }
    }
    final cached =
        await (database.select(database.responseCache)
              ..orderBy([(r) => OrderingTerm.asc(r.updated)])
              ..limit(4))
            .get();
    for (final row in cached) {
      final started = await database.beginRead();
      final value = await _wire('GET', row.path);
      await database.cacheResponse(row.path, value, started);
    }
  }

  Future<void> sync({bool force = false}) {
    if (localOnly) return Future.value();
    if (_closed) return Future.error(const DataException(0, 'CLIENT_CLOSED'));
    if (_active != null) return _active!;
    Future<Null> work() async {
      _checkOpen();
      final target = await database.target();
      if (!force &&
          (target['retryAt'] as int) > DateTime.now().millisecondsSinceEpoch) {
        return;
      }
      try {
        await _pass();
        await database.patchTarget({
          'failures': 0,
          'retryAt': DateTime.now().millisecondsSinceEpoch + 5000,
          'error': '',
        });
      } catch (error) {
        _checkOpen();
        final failures = min(
          ((await database.target())['failures'] as int) + 1,
          6,
        );
        await database.patchTarget({
          'failures': failures,
          'retryAt':
              DateTime.now().millisecondsSinceEpoch +
              min(60000, 1000 * (1 << failures)),
          'error': error is DataException ? error.code : 'SYNC_RETRY_PENDING',
        });
        rethrow;
      }
    }

    _active = _track(work).whenComplete(() => _active = null);
    return _active!;
  }

  void start({bool observeLifecycle = true}) {
    if (localOnly) return;
    _checkOpen();
    if (_started) return;
    _started = true;
    final generation = ++_generation;
    if (observeLifecycle && !_observing) {
      WidgetsBinding.instance.addObserver(this);
      _observing = true;
    }
    void loop() {
      if (!_started || generation != _generation) return;
      unawaited(
        sync().catchError((Object _) {}).whenComplete(() {
          if (_started && generation == _generation) {
            _timer = Timer(const Duration(seconds: 5), loop);
          }
        }),
      );
    }

    loop();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed && _started && !_closed) {
      unawaited(sync(force: true).catchError((Object _) {}));
    }
  }

  void stop() {
    _started = false;
    ++_generation;
    _timer?.cancel();
    if (_observing) {
      WidgetsBinding.instance.removeObserver(this);
      _observing = false;
    }
  }

  Future<void> close() => _closing ??= _shutdown();
  Future<void> _shutdown() async {
    _closed = true;
    stop();
    for (final abort in _aborts) {
      if (!abort.isCompleted) abort.complete();
    }
    _client.close();
    await Future.wait(_operations.toList());
    await _active?.catchError((Object _) {});
    await database.close();
  }
}
