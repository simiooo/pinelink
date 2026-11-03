import 'dart:js_interop';

@JS('window.open')
external JSObject? _open(String address, String name);

extension type _AuthWindow(JSObject object) implements JSObject {
  external set location(String address);
  external void close();
}

class AuthBrowser {
  // Reserve the window during the button gesture, before asynchronous work.
  AuthBrowser() : _window = _open('about:blank', 'PinelinkSignIn');
  final JSObject? _window;
  Future<bool> open(Uri uri) async {
    if (_window == null) return false;
    _AuthWindow(_window).location = uri.toString();
    return true;
  }

  void close() {
    if (_window != null) {
      try {
        _AuthWindow(_window).close();
      } catch (_) {}
    }
  }
}
