/// Small display helpers. Deliberately not a package: "3d" and a capitalised
/// provider name is the whole requirement.
library;

/// shortAgo renders a compact age: 5m, 3h, 12d. Anything older than a year is
/// rare enough on a job board to fall back to years.
String shortAgo(DateTime? time) {
  if (time == null) return '';
  final elapsed = DateTime.now().difference(time);
  if (elapsed.inMinutes < 1) return 'now';
  if (elapsed.inMinutes < 60) return '${elapsed.inMinutes}m';
  if (elapsed.inHours < 24) return '${elapsed.inHours}h';
  if (elapsed.inDays < 365) return '${elapsed.inDays}d';
  return '${elapsed.inDays ~/ 365}y';
}

/// providerLabel turns a registry key into something worth showing.
String providerLabel(String provider) => switch (provider) {
  'greenhouse' => 'Greenhouse',
  'lever' => 'Lever',
  'ashby' => 'Ashby',
  'smartrecruiters' => 'SmartRecruiters',
  'workable' => 'Workable',
  'recruitee' => 'Recruitee',
  _ => provider,
};
