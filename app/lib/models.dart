/// The three shapes the API returns. Parsing only: nothing here is sent back to
/// the server except through the explicit arguments on JobPulseApi.
library;

class Job {
  const Job({
    required this.id,
    required this.provider,
    required this.company,
    required this.title,
    required this.location,
    required this.remote,
    required this.url,
    required this.matchedAt,
    this.postedAt,
    this.seenAt,
  });

  final int id;
  final String provider;
  final String company;
  final String title;
  final String location;
  final bool remote;

  /// The official application page. This is what the Apply button opens.
  final String url;

  final DateTime matchedAt;
  final DateTime? postedAt;
  final DateTime? seenAt;

  bool get unread => seenAt == null;

  factory Job.fromJson(Map<String, dynamic> json) => Job(
    id: json['id'] as int,
    provider: json['provider'] as String? ?? '',
    company: json['company'] as String? ?? '',
    title: json['title'] as String? ?? '',
    location: json['location'] as String? ?? '',
    remote: json['remote'] as bool? ?? false,
    url: json['url'] as String? ?? '',
    matchedAt: parseTime(json['matched_at']) ?? DateTime.now(),
    postedAt: parseTime(json['posted_at']),
    seenAt: parseTime(json['seen_at']),
  );
}

class Profile {
  const Profile({
    required this.id,
    required this.name,
    required this.keywords,
    required this.locations,
    required this.remoteOnly,
  });

  final int id;
  final String name;
  final List<String> keywords;
  final List<String> locations;
  final bool remoteOnly;

  factory Profile.fromJson(Map<String, dynamic> json) => Profile(
    id: json['id'] as int,
    name: json['name'] as String? ?? '',
    keywords: stringList(json['keywords']),
    locations: stringList(json['locations']),
    remoteOnly: json['remote_only'] as bool? ?? false,
  );
}

/// One match event: a job plus the profile that matched it.
class MatchEvent {
  const MatchEvent({required this.profileId, required this.profileName, required this.job});

  final int profileId;
  final String profileName;
  final Job job;

  factory MatchEvent.fromJson(Map<String, dynamic> json) => MatchEvent(
    profileId: json['profile_id'] as int,
    profileName: json['profile_name'] as String? ?? '',
    job: Job.fromJson(json['job'] as Map<String, dynamic>),
  );
}

class Notifications {
  const Notifications({required this.events, required this.unread});

  final List<MatchEvent> events;
  final int unread;
}

DateTime? parseTime(Object? value) {
  if (value is! String || value.isEmpty) return null;
  return DateTime.tryParse(value)?.toLocal();
}

List<String> stringList(Object? value) =>
    value is List ? value.map((v) => v.toString()).toList() : const [];
