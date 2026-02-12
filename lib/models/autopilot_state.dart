enum AutoPilotStatus {
  stopped,
  monitoring,
  checking,
  recovering,
  stabilizing,
  error,
}

class AutoPilotState {
  final AutoPilotStatus status;
  final int failCount;
  final String? message;
  final DateTime? lastCheck;
  final bool hasInternet;

  const AutoPilotState({
    required this.status,
    required this.failCount,
    this.message,
    this.lastCheck,
    required this.hasInternet,
  });

  AutoPilotState copyWith({
    AutoPilotStatus? status,
    int? failCount,
    String? message,
    DateTime? lastCheck,
    bool? hasInternet,
  }) {
    return AutoPilotState(
      status: status ?? this.status,
      failCount: failCount ?? this.failCount,
      message: message ?? this.message,
      lastCheck: lastCheck ?? this.lastCheck,
      hasInternet: hasInternet ?? this.hasInternet,
    );
  }
}
