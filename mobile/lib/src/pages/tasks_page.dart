import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/courier.dart';
import '../widgets/file_icon.dart';

class TaskData {
  const TaskData(this.tasks, this.transfers, this.pullNames);
  final List<TransferTask> tasks;
  final Map<String, TransferRequest> transfers;
  final Map<String, String> pullNames;
}

String taskStatus(TransferTask task, [Locale? locale]) {
  final current = locale ?? const Locale('zh', 'CN');
  final direction = translateLocale(
    current,
    task.kind == 'DOWNLOAD' ? 'directionReceive' : 'directionSend',
  );
  final key = switch (task.state) {
    'RUNNING' => 'statusRunning',
    'PAUSED' => 'statusPaused',
    'QUEUED' => 'statusQueued',
    'COMPLETED' => 'statusCompleted',
    'CANCELLED' => 'statusCancelled',
    'FAILED' => 'statusFailed',
    'DELETED' => 'statusDeleted',
    _ => 'statusUnknown',
  };
  return translateLocale(current, key, {'direction': direction});
}

String shortTaskId(String id) => id.length > 8 ? id.substring(0, 8) : id;

Future<TaskData> loadTaskData(
  PinelinkApi api, {
  bool includeDeleted = false,
}) async {
  final tasks = await api.getTasks(
    PinelinkApi.owner,
    PinelinkApi.device,
    includeDeleted: includeDeleted,
  );
  final transfers = await api.getTransfers(
    PinelinkApi.owner,
    PinelinkApi.device,
  );
  final byId = {for (final item in transfers) item.id: item};
  final names = <String, String>{};
  for (final task in tasks) {
    if (task.kind == 'DOWNLOAD' && !byId.containsKey(task.referenceId)) {
      try {
        names[task.referenceId] = await api.getPull(
          task.referenceId,
          PinelinkApi.owner,
          PinelinkApi.device,
        );
      } catch (_) {}
    } else if (task.kind == 'UPLOAD') {
      try {
        names[task.referenceId] = (await api.getFile(
          task.referenceId,
          PinelinkApi.owner,
        )).fileName;
      } catch (_) {}
    }
  }
  return TaskData(tasks, byId, names);
}

class TasksPage extends StatefulWidget {
  const TasksPage({super.key, required this.api});
  final PinelinkApi api;
  @override
  State<TasksPage> createState() => _TasksPageState();
}

class _TasksPageState extends State<TasksPage> {
  late Future<TaskData> _data = _load();
  String? _selected;
  String? _notice;
  bool _responding = false;
  @override
  void initState() {
    super.initState();
    widget.api.addListener(_synced);
  }

