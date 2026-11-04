import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:bonsoir/bonsoir.dart';
import 'package:flutter/services.dart';

import 'protocol.dart';

const localServiceType = '_pinelink-rtc._tcp';
bool privateAddress(InternetAddress a) {
  final b = a.rawAddress;
  if (b.length == 16 &&
      b.take(10).every((n) => n == 0) &&
      b[10] == 255 &&
      b[11] == 255) {
    return privateAddress(InternetAddress.fromRawAddress(b.sublist(12)));
  }
  if (a.isLoopback || a.isLinkLocal) return true;
  if (b.length == 4) {
    return b[0] == 10 ||
        b[0] == 172 && b[1] >= 16 && b[1] <= 31 ||
        b[0] == 192 && b[1] == 168;
  }
  return b.length == 16 && (b[0] & 0xfe) == 0xfc;
}

class LocalDiscovery {
  LocalDiscovery(this.root, this.name, this.incoming, this.changed);
  final String root, name;
  final Future<String?> Function(String) incoming;
  final void Function() changed;
  final Map<String, BonsoirService> _services = {};
  final Map<String, DateTime> _rates = {};
  BonsoirBroadcast? _broadcast;
  BonsoirDiscovery? _discovery;
  StreamSubscription<BonsoirDiscoveryEvent>? _events;
  HttpServer? _server;
  bool running = false, _closed = false, _handling = false;
  Future<void>? _starting;
  String? error;
  List<Json> get nearby => <String, Json>{
    for (final e in _services.entries)
      e.value.attributes['root']!: <String, dynamic>{
        'id': e.key,
        'name': e.value.name,
        'root': e.value.attributes['root'],
      },
  }.values.toList();
  Future<void> start() => _starting ??= _start().whenComplete(() {
    _starting = null;
  });
  Future<void> _start() async {
    if (_closed || running) return;
    error = null;
    try {
      if (_server == null) {
        try {
          _server = await HttpServer.bind(
            InternetAddress.anyIPv6,
            0,
            v6Only: false,
          );
        } on SocketException {
          _server = await HttpServer.bind(InternetAddress.anyIPv4, 0);
        }
        _server!.idleTimeout = const Duration(seconds: 15);
        _server!.listen((request) {
          unawaited(_request(request));
        });
        _broadcast = BonsoirBroadcast(
          printLogs: false,
          service: BonsoirService(
            name: name,
            type: localServiceType,
            port: _server!.port,
            attributes: {'v': '1', 'root': root},
          ),
        );
        await _broadcast!.initialize();
        if (_closed) return;
        await _broadcast!.start();
      }
      final discovery = BonsoirDiscovery(
        type: localServiceType,
        printLogs: false,
      );
      _discovery = discovery;
      await discovery.initialize();
      if (_closed) return;
      _events = discovery.eventStream!.listen(
        (event) {
          if (_closed) return;
          final s = event.service;
          if (event is BonsoirDiscoveryServiceFoundEvent) {
            discovery.serviceResolver.resolveService(event.service);
          }
          if (s != null &&
              (event is BonsoirDiscoveryServiceResolvedEvent ||
                  event is BonsoirDiscoveryServiceUpdatedEvent) &&
              s.attributes['v'] == '1' &&
              s.attributes['root'] != root &&
              RegExp(r'^[a-f0-9]{64}$').hasMatch(s.attributes['root'] ?? '') &&
              s.port > 0 &&
              s.port <= 65535 &&
              s.hostAddresses.isNotEmpty) {
            _services[s.name] = s;
            if (_services.length > 128) _services.remove(_services.keys.first);
            changed();
          }
          if (event is BonsoirDiscoveryServiceLostEvent) {
            _services.remove(event.service.name);
            changed();
          }
        },
        onError: (Object e) {
          error = _reason(e);
          running = false;
          changed();
        },
      );
      await discovery.start();
      running = true;
    } catch (e) {
      error = _reason(e);
      running = false;
    }
    changed();
  }

