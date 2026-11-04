// Test-only native entry point. Never imported by lib/main.dart or release.
// adb forwards this loopback port for a kernel-isolated interoperability runner.
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:pinelink_mobile/main.dart' show PinelinkApp;
import 'package:pinelink_mobile/src/local/runtime.dart';
import 'package:pinelink_mobile/src/local/protocol.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final r = (await openLocalController())!;
  runApp(PinelinkApp(local: r));
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 17844);
  await for (final req in server) {
    req.response.headers.contentType = ContentType.json;
    try {
      final v = req.method == 'POST'
          ? jsonDecode(await utf8.decoder.bind(req).join()) as Json
          : <String, dynamic>{};
      dynamic result;
      switch (req.uri.path) {
        case '/state':
          final native = r as NativeLocalRuntime;
          final chunks = await native.store.db
              .customSelect(
                'SELECT job,kind,idx,length(bytes) AS size FROM local_bytes',
              )
              .get();
          final journal = await native.store.db
              .customSelect('SELECT COUNT(*) AS n FROM local_journal')
              .getSingle();
          result = {
            'root': native.identity.root,
            'routes': await native.inspectRoutes(),
            'stored': [
              for (final row in chunks)
                {
                  'job': row.read<String>('job'),
                  'kind': row.read<String>('kind'),
                  'index': row.read<int>('idx'),
                  'size': row.read<int>('size'),
                },
            ],
            'journal': journal.read<int>('n'),
            'pairing': r.pairing,
            'peers': r.peers,
            'jobs': r.jobs,
            'nearby': r.nearby,
            'discoveryError': r.discoveryError,
            'online': {
              for (final p in r.peers) p['root']: r.reachable(p['root']),
            },
          };
        case '/invite':
          result = await r.invite(v['expected']);
        case '/join':
          result = await r.join(v['code'], v['expected']);
        case '/confirm':
          await r.confirm();
          result = true;
        case '/cancel-pair':
          await r.cancelPairing();
          result = true;
        case '/nearby':
          await r.connectNearby(v['id']);
          result = true;
        case '/discover':
          await r.discover();
          result = true;
        case '/action':
          await r.action(v['id'], v['action']);
          result = true;
        case '/send':
          final peer = r.peers.firstWhere((p) => p['root'] == v['root']);
          final bytes = Uint8List.fromList(
            List.generate(v['size'], (i) => (i * 29 + 7) & 255),
          );
          result = await r.send(bytes, v['name'], peer);
        case '/plain':
          final plain = await r.plaintext(v['id']);
          result = plain == null ? null : base64Encode(plain);
        case '/rename':
          await r.rename(v['root'], v['name']);
          result = true;
        case '/remove':
          await r.remove(v['root']);
          result = true;
        default:
          req.response.statusCode = 404;
      }
      req.response.write(jsonEncode({'result': result}));
    } catch (e, stack) {
      stderr.writeln('$e\n$stack');
      req.response.statusCode = 400;
      req.response.write(jsonEncode({'error': '$e'}));
    }
    await req.response.close();
  }
}
