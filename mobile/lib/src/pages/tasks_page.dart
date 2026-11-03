import '../motion/surface.dart';
import '../motion/transfer_graph.dart';
import '../motion/motion.dart';

import 'package:flutter/material.dart';

import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../theme/tokens.dart';
import '../widgets/file_icon.dart'
    show displayFileName, formatBytes, formatTime;

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
  String? _selected, _notice;
  bool _responding = false, _confirmCancel = false;
  int _epoch = 0;
  StateSetter? _detailsRefresh;
  ModalRoute<dynamic>? _detailsRoute;
  Object? _detailsToken;
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
      _epoch++;
      _responding = false;
      _notice = null;
      _confirmCancel = false;
      _selected = null;
      _closeDetails();
      _data = _load();
    }
  }

  void _update(VoidCallback update) {
    if (!mounted) return;
    setState(() {
      update();
    });
    _detailsRefresh?.call(() {});
  }

  void _synced() {
    if (mounted && !_responding) _update(() => _data = _load());
  }

  Future<TaskData> _load() => loadTaskData(widget.api);
  void _closeDetails() {
    final route = _detailsRoute;
    _detailsRoute = null;
    _detailsRefresh = null;
    _detailsToken = null;
    if (route != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        final navigator = route.navigator;
        if (navigator != null && navigator.mounted && route.isActive) {
          navigator.removeRoute(route);
        }
      });
    }
  }

  @override
  void dispose() {
    _epoch++;
    widget.api.removeListener(_synced);
    _closeDetails();
    super.dispose();
  }

  String _name(TransferTask task, TaskData data, Locale locale) =>
      displayFileName(
        data.transfers[task.referenceId]?.fileName ??
            data.pullNames[task.referenceId],
        locale,
      ) ??
      tr(context, task.kind == 'DOWNLOAD' ? 'receivedFile' : 'sentFile');
  String _status(TransferTask task) =>
      task.kind == 'UPLOAD' && task.state == 'COMPLETED'
      ? tr(context, 'cloudStored')
      : task.state == 'RUNNING' && task.total > 0 && task.progress >= task.total
      ? tr(context, 'awaitingConfirmation')
      : taskStatus(task, i18nLocale(context));
  bool _cloud(TransferTask task, TaskData data) {
    final transfer = data.transfers[task.referenceId];
    return task.kind == 'UPLOAD' ||
        transfer == null ||
        (transfer.deliveryMode.isNotEmpty &&
            !transfer.deliveryMode.startsWith('P2P'));
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<TaskData>(
    future: _data,
    builder: (context, snapshot) {
      if (snapshot.connectionState == ConnectionState.waiting &&
          !snapshot.hasData) {
        return Center(
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const MotionSpinner(),
              const SizedBox(width: 8),
              Text(tr(context, 'loadingTasks')),
            ],
          ),
        );
      }
      if (snapshot.hasError || !snapshot.hasData) {
        return Center(
          child: MotionReveal(
            identity: 'error',
            child: Text(tr(context, 'actionFailed')),
          ),
        );
      }
      final data = snapshot.data!,
          task =
              data.tasks.where((item) => item.id == _selected).firstOrNull ??
              data.tasks
                  .where(
                    (item) =>
                        ['RUNNING', 'PAUSED', 'QUEUED'].contains(item.state),
                  )
                  .firstOrNull ??
              data.tasks.firstOrNull;
      return ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Text(
            tr(context, 'tasksTitle'),
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 24),
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
              onChanged: (value) => _update(() {
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
            TransferGraph(
              key: ValueKey(task.id),
              value: task.total > 0
                  ? task.progress / task.total
                  : task.state == 'COMPLETED'
                  ? 1
                  : 0,
              running: task.state == 'RUNNING' && widget.api.tasksNetworkFresh,
              completed: task.state == 'COMPLETED',
              cloud: _cloud(task, data),
              incoming: task.kind == 'DOWNLOAD',
              localLabel: tr(context, 'thisDevice'),
              peerLabel: _cloud(task, data)
                  ? tr(context, 'cloudNode')
                  : data.transfers[task.referenceId]!.senderDevice,
              statusLabel: _status(task),
              detailsLabel: tr(context, 'transferDetails'),
              onDetails: () => _showDetails(task, data),
            ),
            Center(
              child: MotionButton(
                onPressed: () {
                  _showDetails(task, data);
                },
                child: Text(_status(task)),
              ),
            ),
          ],
        ],
      );
    },
  );

  Future<void> _showDetails(TransferTask initial, TaskData data) async {
    if (_detailsToken != null) return;
    final token = Object(), api = widget.api;
    _detailsToken = token;
    _selected = initial.id;
    try {
      await showMotionSurface<void>(
        context,
        title: _name(initial, data, i18nLocale(context)),
        builder: (sheetContext) {
          _detailsRoute = ModalRoute.of(sheetContext);
          return StatefulBuilder(
            builder: (context, refresh) {
              _detailsRefresh = refresh;
              if (!mounted || widget.api != api) return const SizedBox.shrink();
              return FutureBuilder<TaskData>(
                future: _data,
                initialData: data,
                builder: (context, snapshot) {
                  if (snapshot.hasError) {
                    return Text(tr(context, 'actionFailed'));
                  }
                  final current = snapshot.data ?? data,
                      task = current.tasks
                          .where((item) => item.id == initial.id)
                          .firstOrNull;
                  if (task == null) return Text(tr(context, 'statusDeleted'));
                  final locale = i18nLocale(context),
                      peer = current.transfers[task.referenceId]?.senderDevice;
                  final percent = task.total > 0
                      ? (task.progress * 100 / task.total).floor().clamp(0, 100)
                      : task.state == 'COMPLETED'
                      ? 100
                      : null;
                  final active = [
                    'RUNNING',
                    'PAUSED',
                    'QUEUED',
                  ].contains(task.state);
                  return Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        percent == null
                            ? tr(context, 'waitingData')
                            : '$percent%',
                        style: const TextStyle(
                          fontSize: 24,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 24),
                      Text(_status(task)),
                      const SizedBox(height: 16),
                      Text(
                        '${formatBytes(task.progress, locale)} / ${task.total > 0 ? formatBytes(task.total, locale) : tr(context, 'sizePending')}',
                      ),
                      if (peer != null) ...[
                        const SizedBox(height: 16),
                        Text(peer),
                      ],
                      if (task.updatedAt != null) ...[
                        const SizedBox(height: 16),
                        Text(formatTime(task.updatedAt!, locale)),
                      ],
                      if (_notice != null) ...[
                        const SizedBox(height: 24),
                        MotionReveal(
                          identity: _notice,
                          child: Text(
                            _notice!,
                            style: const TextStyle(
                              color: PinelinkColors.accent,
                            ),
                          ),
                        ),
                      ],
                      if (task.errorMessage?.isNotEmpty == true) ...[
                        const SizedBox(height: 16),
                        Text(task.errorMessage!),
                      ],
                      if (_confirmCancel) ...[
                        const SizedBox(height: 16),
                        Text(tr(context, 'cancelWarning')),
                      ],
                      if (active) ...[
                        const SizedBox(height: 24),
                        Row(
                          children: [
                            Expanded(
                              child: MotionButton.primary(
                                onPressed: _responding
                                    ? null
                                    : () => _control(
                                        task,
                                        task.state == 'PAUSED'
                                            ? 'resume'
                                            : 'pause',
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
                              child: MotionButton(
                                onPressed: _responding
                                    ? null
                                    : () {
                                        if (_confirmCancel) {
                                          return _control(task, 'cancel');
                                        }
                                        _update(() => _confirmCancel = true);
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
                  );
                },
              );
            },
          );
        },
      );
    } finally {
      if (_detailsToken == token) {
        _detailsRefresh = null;
        _detailsRoute = null;
        _detailsToken = null;
      }
    }
  }

  Future<void> _control(TransferTask task, String action) async {
    if (_responding) return;
    final api = widget.api, epoch = _epoch;
    _update(() {
      _responding = true;
      _notice = null;
    });
    bool current() => mounted && widget.api == api && epoch == _epoch;
    var refreshed = false;
    try {
      await api.controlTask(
        task.id,
        action,
        PinelinkApi.owner,
        PinelinkApi.device,
        task.version,
      );
      if (!current()) return;
      final loading = _load();
      _update(() => _data = loading);
      final fresh = await loading;
      refreshed = true;
      if (!current() || _selected != task.id) return;
      final result = fresh.tasks
          .where((item) => item.id == task.id)
          .firstOrNull;
      final expected = action == 'pause'
          ? 'PAUSED'
          : action == 'resume'
          ? 'RUNNING'
          : 'CANCELLED';
      _update(() {
        _confirmCancel = false;
        _notice = tr(
          context,
          result?.state == expected
              ? action == 'pause'
                    ? 'pauseSuccess'
                    : action == 'resume'
                    ? 'resumeSuccess'
                    : 'cancelSuccess'
              : 'stateChangedRefresh',
        );
      });
    } on ApiException catch (error) {
      if (current()) {
        _update(
          () => _notice = tr(
            context,
            error.status == 409 ? 'stateChangedRefresh' : 'actionFailed',
          ),
        );
      }
    } catch (_) {
      if (current()) _update(() => _notice = tr(context, 'connectionFailed'));
    } finally {
      if (current()) {
        _update(() {
          _responding = false;
          if (!refreshed) _data = _load();
        });
      }
    }
  }
}

class EmptyTasks extends StatelessWidget {
  const EmptyTasks({super.key});
  @override
  Widget build(BuildContext context) => MotionReveal(
    identity: 'empty',
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Text(tr(context, 'noActiveTasks')),
    ),
  );
}
