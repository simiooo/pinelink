import 'dart:async';

import 'package:flutter/foundation.dart';

import '../data/runtime.dart' as local;

class TransferTask {
  const TransferTask(
    this.id,
    this.kind,
    this.state,
    this.progress,
    this.total, {
    this.referenceId = '',
    this.version = 1,
    this.errorCode,
    this.errorMessage,
    this.updatedAt,
  });
  final String id;
  final String kind;
  final String state;
  final int progress;
  final int total;
  final String referenceId;
  final int version;
  final String? errorCode;
  final String? errorMessage;
  final String? updatedAt;
}

class TransferRequest {
  const TransferRequest(
    this.id,
    this.state,
    this.version,
    this.fileName,
    this.sizeBytes,
    this.senderOwner,
    this.senderDevice, {
    this.recipientDevice = '',
    this.entryId = '',
    this.deliveryMode = '',
  });
  final String id;
  final String state;
  final int version;
  final String? fileName;
  final int? sizeBytes;
  final String senderOwner;
  final String senderDevice;
  final String recipientDevice;
  final String entryId;
  final String deliveryMode;
}

class Device {
  const Device(this.id, this.status, this.lastSeenAt);
  final String id;
  final String status;
  final String lastSeenAt;
}

class Quota {
  const Quota(this.capacity, this.used);
  final int capacity;
  final int used;
}

class FileSnapshot {
  const FileSnapshot(
    this.id,
    this.fileName,
    this.state,
    this.version,
    this.retentionClass,
    this.expiresAt,
  );
  final String id;
  final String fileName;
  final String state;
  final int version;
  final String retentionClass;
  final String? expiresAt;
}

class ApiException implements Exception {
  const ApiException(this.status, this.code);
  final int status;
  final String code;
}

class PinelinkApi extends ChangeNotifier {
  bool tasksNetworkFresh = false;
  static const owner = String.fromEnvironment(
    'PINELINK_OWNER',
    defaultValue: 'dev-owner',
  );
  static const device = String.fromEnvironment(
    'PINELINK_DEVICE',
    defaultValue: 'dev-device',
  );
  static const scope = String.fromEnvironment(
    'PINELINK_SCOPE',
    defaultValue: 'dev-scope',
  );

  PinelinkApi({String? baseUrl, this.openRuntime})
    : baseUrl =
          baseUrl ??
          const String.fromEnvironment(
            'PINELINK_API_URL',
            defaultValue: 'http://127.0.0.1:8080',
          );
  final String baseUrl;
  final Future<local.ClientRuntime> Function()? openRuntime;
  Future<local.ClientRuntime>? _runtime;
  StreamSubscription<String>? _changes;
  bool _closed = false;
  Future<void>? _closing;
  String? _deviceIdentity;
  String get currentDevice => _deviceIdentity ?? device;
  Future<local.ClientRuntime> get runtime {
    if (_closed) return Future.error(StateError('CLIENT_CLOSED'));
    if (_runtime != null) return _runtime!;
    final opening = _open();
    _runtime = opening;
    unawaited(
      opening.then<void>(
        (_) {},
        onError: (Object _, StackTrace __) {
          if (identical(_runtime, opening)) _runtime = null;
        },
      ),
    );
    return opening;
  }

  Future<local.ClientRuntime> _open() async {
    final value =
        await (openRuntime?.call() ??
            local.ClientRuntime.open(
              server: baseUrl,
              owner: owner,
              device: const String.fromEnvironment('PINELINK_DEVICE'),
              pin: const String.fromEnvironment('PINELINK_HISTORY_KEY'),
            ));
    if (_closed) {
      await value.close();
      throw StateError('CLIENT_CLOSED');
    }
    _deviceIdentity = value.identity.device;
    _changes = value.database.watchResponses().listen((_) {
      if (!_closed) notifyListeners();
    });
    return value;
  }

  Future<void> close() => _closing ??= _shutdown();
  Future<void> _shutdown() async {
    _closed = true;
    await _changes?.cancel();
    if (_runtime != null) {
      try {
        await (await _runtime!).close();
      } catch (_) {}
    }
  }

