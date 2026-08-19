import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api.dart';
import '../models.dart';
import '../providers.dart';
import '../widgets/states.dart';

/// Settings: the search profiles, and where the backend is.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profiles = ref.watch(profilesProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Settings'),
        actions: [
          IconButton(
            tooltip: 'New profile',
            icon: const Icon(Icons.add, size: 22),
            onPressed: () => _editProfile(context, ref, null),
          ),
        ],
      ),
      body: ListView(
        children: [
          _SectionHeader('Search profiles'),
          switch (profiles) {
            AsyncError(:final error) => ErrorView(
              message: describeError(error),
              onRetry: () => ref.invalidate(profilesProvider),
            ),
            AsyncData(:final value) when value.isEmpty => const Padding(
              padding: EdgeInsets.fromLTRB(16, 8, 16, 24),
              child: Text('None yet. Use + to add one.'),
            ),
            AsyncData(:final value) => Column(
              children: [
                for (final profile in value)
                  _ProfileRow(
                    profile: profile,
                    onEdit: () => _editProfile(context, ref, profile),
                    onDelete: () => _deleteProfile(context, ref, profile),
                  ),
              ],
            ),
            _ => const Padding(padding: EdgeInsets.all(24), child: LoadingView()),
          },
          const SizedBox(height: 8),
          _SectionHeader('Backend'),
          _InfoRow(label: 'URL', value: ref.watch(apiProvider).baseUrl),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 6, 16, 16),
            child: Text(
              'Set at launch with --dart-define=JOBPULSE_API=http://192.168.1.20:8080 — a phone '
              'cannot reach localhost, so use your computer\'s address on the network.',
              style: TextStyle(
                fontSize: 12.5,
                height: 1.45,
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
          ),
          // TODO(M4): show the FCM registration token here, with a copy button.
          const SizedBox(height: 24),
        ],
      ),
    );
  }
}

Future<void> _deleteProfile(BuildContext context, WidgetRef ref, Profile profile) async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: Text('Delete "${profile.name}"?'),
      content: const Text('Its matched jobs disappear with it. The jobs themselves stay.'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
        TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('Delete')),
      ],
    ),
  );
  if (confirmed != true || !context.mounted) return;

  final messenger = ScaffoldMessenger.of(context);
  try {
    await ref.read(apiProvider).deleteProfile(profile.id);
    ref.invalidate(profilesProvider);
    ref.invalidate(notificationsProvider);
  } on Object catch (error) {
    messenger.showSnackBar(SnackBar(content: Text(describeError(error))));
  }
}

/// The editor is one sheet for both creating and editing: the fields are the same
/// four either way.
Future<void> _editProfile(BuildContext context, WidgetRef ref, Profile? existing) async {
  final saved = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    builder: (context) => _ProfileForm(existing: existing),
  );
  if (saved == true) {
    ref.invalidate(profilesProvider);
    ref.invalidate(notificationsProvider);
    if (existing != null) ref.invalidate(jobsProvider(existing.id));
  }
}

class _ProfileForm extends ConsumerStatefulWidget {
  const _ProfileForm({required this.existing});

  final Profile? existing;

  @override
  ConsumerState<_ProfileForm> createState() => _ProfileFormState();
}

class _ProfileFormState extends ConsumerState<_ProfileForm> {
  late final TextEditingController _name;
  late final TextEditingController _keywords;
  late final TextEditingController _locations;
  late bool _remoteOnly;
  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final existing = widget.existing;
    _name = TextEditingController(text: existing?.name ?? '');
    _keywords = TextEditingController(text: existing?.keywords.join(', ') ?? '');
    _locations = TextEditingController(text: existing?.locations.join(', ') ?? '');
    _remoteOnly = existing?.remoteOnly ?? false;
  }

  @override
  void dispose() {
    _name.dispose();
    _keywords.dispose();
    _locations.dispose();
    super.dispose();
  }

  List<String> _split(TextEditingController controller) => controller.text
      .split(',')
      .map((value) => value.trim())
      .where((value) => value.isNotEmpty)
      .toList();

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    final api = ref.read(apiProvider);
    final existing = widget.existing;
    try {
      if (existing == null) {
        await api.createProfile(
          name: _name.text.trim(),
          keywords: _split(_keywords),
          locations: _split(_locations),
          remoteOnly: _remoteOnly,
        );
      } else {
        await api.updateProfile(
          id: existing.id,
          name: _name.text.trim(),
          keywords: _split(_keywords),
          locations: _split(_locations),
          remoteOnly: _remoteOnly,
        );
      }
      if (mounted) Navigator.pop(context, true);
    } on Object catch (error) {
      setState(() {
        _saving = false;
        _error = describeError(error);
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Padding(
      padding: EdgeInsets.fromLTRB(20, 20, 20, MediaQuery.viewInsetsOf(context).bottom + 20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            widget.existing == null ? 'New profile' : 'Edit profile',
            style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _name,
            autofocus: widget.existing == null,
            decoration: const InputDecoration(labelText: 'Name', hintText: 'Backend Go'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _keywords,
            decoration: const InputDecoration(
              labelText: 'Keywords',
              hintText: 'go, backend, platform',
              helperText: 'Comma separated. Any one of them in the title is a match.',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _locations,
            decoration: const InputDecoration(
              labelText: 'Locations',
              hintText: 'berlin, remote',
              helperText: 'Comma separated. Leave empty for anywhere.',
            ),
          ),
          const SizedBox(height: 4),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('Remote only', style: TextStyle(fontSize: 14.5)),
            value: _remoteOnly,
            onChanged: (value) => setState(() => _remoteOnly = value),
          ),
          if (_error != null) ...[
            const SizedBox(height: 4),
            Text(_error!, style: TextStyle(color: colors.error, fontSize: 13)),
          ],
          const SizedBox(height: 12),
          FilledButton(
            onPressed: _saving ? null : _save,
            child: Text(_saving ? 'Saving…' : 'Save'),
          ),
        ],
      ),
    );
  }
}

class _ProfileRow extends StatelessWidget {
  const _ProfileRow({required this.profile, required this.onEdit, required this.onDelete});

  final Profile profile;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    final terms = [
      ...profile.keywords,
      ...profile.locations.map((location) => '@$location'),
      if (profile.remoteOnly) 'remote only',
    ];

    return Column(
      children: [
        ListTile(
          onTap: onEdit,
          title: Text(
            profile.name,
            style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600),
          ),
          subtitle: Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              terms.isEmpty ? 'Everything' : terms.join('  ·  '),
              style: TextStyle(fontSize: 12.5, color: colors.onSurfaceVariant),
            ),
          ),
          trailing: IconButton(
            tooltip: 'Delete',
            icon: const Icon(Icons.delete_outline, size: 20),
            onPressed: onDelete,
          ),
        ),
        const Divider(),
      ],
    );
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader(this.title);

  final String title;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
    child: Text(
      title.toUpperCase(),
      style: TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w600,
        letterSpacing: 0.7,
        color: Theme.of(context).colorScheme.onSurfaceVariant,
      ),
    ),
  );
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.fromLTRB(16, 4, 16, 4),
    child: Row(
      children: [
        Text(label, style: const TextStyle(fontSize: 14)),
        const Spacer(),
        Flexible(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: TextStyle(
              fontSize: 13,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
          ),
        ),
      ],
    ),
  );
}