  @override
  void didUpdateWidget(covariant TasksPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      oldWidget.api.removeListener(_synced);
      widget.api.addListener(_synced);
      _responding = false;
      _notice = null;
      _confirmCancel = false;
      _data = _load();
    }
  }

  void _synced() {
    if (mounted && !_responding) {
      setState(() {
        _data = _load();
      });
    }
  }

  @override
  void dispose() {
    widget.api.removeListener(_synced);
    super.dispose();
  }

  bool _confirmCancel = false;

  Future<TaskData> _load() => loadTaskData(widget.api);

  void _refresh() => setState(() {
    _data = _load();
  });

  @override
  Widget build(BuildContext context) => FutureBuilder<TaskData>(
    future: _data,
    builder: (context, snapshot) {
      if (snapshot.connectionState == ConnectionState.waiting) {
        return Center(child: Text(tr(context, 'loadingTasks')));
      }
      if (snapshot.hasError) {
        return Center(child: Text(tr(context, 'actionFailed')));
      }
      final data = snapshot.data!;
      final task =
          data.tasks.where((item) => item.id == _selected).firstOrNull ??
          data.tasks
              .where(
                (item) => ['RUNNING', 'PAUSED', 'QUEUED'].contains(item.state),
              )
              .firstOrNull ??
          data.tasks.firstOrNull;
      return ListView(
        padding: const EdgeInsets.all(22),
        children: [
          Text(
            tr(context, 'tasksTitle'),
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            tr(context, 'tasksSubtitle'),
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
          if (data.tasks.length > 1) ...[
            DropdownButtonFormField<String>(
              initialValue: task?.id,
              decoration: InputDecoration(
                labelText: tr(context, 'selectTask'),
                border: const OutlineInputBorder(),
              ),
              items: [
                for (final item in data.tasks)
                  DropdownMenuItem(
                    value: item.id,
                    child: Text(
                      '${taskStatus(item, i18nLocale(context))} · ${shortTaskId(item.id)}',
                    ),
                  ),
              ],
              onChanged: (value) => setState(() {
                _selected = value;
                _confirmCancel = false;
                _notice = null;
              }),
            ),
            const SizedBox(height: 16),
          ],
          if (task == null)
            const EmptyTasks()
          else ...[
            Builder(
              builder: (context) {
                final transfer = data.transfers[task.referenceId];
                final locale = i18nLocale(context);
                final fileName =
                    displayFileName(
                      transfer?.fileName ?? data.pullNames[task.referenceId],
                      locale,
                    ) ??
                    tr(
                      context,
                      task.kind == 'DOWNLOAD' ? 'receivedFile' : 'sentFile',
                    );
                final percent = task.total > 0
                    ? (task.progress * 100 / task.total).round().clamp(0, 100)
                    : task.state == 'COMPLETED'
                    ? 100
                    : null;
                final status = taskStatus(task, locale);
                final note = task.state == 'PAUSED'
                    ? tr(context, 'pausedNote')
                    : task.state == 'QUEUED'
                    ? tr(context, 'queuedNote')
                    : task.state == 'COMPLETED'
                    ? tr(
                        context,
                        task.kind == 'DOWNLOAD'
                            ? 'downloadCompleteNote'
                            : 'uploadCompleteNote',
                      )
                    : tr(context, 'activeNote', {
                        'direction': tr(
                          context,
                          task.kind == 'DOWNLOAD'
                              ? 'directionReceive'
                              : 'directionSend',
                        ),
                      });
                return Container(
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
                          Expanded(
                            child: Text(
                              fileName,
                              style: const TextStyle(
                                fontSize: 16,
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                          ),
                          const SizedBox(width: 12),
                          Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 10,
                              vertical: 6,
                            ),
                            decoration: BoxDecoration(
                              color: PinelinkColors.soft,
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: Text(
                              status,
                              style: const TextStyle(
                                fontSize: 12,
                                color: PinelinkColors.accent,
                              ),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 24),
                      Row(
                        children: [
                          Courier(
                            running: task.state == 'RUNNING',
                            action: task.state == 'PAUSED'
                                ? CourierAction.pause
                                : task.state == 'COMPLETED'
                                ? (task.kind == 'UPLOAD'
                                      ? CourierAction.store
                                      : CourierAction.complete)
                                : null,
                            label: tr(context, 'courierStatus', {
                              'status': status,
                            }),
                          ),
                          const SizedBox(width: 20),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  percent == null
                                      ? tr(context, 'waitingData')
                                      : '$percent%',
                                  style: const TextStyle(
                                    fontSize: 25,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                                const SizedBox(height: 14),
                                if (percent != null)
                                  LinearProgressIndicator(
                                    value: percent / 100,
                                    minHeight: 8,
                                    borderRadius: BorderRadius.circular(4),
                                    backgroundColor: PinelinkColors.line,
                                    color: PinelinkColors.accent,
                                  ),
                                const SizedBox(height: 14),
                                Text(
                                  '${formatBytes(task.progress, locale)} / ${task.total > 0 ? formatBytes(task.total, locale) : tr(context, 'sizePending')}',
                                  style: const TextStyle(
                                    fontSize: 12,
                                    color: PinelinkColors.muted,
                                  ),
                                ),
                                if (transfer != null) ...[
                                  const SizedBox(height: 12),
                                  Text(
                                    '${tr(context, task.kind == 'DOWNLOAD' ? 'from' : 'to')} ${task.kind == 'DOWNLOAD' ? transfer.senderDevice : transfer.recipientDevice}',
                                    style: const TextStyle(
                                      fontSize: 12,
                                      color: PinelinkColors.muted,
                                    ),
                                  ),
                                ],
                                if (task.updatedAt != null) ...[
                                  const SizedBox(height: 8),
                                  Text(
                                    tr(context, 'updatedAt', {
                                      'time': formatTime(
                                        task.updatedAt,
                                        locale,
                                      ),
                                    }),
                                    style: const TextStyle(
                                      fontSize: 12,
                                      color: PinelinkColors.muted,
                                    ),
                                  ),
                                ],
                              ],
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 16),
                      Text(
                        note,
                        style: const TextStyle(
                          fontSize: 12,
                          color: PinelinkColors.muted,
                          height: 1.7,
                        ),
                      ),
                      if (task.errorMessage != null &&
                          task.errorMessage!.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(top: 12),
                          child: Text(
                            tr(context, 'errorReason', {
                              'reason': task.errorMessage!,
                            }),
                            style: const TextStyle(
                              fontSize: 12,
                              color: Color(0xFF85463F),
                            ),
                          ),
                        ),
                    ],
                  ),
                );
              },
            ),
            if (['RUNNING', 'PAUSED', 'QUEUED'].contains(task.state)) ...[
              const SizedBox(height: 20),
              Row(
                children: [
                  Expanded(
                    child: ElevatedButton(
                      onPressed: _responding
                          ? null
                          : () => _control(
                              task,
                              task.state == 'PAUSED' ? 'resume' : 'pause',
                            ),
                      child: Text(
                        tr(
                          context,
                          task.state == 'PAUSED' ? 'resume' : 'pause',
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: OutlinedButton(
                      onPressed: _responding
                          ? null
                          : () {
                              if (_confirmCancel) {
                                _control(task, 'cancel');
                              } else {
                                setState(() {
                                  _confirmCancel = true;
                                  _notice = tr(context, 'cancelWarning');
                                });
                              }
                            },
                      child: Text(
                        tr(
                          context,
                          _confirmCancel ? 'confirmCancel' : 'cancel',
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ],
          ],
        ],
      );
    },
  );

  Future<void> _control(TransferTask task, String action) async {
    final api = widget.api;
    setState(() {
      _responding = true;
      _notice = null;
    });
    try {
      await api.controlTask(
        task.id,
        action,
        PinelinkApi.owner,
        PinelinkApi.device,
        task.version,
      );
      if (!mounted || widget.api != api) return;
      setState(() {
        _confirmCancel = false;
        _notice = tr(
          context,
          action == 'pause'
              ? 'pauseSuccess'
              : action == 'resume'
              ? 'resumeSuccess'
              : 'cancelSuccess',
        );
      });
    } on ApiException catch (error) {
      if (!mounted || widget.api != api) return;
      setState(
        () => _notice = tr(
          context,
          error.status == 409 ? 'stateChangedRefresh' : 'actionFailed',
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
}

class EmptyTasks extends StatelessWidget {
  const EmptyTasks({super.key});
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
          tr(context, 'noActiveTasks'),
          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          tr(context, 'noActiveTasksHint'),
          style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
        ),
      ],
    ),
  );
}
