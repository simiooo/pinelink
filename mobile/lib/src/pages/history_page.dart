import '../motion/motion.dart';

import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart';
import 'tasks_page.dart';

String taskError(TransferTask task, [Locale? locale]) {
  final current = locale ?? const Locale('zh', 'CN');
  final key = switch (task.errorCode) {
    'TRANSFER_CANCELLED' => 'errTransferCancelled',
    'TRANSFER_REJECTED' => 'errTransferRejected',
    'DEVICE_REVOKED' => 'errDeviceRevoked',
    'FILE_EXPIRED' => 'errFileExpired',
    'TASK_CANCELLED' => 'errTaskCancelled',
    'TASK_DELETED' => 'errTaskDeleted',
    'TASK_FAILED' => 'errTaskFailed',
    null => '',
    _ => 'errUnknown',
  };
  return key.isEmpty
      ? (task.errorMessage ?? '')
      : translateLocale(current, key);
}

class HistoryPage extends StatefulWidget {
  const HistoryPage({super.key, required this.api});
  final PinelinkApi api;
  @override
  State<HistoryPage> createState() => _HistoryPageState();
}

class _HistoryPageState extends State<HistoryPage> {
  late Future<TaskData> _data = loadTaskData(widget.api, includeDeleted: true);
  String? _selected;
  String _state = '';
  String _kind = '';
  String? _notice;
  bool _confirm = false;
  bool _responding = false;
  @override
  void initState() {
    super.initState();
    widget.api.addListener(_synced);
  }

