enum ConnectionKind { local, online }

typedef ConnectionInput = ({ConnectionKind kind, String value});

/// Never redeem a link from a different service, even if it contains a token.
ConnectionInput parseConnectionInput(String raw, String server) {
  final value = raw.trim();
  if (value.isEmpty || value.length > 18000) {
    throw StateError('GUEST_CODE_INVALID');
  }
  if (value.startsWith('PL2:') || value.startsWith('PL1.')) {
    return (kind: ConnectionKind.local, value: value);
  }
  final token = RegExp(r'^[A-Za-z0-9_-]{22}$');
  if (token.hasMatch(value)) return (kind: ConnectionKind.online, value: value);
  final short = value.toUpperCase().replaceAll(RegExp(r'[ -]'), '');
  if (RegExp(r'^[A-Z0-9]{8}$').hasMatch(short)) {
    return (kind: ConnectionKind.online, value: short);
  }
  try {
    final link = Uri.parse(value), service = Uri.parse(server);
    if (['https', 'http'].contains(link.scheme) &&
        link.hasAuthority &&
        link.userInfo.isEmpty &&
        link.origin == service.origin &&
        link.path == '/connect' &&
        !link.hasQuery &&
        token.hasMatch(link.fragment)) {
      return (kind: ConnectionKind.online, value: link.fragment);
    }
  } on FormatException {
    // Report all malformed or foreign links through the same input error.
  }
  throw StateError('GUEST_CODE_INVALID');
}
