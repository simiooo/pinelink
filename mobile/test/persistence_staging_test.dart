import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pinelink_mobile/src/data/database.dart';
import 'package:pinelink_mobile/src/data/identity.dart';
import 'package:pinelink_mobile/src/data/runtime.dart';

import 'persistence_test.dart' show MemoryVault;

void main() {
  final api = Platform.environment['PINELINK_STAGING_API'];
  test(
    'real signed staging facts catch up after offline SQLite restart',
    () async {
      final root = await Directory.systemTemp.createTemp('pinelink-staging-');
      final sender = 'source-${randomUuid()}';
      final owner = 'persistence-${randomUuid()}';
      final binary = File(
        '../.artifacts/persistence-cli${Platform.isWindows ? '.exe' : ''}',
      ).absolute.path;
      final built = await Process.run('go', [
        'build',
        '-o',
        binary,
        './cmd/pinelink',
      ], workingDirectory: '../cli');
      expect(built.exitCode, 0);
      Future<Map<String, dynamic>> cli(List<String> command) async {
        final process = await Process.start(
          binary,
          [
            '--server',
            api!,
            '--owner',
            owner,
            '--device',
            sender,
            '--scope',
            owner,
            '--encryption-domain',
            owner,
            '--json',
            ...command,
          ],
          environment: {
            ...Platform.environment,
            'PINELINK_CONFIG_HOME': '${root.path}/keys',
          },
        );
        final output = process.stdout.transform(utf8.decoder).join();
        final errors = process.stderr.transform(utf8.decoder).join();
        final code = await process.exitCode.timeout(
          const Duration(seconds: 90),
          onTimeout: () {
            process.kill();
            throw TimeoutException('owned staging CLI command timed out');
          },
        );
        await errors;
        if (code != 0) {
          throw StateError('owned staging CLI command failed');
        }
        return jsonDecode(await output) as Map<String, dynamic>;
      }

      final remote = http.Client();
      final info = jsonDecode(
        (await remote.get(
          Uri.parse('$api/v1/replication/info?nonce=${'b' * 43}'),
        )).body,
      ) as Map<String, dynamic>;
      var offline = false;
      http.Client transport() => MockClient((request) async {
        if (offline) throw http.ClientException('offline');
        final forwarded = http.Request(request.method, request.url)
          ..headers.addAll(request.headers)
          ..bodyBytes = request.bodyBytes;
        return http.Response.fromStream(await remote.send(forwarded));
      });
      final vault = MemoryVault();
      final file = File('${root.path}/history.sqlite');
      var runtime = await ClientRuntime.open(
        server: api!,
        owner: owner,
        pin: info['public_key'],
        database: ClientDatabase.testing(NativeDatabase(file)),
        vault: vault,
        client: transport(),
        autoStart: false,
      );
      addTearDown(() async {
        await runtime.close();
        remote.close();
        final target = root.absolute.path;
        if (!target.startsWith(
          '${Directory.systemTemp.absolute.path}${Platform.pathSeparator}pinelink-staging-',
        )) {
          throw StateError('unexpected staging fixture directory');
        }
        await root.delete(recursive: true);
      });
      await runtime.sync(force: true);
      final original = runtime.identity.root;
      await cli(['device', 'register']);
      final input = File('${root.path}/private-staging.fig');
      await input.writeAsBytes(List<int>.generate(8192, (i) => i % 251));
      final sent = await cli([
        'p2p',
        'send',
        input.path,
        '--to-owner',
        owner,
        '--to-device',
        runtime.identity.device,
        '--path',
        'relay',
      ]);
      expect(sent['stored'], true);
      final id = sent['id'] as String;
      await runtime.request('POST', '/v1/transfers/$id/accept', {
        'owner_id': owner,
        'device_id': runtime.identity.device,
        'expected_version': sent['version'],
      });
      final tasks = await runtime.request(
        'GET',
        '/v1/tasks?include_deleted=true',
      );
      expect(tasks['tasks'], isNotEmpty);
      final transfers = await runtime.request('GET', '/v1/transfers');
      expect(
        (transfers['transfers'] as List).any(
          (t) => t['id'] == id && t['state'] == 'ACCEPTED',
        ),
        true,
      );
      final cursor = (await runtime.database.target())['cursor'] as int;
      offline = true;
      await cli(['cancel', id]);
      await runtime.close();
      runtime = await ClientRuntime.open(
        server: api,
        owner: owner,
        pin: info['public_key'],
        database: ClientDatabase.testing(NativeDatabase(file)),
        vault: vault,
        client: transport(),
        autoStart: false,
      );
      expect(runtime.identity.root, original);
      expect(
        (await runtime.request(
          'GET',
          '/v1/tasks?include_deleted=true',
        ))['tasks'],
        isNotEmpty,
      );
      final cached = await runtime.request('GET', '/v1/transfers');
      expect(
        (cached['transfers'] as List).any(
          (t) => t['id'] == id && t['state'] == 'ACCEPTED',
        ),
        true,
      );
      offline = false;
      runtime.start(observeLifecycle: false);
      final deadline = DateTime.now().add(const Duration(seconds: 40));
      var caught = false;
      while (DateTime.now().isBefore(deadline)) {
        final rows = await runtime.database
            .select(runtime.database.historyProjection)
            .get();
        caught = rows.any(
          (r) => r.aggregateId == id && r.body.contains('CANCELLED'),
        );
        if (caught) break;
        await Future<void>.delayed(const Duration(milliseconds: 400));
      }
      expect(caught, true);
      expect((await runtime.database.target())['cursor'], greaterThan(cursor));
      final updated = await runtime.request('GET', '/v1/transfers');
      expect(
        (updated['transfers'] as List).any(
          (t) => t['id'] == id && t['state'] == 'CANCELLED',
        ),
        true,
      );
      await File('../.artifacts/replication/flutter-native-staging.json')
          .writeAsString(
            jsonEncode({
              'api': api,
              'tested_at': DateTime.now().toUtc().toIso8601String(),
              'root_preserved': runtime.identity.root == original,
              'cursor_before': cursor,
              'cursor_after': (await runtime.database.target())['cursor'],
              'checks': {
                'signed_registration_and_binding': true,
                'real_encrypted_cloud_request': true,
                'sqlite_restart_and_offline_history': true,
                'background_catchup_without_manual_sync': true,
                'server_cancellation_reaches_history_and_cache': true,
              },
            }),
          );
    },
    skip: api == null,
    timeout: const Timeout(Duration(minutes: 4)),
  );
}
