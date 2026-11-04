import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../data/identity.dart';
import 'protocol.dart';

/// One authenticated, reliable data channel. No STUN, TURN, centre signalling,
/// media capture, or automatic relay exists on this path.
class LocalSession {
  LocalSession._(
    this.identity,
    this.name,
    this.incoming,
    this.id,
    this.pc,
    this.opened,
    this.control,
    this.data,
    this.changed,
    this.onClosed,
  ) {
    pc.onDataChannel = (channel) {
      if (!incoming || _channel != null) {
        unawaited(channel.close());
        unawaited(close());
      } else {
        _attach(channel);
      }
    };
    pc.onConnectionState = (state) {
      _disconnected?.cancel();
      if ([
        RTCPeerConnectionState.RTCPeerConnectionStateClosed,
        RTCPeerConnectionState.RTCPeerConnectionStateFailed,
      ].contains(state)) {
        unawaited(close());
      }
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateDisconnected) {
        _disconnected = Timer(
          const Duration(seconds: 3),
          () => unawaited(close()),
        );
      }
    };
    _expiry = Timer(const Duration(minutes: 2), () => unawaited(close()));
  }
  static Future<LocalSession> create(
    LocalIdentity identity,
    String name,
    bool incoming, {
    String? id,
    required Future<void> Function(LocalSession) opened,
    required Future<void> Function(Json, LocalSession) control,
    required Future<void> Function(Uint8List, LocalSession) data,
    required void Function() changed,
    required Future<void> Function(LocalSession) closed,
  }) async {
    final pc = await createPeerConnection({
      'iceServers': <dynamic>[],
      'sdpSemantics': 'unified-plan',
    });
    return LocalSession._(
      identity,
      name,
      incoming,
      id ?? randomUuid(),
      pc,
      opened,
      control,
      data,
      changed,
      closed,
    );
  }

  final LocalIdentity identity;
  final String name, id;
  final bool incoming;
  final RTCPeerConnection pc;
  final Future<void> Function(LocalSession) opened, onClosed;
  final Future<void> Function(Json, LocalSession) control;
  final Future<void> Function(Uint8List, LocalSession) data;
  final void Function() changed;
  RTCDataChannel? _channel;
  Json? _offer, _answer, peer;
  String code = '', error = 'LOCAL_CONNECTION_LOST', _transcript = '';
  bool selfConfirmed = false,
      peerConfirmed = false,
      _confirmed = false,
      closed = false;
  int _sequence = 0, _received = 0, _queuedBytes = 0;
  Future<void> _queue = Future.value(), _sends = Future.value();
  Future<void>? _opening, _closing;
  Timer? _expiry, _heartbeat, _disconnected;
  DateTime _lastMessage = DateTime.now();
  void check() {
    if (closed) bad('LOCAL_CONNECTION_LOST');
  }

  bool get ready =>
      !closed &&
      _confirmed &&
      _channel?.state == RTCDataChannelState.RTCDataChannelOpen;
  Future<void> _wait(bool Function() condition, {int seconds = 12}) async {
    final deadline = DateTime.now().add(Duration(seconds: seconds));
    while (!condition()) {
      check();
      if (DateTime.now().isAfter(deadline)) bad('LOCAL_CONNECTION_LOST');
      await Future<void>.delayed(const Duration(milliseconds: 25));
    }
    check();
  }

  void _attach(RTCDataChannel channel) {
    if (channel.label != localChannel) {
      unawaited(channel.close());
      unawaited(close());
      return;
    }
    _channel = channel;
    channel.bufferedAmountLowThreshold = 128 << 10;
    channel.onDataChannelState = (state) {
      if (state == RTCDataChannelState.RTCDataChannelOpen) {
        _opening ??= _opened().catchError(_failed);
      }
      if (state == RTCDataChannelState.RTCDataChannelClosed) unawaited(close());
    };
    channel.onMessage = (message) {
      if (closed) return;
      final length = message.isBinary
          ? message.binary.length
          : message.text.length * 2;
      _queuedBytes += length;
      if (_queuedBytes > (512 << 10) ||
          length > (message.isBinary ? 16384 : 64000)) {
        _failed(StateError('LOCAL_MESSAGE_INVALID'));
        return;
      }
      _lastMessage = DateTime.now();
      _queue = _queue
          .then((_) async {
            check();
            await _opening;
            check();
            if (message.isBinary) {
              if (!_confirmed || message.binary.length > 16384) {
                bad('LOCAL_PAIR_REQUIRED');
              }
              await data(message.binary, this);
              return;
            }
            if (message.text.length > 32000) bad();
            final m = jsonDecode(message.text) as Json;
            if (peer == null ||
                m['session'] != _transcript ||
                m['sequence'] != _received + 1 ||
                m['payload'] is! Json ||
                !await verified(peer!['signing_key'], 'control', m)) {
              bad();
            }
            _received++;
            final p = m['payload'] as Json;
            if (p['type'] == 'confirm') {
              peerConfirmed = true;
              await _finish();
              return;
            }
            if (!_confirmed) bad('LOCAL_PAIR_REQUIRED');
            if (p['type'] == 'ping') {
              await send({'type': 'pong'});
              return;
            }
            if (p['type'] != 'pong') await control(p, this);
          })
          .whenComplete(() {
            _queuedBytes -= length;
          })
          .catchError(_failed);
    };
    if (channel.state == RTCDataChannelState.RTCDataChannelOpen) {
      _opening ??= _opened().catchError(_failed);
    }
  }

  void _failed(Object e) {
    error = e is StateError ? e.message.toString() : 'LOCAL_CONNECTION_LOST';
    unawaited(close());
  }

  Future<String> _description(String type) async {
    const constraints = <String, dynamic>{
      'mandatory': <String, dynamic>{
        'OfferToReceiveAudio': false,
        'OfferToReceiveVideo': false,
      },
      'optional': <dynamic>[],
    };
    final description = type == 'offer'
        ? await pc.createOffer(constraints)
        : await pc.createAnswer(constraints);
    await pc.setLocalDescription(description);
    await _wait(
      () =>
          pc.iceGatheringState ==
          RTCIceGatheringState.RTCIceGatheringStateComplete,
    );
    final sdp = (await pc.getLocalDescription())?.sdp;
    if (sdp == null) bad();
    return sdp;
  }

  Future<Json> _package(String type, String sdp, [String reply = '']) =>
      signed(identity, 'connection', {
        'v': 1,
        'type': type,
        'id': id,
        'nonce': base64UrlEncode(randomBytes(32)).replaceAll('=', ''),
        'peer': publicIdentity(identity, name),
        'sdp': sdp,
        'reply': reply,
      });
  Future<Json> offer() async {
    _attach(
      await pc.createDataChannel(
        localChannel,
        RTCDataChannelInit()
          ..ordered = true
          ..protocol = localChannel,
      ),
    );
    return _offer = await _package('offer', await _description('offer'));
  }

  Future<Json> answer(Json offer) async {
    _offer = await validatePackage(offer);
    peer = offer['peer'];
    await pc.setRemoteDescription(RTCSessionDescription(offer['sdp'], 'offer'));
    return _answer = await _package(
      'answer',
      await _description('answer'),
      await packageHash(offer),
    );
  }

  Future<void> setAnswer(Json answer) async {
    await validatePackage(answer);
    if (_offer == null ||
        _answer != null ||
        answer['type'] != 'answer' ||
        answer['id'] != id ||
        answer['reply'] != await packageHash(_offer!)) {
      bad('LOCAL_CODE_EXPIRED');
    }
    peer = answer['peer'];
    _answer = answer;
    await pc.setRemoteDescription(
      RTCSessionDescription(answer['sdp'], 'answer'),
    );
  }

  Future<void> _opened() async {
    await _wait(() => _offer != null && _answer != null);
    final until = DateTime.now().add(const Duration(seconds: 12));
    while (await proof() == null) {
      check();
      if (DateTime.now().isAfter(until)) bad('LOCAL_NOT_DIRECT');
      await Future<void>.delayed(const Duration(milliseconds: 50));
    }
    _transcript = await packageHash(_answer!);
    code = await comparison(_offer!, _answer!);
    check();
    changed();
    await opened(this);
  }

  Future<void> confirm() async {
    check();
    if (code.isEmpty || peer == null) bad('LOCAL_PAIR_REQUIRED');
    if (!selfConfirmed) {
      await send({'type': 'confirm'}, unconfirmed: true);
      selfConfirmed = true;
    }
    await _finish();
  }

  Future<void> _finish() async {
    check();
    if (selfConfirmed && peerConfirmed && !_confirmed) {
      _confirmed = true;
      _expiry?.cancel();
      _lastMessage = DateTime.now();
      _heartbeat = Timer.periodic(const Duration(seconds: 2), (_) {
        if (DateTime.now().difference(_lastMessage).inSeconds >= 12) {
          unawaited(close());
          return;
        }
        unawaited(send({'type': 'ping'}).catchError(_failed));
      });
      await control({'type': 'paired'}, this);
    }
    changed();
  }

  Future<void> send(Json payload, {bool unconfirmed = false}) {
    final task = _sends.then((_) async {
      check();
      if ((!unconfirmed && !ready) || _transcript.isEmpty) {
        bad('LOCAL_CONNECTION_LOST');
      }
      final m = await signed(identity, 'control', {
        'session': _transcript,
        'sequence': _sequence + 1,
        'payload': payload,
      });
      final text = jsonEncode(m);
      if (text.length > 32000) bad();
      check();
      await _channel!.send(RTCDataChannelMessage(text));
      _sequence++;
    });
    _sends = task.catchError((Object _) {});
    return task;
  }

  Future<void> sendData(Uint8List bytes) async {
    if (bytes.length > 16384) bad();
    await _wait(() => (_channel?.bufferedAmount ?? 0) <= (256 << 10));
    if (!ready) bad('LOCAL_CONNECTION_LOST');
    await _channel!.send(RTCDataChannelMessage.fromBinary(bytes));
  }

  Future<Json?> proof() async {
    final rows = await pc.getStats();
    final byId = {for (final r in rows) r.id: r};
    dynamic pair;
    for (final r in rows) {
      if (r.type == 'transport') {
        pair = byId[r.values['selectedCandidatePairId']] ?? pair;
      }
    }
    for (final r in rows) {
      if (pair == null &&
          r.type == 'candidate-pair' &&
          r.values['nominated'] == true &&
          r.values['state'] == 'succeeded') {
        pair = r;
      }
    }
    final local = byId[pair?.values['localCandidateId']]?.values,
        remote = byId[pair?.values['remoteCandidateId']]?.values;
    if (local != null &&
        remote != null &&
        ['host', 'prflx'].contains(local['candidateType']) &&
        ['host', 'prflx'].contains(remote['candidateType'])) {
      return {
        'local': local['candidateType'],
        'remote': remote['candidateType'],
        'protocol': local['protocol'],
        'sent': pair.values['bytesSent'] ?? 0,
        'received': pair.values['bytesReceived'] ?? 0,
      };
    }
    return null;
  }

  Future<void> close() {
    if (_closing != null) return _closing!;
    closed = true;
    _expiry?.cancel();
    _heartbeat?.cancel();
    _disconnected?.cancel();
    pc.onDataChannel = null;
    pc.onConnectionState = null;
    final channel = _channel;
    if (channel != null) {
      channel.onMessage = null;
      channel.onDataChannelState = null;
    }
    return _closing = (() async {
      await channel?.close();
      await pc.close();
      await pc.dispose();
      await onClosed(this);
    })();
  }

  Future<void> drain() async {
    await Future.wait([
      _queue,
      _sends,
      if (_opening != null) _opening!,
      if (_closing != null) _closing!,
    ]);
  }
}
