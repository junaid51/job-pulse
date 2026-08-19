import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../format.dart';
import '../models.dart';

/// One row of the Jobs and Notifications lists: title, then a line of metadata,
/// with Apply on the right. Tapping anywhere does the same thing as Apply.
class JobTile extends StatelessWidget {
  const JobTile({required this.job, this.label, this.showUnread = false, super.key});

  final Job job;

  /// An extra leading tag, used by Notifications to name the profile that matched.
  final String? label;
  final bool showUnread;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    // Boards often already say "Remote" in the location, so only add the tag when
    // it would tell you something new.
    final locationSaysRemote = job.location.toLowerCase().contains('remote');
    final meta = [
      job.company,
      if (job.location.isNotEmpty) job.location,
      if (job.remote && !locationSaysRemote) 'Remote',
      providerLabel(job.provider),
    ].join('  ·  ');

    return InkWell(
      onTap: () => openApplyPage(context, job),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 13, 8, 13),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (showUnread)
              Container(
                width: 6,
                height: 6,
                margin: const EdgeInsets.only(top: 6, right: 10),
                decoration: BoxDecoration(
                  color: job.unread ? colors.primary : Colors.transparent,
                  shape: BoxShape.circle,
                ),
              ),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (label != null) ...[
                    Text(
                      label!.toUpperCase(),
                      style: TextStyle(
                        fontSize: 10.5,
                        fontWeight: FontWeight.w600,
                        letterSpacing: 0.6,
                        color: colors.primary,
                      ),
                    ),
                    const SizedBox(height: 4),
                  ],
                  Text(
                    job.title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w600,
                      height: 1.25,
                      letterSpacing: -0.2,
                      color: colors.onSurface,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    meta,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 12.5,
                      color: colors.onSurfaceVariant,
                      fontFeatures: const [FontFeature.tabularFigures()],
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text(
                  shortAgo(job.postedAt ?? job.matchedAt),
                  style: TextStyle(
                    fontSize: 12,
                    color: colors.onSurfaceVariant,
                    fontFeatures: const [FontFeature.tabularFigures()],
                  ),
                ),
                const SizedBox(height: 2),
                TextButton(
                  onPressed: () => openApplyPage(context, job),
                  style: TextButton.styleFrom(
                    minimumSize: const Size(0, 30),
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                    visualDensity: VisualDensity.compact,
                  ),
                  child: const Text('Apply', style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Opens the posting in the browser rather than a web view: applying means
/// forms, uploads and logins, which belong in a real browser.
Future<void> openApplyPage(BuildContext context, Job job) async {
  final messenger = ScaffoldMessenger.of(context);
  final uri = Uri.tryParse(job.url);
  if (uri == null || !await launchUrl(uri, mode: LaunchMode.externalApplication)) {
    messenger.showSnackBar(SnackBar(content: Text('Could not open ${job.url}')));
  }
}
