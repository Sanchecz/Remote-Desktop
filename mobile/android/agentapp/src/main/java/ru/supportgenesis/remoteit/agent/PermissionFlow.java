package ru.supportgenesis.remoteit.agent;

import java.util.Locale;

final class PermissionFlow {
    private PermissionFlow() { }

    static State evaluate(boolean accessibilityEnabled, boolean screenSharing) {
        int completed = (accessibilityEnabled ? 1 : 0) + (screenSharing ? 1 : 0);
        return new State(completed, 2, accessibilityEnabled && screenSharing, accessibilityEnabled && !screenSharing);
    }

    static String accessibilitySectionFor(String manufacturer) {
        String normalized = manufacturer == null ? "" : manufacturer.toLowerCase(Locale.ROOT);
        if (normalized.contains("samsung")) return "«Установленные приложения»";
        if (normalized.contains("xiaomi") || normalized.contains("redmi") || normalized.contains("poco")) return "«Скачанные приложения»";
        if (normalized.contains("huawei") || normalized.contains("honor")) return "«Установленные службы»";
        return "«Установленные приложения» или «Скачанные приложения»";
    }

    static final class State {
        final int completed;
        final int total;
        final boolean ready;
        final boolean canStartSharing;

        State(int completed, int total, boolean ready, boolean canStartSharing) {
            this.completed = completed;
            this.total = total;
            this.ready = ready;
            this.canStartSharing = canStartSharing;
        }
    }
}