  String _reason(Object e) =>
      e is PlatformException &&
          '${e.code} ${e.message}'.toLowerCase().contains('denied')
      ? 'LOCAL_PERMISSION_DENIED'
      : 'LOCAL_NETWORK_UNAVAILABLE';
  Future<void> _request(HttpRequest request) async {
    final response = request.response;
    response.headers.set('Cache-Control', 'no-store');
    response.headers.set('X-Content-Type-Options', 'nosniff');
    final address = request.connectionInfo?.remoteAddress;
    try {
      if (_closed ||
          address == null ||
          !privateAddress(address) ||
          request.method != 'POST' ||
          request.uri.path != '/pinelink/local/v1' ||
          request.uri.hasQuery ||
          request.headers.value('origin') != null ||
          request.headers.contentType?.mimeType != 'text/plain') {
        response.statusCode = 403;
        return;
      }
      final key = address.address, last = _rates[key], now = DateTime.now();
      _rates.removeWhere((_, at) => now.difference(at).inMinutes > 1);
      if (_handling ||
          _rates.length > 64 ||
          last != null && now.difference(last).inSeconds < 2) {
        response.statusCode = 429;
        return;
      }
      _rates[key] = now;
      _handling = true;
      try {
        var count = 0;
        final bytes = <int>[];
        await for (final chunk in request.timeout(const Duration(seconds: 5))) {
          count += chunk.length;
          if (count > 18000) bad();
          bytes.addAll(chunk);
        }
        final reply = await incoming(utf8.decode(bytes));
        if (reply == null) {
          response.statusCode = 409;
        } else {
          response.headers.contentType = ContentType.text;
          response.write(reply);
        }
      } finally {
        _handling = false;
      }
    } catch (_) {
      response.statusCode = 400;
    } finally {
      await response.close();
    }
  }

  Future<String> exchange(String id, String offer) async {
    final service = _services[id];
    if (service == null) bad('LOCAL_DEVICE_GONE');
    final endpoints =
        <({InternetAddress address, int port})>[
          for (final candidate
              in _services.values
                  .toList()
                  .reversed
                  .where(
                    (s) => s.attributes['root'] == service.attributes['root'],
                  )
                  .take(4))
            for (final address
                in candidate.hostAddresses
                    .map(InternetAddress.tryParse)
                    .whereType<InternetAddress>()
                    .where(privateAddress))
              (address: address, port: candidate.port),
        ]..sort(
          (a, b) => a.address.type == b.address.type
              ? 0
              : a.address.type == InternetAddressType.IPv4
              ? -1
              : 1,
        );
    if (endpoints.isEmpty) bad('LOCAL_NETWORK_UNAVAILABLE');
    // Resolve through NSD once, then pin the socket to that private address.
    // The logical .local host permits a narrow Android cleartext exception;
    // official/custom HTTPS endpoints never receive this client's traffic.
    Object? failure;
    for (final endpoint in endpoints) {
      final client = HttpClient()
        ..connectionTimeout = const Duration(seconds: 4)
        ..findProxy = (_) => 'DIRECT';
      client.connectionFactory = (uri, _, _) =>
          Socket.startConnect(endpoint.address, endpoint.port);
      try {
        final request = await client.postUrl(
          Uri(
            scheme: 'http',
            host: 'pinelink.local',
            port: endpoint.port,
            path: '/pinelink/local/v1',
          ),
        );
        request.headers.contentType = ContentType.text;
        request.write(offer);
        final response = await request.close().timeout(
          const Duration(seconds: 18),
        );
        if (response.statusCode != 200) {
          bad(
            response.statusCode == 429 || response.statusCode == 409
                ? 'LOCAL_DEVICE_BUSY'
                : 'LOCAL_CONNECTION_LOST',
          );
        }
        var count = 0;
        final bytes = <int>[];
        await for (final chunk in response.timeout(
          const Duration(seconds: 5),
        )) {
          count += chunk.length;
          if (count > 18000) bad();
          bytes.addAll(chunk);
        }
        return utf8.decode(bytes);
      } catch (e) {
        failure = e;
      } finally {
        client.close(force: true);
      }
    }
    throw failure ?? StateError('LOCAL_CONNECTION_LOST');
  }

  Future<void> pause() async {
    await _starting;
    await _events?.cancel();
    _events = null;
    if (_discovery?.isReady == true && _discovery?.isStopped != true) {
      await _discovery!.stop();
    }
    _discovery = null;
    running = false;
    _services.clear();
    changed();
  }

  Future<void> close() async {
    _closed = true;
    await pause();
    if (_broadcast?.isReady == true && _broadcast?.isStopped != true) {
      await _broadcast!.stop();
    }
    await _server?.close(force: true);
  }
}