  @override
  void dispose() {
    unawaited(close());
    super.dispose();
  }

  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async {
    final path =
        '/v1/tasks?owner_id=$owner&device_id=$device&include_deleted=$includeDeleted';
    final data = await _get(path);
    tasksNetworkFresh = (await runtime).isNetworkFresh(path);
    return (data['tasks'] as List? ?? [])
        .map(
          (item) => TransferTask(
            item['id'],
            item['kind'],
            item['state'],
            item['progress_bytes'],
            item['total_bytes'],
            referenceId: item['reference_id'] ?? '',
            version: item['version'] ?? 1,
            errorCode: item['error_code'],
            errorMessage: item['error_message'],
            updatedAt: item['updated_at'],
          ),
        )
        .toList();
  }

  Future<List<TransferRequest>> getPending(String owner, String device) async {
    final data = await _get(
      '/v1/notifications?owner_id=$owner&device_id=$device&state=PENDING',
    );
    return (data['transfers'] as List? ?? []).map(_transfer).toList();
  }

  Future<List<TransferRequest>> getTransfers(
    String owner,
    String device,
  ) async {
    final data = await _get('/v1/transfers?owner_id=$owner&device_id=$device');
    return (data['transfers'] as List? ?? []).map(_transfer).toList();
  }

  TransferRequest _transfer(dynamic item) => TransferRequest(
    item['id'],
    item['state'],
    item['version'],
    item['file_name'],
    item['size_bytes'],
    item['sender_owner_id'],
    item['sender_device_id'],
    recipientDevice: item['recipient_device_id'] ?? '',
    entryId: item['entry_id'] ?? '',
    deliveryMode: item['delivery_mode'] ?? '',
  );

  Future<String> getPull(String id, String owner, String device) async {
    final data = await _get('/v1/pulls/$id?owner_id=$owner&device_id=$device');
    return data['file_name'] as String;
  }

  Future<void> respond(
    String id,
    String action,
    String owner,
    String device,
    int version,
  ) async {
    await _post('/v1/transfers/$id/$action', {
      'owner_id': owner,
      'device_id': device,
      'expected_version': version,
    });
  }

  Future<void> controlTask(
    String id,
    String action,
    String owner,
    String device,
    int version,
  ) async {
    await _post('/v1/tasks/$id/$action', {
      'owner_id': owner,
      'device_id': device,
      'expected_version': version,
    });
  }

  Future<List<Device>> getDevices(String owner) async {
    final data = await _get('/v1/devices?owner_id=$owner');
    return (data['devices'] as List? ?? [])
        .map(
          (item) =>
              Device(item['device_id'], item['status'], item['last_seen_at']),
        )
        .toList();
  }

  Future<Quota> getQuota(String owner) async {
    final data = await _get('/v1/quota?scope_id=$owner');
    return Quota(data['quota_bytes'], data['used_bytes']);
  }

  Future<FileSnapshot> getFile(String id, String owner) async {
    final data = await _get('/v1/files/$id?owner_id=$owner');
    return FileSnapshot(
      data['id'],
      data['file_name'],
      data['state'],
      data['version'],
      data['retention_class'],
      data['expires_at'],
    );
  }

  Future<void> promoteFile(
    String id,
    String owner,
    String device,
    int version,
  ) async {
    await _post('/v1/files/$id/promote', {
      'owner_id': owner,
      'device_id': device,
      'expected_version': version,
    });
  }

  Future<void> revokeDevice(String owner, String device) async {
    await _post('/v1/devices/revoke', {'owner_id': owner, 'device_id': device});
  }

  Future<Map<String, dynamic>> _get(String path) async {
    try {
      return await (await runtime).request('GET', path);
    } on local.DataException catch (error) {
      throw ApiException(error.status, error.code);
    }
  }

  Future<void> _post(String path, Map<String, dynamic> body) async {
    try {
      await (await runtime).request('POST', path, body);
    } on local.DataException catch (error) {
      throw ApiException(error.status, error.code);
    }
  }
}