  @override
  void didUpdateWidget(covariant HistoryPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      oldWidget.api.removeListener(_synced);
      widget.api.addListener(_synced);
      _responding = false;
      _notice = null;
      _confirm = false;
      _data = loadTaskData(widget.api, includeDeleted: true);
    }
  }

  void _synced() {
    if (mounted && !_responding) {
      setState(() {
        _data = loadTaskData(widget.api, includeDeleted: true);
      });
    }
  }

  @override
  void dispose() {
    widget.api.removeListener(_synced);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<TaskData>(
    future: _data,
    builder: (context, snapshot) {
      final filtered = (snapshot.data?.tasks ?? <TransferTask>[])
          .where(
            (task) =>
                (_state.isEmpty || task.state == _state) &&
                (_kind.isEmpty || task.kind == _kind),
          )
          .toList();
      final task =
          filtered.where((item) => item.id == _selected).firstOrNull ??
          filtered.firstOrNull;
      return ListView(
        padding: const EdgeInsets.all(22),
        children: [
          Text(
            tr(context, 'historyTitle'),
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            tr(context, 'historySubtitle'),
            style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
          ),
          const SizedBox(height: 28),
          Row(
            children: [
              Expanded(
                child: DropdownButtonFormField<String>(
                  initialValue: _state,
                  decoration: InputDecoration(
                    labelText: tr(context, 'stateFilter'),
                    border: const OutlineInputBorder(),
                  ),
                  items: [
                    DropdownMenuItem(
                      value: '',
                      child: Text(tr(context, 'allStates')),
                    ),
                    DropdownMenuItem(
                      value: 'COMPLETED',
                      child: Text(tr(context, 'completedState')),
                    ),
                    DropdownMenuItem(
                      value: 'PAUSED',
                      child: Text(tr(context, 'pausedState')),
                    ),
                    DropdownMenuItem(
                      value: 'CANCELLED',
                      child: Text(tr(context, 'cancelledState')),
                    ),
                    DropdownMenuItem(
                      value: 'FAILED',
                      child: Text(tr(context, 'failedState')),
                    ),
                    DropdownMenuItem(
                      value: 'DELETED',
                      child: Text(tr(context, 'deletedState')),
                    ),
                    DropdownMenuItem(
                      value: 'RUNNING',
                      child: Text(tr(context, 'runningState')),
                    ),
                  ],
                  onChanged: (value) => setState(() {
                    _state = value ?? '';
                    _selected = null;
                    _confirm = false;
                  }),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: DropdownButtonFormField<String>(
                  initialValue: _kind,
                  decoration: InputDecoration(
                    labelText: tr(context, 'kindFilter'),
                    border: const OutlineInputBorder(),
                  ),
                  items: [
                    DropdownMenuItem(
                      value: '',
                      child: Text(tr(context, 'allKinds')),
                    ),
                    DropdownMenuItem(
                      value: 'DOWNLOAD',
                      child: Text(tr(context, 'directionReceive')),
                    ),
                    DropdownMenuItem(
                      value: 'UPLOAD',
                      child: Text(tr(context, 'directionSend')),
                    ),
                  ],
                  onChanged: (value) => setState(() {
                    _kind = value ?? '';
                    _selected = null;
                    _confirm = false;
                  }),
                ),
              ),
            ],
          ),
          if (filtered.length > 1) ...[
            const SizedBox(height: 16),
            DropdownButtonFormField<String>(
              initialValue: task?.id,
              decoration: InputDecoration(
                labelText: tr(context, 'recordFilter'),
                border: const OutlineInputBorder(),
              ),
              items: [
                for (final item in filtered)
                  DropdownMenuItem(
                    value: item.id,
                    child: Text(
                      '${taskStatus(item, i18nLocale(context))} · ${shortTaskId(item.id)}',
                    ),
                  ),
              ],
              onChanged: (value) => setState(() {
                _selected = value;
                _confirm = false;
                _notice = null;
              }),
            ),
          ],
          const SizedBox(height: 24),
          if (_notice != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Text(
                _notice!,
                style: const TextStyle(
                  fontSize: 12,
                  color: PinelinkColors.accent,
                ),
              ),
            ),
          if (snapshot.connectionState == ConnectionState.waiting &&
              !snapshot.hasData)
            Text(tr(context, 'historyLoading'))
          else if (snapshot.hasError)
            Text(tr(context, 'actionFailed'))
          else if (task == null)
            Text(tr(context, 'noHistory'))
          else ...[
            Builder(
              builder: (context) {
                final locale = i18nLocale(context);
                final transfer = snapshot.data!.transfers[task.referenceId];
                final fileName = transfer?.fileName == 'encrypted'
                    ? tr(context, 'encryptedFile')
                    : displayFileName(
                            transfer?.fileName ??
                                snapshot.data!.pullNames[task.referenceId],
                            locale,
                          ) ??
                          tr(
                            context,
                            task.kind == 'DOWNLOAD'
                                ? 'receivedFile'
                                : 'sentFile',
                          );
                return Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            fileName,
                            style: const TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                        ),
                        Container(
                          padding: const EdgeInsets.all(8),
                          decoration: BoxDecoration(
                            color: PinelinkColors.soft,
                            borderRadius: BorderRadius.circular(8),
                          ),
                          child: Text(
                            taskStatus(task, locale),
                            style: const TextStyle(
                              fontSize: 12,
                              color: PinelinkColors.accent,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),
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
                          HistoryMetadata(
                            label: tr(context, 'direction'),
                            value: tr(
                              context,
                              task.kind == 'DOWNLOAD'
                                  ? 'directionReceive'
                                  : 'directionSend',
                            ),
                          ),
                          if (transfer != null)
                            HistoryMetadata(
                              label: tr(context, 'peerDevice'),
                              value: task.kind == 'DOWNLOAD'
                                  ? transfer.senderDevice
                                  : transfer.recipientDevice,
                            ),
                          HistoryMetadata(
                            label: tr(context, 'transferred'),
                            value:
                                '${formatBytes(task.progress, locale)} / ${task.total > 0 ? formatBytes(task.total, locale) : tr(context, 'sizeUnknown')}',
                          ),
                          if (task.updatedAt != null)
                            HistoryMetadata(
                              label: tr(context, 'time'),
                              value: formatTime(task.updatedAt, locale),
                            ),
                          if (taskError(task, locale).isNotEmpty)
                            Padding(
                              padding: const EdgeInsets.only(top: 20),
                              child: Text(
                                taskError(task, locale),
                                style: const TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF85463F),
                                  height: 1.7,
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                    if (task.state != 'RUNNING' && task.state != 'DELETED') ...[
                      const SizedBox(height: 20),
                      MotionButton(
                        onPressed: _responding
                            ? null
                            : () {
                                if (_confirm) {
                                  _delete(task);
                                } else {
                                  setState(() {
                                    _confirm = true;
                                    _notice = tr(context, 'deleteWarning');
                                  });
                                }
                              },
                        child: Text(
                          tr(
                            context,
                            _confirm ? 'confirmDelete' : 'deleteTask',
                          ),
                        ),
                      ),
                    ],
                    const SizedBox(height: 12),
                    Text(
                      tr(context, 'deleteNote'),
                      style: const TextStyle(
                        fontSize: 12,
                        color: PinelinkColors.muted,
                      ),
                    ),
                  ],
                );
              },
            ),
          ],
        ],
      );
    },
  );

  Future<void> _delete(TransferTask task) async {
    final api = widget.api;
    setState(() => _responding = true);
    try {
      await api.controlTask(
        task.id,
        'delete',
        widget.api.currentOwner,
        widget.api.currentDevice,
        task.version,
      );
      if (!mounted || widget.api != api) return;
      setState(() {
        _notice = tr(context, 'deletedSuccess');
        _confirm = false;
      });
    } on ApiException catch (error) {
      if (!mounted || widget.api != api) return;
      setState(
        () => _notice = tr(
          context,
          error.status == 409 ? 'stateChangedRefresh' : 'deleteFailed',
        ),
      );
    } catch (_) {
      if (!mounted || widget.api != api) return;
      setState(() => _notice = tr(context, 'connectionFailed'));
    } finally {
      if (mounted && widget.api == api) {
        setState(() {
          _responding = false;
          _data = loadTaskData(widget.api, includeDeleted: true);
        });
      }
    }
  }
}

class HistoryMetadata extends StatelessWidget {
  const HistoryMetadata({super.key, required this.label, required this.value});
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Row(
      children: [
        SizedBox(
          width: 92,
          child: Text(
            label,
            style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
          ),
        ),
        Expanded(child: Text(value, style: const TextStyle(fontSize: 12))),
      ],
    ),
  );
}
