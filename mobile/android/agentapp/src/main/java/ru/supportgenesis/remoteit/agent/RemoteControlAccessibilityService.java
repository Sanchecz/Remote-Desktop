package ru.supportgenesis.remoteit.agent;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.GestureDescription;
import android.content.ComponentName;
import android.content.Context;
import android.graphics.Path;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;

import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;

public final class RemoteControlAccessibilityService extends AccessibilityService {
    private static volatile RemoteControlAccessibilityService instance;
    private static final Handler MAIN = new Handler(Looper.getMainLooper());
    private final List<float[]> pointerPath = new ArrayList<>();
    private boolean pointerDown;
    private String pointerButton = "left";

    static boolean isEnabled(Context context) {
        String enabled = Settings.Secure.getString(context.getContentResolver(), Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES);
        if (enabled == null) return false;
        ComponentName expected = new ComponentName(context, RemoteControlAccessibilityService.class);
        for (String value : enabled.split(":")) {
            ComponentName candidate = ComponentName.unflattenFromString(value);
            if (expected.equals(candidate)) return true;
        }
        return false;
    }

    static boolean dispatch(JSONObject event) {
        RemoteControlAccessibilityService service = instance;
        if (service == null) return false;
        MAIN.post(() -> service.applyRemoteEvent(event));
        return true;
    }

    @Override
    protected void onServiceConnected() {
        super.onServiceConnected();
        instance = this;
    }

    @Override
    public void onDestroy() {
        if (instance == this) instance = null;
        super.onDestroy();
    }

    @Override public void onAccessibilityEvent(AccessibilityEvent event) { }
    @Override public void onInterrupt() { }

    private void applyRemoteEvent(JSONObject event) {
        String type = event.optString("type");
        if ("pointer".equals(type)) applyPointer(event);
        else if ("wheel".equals(type)) applyWheel(event.optInt("delta"));
        else if ("text".equals(type)) appendText(event.optString("text"));
        else if ("key".equals(type) && !"up".equalsIgnoreCase(event.optString("action"))) applyKey(event.optInt("keyCode"));
    }

    private float[] screenPoint(JSONObject event) {
        int coordinateWidth = Math.max(1, event.optInt("coordinateWidth", 1));
        int coordinateHeight = Math.max(1, event.optInt("coordinateHeight", 1));
        float width = getResources().getDisplayMetrics().widthPixels;
        float height = getResources().getDisplayMetrics().heightPixels;
        float x = Math.max(0, Math.min(width - 1, event.optInt("x") * width / coordinateWidth));
        float y = Math.max(0, Math.min(height - 1, event.optInt("y") * height / coordinateHeight));
        return new float[]{x, y};
    }

    private void applyPointer(JSONObject event) {
        String action = event.optString("action", "move");
        float[] point = screenPoint(event);
        if ("down".equals(action)) {
            pointerDown = true;
            pointerButton = event.optString("button", "left");
            pointerPath.clear();
            pointerPath.add(point);
            return;
        }
        if (pointerDown && "move".equals(action)) {
            if (pointerPath.size() < 80) pointerPath.add(point);
            else pointerPath.set(pointerPath.size() - 1, point);
            return;
        }
        if (!"up".equals(action)) return;
        if (!pointerDown) pointerPath.add(point);
        else pointerPath.add(point);
        pointerDown = false;
        Path path = new Path();
        float[] first = pointerPath.get(0);
        path.moveTo(first[0], first[1]);
        float distance = 0;
        float[] previous = first;
        for (float[] sample : pointerPath) {
            path.lineTo(sample[0], sample[1]);
            distance += Math.hypot(sample[0] - previous[0], sample[1] - previous[1]);
            previous = sample;
        }
        long duration = "right".equals(pointerButton) ? 650 : distance > 12 ? Math.min(900, Math.max(120, pointerPath.size() * 18L)) : 45;
        GestureDescription gesture = new GestureDescription.Builder().addStroke(new GestureDescription.StrokeDescription(path, 0, duration)).build();
        dispatchGesture(gesture, null, null);
        pointerPath.clear();
    }

    private void applyWheel(int delta) {
        float width = getResources().getDisplayMetrics().widthPixels;
        float height = getResources().getDisplayMetrics().heightPixels;
        float startY = delta > 0 ? height * .35f : height * .68f;
        float endY = delta > 0 ? height * .68f : height * .35f;
        Path path = new Path(); path.moveTo(width * .72f, startY); path.lineTo(width * .72f, endY);
        dispatchGesture(new GestureDescription.Builder().addStroke(new GestureDescription.StrokeDescription(path, 0, 180)).build(), null, null);
    }

    private void applyKey(int keyCode) {
        if (keyCode == 8) removeLastCharacter();
        else if (keyCode == 13) appendText("\n");
        else if (keyCode == 27) performGlobalAction(GLOBAL_ACTION_BACK);
        else if (keyCode == 37 || keyCode == 38) applyWheel(120);
        else if (keyCode == 39 || keyCode == 40) applyWheel(-120);
    }

    private AccessibilityNodeInfo focusedEditableNode() {
        AccessibilityNodeInfo root = getRootInActiveWindow();
        if (root == null) return null;
        AccessibilityNodeInfo focused = root.findFocus(AccessibilityNodeInfo.FOCUS_INPUT);
        if (focused != null && focused.isEditable()) return focused;
        return null;
    }

    private void appendText(String text) {
        if (text == null || text.isEmpty()) return;
        AccessibilityNodeInfo node = focusedEditableNode();
        if (node == null) return;
        CharSequence current = node.getText();
        android.os.Bundle arguments = new android.os.Bundle();
        arguments.putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, (current == null ? "" : current.toString()) + text);
        node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, arguments);
        recycleCompatibility(node);
    }

    private void removeLastCharacter() {
        AccessibilityNodeInfo node = focusedEditableNode();
        if (node == null) return;
        String current = node.getText() == null ? "" : node.getText().toString();
        if (!current.isEmpty()) current = current.substring(0, current.offsetByCodePoints(current.length(), -1));
        android.os.Bundle arguments = new android.os.Bundle();
        arguments.putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, current);
        node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, arguments);
        recycleCompatibility(node);
    }

    @SuppressWarnings("deprecation")
    private void recycleCompatibility(AccessibilityNodeInfo node) {
        if (android.os.Build.VERSION.SDK_INT < 33) node.recycle();
    }
}
