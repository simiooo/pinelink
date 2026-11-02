// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'database.dart';

// ignore_for_file: type=lint
class $ClientMetadataTable extends ClientMetadata
    with TableInfo<$ClientMetadataTable, ClientMetadataData> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ClientMetadataTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _keyMeta = const VerificationMeta('key');
  @override
  late final GeneratedColumn<String> key = GeneratedColumn<String>(
    'key',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _bodyMeta = const VerificationMeta('body');
  @override
  late final GeneratedColumn<String> body = GeneratedColumn<String>(
    'body',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [key, body];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'client_metadata';
  @override
  VerificationContext validateIntegrity(
    Insertable<ClientMetadataData> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('key')) {
      context.handle(
        _keyMeta,
        key.isAcceptableOrUnknown(data['key']!, _keyMeta),
      );
    } else if (isInserting) {
      context.missing(_keyMeta);
    }
    if (data.containsKey('body')) {
      context.handle(
        _bodyMeta,
        body.isAcceptableOrUnknown(data['body']!, _bodyMeta),
      );
    } else if (isInserting) {
      context.missing(_bodyMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {key};
  @override
  ClientMetadataData map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return ClientMetadataData(
      key: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}key'],
      )!,
      body: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}body'],
      )!,
    );
  }

  @override
  $ClientMetadataTable createAlias(String alias) {
    return $ClientMetadataTable(attachedDatabase, alias);
  }
}

class ClientMetadataData extends DataClass
    implements Insertable<ClientMetadataData> {
  final String key;
  final String body;
  const ClientMetadataData({required this.key, required this.body});
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['key'] = Variable<String>(key);
    map['body'] = Variable<String>(body);
    return map;
  }

  ClientMetadataCompanion toCompanion(bool nullToAbsent) {
    return ClientMetadataCompanion(key: Value(key), body: Value(body));
  }

  factory ClientMetadataData.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return ClientMetadataData(
      key: serializer.fromJson<String>(json['key']),
      body: serializer.fromJson<String>(json['body']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'key': serializer.toJson<String>(key),
      'body': serializer.toJson<String>(body),
    };
  }

  ClientMetadataData copyWith({String? key, String? body}) =>
      ClientMetadataData(key: key ?? this.key, body: body ?? this.body);
  ClientMetadataData copyWithCompanion(ClientMetadataCompanion data) {
    return ClientMetadataData(
      key: data.key.present ? data.key.value : this.key,
      body: data.body.present ? data.body.value : this.body,
    );
  }

  @override
  String toString() {
    return (StringBuffer('ClientMetadataData(')
          ..write('key: $key, ')
          ..write('body: $body')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(key, body);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is ClientMetadataData &&
          other.key == this.key &&
          other.body == this.body);
}

