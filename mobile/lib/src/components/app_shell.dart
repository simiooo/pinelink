import 'package:flutter/material.dart';
import '../api/pinelink_api.dart';
import '../i18n/controller.dart';
import '../pages/new_transfer_page.dart';
import '../pages/notifications_page.dart';
import '../pages/history_page.dart';
import '../pages/settings_page.dart';
import '../pages/tasks_page.dart';
import '../theme/tokens.dart';

enum AppPage { home, notifications, tasks, history, settings }

class PinelinkShell extends StatefulWidget {
  const PinelinkShell({
    super.key,
    required this.api,
    this.initialPage = AppPage.home,
  });
  final PinelinkApi api;
  final AppPage initialPage;
  @override
  State<PinelinkShell> createState() => _PinelinkShellState();
}

class _PinelinkShellState extends State<PinelinkShell> {
  late AppPage _page = widget.initialPage;
  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: PinelinkColors.paper,
    appBar: PinelinkAppBar(
      page: _page,
      onNavigate: (page) => setState(() => _page = page),
    ),
    body: SafeArea(
      child: switch (_page) {
        AppPage.home => NewTransferPage(api: widget.api),
        AppPage.notifications => NotificationsPage(api: widget.api),
        AppPage.tasks => TasksPage(api: widget.api),
        AppPage.history => HistoryPage(api: widget.api),
        AppPage.settings => SettingsPage(api: widget.api),
      },
    ),
  );
}

class PinelinkAppBar extends StatelessWidget implements PreferredSizeWidget {
  const PinelinkAppBar({
    super.key,
    required this.page,
    required this.onNavigate,
  });
  final AppPage page;
  final ValueChanged<AppPage> onNavigate;
  @override
  Size get preferredSize => const Size.fromHeight(76);

  @override
  Widget build(BuildContext context) => Material(
    color: PinelinkColors.paper,
    child: Container(
      height: 76,
      padding: const EdgeInsets.symmetric(horizontal: 22),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: PinelinkColors.line)),
      ),
      child: Row(
        children: [
          InkWell(
            onTap: () => onNavigate(AppPage.home),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Text(
                'Pinelink',
                style: const TextStyle(
                  fontSize: 23,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ),
          const Spacer(),
          if (page == AppPage.home) ...[
            _NavButton(
              label: tr(context, 'notifications'),
              onTap: () => onNavigate(AppPage.notifications),
            ),
            const SizedBox(width: 12),
          ],
          PopupMenuButton<String>(
            onSelected: (value) {
              if (value.startsWith('locale:')) {
                PinelinkI18n.of(context)?.setPreference(value.substring(7));
              } else {
                onNavigate(AppPage.values.byName(value));
              }
            },
            itemBuilder: (_) {
              final controller = PinelinkI18n.of(context);
              return [
                PopupMenuItem(
                  value: 'tasks',
                  child: Text(tr(context, 'currentTasks')),
                ),
                PopupMenuItem(
                  value: 'history',
                  child: Text(tr(context, 'history')),
                ),
                PopupMenuItem(
                  value: 'settings',
                  child: Text(tr(context, 'settings')),
                ),
                if (page != AppPage.home)
                  PopupMenuItem(
                    value: 'notifications',
                    child: Text(tr(context, 'fileNotifications')),
                  ),
                const PopupMenuDivider(),
                PopupMenuItem(
                  enabled: false,
                  child: Text(
                    tr(context, 'language'),
                    style: const TextStyle(
                      fontSize: 12,
                      color: PinelinkColors.muted,
                    ),
                  ),
                ),
                PopupMenuItem(
                  value: 'locale:system',
                  child: _LanguageOption(
                    label: tr(context, 'languageSystem'),
                    selected: controller?.preference == 'system',
                  ),
                ),
                PopupMenuItem(
                  value: 'locale:zh-CN',
                  child: _LanguageOption(
                    label: tr(context, 'languageZh'),
                    selected: controller?.preference == 'zh-CN',
                  ),
                ),
                PopupMenuItem(
                  value: 'locale:en-US',
                  child: _LanguageOption(
                    label: tr(context, 'languageEn'),
                    selected: controller?.preference == 'en-US',
                  ),
                ),
                if (controller?.hasSavedPreference == true)
                  PopupMenuItem(
                    enabled: false,
                    child: Text(
                      tr(context, 'languageSaved'),
                      style: const TextStyle(fontSize: 12),
                    ),
                  ),
              ];
            },
            child: _NavButton(label: tr(context, 'more')),
          ),
        ],
      ),
    ),
  );
}

class _LanguageOption extends StatelessWidget {
  const _LanguageOption({required this.label, required this.selected});
  final String label;
  final bool selected;
  @override
  Widget build(BuildContext context) => Row(
    children: [
      Expanded(child: Text(label)),
      if (selected)
        const Icon(Icons.check, size: 16, color: PinelinkColors.accent),
    ],
  );
}

class _NavButton extends StatelessWidget {
  const _NavButton({required this.label, this.onTap});
  final String label;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => InkWell(
    onTap: onTap,
    borderRadius: BorderRadius.circular(12),
    child: Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
      decoration: BoxDecoration(
        color: PinelinkColors.background,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: PinelinkColors.line),
      ),
      child: Text(
        label,
        style: const TextStyle(fontSize: 12, color: PinelinkColors.muted),
      ),
    ),
  );
}
