import 'package:url_launcher/url_launcher.dart';

class AuthBrowser {
  Future<bool> open(Uri uri) =>
      launchUrl(uri, mode: LaunchMode.externalApplication);
  void close() {}
}
