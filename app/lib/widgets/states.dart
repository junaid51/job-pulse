import 'package:flutter/material.dart';

/// The three things a screen shows when it has no list to show. Shared because
/// all three screens need all three.
class LoadingView extends StatelessWidget {
  const LoadingView({super.key});

  @override
  Widget build(BuildContext context) => const Center(
    child: SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)),
  );
}

class ErrorView extends StatelessWidget {
  const ErrorView({required this.message, this.onRetry, super.key});

  final String message;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(message, textAlign: TextAlign.center, style: TextStyle(color: colors.onSurface)),
            if (onRetry != null) ...[
              const SizedBox(height: 16),
              TextButton(onPressed: onRetry, child: const Text('Try again')),
            ],
          ],
        ),
      ),
    );
  }
}

class EmptyView extends StatelessWidget {
  const EmptyView({required this.title, this.detail, super.key});

  final String title;
  final String? detail;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(
              title,
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: colors.onSurface),
            ),
            if (detail != null) ...[
              const SizedBox(height: 6),
              Text(
                detail!,
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 13.5, height: 1.4, color: colors.onSurfaceVariant),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// Wraps a full-screen state (empty, error) so a RefreshIndicator above it
/// still works: pull-to-refresh needs a scrollable descendant, and an empty
/// list is exactly when you want to pull.
class RefreshableState extends StatelessWidget {
  const RefreshableState({required this.child, super.key});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) => SingleChildScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        child: SizedBox(
          height: constraints.maxHeight,
          width: constraints.maxWidth,
          child: child,
        ),
      ),
    );
  }
}
