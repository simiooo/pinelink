import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/guest/connection_input.dart';

void main() {
  const server = 'https://staging.example.test/api';
  const token = 'abcdefghijklmnopqrstuv';
  test('recognizes online codes, same-service links and local packages', () {
    expect(parseConnectionInput(' 5v9a vdfj ', server), (
      kind: ConnectionKind.online,
      value: '5V9AVDFJ',
    ));
    expect(parseConnectionInput(token, server), (
      kind: ConnectionKind.online,
      value: token,
    ));
    expect(
      parseConnectionInput(
        'https://staging.example.test/connect#$token',
        server,
      ),
      (kind: ConnectionKind.online, value: token),
    );
    expect(
      parseConnectionInput('PL2:packet', server).kind,
      ConnectionKind.local,
    );
    expect(
      parseConnectionInput('PL1.packet', server).kind,
      ConnectionKind.local,
    );
  });
  test('rejects foreign, credentialed, malformed and oversized links', () {
    for (final value in [
      'https://other.example.test/connect#$token',
      'https://staging.example.test.evil.test/connect#$token',
      'https://user@staging.example.test/connect#$token',
      'https://staging.example.test/connect?next=1#$token',
      'https://staging.example.test/other#$token',
      'ftp://staging.example.test/connect#$token',
      'x' * 18001,
      '',
    ]) {
      expect(() => parseConnectionInput(value, server), throwsStateError);
    }
  });
}
