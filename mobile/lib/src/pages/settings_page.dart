import '../motion/motion.dart';

import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart';

class SettingsData {
  const SettingsData(this.devices, this.quota, this.owned, this.file);
  final List<Device> devices;
  final Quota quota;
  final List<TransferRequest> owned;
  final FileSnapshot? file;
}

class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key, required this.api});
  final PinelinkApi api;
  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  late Future<SettingsData> _data = _load();
  String? _entry;
  String? _device;
  String? _notice;
  String? _noticeZone;
  String? _savedFile;
  bool _confirm = false;
  bool _responding = false;
  @override
  void initState() {
    super.initState();
    widget.api.addListener(_synced);
  }

  @override
  void didUpdateWidget(covariant SettingsPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      oldWidget.api.removeListener(_synced);
      widget.api.addListener(_synced);
      _responding = false;
      _notice = null;
      _confirm = false;
      _savedFile = null;
      _data = _load();
    }
  }

  void _synced() {
    if (mounted && !_responding) _refresh();
  }

  @override
  void dispose() {
    widget.api.removeListener(_synced);
    super.dispose();
  }

  Future<SettingsData> _load() async {
    final api = widget.api;
    final devices = await api.getDevices(PinelinkApi.owner);
    final quota = await api.getQuota(PinelinkApi.scope);
    final transfers = await api.getTransfers(
      PinelinkApi.owner,
      PinelinkApi.device,
    );
    final owned = <String, TransferRequest>{};
    for (final item in transfers) {
      if (item.senderOwner == PinelinkApi.owner && item.entryId.isNotEmpty) {
        owned[item.entryId] = item;
      }
    }
    FileSnapshot? file;
    final entry = _entry ?? owned.keys.firstOrNull;
    if (entry != null) {
      try {
        file = await api.getFile(entry, PinelinkApi.owner);
      } catch (_) {}
    }
    return SettingsData(devices, quota, owned.values.toList(), file);
  }

  void _refresh() => setState(() {
    _data = _load();
  });

  @override
  Widget build(BuildContext context) => FutureBuilder<SettingsData>(
    future: _data,
    builder: (context, snapshot) {
      if (snapshot.connectionState == ConnectionState.waiting &&
          !snapshot.hasData) {
        return Center(child: Text(tr(context, 'loadingDevices')));
      }
      if (snapshot.hasError) {
        return Center(child: Text(tr(context, 'actionFailed')));
      }
      final data = snapshot.data!;
      final locale = i18nLocale(context);
      final current = data.devices
          .where((item) => item.id == widget.api.currentDevice)
          .firstOrNull;
      final selectedDevice =
          _device ??
          current?.id ??
          data.devices.where((item) => item.status == 'ACTIVE').firstOrNull?.id;
      final selectedEntry = _entry ?? data.owned.firstOrNull?.entryId;
      final file = data.file;
      final percent = data.quota.capacity > 0
          ? (data.quota.used * 100 / data.quota.capacity).round().clamp(0, 100)
          : 0;
      return ListView(
        padding: const EdgeInsets.all(22),
        children: [
          Text(
            tr(context, 'settingsTitle'),
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            tr(context, 'settingsSubtitle'),
            style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
          ),
          SettingsHeading(title: tr(context, 'thisDevice')),
          SettingsCard(
            children: [
              Text(
                widget.api.currentDevice,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              Text(
                tr(
                  context,
                  current == null
                      ? 'currentDeviceUnknown'
                      : current.status == 'ACTIVE'
                      ? 'currentDeviceRegistered'
                      : 'currentDeviceRevoked',
                ),
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                ),
              ),
              Text(
                tr(context, 'account', {'owner': PinelinkApi.owner}),
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                ),
              ),
            ],
          ),
          SettingsHeading(title: tr(context, 'storageSpace')),
          SettingsCard(
            children: [
              Text(
                '${formatBytes(data.quota.used, locale)} / ${data.quota.capacity > 0 ? formatBytes(data.quota.capacity, locale) : tr(context, 'quotaUnknown')}',
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              MotionMeter(
                label: tr(context, 'quotaLabel'),
                value: percent / 100,
              ),
              if (percent >= 80)
                Text(
                  tr(context, 'quotaWarning'),
                  style: const TextStyle(
                    fontSize: 12,
                    color: Color(0xFF85463F),
                  ),
                ),
              Text(
                tr(context, 'storageNote'),
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.muted,
                  height: 1.7,
                ),
              ),
            ],
          ),
          SettingsHeading(title: tr(context, 'fileStorage')),
          if (data.owned.isEmpty)
            SettingsCard(
              children: [
                Text(
                  tr(context, 'noOwnedFiles'),
                  style: const TextStyle(
                    fontSize: 12,
                    color: PinelinkColors.muted,
                  ),
                ),
              ],
            )
          else ...[
            DropdownButtonFormField<String>(
              initialValue: selectedEntry,
              decoration: InputDecoration(
                labelText: tr(context, 'chooseFileLabel'),
                border: const OutlineInputBorder(),
              ),
              items: [
                for (final item in data.owned)
                  DropdownMenuItem(
                    value: item.entryId,
                    child: Text(
                      displayFileName(item.fileName, locale) ??
                          tr(context, 'readingFile'),
                    ),
                  ),
              ],
              onChanged: (value) => setState(() {
                _entry = value;
                _savedFile = null;
                _notice = null;
                _data = _load();
              }),
            ),
            const SizedBox(height: 16),
            SettingsCard(
              children: [
                Text(
                  displayFileName(file?.fileName, locale) ??
                      tr(context, 'fileStorageError'),
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                Text(
                  file == null
                      ? tr(context, 'fileStorageError')
                      : file.retentionClass == 'PERSISTENT'
                      ? tr(context, 'persistedFile')
                      : file.expiresAt != null
                      ? tr(context, 'temporaryUntil', {
                          'time': formatTime(file.expiresAt, locale),
                        })
                      : tr(context, 'storageAvailable'),
                  style: const TextStyle(
                    fontSize: 12,
                    color: PinelinkColors.muted,
                  ),
                ),
                if (_savedFile == file?.id)
                  MotionStateIcon(
                    identity: file!.id,
                    icon: Icons.check,
                    label: tr(context, 'savedPersistent'),
                    color: PinelinkColors.accent,
                  ),
                if (file != null && file.retentionClass != 'PERSISTENT')
                  MotionButton(
                    onPressed: _responding || file.state != 'AVAILABLE'
                        ? null
                        : () => _promote(file),
                    child: Text(tr(context, 'savePersistent')),
                  ),
                if (file == null)
                  MotionStateIcon(
                    identity: 'unavailable',
                    icon: Icons.cloud_off_outlined,
                    label: tr(context, 'fileStorageError'),
                  ),
                if (_noticeZone == 'file' && _notice != null) _feedback(),
              ],
            ),
          ],
          SettingsHeading(title: tr(context, 'devicesTitle')),
          if (data.devices.isEmpty)
            SettingsCard(
              children: [
                Text(
                  tr(context, 'noDevices'),
                  style: const TextStyle(
                    fontSize: 12,
                    color: PinelinkColors.muted,
                  ),
                ),
              ],
            )
          else ...[
            DropdownButtonFormField<String>(
              initialValue: selectedDevice,
              decoration: InputDecoration(
                labelText: tr(context, 'recipientDevice'),
                border: const OutlineInputBorder(),
              ),
              items: [
                for (final item in data.devices)
                  DropdownMenuItem(
                    value: item.id,
                    child: Text(
                      '${item.id} · ${item.status == 'ACTIVE' ? tr(context, 'deviceUsable') : tr(context, 'currentDeviceRevoked')} · ${formatTime(item.lastSeenAt, locale)}',
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
              ],
              onChanged: (value) => setState(() {
                _device = value;
                _confirm = false;
                _notice = null;
              }),
            ),
            if (selectedDevice != null &&
                data.devices
                    .where(
                      (item) =>
                          item.id == selectedDevice && item.status == 'ACTIVE',
                    )
                    .isNotEmpty) ...[
              const SizedBox(height: 16),
              PopupMenuButton<String>(
                popUpAnimationStyle: PinelinkMotion.menu(context),
                onSelected: (_) => _revoke(selectedDevice),
                itemBuilder: (_) => [
                  PopupMenuItem(
                    value: 'revoke',
                    child: Text(
                      tr(context, _confirm ? 'confirmRevoke' : 'revokeDevice'),
                    ),
                  ),
                ],
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 20,
                    vertical: 12,
                  ),
                  decoration: BoxDecoration(
                    border: Border.all(color: PinelinkColors.line),
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Text(
                    tr(context, 'manageDevice'),
                    style: const TextStyle(fontSize: 14),
                  ),
                ),
              ),
            ],
          ],
          if (_noticeZone == 'device' && _notice != null) _feedback(),
        ],
      );
    },
  );

  Future<void> _promote(FileSnapshot file) async {
    if (_responding) return;
    final api = widget.api;
    setState(() {
      _responding = true;
      _notice = null;
      _noticeZone = 'file';
    });
    try {
      await api.promoteFile(
        file.id,
        PinelinkApi.owner,
        PinelinkApi.device,
        file.version,
      );
      if (!mounted || widget.api != api) return;
      setState(() {
        _savedFile = file.id;
        _notice = tr(context, 'savedPersistent');
      });
    } on ApiException catch (error) {
      if (!mounted || widget.api != api) return;
      setState(
        () => _notice = tr(
          context,
          error.status == 401 || error.status == 403
              ? 'permissionDenied'
              : error.status == 410
              ? 'errFileExpired'
              : error.code == 'STATE_CONFLICT'
              ? 'fileConflict'
              : error.status == 409
              ? 'fileIncompleteOrQuota'
              : 'storageFailed',
        ),
      );
    } catch (_) {
      if (!mounted || widget.api != api) return;
      setState(() => _notice = tr(context, 'connectionFailed'));
    } finally {
      if (mounted && widget.api == api) {
        setState(() => _responding = false);
        _refresh();
      }
    }
  }

  Future<void> _revoke(String device) async {
    if (_responding) return;
    final api = widget.api;
    _noticeZone = 'device';
    if (!_confirm) {
      setState(() {
        _confirm = true;
        _notice = tr(context, 'revokeHint', {'device': device});
      });
      return;
    }
    setState(() => _responding = true);
    try {
      await api.revokeDevice(PinelinkApi.owner, device);
      if (!mounted || widget.api != api) return;
      setState(() {
        _notice = tr(context, 'revokedSuccess');
        _confirm = false;
      });
    } catch (_) {
      if (!mounted || widget.api != api) return;
      setState(() => _notice = tr(context, 'revokeFailed'));
    } finally {
      if (mounted && widget.api == api) {
        setState(() => _responding = false);
        _refresh();
      }
    }
  }

  Widget _feedback() => MotionReveal(
    identity: _notice!,
    child: Semantics(
      liveRegion: true,
      child: Text(_notice!, style: const TextStyle(fontSize: 12)),
    ),
  );
}

class SettingsHeading extends StatelessWidget {
  const SettingsHeading({super.key, required this.title});
  final String title;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(top: 32, bottom: 16),
    child: Text(
      title,
      style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
    ),
  );
}

class SettingsCard extends StatelessWidget {
  const SettingsCard({super.key, required this.children});
  final List<Widget> children;
  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(22),
    decoration: BoxDecoration(
      color: PinelinkColors.background,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: PinelinkColors.line),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var index = 0; index < children.length; index++) ...[
          if (index > 0) const SizedBox(height: 16),
          children[index],
        ],
      ],
    ),
  );
}
