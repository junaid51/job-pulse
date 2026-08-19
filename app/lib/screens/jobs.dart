import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api.dart' show describeError;
import '../models.dart';
import '../providers.dart';
import '../widgets/job_tile.dart';
import '../widgets/states.dart';

/// Jobs: the matches for one profile, newest first.
class JobsScreen extends ConsumerWidget {
  const JobsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profiles = ref.watch(profilesProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Jobs'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            icon: const Icon(Icons.refresh, size: 22),
            onPressed: () => refreshFeeds(ref),
          ),
        ],
      ),
      body: switch (profiles) {
        AsyncError(:final error) => ErrorView(
          message: describeError(error),
          onRetry: () => ref.invalidate(profilesProvider),
        ),
        AsyncData(:final value) when value.isEmpty => const EmptyView(
          title: 'No search profiles yet',
          detail: 'Add one in Settings — keywords, locations, and whether it has to be remote.',
        ),
        AsyncData(:final value) => _Jobs(profiles: value),
        _ => const LoadingView(),
      },
    );
  }
}

class _Jobs extends ConsumerWidget {
  const _Jobs({required this.profiles});

  final List<Profile> profiles;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Null selection means the first profile, so this works before any tap and
    // recovers on its own if the selected profile is deleted.
    final selected = ref.watch(selectedProfileProvider);
    final profile = profiles.firstWhere((p) => p.id == selected, orElse: () => profiles.first);
    final jobs = ref.watch(jobsProvider(profile.id));

    return Column(
      children: [
        if (profiles.length > 1) _ProfileChips(profiles: profiles, selected: profile),
        Expanded(
          child: RefreshIndicator(
            onRefresh: () async {
              await refreshFeeds(ref);
              await ref.read(jobsProvider(profile.id).future);
            },
            child: switch (jobs) {
              AsyncError(:final error) => RefreshableState(
                child: ErrorView(
                  message: describeError(error),
                  onRetry: () => ref.invalidate(jobsProvider(profile.id)),
                ),
              ),
              AsyncData(:final value) when value.isEmpty => const RefreshableState(
                child: EmptyView(
                  title: 'Nothing matched yet',
                  detail: 'The next poll may find something. Pull down to check now.',
                ),
              ),
              AsyncData(:final value) => ListView.separated(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.only(bottom: 24),
                itemCount: value.length,
                separatorBuilder: (_, _) => const Divider(),
                itemBuilder: (_, i) => JobTile(job: value[i]),
              ),
              _ => const LoadingView(),
            },
          ),
        ),
      ],
    );
  }
}

class _ProfileChips extends ConsumerWidget {
  const _ProfileChips({required this.profiles, required this.selected});

  final List<Profile> profiles;
  final Profile selected;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return SizedBox(
      height: 48,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
        itemCount: profiles.length,
        separatorBuilder: (_, _) => const SizedBox(width: 8),
        itemBuilder: (_, i) {
          final profile = profiles[i];
          return ChoiceChip(
            label: Text(profile.name),
            selected: profile.id == selected.id,
            onSelected: (_) => ref.read(selectedProfileProvider.notifier).select(profile.id),
          );
        },
      ),
    );
  }
}