class ClientMetadataCompanion extends UpdateCompanion<ClientMetadataData> {
  final Value<String> key;
  final Value<String> body;
  final Value<int> rowid;
  const ClientMetadataCompanion({
    this.key = const Value.absent(),
    this.body = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ClientMetadataCompanion.insert({
    required String key,
    required String body,
    this.rowid = const Value.absent(),
  }) : key = Value(key),
       body = Value(body);
  static Insertable<ClientMetadataData> custom({
    Expression<String>? key,
    Expression<String>? body,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (key != null) 'key': key,
      if (body != null) 'body': body,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ClientMetadataCompanion copyWith({
    Value<String>? key,
    Value<String>? body,
    Value<int>? rowid,
  }) {
    return ClientMetadataCompanion(
      key: key ?? this.key,
      body: body ?? this.body,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (key.present) {
      map['key'] = Variable<String>(key.value);
    }
    if (body.present) {
      map['body'] = Variable<String>(body.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ClientMetadataCompanion(')
          ..write('key: $key, ')
          ..write('body: $body, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $ConfirmedPeersTable extends ConfirmedPeers
    with TableInfo<$ConfirmedPeersTable, PeerRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ConfirmedPeersTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _rootMeta = const VerificationMeta('root');
  @override
  late final GeneratedColumn<String> root = GeneratedColumn<String>(
    'root',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _signingKeyMeta = const VerificationMeta(
    'signingKey',
  );
  @override
  late final GeneratedColumn<String> signingKey = GeneratedColumn<String>(
    'signing_key',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _encryptionKeyMeta = const VerificationMeta(
    'encryptionKey',
  );
  @override
  late final GeneratedColumn<String> encryptionKey = GeneratedColumn<String>(
    'encryption_key',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _revokedMeta = const VerificationMeta(
    'revoked',
  );
  @override
  late final GeneratedColumn<bool> revoked = GeneratedColumn<bool>(
    'revoked',
    aliasedName,
    false,
    type: DriftSqlType.bool,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'CHECK ("revoked" IN (0, 1))',
    ),
    defaultValue: const Constant(false),
  );
  @override
  List<GeneratedColumn> get $columns => [
    root,
    signingKey,
    encryptionKey,
    revoked,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'confirmed_peers';
  @override
  VerificationContext validateIntegrity(
    Insertable<PeerRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('root')) {
      context.handle(
        _rootMeta,
        root.isAcceptableOrUnknown(data['root']!, _rootMeta),
      );
    } else if (isInserting) {
      context.missing(_rootMeta);
    }
    if (data.containsKey('signing_key')) {
      context.handle(
        _signingKeyMeta,
        signingKey.isAcceptableOrUnknown(data['signing_key']!, _signingKeyMeta),
      );
    } else if (isInserting) {
      context.missing(_signingKeyMeta);
    }
    if (data.containsKey('encryption_key')) {
      context.handle(
        _encryptionKeyMeta,
        encryptionKey.isAcceptableOrUnknown(
          data['encryption_key']!,
          _encryptionKeyMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_encryptionKeyMeta);
    }
    if (data.containsKey('revoked')) {
      context.handle(
        _revokedMeta,
        revoked.isAcceptableOrUnknown(data['revoked']!, _revokedMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {root};
  @override
  PeerRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return PeerRow(
      root: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}root'],
      )!,
      signingKey: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}signing_key'],
      )!,
      encryptionKey: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}encryption_key'],
      )!,
      revoked: attachedDatabase.typeMapping.read(
        DriftSqlType.bool,
        data['${effectivePrefix}revoked'],
      )!,
    );
  }

  @override
  $ConfirmedPeersTable createAlias(String alias) {
    return $ConfirmedPeersTable(attachedDatabase, alias);
  }
}

class PeerRow extends DataClass implements Insertable<PeerRow> {
  final String root;
  final String signingKey;
  final String encryptionKey;
  final bool revoked;
  const PeerRow({
    required this.root,
    required this.signingKey,
    required this.encryptionKey,
    required this.revoked,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['root'] = Variable<String>(root);
    map['signing_key'] = Variable<String>(signingKey);
    map['encryption_key'] = Variable<String>(encryptionKey);
    map['revoked'] = Variable<bool>(revoked);
    return map;
  }

  ConfirmedPeersCompanion toCompanion(bool nullToAbsent) {
    return ConfirmedPeersCompanion(
      root: Value(root),
      signingKey: Value(signingKey),
      encryptionKey: Value(encryptionKey),
      revoked: Value(revoked),
    );
  }

  factory PeerRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return PeerRow(
      root: serializer.fromJson<String>(json['root']),
      signingKey: serializer.fromJson<String>(json['signingKey']),
      encryptionKey: serializer.fromJson<String>(json['encryptionKey']),
      revoked: serializer.fromJson<bool>(json['revoked']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'root': serializer.toJson<String>(root),
      'signingKey': serializer.toJson<String>(signingKey),
      'encryptionKey': serializer.toJson<String>(encryptionKey),
      'revoked': serializer.toJson<bool>(revoked),
    };
  }

  PeerRow copyWith({
    String? root,
    String? signingKey,
    String? encryptionKey,
    bool? revoked,
  }) => PeerRow(
    root: root ?? this.root,
    signingKey: signingKey ?? this.signingKey,
    encryptionKey: encryptionKey ?? this.encryptionKey,
    revoked: revoked ?? this.revoked,
  );
  PeerRow copyWithCompanion(ConfirmedPeersCompanion data) {
    return PeerRow(
      root: data.root.present ? data.root.value : this.root,
      signingKey: data.signingKey.present
          ? data.signingKey.value
          : this.signingKey,
      encryptionKey: data.encryptionKey.present
          ? data.encryptionKey.value
          : this.encryptionKey,
      revoked: data.revoked.present ? data.revoked.value : this.revoked,
    );
  }

  @override
  String toString() {
    return (StringBuffer('PeerRow(')
          ..write('root: $root, ')
          ..write('signingKey: $signingKey, ')
          ..write('encryptionKey: $encryptionKey, ')
          ..write('revoked: $revoked')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(root, signingKey, encryptionKey, revoked);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is PeerRow &&
          other.root == this.root &&
          other.signingKey == this.signingKey &&
          other.encryptionKey == this.encryptionKey &&
          other.revoked == this.revoked);
}

class ConfirmedPeersCompanion extends UpdateCompanion<PeerRow> {
  final Value<String> root;
  final Value<String> signingKey;
  final Value<String> encryptionKey;
  final Value<bool> revoked;
  final Value<int> rowid;
  const ConfirmedPeersCompanion({
    this.root = const Value.absent(),
    this.signingKey = const Value.absent(),
    this.encryptionKey = const Value.absent(),
    this.revoked = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ConfirmedPeersCompanion.insert({
    required String root,
    required String signingKey,
    required String encryptionKey,
    this.revoked = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : root = Value(root),
       signingKey = Value(signingKey),
       encryptionKey = Value(encryptionKey);
  static Insertable<PeerRow> custom({
    Expression<String>? root,
    Expression<String>? signingKey,
    Expression<String>? encryptionKey,
    Expression<bool>? revoked,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (root != null) 'root': root,
      if (signingKey != null) 'signing_key': signingKey,
      if (encryptionKey != null) 'encryption_key': encryptionKey,
      if (revoked != null) 'revoked': revoked,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ConfirmedPeersCompanion copyWith({
    Value<String>? root,
    Value<String>? signingKey,
    Value<String>? encryptionKey,
    Value<bool>? revoked,
    Value<int>? rowid,
  }) {
    return ConfirmedPeersCompanion(
      root: root ?? this.root,
      signingKey: signingKey ?? this.signingKey,
      encryptionKey: encryptionKey ?? this.encryptionKey,
      revoked: revoked ?? this.revoked,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (root.present) {
      map['root'] = Variable<String>(root.value);
    }
    if (signingKey.present) {
      map['signing_key'] = Variable<String>(signingKey.value);
    }
    if (encryptionKey.present) {
      map['encryption_key'] = Variable<String>(encryptionKey.value);
    }
    if (revoked.present) {
      map['revoked'] = Variable<bool>(revoked.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ConfirmedPeersCompanion(')
          ..write('root: $root, ')
          ..write('signingKey: $signingKey, ')
          ..write('encryptionKey: $encryptionKey, ')
          ..write('revoked: $revoked, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $ReceivedHistoryTable extends ReceivedHistory
    with TableInfo<$ReceivedHistoryTable, InboxRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ReceivedHistoryTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _hashMeta = const VerificationMeta('hash');
  @override
  late final GeneratedColumn<String> hash = GeneratedColumn<String>(
    'hash',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _bodyMeta = const VerificationMeta('body');
  @override
  late final GeneratedColumn<String> body = GeneratedColumn<String>(
    'body',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _statusMeta = const VerificationMeta('status');
  @override
  late final GeneratedColumn<String> status = GeneratedColumn<String>(
    'status',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [id, hash, body, status];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'received_history';
  @override
  VerificationContext validateIntegrity(
    Insertable<InboxRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('hash')) {
      context.handle(
        _hashMeta,
        hash.isAcceptableOrUnknown(data['hash']!, _hashMeta),
      );
    } else if (isInserting) {
      context.missing(_hashMeta);
    }
    if (data.containsKey('body')) {
      context.handle(
        _bodyMeta,
        body.isAcceptableOrUnknown(data['body']!, _bodyMeta),
      );
    } else if (isInserting) {
      context.missing(_bodyMeta);
    }
    if (data.containsKey('status')) {
      context.handle(
        _statusMeta,
        status.isAcceptableOrUnknown(data['status']!, _statusMeta),
      );
    } else if (isInserting) {
      context.missing(_statusMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  InboxRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return InboxRow(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      hash: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}hash'],
      )!,
      body: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}body'],
      )!,
      status: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}status'],
      )!,
    );
  }

  @override
  $ReceivedHistoryTable createAlias(String alias) {
    return $ReceivedHistoryTable(attachedDatabase, alias);
  }
}

class InboxRow extends DataClass implements Insertable<InboxRow> {
  final String id;
  final String hash;
  final String body;
  final String status;
  const InboxRow({
    required this.id,
    required this.hash,
    required this.body,
    required this.status,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['hash'] = Variable<String>(hash);
    map['body'] = Variable<String>(body);
    map['status'] = Variable<String>(status);
    return map;
  }

  ReceivedHistoryCompanion toCompanion(bool nullToAbsent) {
    return ReceivedHistoryCompanion(
      id: Value(id),
      hash: Value(hash),
      body: Value(body),
      status: Value(status),
    );
  }

  factory InboxRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return InboxRow(
      id: serializer.fromJson<String>(json['id']),
      hash: serializer.fromJson<String>(json['hash']),
      body: serializer.fromJson<String>(json['body']),
      status: serializer.fromJson<String>(json['status']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'hash': serializer.toJson<String>(hash),
      'body': serializer.toJson<String>(body),
      'status': serializer.toJson<String>(status),
    };
  }

  InboxRow copyWith({String? id, String? hash, String? body, String? status}) =>
      InboxRow(
        id: id ?? this.id,
        hash: hash ?? this.hash,
        body: body ?? this.body,
        status: status ?? this.status,
      );
  InboxRow copyWithCompanion(ReceivedHistoryCompanion data) {
    return InboxRow(
      id: data.id.present ? data.id.value : this.id,
      hash: data.hash.present ? data.hash.value : this.hash,
      body: data.body.present ? data.body.value : this.body,
      status: data.status.present ? data.status.value : this.status,
    );
  }

  @override
  String toString() {
    return (StringBuffer('InboxRow(')
          ..write('id: $id, ')
          ..write('hash: $hash, ')
          ..write('body: $body, ')
          ..write('status: $status')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(id, hash, body, status);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is InboxRow &&
          other.id == this.id &&
          other.hash == this.hash &&
          other.body == this.body &&
          other.status == this.status);
}

class ReceivedHistoryCompanion extends UpdateCompanion<InboxRow> {
  final Value<String> id;
  final Value<String> hash;
  final Value<String> body;
  final Value<String> status;
  final Value<int> rowid;
  const ReceivedHistoryCompanion({
    this.id = const Value.absent(),
    this.hash = const Value.absent(),
    this.body = const Value.absent(),
    this.status = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ReceivedHistoryCompanion.insert({
    required String id,
    required String hash,
    required String body,
    required String status,
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       hash = Value(hash),
       body = Value(body),
       status = Value(status);
  static Insertable<InboxRow> custom({
    Expression<String>? id,
    Expression<String>? hash,
    Expression<String>? body,
    Expression<String>? status,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (hash != null) 'hash': hash,
      if (body != null) 'body': body,
      if (status != null) 'status': status,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ReceivedHistoryCompanion copyWith({
    Value<String>? id,
    Value<String>? hash,
    Value<String>? body,
    Value<String>? status,
    Value<int>? rowid,
  }) {
    return ReceivedHistoryCompanion(
      id: id ?? this.id,
      hash: hash ?? this.hash,
      body: body ?? this.body,
      status: status ?? this.status,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (hash.present) {
      map['hash'] = Variable<String>(hash.value);
    }
    if (body.present) {
      map['body'] = Variable<String>(body.value);
    }
    if (status.present) {
      map['status'] = Variable<String>(status.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ReceivedHistoryCompanion(')
          ..write('id: $id, ')
          ..write('hash: $hash, ')
          ..write('body: $body, ')
          ..write('status: $status, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $HistoryProjectionTable extends HistoryProjection
    with TableInfo<$HistoryProjectionTable, HistoryRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $HistoryProjectionTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _aggregateIdMeta = const VerificationMeta(
    'aggregateId',
  );
  @override
  late final GeneratedColumn<String> aggregateId = GeneratedColumn<String>(
    'aggregate_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _authorityMeta = const VerificationMeta(
    'authority',
  );
  @override
  late final GeneratedColumn<String> authority = GeneratedColumn<String>(
    'authority',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _sourceMeta = const VerificationMeta('source');
  @override
  late final GeneratedColumn<String> source = GeneratedColumn<String>(
    'source',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _kindMeta = const VerificationMeta('kind');
  @override
  late final GeneratedColumn<String> kind = GeneratedColumn<String>(
    'kind',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _versionMeta = const VerificationMeta(
    'version',
  );
  @override
  late final GeneratedColumn<int> version = GeneratedColumn<int>(
    'version',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _bodyMeta = const VerificationMeta('body');
  @override
  late final GeneratedColumn<String> body = GeneratedColumn<String>(
    'body',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    aggregateId,
    authority,
    source,
    kind,
    version,
    body,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'history_projection';
  @override
  VerificationContext validateIntegrity(
    Insertable<HistoryRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('aggregate_id')) {
      context.handle(
        _aggregateIdMeta,
        aggregateId.isAcceptableOrUnknown(
          data['aggregate_id']!,
          _aggregateIdMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_aggregateIdMeta);
    }
    if (data.containsKey('authority')) {
      context.handle(
        _authorityMeta,
        authority.isAcceptableOrUnknown(data['authority']!, _authorityMeta),
      );
    } else if (isInserting) {
      context.missing(_authorityMeta);
    }
    if (data.containsKey('source')) {
      context.handle(
        _sourceMeta,
        source.isAcceptableOrUnknown(data['source']!, _sourceMeta),
      );
    } else if (isInserting) {
      context.missing(_sourceMeta);
    }
    if (data.containsKey('kind')) {
      context.handle(
        _kindMeta,
        kind.isAcceptableOrUnknown(data['kind']!, _kindMeta),
      );
    } else if (isInserting) {
      context.missing(_kindMeta);
    }
    if (data.containsKey('version')) {
      context.handle(
        _versionMeta,
        version.isAcceptableOrUnknown(data['version']!, _versionMeta),
      );
    } else if (isInserting) {
      context.missing(_versionMeta);
    }
    if (data.containsKey('body')) {
      context.handle(
        _bodyMeta,
        body.isAcceptableOrUnknown(data['body']!, _bodyMeta),
      );
    } else if (isInserting) {
      context.missing(_bodyMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  HistoryRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return HistoryRow(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      aggregateId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}aggregate_id'],
      )!,
      authority: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}authority'],
      )!,
      source: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}source'],
      )!,
      kind: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}kind'],
      )!,
      version: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}version'],
      )!,
      body: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}body'],
      )!,
    );
  }

  @override
  $HistoryProjectionTable createAlias(String alias) {
    return $HistoryProjectionTable(attachedDatabase, alias);
  }
}

class HistoryRow extends DataClass implements Insertable<HistoryRow> {
  final String id;
  final String aggregateId;
  final String authority;
  final String source;
  final String kind;
  final int version;
  final String body;
  const HistoryRow({
    required this.id,
    required this.aggregateId,
    required this.authority,
    required this.source,
    required this.kind,
    required this.version,
    required this.body,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['aggregate_id'] = Variable<String>(aggregateId);
    map['authority'] = Variable<String>(authority);
    map['source'] = Variable<String>(source);
    map['kind'] = Variable<String>(kind);
    map['version'] = Variable<int>(version);
    map['body'] = Variable<String>(body);
    return map;
  }

  HistoryProjectionCompanion toCompanion(bool nullToAbsent) {
    return HistoryProjectionCompanion(
      id: Value(id),
      aggregateId: Value(aggregateId),
      authority: Value(authority),
      source: Value(source),
      kind: Value(kind),
      version: Value(version),
      body: Value(body),
    );
  }

  factory HistoryRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return HistoryRow(
      id: serializer.fromJson<String>(json['id']),
      aggregateId: serializer.fromJson<String>(json['aggregateId']),
      authority: serializer.fromJson<String>(json['authority']),
      source: serializer.fromJson<String>(json['source']),
      kind: serializer.fromJson<String>(json['kind']),
      version: serializer.fromJson<int>(json['version']),
      body: serializer.fromJson<String>(json['body']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'aggregateId': serializer.toJson<String>(aggregateId),
      'authority': serializer.toJson<String>(authority),
      'source': serializer.toJson<String>(source),
      'kind': serializer.toJson<String>(kind),
      'version': serializer.toJson<int>(version),
      'body': serializer.toJson<String>(body),
    };
  }

  HistoryRow copyWith({
    String? id,
    String? aggregateId,
    String? authority,
    String? source,
    String? kind,
    int? version,
    String? body,
  }) => HistoryRow(
    id: id ?? this.id,
    aggregateId: aggregateId ?? this.aggregateId,
    authority: authority ?? this.authority,
    source: source ?? this.source,
    kind: kind ?? this.kind,
    version: version ?? this.version,
    body: body ?? this.body,
  );
  HistoryRow copyWithCompanion(HistoryProjectionCompanion data) {
    return HistoryRow(
      id: data.id.present ? data.id.value : this.id,
      aggregateId: data.aggregateId.present
          ? data.aggregateId.value
          : this.aggregateId,
      authority: data.authority.present ? data.authority.value : this.authority,
      source: data.source.present ? data.source.value : this.source,
      kind: data.kind.present ? data.kind.value : this.kind,
      version: data.version.present ? data.version.value : this.version,
      body: data.body.present ? data.body.value : this.body,
    );
  }

  @override
  String toString() {
    return (StringBuffer('HistoryRow(')
          ..write('id: $id, ')
          ..write('aggregateId: $aggregateId, ')
          ..write('authority: $authority, ')
          ..write('source: $source, ')
          ..write('kind: $kind, ')
          ..write('version: $version, ')
          ..write('body: $body')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode =>
      Object.hash(id, aggregateId, authority, source, kind, version, body);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is HistoryRow &&
          other.id == this.id &&
          other.aggregateId == this.aggregateId &&
          other.authority == this.authority &&
          other.source == this.source &&
          other.kind == this.kind &&
          other.version == this.version &&
          other.body == this.body);
}

class HistoryProjectionCompanion extends UpdateCompanion<HistoryRow> {
  final Value<String> id;
  final Value<String> aggregateId;
  final Value<String> authority;
  final Value<String> source;
  final Value<String> kind;
  final Value<int> version;
  final Value<String> body;
  final Value<int> rowid;
  const HistoryProjectionCompanion({
    this.id = const Value.absent(),
    this.aggregateId = const Value.absent(),
    this.authority = const Value.absent(),
    this.source = const Value.absent(),
    this.kind = const Value.absent(),
    this.version = const Value.absent(),
    this.body = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  HistoryProjectionCompanion.insert({
    required String id,
    required String aggregateId,
    required String authority,
    required String source,
    required String kind,
    required int version,
    required String body,
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       aggregateId = Value(aggregateId),
       authority = Value(authority),
       source = Value(source),
       kind = Value(kind),
       version = Value(version),
       body = Value(body);
  static Insertable<HistoryRow> custom({
    Expression<String>? id,
    Expression<String>? aggregateId,
    Expression<String>? authority,
    Expression<String>? source,
    Expression<String>? kind,
    Expression<int>? version,
    Expression<String>? body,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (aggregateId != null) 'aggregate_id': aggregateId,
      if (authority != null) 'authority': authority,
      if (source != null) 'source': source,
      if (kind != null) 'kind': kind,
      if (version != null) 'version': version,
      if (body != null) 'body': body,
      if (rowid != null) 'rowid': rowid,
    });
  }

  HistoryProjectionCompanion copyWith({
    Value<String>? id,
    Value<String>? aggregateId,
    Value<String>? authority,
    Value<String>? source,
    Value<String>? kind,
    Value<int>? version,
    Value<String>? body,
    Value<int>? rowid,
  }) {
    return HistoryProjectionCompanion(
      id: id ?? this.id,
      aggregateId: aggregateId ?? this.aggregateId,
      authority: authority ?? this.authority,
      source: source ?? this.source,
      kind: kind ?? this.kind,
      version: version ?? this.version,
      body: body ?? this.body,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (aggregateId.present) {
      map['aggregate_id'] = Variable<String>(aggregateId.value);
    }
    if (authority.present) {
      map['authority'] = Variable<String>(authority.value);
    }
    if (source.present) {
      map['source'] = Variable<String>(source.value);
    }
    if (kind.present) {
      map['kind'] = Variable<String>(kind.value);
    }
    if (version.present) {
      map['version'] = Variable<int>(version.value);
    }
    if (body.present) {
      map['body'] = Variable<String>(body.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('HistoryProjectionCompanion(')
          ..write('id: $id, ')
          ..write('aggregateId: $aggregateId, ')
          ..write('authority: $authority, ')
          ..write('source: $source, ')
          ..write('kind: $kind, ')
          ..write('version: $version, ')
          ..write('body: $body, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $ResponseCacheTable extends ResponseCache
    with TableInfo<$ResponseCacheTable, ResponseRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ResponseCacheTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _pathMeta = const VerificationMeta('path');
  @override
  late final GeneratedColumn<String> path = GeneratedColumn<String>(
    'path',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _bodyMeta = const VerificationMeta('body');
  @override
  late final GeneratedColumn<String> body = GeneratedColumn<String>(
    'body',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _updatedMeta = const VerificationMeta(
    'updated',
  );
  @override
  late final GeneratedColumn<int> updated = GeneratedColumn<int>(
    'updated',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [path, body, updated];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'response_cache';
  @override
  VerificationContext validateIntegrity(
    Insertable<ResponseRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('path')) {
      context.handle(
        _pathMeta,
        path.isAcceptableOrUnknown(data['path']!, _pathMeta),
      );
    } else if (isInserting) {
      context.missing(_pathMeta);
    }
    if (data.containsKey('body')) {
      context.handle(
        _bodyMeta,
        body.isAcceptableOrUnknown(data['body']!, _bodyMeta),
      );
    } else if (isInserting) {
      context.missing(_bodyMeta);
    }
    if (data.containsKey('updated')) {
      context.handle(
        _updatedMeta,
        updated.isAcceptableOrUnknown(data['updated']!, _updatedMeta),
      );
    } else if (isInserting) {
      context.missing(_updatedMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {path};
  @override
  ResponseRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return ResponseRow(
      path: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}path'],
      )!,
      body: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}body'],
      )!,
      updated: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}updated'],
      )!,
    );
  }

  @override
  $ResponseCacheTable createAlias(String alias) {
    return $ResponseCacheTable(attachedDatabase, alias);
  }
}

class ResponseRow extends DataClass implements Insertable<ResponseRow> {
  final String path;
  final String body;
  final int updated;
  const ResponseRow({
    required this.path,
    required this.body,
    required this.updated,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['path'] = Variable<String>(path);
    map['body'] = Variable<String>(body);
    map['updated'] = Variable<int>(updated);
    return map;
  }

  ResponseCacheCompanion toCompanion(bool nullToAbsent) {
    return ResponseCacheCompanion(
      path: Value(path),
      body: Value(body),
      updated: Value(updated),
    );
  }

  factory ResponseRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return ResponseRow(
      path: serializer.fromJson<String>(json['path']),
      body: serializer.fromJson<String>(json['body']),
      updated: serializer.fromJson<int>(json['updated']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'path': serializer.toJson<String>(path),
      'body': serializer.toJson<String>(body),
      'updated': serializer.toJson<int>(updated),
    };
  }

  ResponseRow copyWith({String? path, String? body, int? updated}) =>
      ResponseRow(
        path: path ?? this.path,
        body: body ?? this.body,
        updated: updated ?? this.updated,
      );
  ResponseRow copyWithCompanion(ResponseCacheCompanion data) {
    return ResponseRow(
      path: data.path.present ? data.path.value : this.path,
      body: data.body.present ? data.body.value : this.body,
      updated: data.updated.present ? data.updated.value : this.updated,
    );
  }

  @override
  String toString() {
    return (StringBuffer('ResponseRow(')
          ..write('path: $path, ')
          ..write('body: $body, ')
          ..write('updated: $updated')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(path, body, updated);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is ResponseRow &&
          other.path == this.path &&
          other.body == this.body &&
          other.updated == this.updated);
}

class ResponseCacheCompanion extends UpdateCompanion<ResponseRow> {
  final Value<String> path;
  final Value<String> body;
  final Value<int> updated;
  final Value<int> rowid;
  const ResponseCacheCompanion({
    this.path = const Value.absent(),
    this.body = const Value.absent(),
    this.updated = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ResponseCacheCompanion.insert({
    required String path,
    required String body,
    required int updated,
    this.rowid = const Value.absent(),
  }) : path = Value(path),
       body = Value(body),
       updated = Value(updated);
  static Insertable<ResponseRow> custom({
    Expression<String>? path,
    Expression<String>? body,
    Expression<int>? updated,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (path != null) 'path': path,
      if (body != null) 'body': body,
      if (updated != null) 'updated': updated,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ResponseCacheCompanion copyWith({
    Value<String>? path,
    Value<String>? body,
    Value<int>? updated,
    Value<int>? rowid,
  }) {
    return ResponseCacheCompanion(
      path: path ?? this.path,
      body: body ?? this.body,
      updated: updated ?? this.updated,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (path.present) {
      map['path'] = Variable<String>(path.value);
    }
    if (body.present) {
      map['body'] = Variable<String>(body.value);
    }
    if (updated.present) {
      map['updated'] = Variable<int>(updated.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ResponseCacheCompanion(')
          ..write('path: $path, ')
          ..write('body: $body, ')
          ..write('updated: $updated, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

abstract class _$ClientDatabase extends GeneratedDatabase {
  _$ClientDatabase(QueryExecutor e) : super(e);
  $ClientDatabaseManager get managers => $ClientDatabaseManager(this);
  late final $ClientMetadataTable clientMetadata = $ClientMetadataTable(this);
  late final $ConfirmedPeersTable confirmedPeers = $ConfirmedPeersTable(this);
  late final $ReceivedHistoryTable receivedHistory = $ReceivedHistoryTable(
    this,
  );
  late final $HistoryProjectionTable historyProjection =
      $HistoryProjectionTable(this);
  late final $ResponseCacheTable responseCache = $ResponseCacheTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    clientMetadata,
    confirmedPeers,
    receivedHistory,
    historyProjection,
    responseCache,
  ];
}

typedef $$ClientMetadataTableCreateCompanionBuilder =
    ClientMetadataCompanion Function({
      required String key,
      required String body,
      Value<int> rowid,
    });
typedef $$ClientMetadataTableUpdateCompanionBuilder =
    ClientMetadataCompanion Function({
      Value<String> key,
      Value<String> body,
      Value<int> rowid,
    });

class $$ClientMetadataTableFilterComposer
    extends Composer<_$ClientDatabase, $ClientMetadataTable> {
  $$ClientMetadataTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get key => $composableBuilder(
    column: $table.key,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnFilters(column),
  );
}

class $$ClientMetadataTableOrderingComposer
    extends Composer<_$ClientDatabase, $ClientMetadataTable> {
  $$ClientMetadataTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get key => $composableBuilder(
    column: $table.key,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$ClientMetadataTableAnnotationComposer
    extends Composer<_$ClientDatabase, $ClientMetadataTable> {
  $$ClientMetadataTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get key =>
      $composableBuilder(column: $table.key, builder: (column) => column);

  GeneratedColumn<String> get body =>
      $composableBuilder(column: $table.body, builder: (column) => column);
}

class $$ClientMetadataTableTableManager
    extends
        RootTableManager<
          _$ClientDatabase,
          $ClientMetadataTable,
          ClientMetadataData,
          $$ClientMetadataTableFilterComposer,
          $$ClientMetadataTableOrderingComposer,
          $$ClientMetadataTableAnnotationComposer,
          $$ClientMetadataTableCreateCompanionBuilder,
          $$ClientMetadataTableUpdateCompanionBuilder,
          (
            ClientMetadataData,
            BaseReferences<
              _$ClientDatabase,
              $ClientMetadataTable,
              ClientMetadataData
            >,
          ),
          ClientMetadataData,
          PrefetchHooks Function()
        > {
  $$ClientMetadataTableTableManager(
    _$ClientDatabase db,
    $ClientMetadataTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ClientMetadataTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ClientMetadataTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ClientMetadataTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback: ({
            Value<String> key = const Value.absent(),
            Value<String> body = const Value.absent(),
            Value<int> rowid = const Value.absent(),
          }) => ClientMetadataCompanion(key: key, body: body, rowid: rowid),
          createCompanionCallback:
              ({
                required String key,
                required String body,
                Value<int> rowid = const Value.absent(),
              }) => ClientMetadataCompanion.insert(
                key: key,
                body: body,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable<$ClientMetadataTable, ClientMetadataData>(table),
                  BaseReferences<
                    _$ClientDatabase,
                    $ClientMetadataTable,
                    ClientMetadataData
                  >(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$ClientMetadataTableProcessedTableManager =
    ProcessedTableManager<
      _$ClientDatabase,
      $ClientMetadataTable,
      ClientMetadataData,
      $$ClientMetadataTableFilterComposer,
      $$ClientMetadataTableOrderingComposer,
      $$ClientMetadataTableAnnotationComposer,
      $$ClientMetadataTableCreateCompanionBuilder,
      $$ClientMetadataTableUpdateCompanionBuilder,
      (
        ClientMetadataData,
        BaseReferences<
          _$ClientDatabase,
          $ClientMetadataTable,
          ClientMetadataData
        >,
      ),
      ClientMetadataData,
      PrefetchHooks Function()
    >;
typedef $$ConfirmedPeersTableCreateCompanionBuilder =
    ConfirmedPeersCompanion Function({
      required String root,
      required String signingKey,
      required String encryptionKey,
      Value<bool> revoked,
      Value<int> rowid,
    });
typedef $$ConfirmedPeersTableUpdateCompanionBuilder =
    ConfirmedPeersCompanion Function({
      Value<String> root,
      Value<String> signingKey,
      Value<String> encryptionKey,
      Value<bool> revoked,
      Value<int> rowid,
    });

class $$ConfirmedPeersTableFilterComposer
    extends Composer<_$ClientDatabase, $ConfirmedPeersTable> {
  $$ConfirmedPeersTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get root => $composableBuilder(
    column: $table.root,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get signingKey => $composableBuilder(
    column: $table.signingKey,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get encryptionKey => $composableBuilder(
    column: $table.encryptionKey,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<bool> get revoked => $composableBuilder(
    column: $table.revoked,
    builder: (column) => ColumnFilters(column),
  );
}

class $$ConfirmedPeersTableOrderingComposer
    extends Composer<_$ClientDatabase, $ConfirmedPeersTable> {
  $$ConfirmedPeersTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get root => $composableBuilder(
    column: $table.root,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get signingKey => $composableBuilder(
    column: $table.signingKey,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get encryptionKey => $composableBuilder(
    column: $table.encryptionKey,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<bool> get revoked => $composableBuilder(
    column: $table.revoked,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$ConfirmedPeersTableAnnotationComposer
    extends Composer<_$ClientDatabase, $ConfirmedPeersTable> {
  $$ConfirmedPeersTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get root =>
      $composableBuilder(column: $table.root, builder: (column) => column);

  GeneratedColumn<String> get signingKey => $composableBuilder(
    column: $table.signingKey,
    builder: (column) => column,
  );

  GeneratedColumn<String> get encryptionKey => $composableBuilder(
    column: $table.encryptionKey,
    builder: (column) => column,
  );

  GeneratedColumn<bool> get revoked =>
      $composableBuilder(column: $table.revoked, builder: (column) => column);
}

class $$ConfirmedPeersTableTableManager
    extends
        RootTableManager<
          _$ClientDatabase,
          $ConfirmedPeersTable,
          PeerRow,
          $$ConfirmedPeersTableFilterComposer,
          $$ConfirmedPeersTableOrderingComposer,
          $$ConfirmedPeersTableAnnotationComposer,
          $$ConfirmedPeersTableCreateCompanionBuilder,
          $$ConfirmedPeersTableUpdateCompanionBuilder,
          (
            PeerRow,
            BaseReferences<_$ClientDatabase, $ConfirmedPeersTable, PeerRow>,
          ),
          PeerRow,
          PrefetchHooks Function()
        > {
  $$ConfirmedPeersTableTableManager(
    _$ClientDatabase db,
    $ConfirmedPeersTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ConfirmedPeersTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ConfirmedPeersTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ConfirmedPeersTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> root = const Value.absent(),
                Value<String> signingKey = const Value.absent(),
                Value<String> encryptionKey = const Value.absent(),
                Value<bool> revoked = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ConfirmedPeersCompanion(
                root: root,
                signingKey: signingKey,
                encryptionKey: encryptionKey,
                revoked: revoked,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String root,
                required String signingKey,
                required String encryptionKey,
                Value<bool> revoked = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ConfirmedPeersCompanion.insert(
                root: root,
                signingKey: signingKey,
                encryptionKey: encryptionKey,
                revoked: revoked,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable<$ConfirmedPeersTable, PeerRow>(table),
                  BaseReferences<
                    _$ClientDatabase,
                    $ConfirmedPeersTable,
                    PeerRow
                  >(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$ConfirmedPeersTableProcessedTableManager =
    ProcessedTableManager<
      _$ClientDatabase,
      $ConfirmedPeersTable,
      PeerRow,
      $$ConfirmedPeersTableFilterComposer,
      $$ConfirmedPeersTableOrderingComposer,
      $$ConfirmedPeersTableAnnotationComposer,
      $$ConfirmedPeersTableCreateCompanionBuilder,
      $$ConfirmedPeersTableUpdateCompanionBuilder,
      (
        PeerRow,
        BaseReferences<_$ClientDatabase, $ConfirmedPeersTable, PeerRow>,
      ),
      PeerRow,
      PrefetchHooks Function()
    >;
typedef $$ReceivedHistoryTableCreateCompanionBuilder =
    ReceivedHistoryCompanion Function({
      required String id,
      required String hash,
      required String body,
      required String status,
      Value<int> rowid,
    });
typedef $$ReceivedHistoryTableUpdateCompanionBuilder =
    ReceivedHistoryCompanion Function({
      Value<String> id,
      Value<String> hash,
      Value<String> body,
      Value<String> status,
      Value<int> rowid,
    });

class $$ReceivedHistoryTableFilterComposer
    extends Composer<_$ClientDatabase, $ReceivedHistoryTable> {
  $$ReceivedHistoryTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get hash => $composableBuilder(
    column: $table.hash,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnFilters(column),
  );
}

class $$ReceivedHistoryTableOrderingComposer
    extends Composer<_$ClientDatabase, $ReceivedHistoryTable> {
  $$ReceivedHistoryTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get hash => $composableBuilder(
    column: $table.hash,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$ReceivedHistoryTableAnnotationComposer
    extends Composer<_$ClientDatabase, $ReceivedHistoryTable> {
  $$ReceivedHistoryTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get hash =>
      $composableBuilder(column: $table.hash, builder: (column) => column);

  GeneratedColumn<String> get body =>
      $composableBuilder(column: $table.body, builder: (column) => column);

  GeneratedColumn<String> get status =>
      $composableBuilder(column: $table.status, builder: (column) => column);
}

class $$ReceivedHistoryTableTableManager
    extends
        RootTableManager<
          _$ClientDatabase,
          $ReceivedHistoryTable,
          InboxRow,
          $$ReceivedHistoryTableFilterComposer,
          $$ReceivedHistoryTableOrderingComposer,
          $$ReceivedHistoryTableAnnotationComposer,
          $$ReceivedHistoryTableCreateCompanionBuilder,
          $$ReceivedHistoryTableUpdateCompanionBuilder,
          (
            InboxRow,
            BaseReferences<_$ClientDatabase, $ReceivedHistoryTable, InboxRow>,
          ),
          InboxRow,
          PrefetchHooks Function()
        > {
  $$ReceivedHistoryTableTableManager(
    _$ClientDatabase db,
    $ReceivedHistoryTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ReceivedHistoryTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ReceivedHistoryTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ReceivedHistoryTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> hash = const Value.absent(),
                Value<String> body = const Value.absent(),
                Value<String> status = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ReceivedHistoryCompanion(
                id: id,
                hash: hash,
                body: body,
                status: status,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String hash,
                required String body,
                required String status,
                Value<int> rowid = const Value.absent(),
              }) => ReceivedHistoryCompanion.insert(
                id: id,
                hash: hash,
                body: body,
                status: status,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable<$ReceivedHistoryTable, InboxRow>(table),
                  BaseReferences<
                    _$ClientDatabase,
                    $ReceivedHistoryTable,
                    InboxRow
                  >(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$ReceivedHistoryTableProcessedTableManager =
    ProcessedTableManager<
      _$ClientDatabase,
      $ReceivedHistoryTable,
      InboxRow,
      $$ReceivedHistoryTableFilterComposer,
      $$ReceivedHistoryTableOrderingComposer,
      $$ReceivedHistoryTableAnnotationComposer,
      $$ReceivedHistoryTableCreateCompanionBuilder,
      $$ReceivedHistoryTableUpdateCompanionBuilder,
      (
        InboxRow,
        BaseReferences<_$ClientDatabase, $ReceivedHistoryTable, InboxRow>,
      ),
      InboxRow,
      PrefetchHooks Function()
    >;
typedef $$HistoryProjectionTableCreateCompanionBuilder =
    HistoryProjectionCompanion Function({
      required String id,
      required String aggregateId,
      required String authority,
      required String source,
      required String kind,
      required int version,
      required String body,
      Value<int> rowid,
    });
typedef $$HistoryProjectionTableUpdateCompanionBuilder =
    HistoryProjectionCompanion Function({
      Value<String> id,
      Value<String> aggregateId,
      Value<String> authority,
      Value<String> source,
      Value<String> kind,
      Value<int> version,
      Value<String> body,
      Value<int> rowid,
    });

class $$HistoryProjectionTableFilterComposer
    extends Composer<_$ClientDatabase, $HistoryProjectionTable> {
  $$HistoryProjectionTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get aggregateId => $composableBuilder(
    column: $table.aggregateId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get authority => $composableBuilder(
    column: $table.authority,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get source => $composableBuilder(
    column: $table.source,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get kind => $composableBuilder(
    column: $table.kind,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get version => $composableBuilder(
    column: $table.version,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnFilters(column),
  );
}

class $$HistoryProjectionTableOrderingComposer
    extends Composer<_$ClientDatabase, $HistoryProjectionTable> {
  $$HistoryProjectionTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get aggregateId => $composableBuilder(
    column: $table.aggregateId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get authority => $composableBuilder(
    column: $table.authority,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get source => $composableBuilder(
    column: $table.source,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get kind => $composableBuilder(
    column: $table.kind,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get version => $composableBuilder(
    column: $table.version,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$HistoryProjectionTableAnnotationComposer
    extends Composer<_$ClientDatabase, $HistoryProjectionTable> {
  $$HistoryProjectionTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get aggregateId => $composableBuilder(
    column: $table.aggregateId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get authority =>
      $composableBuilder(column: $table.authority, builder: (column) => column);

  GeneratedColumn<String> get source =>
      $composableBuilder(column: $table.source, builder: (column) => column);

  GeneratedColumn<String> get kind =>
      $composableBuilder(column: $table.kind, builder: (column) => column);

  GeneratedColumn<int> get version =>
      $composableBuilder(column: $table.version, builder: (column) => column);

  GeneratedColumn<String> get body =>
      $composableBuilder(column: $table.body, builder: (column) => column);
}

class $$HistoryProjectionTableTableManager
    extends
        RootTableManager<
          _$ClientDatabase,
          $HistoryProjectionTable,
          HistoryRow,
          $$HistoryProjectionTableFilterComposer,
          $$HistoryProjectionTableOrderingComposer,
          $$HistoryProjectionTableAnnotationComposer,
          $$HistoryProjectionTableCreateCompanionBuilder,
          $$HistoryProjectionTableUpdateCompanionBuilder,
          (
            HistoryRow,
            BaseReferences<
              _$ClientDatabase,
              $HistoryProjectionTable,
              HistoryRow
            >,
          ),
          HistoryRow,
          PrefetchHooks Function()
        > {
  $$HistoryProjectionTableTableManager(
    _$ClientDatabase db,
    $HistoryProjectionTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$HistoryProjectionTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$HistoryProjectionTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$HistoryProjectionTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> aggregateId = const Value.absent(),
                Value<String> authority = const Value.absent(),
                Value<String> source = const Value.absent(),
                Value<String> kind = const Value.absent(),
                Value<int> version = const Value.absent(),
                Value<String> body = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => HistoryProjectionCompanion(
                id: id,
                aggregateId: aggregateId,
                authority: authority,
                source: source,
                kind: kind,
                version: version,
                body: body,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String aggregateId,
                required String authority,
                required String source,
                required String kind,
                required int version,
                required String body,
                Value<int> rowid = const Value.absent(),
              }) => HistoryProjectionCompanion.insert(
                id: id,
                aggregateId: aggregateId,
                authority: authority,
                source: source,
                kind: kind,
                version: version,
                body: body,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable<$HistoryProjectionTable, HistoryRow>(table),
                  BaseReferences<
                    _$ClientDatabase,
                    $HistoryProjectionTable,
                    HistoryRow
                  >(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$HistoryProjectionTableProcessedTableManager =
    ProcessedTableManager<
      _$ClientDatabase,
      $HistoryProjectionTable,
      HistoryRow,
      $$HistoryProjectionTableFilterComposer,
      $$HistoryProjectionTableOrderingComposer,
      $$HistoryProjectionTableAnnotationComposer,
      $$HistoryProjectionTableCreateCompanionBuilder,
      $$HistoryProjectionTableUpdateCompanionBuilder,
      (
        HistoryRow,
        BaseReferences<_$ClientDatabase, $HistoryProjectionTable, HistoryRow>,
      ),
      HistoryRow,
      PrefetchHooks Function()
    >;
typedef $$ResponseCacheTableCreateCompanionBuilder =
    ResponseCacheCompanion Function({
      required String path,
      required String body,
      required int updated,
      Value<int> rowid,
    });
typedef $$ResponseCacheTableUpdateCompanionBuilder =
    ResponseCacheCompanion Function({
      Value<String> path,
      Value<String> body,
      Value<int> updated,
      Value<int> rowid,
    });

class $$ResponseCacheTableFilterComposer
    extends Composer<_$ClientDatabase, $ResponseCacheTable> {
  $$ResponseCacheTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get path => $composableBuilder(
    column: $table.path,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get updated => $composableBuilder(
    column: $table.updated,
    builder: (column) => ColumnFilters(column),
  );
}

class $$ResponseCacheTableOrderingComposer
    extends Composer<_$ClientDatabase, $ResponseCacheTable> {
  $$ResponseCacheTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get path => $composableBuilder(
    column: $table.path,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get body => $composableBuilder(
    column: $table.body,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get updated => $composableBuilder(
    column: $table.updated,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$ResponseCacheTableAnnotationComposer
    extends Composer<_$ClientDatabase, $ResponseCacheTable> {
  $$ResponseCacheTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get path =>
      $composableBuilder(column: $table.path, builder: (column) => column);

  GeneratedColumn<String> get body =>
      $composableBuilder(column: $table.body, builder: (column) => column);

  GeneratedColumn<int> get updated =>
      $composableBuilder(column: $table.updated, builder: (column) => column);
}

class $$ResponseCacheTableTableManager
    extends
        RootTableManager<
          _$ClientDatabase,
          $ResponseCacheTable,
          ResponseRow,
          $$ResponseCacheTableFilterComposer,
          $$ResponseCacheTableOrderingComposer,
          $$ResponseCacheTableAnnotationComposer,
          $$ResponseCacheTableCreateCompanionBuilder,
          $$ResponseCacheTableUpdateCompanionBuilder,
          (
            ResponseRow,
            BaseReferences<_$ClientDatabase, $ResponseCacheTable, ResponseRow>,
          ),
          ResponseRow,
          PrefetchHooks Function()
        > {
  $$ResponseCacheTableTableManager(
    _$ClientDatabase db,
    $ResponseCacheTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ResponseCacheTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ResponseCacheTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ResponseCacheTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> path = const Value.absent(),
                Value<String> body = const Value.absent(),
                Value<int> updated = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ResponseCacheCompanion(
                path: path,
                body: body,
                updated: updated,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String path,
                required String body,
                required int updated,
                Value<int> rowid = const Value.absent(),
              }) => ResponseCacheCompanion.insert(
                path: path,
                body: body,
                updated: updated,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable<$ResponseCacheTable, ResponseRow>(table),
                  BaseReferences<
                    _$ClientDatabase,
                    $ResponseCacheTable,
                    ResponseRow
                  >(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$ResponseCacheTableProcessedTableManager =
    ProcessedTableManager<
      _$ClientDatabase,
      $ResponseCacheTable,
      ResponseRow,
      $$ResponseCacheTableFilterComposer,
      $$ResponseCacheTableOrderingComposer,
      $$ResponseCacheTableAnnotationComposer,
      $$ResponseCacheTableCreateCompanionBuilder,
      $$ResponseCacheTableUpdateCompanionBuilder,
      (
        ResponseRow,
        BaseReferences<_$ClientDatabase, $ResponseCacheTable, ResponseRow>,
      ),
      ResponseRow,
      PrefetchHooks Function()
    >;

class $ClientDatabaseManager {
  final _$ClientDatabase _db;
  $ClientDatabaseManager(this._db);
  $$ClientMetadataTableTableManager get clientMetadata =>
      $$ClientMetadataTableTableManager(_db, _db.clientMetadata);
  $$ConfirmedPeersTableTableManager get confirmedPeers =>
      $$ConfirmedPeersTableTableManager(_db, _db.confirmedPeers);
  $$ReceivedHistoryTableTableManager get receivedHistory =>
      $$ReceivedHistoryTableTableManager(_db, _db.receivedHistory);
  $$HistoryProjectionTableTableManager get historyProjection =>
      $$HistoryProjectionTableTableManager(_db, _db.historyProjection);
  $$ResponseCacheTableTableManager get responseCache =>
      $$ResponseCacheTableTableManager(_db, _db.responseCache);
}
