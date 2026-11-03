import '../motion/motion.dart';

import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart';

class NotificationsPage extends StatefulWidget {
  const NotificationsPage({super.key, required this.api});
  final PinelinkApi api;

  @override
  State<NotificationsPage> createState() => _NotificationsPageState();
}

class _NotificationsPageState extends State<NotificationsPage> {
  late Future<List<TransferRequest>> _pending = widget.api.getPending(
    widget.api.currentOwner,
    widget.api.currentDevice,
  );
  String? _selected;
  String? _notice;
  bool _responding = false;
  @override
  void initState() {
    super.initState();
    widget.api.addListener(_synced);
  }

  @override
  void didUpdateWidget(covariant NotificationsPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      oldWidget.api.removeListener(_synced);
      widget.api.addListener(_synced);
      _responding = false;
      _notice = null;
      _pending = widget.api.getPending(
        widget.api.currentOwner,
        widget.api.currentDevice,
      );
    }
  }

  void _synced() {
    if (mounted && !_responding) {
      setState(() {
        _pending = widget.api.getPending(
          widget.api.currentOwner,
          widget.api.currentDevice,
        );
      });
    }
  }

  @override
  void dispose() {
    widget.api.removeListener(_synced);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<List<TransferRequest>>(
    future: _pending,
    builder: (context, snapshot) {
      if (snapshot.connectionState == ConnectionState.waiting &&
          !snapshot.hasData) {
        return Center(child: Text(tr(context, 'loadingNotifications')));
      }
      if (snapshot.hasError) {
        return Center(child: Text(tr(context, 'actionFailed')));
      }
      final pending = snapshot.data ?? const <TransferRequest>[];
      final first =
          pending.where((request) => request.id == _selected).firstOrNull ??
          pending.firstOrNull;
      return ListView(
        padding: const EdgeInsets.all(22),
        children: [
          Text(
            tr(context, 'notificationTitle'),
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            tr(context, 'notificationSubtitle'),
            style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
          ),
          const SizedBox(height: 28),
          if (_notice != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Text(
                _notice!,
                style: const TextStyle(
                  fontSize: 13,
                  color: PinelinkColors.accent,
                ),
              ),
            ),
          if (pending.length > 1) ...[
            DropdownButtonFormField<String>(
              initialValue: first?.id,
              decoration: InputDecoration(
                labelText: tr(context, 'selectRequest'),
                border: const OutlineInputBorder(),
              ),
              items: [
                for (final request in pending)
                  DropdownMenuItem(
                    value: request.id,
                    child: Text(
                      displayFileName(request.fileName, i18nLocale(context)) ??
                          tr(context, 'unknownFile'),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
              ],
              onChanged: (value) => setState(() => _selected = value),
            ),
            const SizedBox(height: 16),
          ],
          if (first == null)
            const EmptyNotifications()
          else ...[
            Container(
              padding: const EdgeInsets.all(22),
              decoration: BoxDecoration(
                color: PinelinkColors.background,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: PinelinkColors.line),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      FileIcon(fileName: first.fileName),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              displayFileName(
                                    first.fileName,
                                    i18nLocale(context),
                                  ) ??
                                  tr(context, 'unknownFile'),
                              style: const TextStyle(
                                fontSize: 16,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            const SizedBox(height: 8),
                            Text(
                              fileMetadataLabel(
                                first.fileName,
                                first.sizeBytes,
                                i18nLocale(context),
                              ),
                              style: const TextStyle(
                                fontSize: 12,
                                color: PinelinkColors.muted,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 28),
                  MetadataRow(
                    label: tr(context, 'senderDevice'),
                    value: first.senderDevice,
                  ),
                  const SizedBox(height: 12),
                  MetadataRow(
                    label: tr(context, 'senderAccount'),
                    value: first.senderOwner,
                  ),
                  const SizedBox(height: 28),
                  Text(
                    tr(context, 'consentNote'),
                    style: const TextStyle(
                      fontSize: 12,
                      color: PinelinkColors.muted,
                      height: 1.7,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 20),
            Row(
              children: [
                Expanded(
                  child: MotionButton(
                    onPressed: _responding
                        ? null
                        : () => _respond(first, 'reject'),
                    child: Text(tr(context, 'reject')),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: MotionButton.primary(
                    onPressed: _responding
                        ? null
                        : () => _respond(first, 'accept'),
                    child: Text(
                      tr(context, _responding ? 'confirming' : 'accept'),
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 20),
            const StorageNote(),
          ],
        ],
      );
    },
  );

  Future<void> _respond(TransferRequest request, String action) async {
    final api = widget.api;
    setState(() {
      _responding = true;
      _notice = null;
    });
    try {
      await api.respond(
        request.id,
        action,
        widget.api.currentOwner,
        widget.api.currentDevice,
        request.version,
      );
      if (!mounted || widget.api != api) return;
      setState(
        () =>
            _notice = tr(context, action == 'accept' ? 'accepted' : 'rejected'),
      );
    } on ApiException catch (error) {
      if (!mounted || widget.api != api) return;
      setState(
        () => _notice = tr(
          context,
          error.status == 410
              ? 'requestExpired'
              : error.status == 409
              ? 'notificationStateChanged'
              : 'actionFailed',
        ),
      );
    } catch (_) {
      if (!mounted || widget.api != api) return;
      setState(() => _notice = tr(context, 'connectionFailed'));
    } finally {
      if (mounted && widget.api == api) {
        setState(() {
          _responding = false;
          _pending = widget.api.getPending(
            widget.api.currentOwner,
            widget.api.currentDevice,
          );
        });
      }
    }
  }
}

class MetadataRow extends StatelessWidget {
  const MetadataRow({super.key, required this.label, required this.value});
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) => Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      SizedBox(
        width: 96,
        child: Text(
          label,
          style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
        ),
      ),
      Expanded(child: Text(value, style: const TextStyle(fontSize: 12))),
    ],
  );
}

class EmptyNotifications extends StatelessWidget {
  const EmptyNotifications({super.key});
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(24),
    decoration: BoxDecoration(
      color: PinelinkColors.background,
      borderRadius: BorderRadius.circular(12),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          tr(context, 'emptyNotifications'),
          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          tr(context, 'emptyNotificationsHint'),
          style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
        ),
      ],
    ),
  );
}

class StorageNote extends StatelessWidget {
  const StorageNote({super.key});
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: PinelinkColors.background,
      borderRadius: BorderRadius.circular(12),
    ),
    child: Text(
      '${tr(context, 'relayNote')}\n${tr(context, 'persistentConsent')}',
      style: const TextStyle(
        fontSize: 12,
        color: PinelinkColors.muted,
        height: 1.7,
      ),
    ),
  );
}
