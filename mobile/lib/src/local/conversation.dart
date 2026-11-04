import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';

import 'controller.dart';
import 'copy.dart';
import 'file_crypto.dart';

class LocalConversation extends StatefulWidget {
  const LocalConversation({
    super.key,
    required this.controller,
    required this.root,
    required this.onJob,
  });
  final LocalController controller;
  final String root;
  final void Function(BuildContext, String) onJob;
  @override
  State<LocalConversation> createState() => _LocalConversationState();
}

class _LocalConversationState extends State<LocalConversation> {
  final input = TextEditingController();
  bool sensitive = false, busy = false;
  @override
  void dispose() {
    input.dispose();
    super.dispose();
  }

  bool validUnicode(String value) {
    final units = value.codeUnits;
    for (var i = 0; i < units.length; i++) {
      final c = units[i];
      if (c >= 0xd800 && c <= 0xdbff) {
        if (++i >= units.length || units[i] < 0xdc00 || units[i] > 0xdfff) {
          return false;
        }
      } else if (c >= 0xdc00 && c <= 0xdfff) {
        return false;
      }
    }
    return true;
  }

  Future<void> send(LocalMap peer) async {
    final value = input.text;
    if (!validUnicode(value)) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(lc(context, 'Invalid text', '文字格式无效'))),
      );
      return;
    }
    final bytes = Uint8List.fromList(utf8.encode(value));
    if (bytes.isEmpty || bytes.length > textByteLimit || busy) return;
    setState(() => busy = true);
    try {
      await widget.controller.sendText(
        bytes,
        peer,
        sensitive ? 'SENSITIVE' : 'NORMAL',
      );
      if (mounted && input.text == value) input.clear();
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(lc(context, 'Could not send', '发送失败'))),
        );
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) {
      final r = widget.controller,
          peer = r.peers.where((p) => p['root'] == widget.root).firstOrNull;
      final jobs =
          r.jobs.where((j) => j['peer']['root'] == widget.root).toList()..sort(
            (a, b) => (a['created'] as int).compareTo(b['created'] as int),
          );
      return Scaffold(
        appBar: AppBar(
          title: Text(
            peer == null ? lc(context, 'Device', '设备') : localPeerName(peer),
          ),
        ),
        body: SafeArea(
          child: Column(
            children: [
              Expanded(
                child: ListView.separated(
                  padding: const EdgeInsets.all(16),
                  itemCount: jobs.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 24),
                  itemBuilder: (context, i) {
                    final job = jobs[i], sent = job['direction'] == 'send';
                    if (job['contentKind'] == 'TEXT') {
                      return _TextMessage(
                        key: ValueKey(job['id']),
                        controller: r,
                        job: job,
                        onJob: () => widget.onJob(context, job['id']),
                      );
                    }
                    return Align(
                      alignment: sent
                          ? Alignment.centerRight
                          : Alignment.centerLeft,
                      child: InkWell(
                        borderRadius: BorderRadius.circular(16),
                        onTap: () => widget.onJob(context, job['id']),
                        child: Container(
                          constraints: const BoxConstraints(maxWidth: 280),
                          padding: const EdgeInsets.all(16),
                          decoration: BoxDecoration(
                            color: const Color(0xfff4f4f5),
                            borderRadius: BorderRadius.circular(16),
                          ),
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const Icon(Icons.insert_drive_file_outlined),
                              const SizedBox(width: 12),
                              Flexible(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      job['name'],
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                    Text(
                                      localBytes(job['plainSize']),
                                      style: const TextStyle(
                                        fontSize: 12,
                                        color: Color(0xff737373),
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    );
                  },
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    Row(
                      children: [
                        TextButton(
                          onPressed: () => setState(() => sensitive = false),
                          child: Text(
                            lc(context, 'Text', '文本'),
                            style: TextStyle(
                              fontWeight: !sensitive
                                  ? FontWeight.bold
                                  : FontWeight.normal,
                            ),
                          ),
                        ),
                        TextButton(
                          onPressed: () => setState(() => sensitive = true),
                          child: Text(
                            lc(context, 'Sensitive', '敏感信息'),
                            style: TextStyle(
                              fontWeight: sensitive
                                  ? FontWeight.bold
                                  : FontWeight.normal,
                            ),
                          ),
                        ),
                      ],
                    ),
                    Row(
                      children: [
                        Expanded(
                          child: TextField(
                            controller: input,
                            minLines: 1,
                            maxLines: 5,
                            decoration: InputDecoration(
                              hintText: lc(context, 'Write a message', '输入文字'),
                              border: const OutlineInputBorder(),
                            ),
                          ),
                        ),
                        IconButton(
                          tooltip: lc(context, 'Send', '发送'),
                          onPressed:
                              peer != null &&
                                  r.supportsText(widget.root) &&
                                  !busy
                              ? () => send(peer)
                              : null,
                          icon: const Icon(Icons.send_outlined),
                        ),
                      ],
                    ),
                    if (!r.supportsText(widget.root))
                      Align(
                        alignment: Alignment.centerLeft,
                        child: Text(
                          lc(
                            context,
                            'Device does not support text',
                            '设备不支持文本',
                          ),
                          style: const TextStyle(
                            color: Color(0xff737373),
                            fontSize: 12,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    },
  );
}

class _TextMessage extends StatefulWidget {
  const _TextMessage({
    super.key,
    required this.controller,
    required this.job,
    required this.onJob,
  });
  final LocalController controller;
  final LocalMap job;
  final VoidCallback onJob;
  @override
  State<_TextMessage> createState() => _TextMessageState();
}

class _TextMessageState extends State<_TextMessage> {
  bool revealed = false;
  late Future<({String value, bool sensitive})?> content;
  @override
  void initState() {
    super.initState();
    content = widget.controller.text(widget.job['id']);
  }

  @override
  void didUpdateWidget(covariant _TextMessage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller ||
        oldWidget.job['textManifest'] != widget.job['textManifest']) {
      content = widget.controller.text(widget.job['id']);
      revealed = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    final sent = widget.job['direction'] == 'send';
    return FutureBuilder(
      future: content,
      builder: (context, snapshot) {
        final value = snapshot.data;
        if (value == null) {
          return Align(
            alignment: sent ? Alignment.centerRight : Alignment.centerLeft,
            child: TextButton(
              onPressed: widget.onJob,
              child: Text(
                snapshot.hasError ||
                        localTerminal(widget.job['state']?['state'])
                    ? lc(context, 'Unavailable', '不可用')
                    : lc(context, 'Waiting', '等待中'),
              ),
            ),
          );
        }
        return Align(
          alignment: sent ? Alignment.centerRight : Alignment.centerLeft,
          child: Column(
            crossAxisAlignment: sent
                ? CrossAxisAlignment.end
                : CrossAxisAlignment.start,
            children: [
              Container(
                constraints: const BoxConstraints(maxWidth: 320),
                padding: const EdgeInsets.symmetric(
                  horizontal: 16,
                  vertical: 12,
                ),
                decoration: BoxDecoration(
                  color: const Color(0xfff4f4f5),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (value.sensitive)
                      Text(
                        lc(context, 'Sensitive', '敏感信息'),
                        style: const TextStyle(
                          color: Color(0xff737373),
                          fontSize: 12,
                        ),
                      ),
                    if (value.sensitive && !revealed)
                      const Text('••••••••••••')
                    else
                      MarkdownBody(
                        data: value.value,
                        selectable: true,
                        imageBuilder: (_, _, _) => const SizedBox.shrink(),
                      ),
                  ],
                ),
              ),
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (value.sensitive)
                    IconButton(
                      tooltip: revealed
                          ? lc(context, 'Hide', '隐藏')
                          : lc(context, 'Reveal', '显示'),
                      onPressed: () => setState(() => revealed = !revealed),
                      icon: Icon(
                        revealed
                            ? Icons.visibility_off_outlined
                            : Icons.visibility_outlined,
                      ),
                    ),
                  IconButton(
                    tooltip: lc(context, 'Copy', '复制'),
                    onPressed: () async {
                      try {
                        await Clipboard.setData(
                          ClipboardData(text: value.value),
                        );
                      } catch (_) {
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(
                              content: Text(
                                lc(context, 'Select text to copy', '请选择文字复制'),
                              ),
                            ),
                          );
                        }
                      }
                    },
                    icon: const Icon(Icons.copy_outlined),
                  ),
                ],
              ),
              TextButton(
                onPressed: widget.onJob,
                child: Text(
                  widget.job['state']?['state'] == 'COMPLETED'
                      ? sent
                            ? lc(context, 'Sent', '已发送')
                            : lc(context, 'Received', '已接收')
                      : localTerminal(widget.job['state']?['state']) ||
                            widget.job['error'] != null
                      ? lc(context, 'Unavailable', '不可用')
                      : lc(context, 'Waiting', '等待中'),
                  style: const TextStyle(
                    fontSize: 12,
                    color: Color(0xff737373),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
