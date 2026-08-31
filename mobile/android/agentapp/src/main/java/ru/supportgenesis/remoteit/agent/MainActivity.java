package ru.supportgenesis.remoteit.agent;

import android.Manifest;
import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final int PROJECTION_REQUEST = 4101;
    private static final int NOTIFICATION_REQUEST = 4102;
    private final ExecutorService network = Executors.newSingleThreadExecutor();
    private LinearLayout content;
    private AgentStore store;
    private TextView state;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        store = AgentStore.load(this);
        render();
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (content != null && store.enrolled()) render();
    }

    private void render() {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(Color.rgb(245, 248, 247));
        content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setGravity(Gravity.CENTER_HORIZONTAL);
        content.setPadding(dp(22), dp(26), dp(22), dp(30));
        scroll.addView(content, new ScrollView.LayoutParams(ScrollView.LayoutParams.MATCH_PARENT, ScrollView.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);

        TextView badge = text("REMOTEIT · AGENT ДЛЯ ANDROID", 12, Color.rgb(8, 168, 115), true);
        badge.setLetterSpacing(.12f);
        content.addView(badge, match());
        TextView title = text(store.enrolled() ? store.deviceName : "Подключить этот телефон", 28, Color.rgb(13, 31, 43), true);
        title.setPadding(0, dp(10), 0, dp(5));
        content.addView(title, match());
        TextView subtitle = text(store.enrolled() ? "Безопасная помощь с явным разрешением владельца телефона" : "Введите токен регистрации из панели администратора", 14, Color.rgb(91, 112, 124), false);
        subtitle.setPadding(0, 0, 0, dp(22));
        content.addView(subtitle, match());

        if (!store.enrolled()) renderEnrollment(); else renderControls();
    }

    private void renderEnrollment() {
        EditText server = input("Адрес сервера", "https://supportgenesis.ru", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        EditText token = input("Токен регистрации", "Вставьте токен", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
		String clipboardToken = enrollmentTokenFromClipboard();
		if (!clipboardToken.isEmpty()) token.setText(clipboardToken);
        EditText name = input("Название устройства", Build.MODEL, InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);
        content.addView(server, field()); content.addView(token, field()); content.addView(name, field());
        state = text("", 13, Color.rgb(204, 55, 55), false); state.setPadding(0, dp(8), 0, dp(8)); content.addView(state, match());
        Button enroll = primary("Зарегистрировать телефон");
        enroll.setOnClickListener(view -> {
            if (token.getText().toString().trim().isEmpty() || name.getText().toString().trim().isEmpty()) {
                state.setText("Укажите токен и название устройства"); return;
            }
            enroll.setEnabled(false); state.setTextColor(Color.rgb(91, 112, 124)); state.setText("Проверяем токен и создаём Remote ID…");
            network.execute(() -> {
                try {
                    AgentStore enrolled = AgentStore.enroll(this, server.getText().toString(), token.getText().toString(), name.getText().toString());
                    runOnUiThread(() -> { store = enrolled; render(); });
                } catch (Exception error) {
                    runOnUiThread(() -> { enroll.setEnabled(true); state.setTextColor(Color.rgb(204, 55, 55)); state.setText(error.getMessage()); });
                }
            });
        });
        content.addView(enroll, matchHeight(50));
        content.addView(note("Токен используется один раз. Захват экрана нельзя включить удалённо без системного окна Android."), match());
    }

	private String enrollmentTokenFromClipboard() {
		ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
		if (clipboard == null || !clipboard.hasPrimaryClip()) return "";
		ClipData clip = clipboard.getPrimaryClip();
		if (clip == null || clip.getItemCount() == 0) return "";
		CharSequence value = clip.getItemAt(0).coerceToText(this);
		String candidate = value == null ? "" : value.toString().trim();
		if (candidate.length() < 24 || candidate.length() > 512 || candidate.contains("\n") || candidate.contains("\r")) return "";
		return candidate;
	}

    private void renderControls() {
        LinearLayout idCard = card();
        idCard.addView(text("REMOTE ID", 11, Color.rgb(91, 112, 124), true), match());
        TextView code = text(store.connectionCode, 27, Color.rgb(13, 31, 43), true); code.setLetterSpacing(.08f); code.setPadding(0, dp(5), 0, dp(2)); idCard.addView(code, match());
        idCard.addView(text("Версия Agent " + AgentStore.VERSION, 11, Color.rgb(91, 112, 124), false), match());
        content.addView(idCard, cardParams());

        boolean accessibility = RemoteControlAccessibilityService.isEnabled(this);
        content.addView(step("1", "Удалённое управление", accessibility ? "Разрешено — касания и прокрутка доступны" : "Откройте системный экран и включите RemoteIt"), match());
        Button accessibilityButton = secondary(accessibility ? "Проверить разрешение" : "Разрешить управление");
        accessibilityButton.setOnClickListener(view -> startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)));
        content.addView(accessibilityButton, matchHeight(46));

        content.addView(step("2", "Трансляция экрана", ScreenShareService.running() ? "Активна — в панели можно подключаться" : "Запускается только после системного подтверждения"), match());
        Button share = primary(ScreenShareService.running() ? "Трансляция уже активна" : "Начать безопасный доступ");
        share.setEnabled(!ScreenShareService.running());
        share.setOnClickListener(view -> requestProjection());
        content.addView(share, matchHeight(50));
        Button stop = secondary("Остановить трансляцию");
        stop.setEnabled(ScreenShareService.running());
        stop.setOnClickListener(view -> { Intent intent = new Intent(this, ScreenShareService.class); intent.setAction(ScreenShareService.ACTION_STOP); startService(intent); view.postDelayed(this::render, 250); });
        content.addView(stop, matchHeight(46));

        state = text(accessibility ? "Готово: администратор увидит устройство после запуска трансляции." : "Просмотр будет работать, но управление потребует разрешения из шага 1.", 13, accessibility ? Color.rgb(8, 143, 96) : Color.rgb(183, 112, 20), false);
        state.setPadding(0, dp(15), 0, dp(12)); content.addView(state, match());
        Button reset = textButton("Отвязать этот телефон");
        reset.setOnClickListener(view -> { if (ScreenShareService.running()) Toast.makeText(this, "Сначала остановите трансляцию", Toast.LENGTH_SHORT).show(); else { AgentStore.clear(this); store = AgentStore.load(this); render(); } });
        content.addView(reset, matchHeight(42));
    }

    private void requestProjection() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, NOTIFICATION_REQUEST);
        }
        MediaProjectionManager manager = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
        startActivityForResult(manager.createScreenCaptureIntent(), PROJECTION_REQUEST);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != PROJECTION_REQUEST) return;
        if (resultCode != RESULT_OK || data == null) { Toast.makeText(this, "Трансляция не запущена", Toast.LENGTH_SHORT).show(); return; }
        Intent service = new Intent(this, ScreenShareService.class);
        service.setAction(ScreenShareService.ACTION_START);
        service.putExtra(ScreenShareService.EXTRA_RESULT_CODE, resultCode);
        service.putExtra(ScreenShareService.EXTRA_RESULT_DATA, data);
        startForegroundService(service);
        content.postDelayed(this::render, 550);
    }

    private LinearLayout step(String number, String title, String detail) {
        LinearLayout row = new LinearLayout(this); row.setOrientation(LinearLayout.HORIZONTAL); row.setGravity(Gravity.CENTER_VERTICAL); row.setPadding(0, dp(17), 0, dp(8));
        TextView marker = text(number, 14, Color.WHITE, true); marker.setGravity(Gravity.CENTER); marker.setBackgroundColor(Color.rgb(8, 168, 115)); row.addView(marker, new LinearLayout.LayoutParams(dp(30), dp(30)));
        LinearLayout copy = new LinearLayout(this); copy.setOrientation(LinearLayout.VERTICAL); copy.setPadding(dp(10), 0, 0, 0); copy.addView(text(title, 14, Color.rgb(13, 31, 43), true), match()); copy.addView(text(detail, 11, Color.rgb(91, 112, 124), false), match()); row.addView(copy, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1)); return row;
    }

    private EditText input(String label, String hint, int type) { EditText value = new EditText(this); value.setHint(label + " · " + hint); value.setTextSize(15); value.setSingleLine(true); value.setInputType(type); value.setPadding(dp(13), 0, dp(13), 0); value.setBackgroundColor(Color.WHITE); return value; }
    private LinearLayout card() { LinearLayout card = new LinearLayout(this); card.setOrientation(LinearLayout.VERTICAL); card.setPadding(dp(16), dp(15), dp(16), dp(15)); card.setBackgroundColor(Color.WHITE); return card; }
    private TextView note(String value) { TextView note = text(value, 12, Color.rgb(91, 112, 124), false); note.setPadding(0, dp(16), 0, 0); note.setGravity(Gravity.CENTER); return note; }
    private Button primary(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.WHITE); button.setTextSize(14); button.setAllCaps(false); button.setBackgroundColor(Color.rgb(8, 168, 115)); return button; }
    private Button secondary(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.rgb(13, 91, 67)); button.setTextSize(14); button.setAllCaps(false); button.setBackgroundColor(Color.WHITE); return button; }
    private Button textButton(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.rgb(123, 137, 145)); button.setTextSize(12); button.setAllCaps(false); button.setBackgroundColor(Color.TRANSPARENT); return button; }
    private TextView text(String value, int size, int color, boolean bold) { TextView view = new TextView(this); view.setText(value); view.setTextSize(size); view.setTextColor(color); if (bold) view.setTypeface(android.graphics.Typeface.DEFAULT, android.graphics.Typeface.BOLD); return view; }
    private LinearLayout.LayoutParams match() { return new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT); }
    private LinearLayout.LayoutParams matchHeight(int height) { LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(height)); params.setMargins(0, dp(5), 0, dp(5)); return params; }
    private LinearLayout.LayoutParams field() { LinearLayout.LayoutParams params = matchHeight(52); params.setMargins(0, dp(5), 0, dp(7)); return params; }
    private LinearLayout.LayoutParams cardParams() { LinearLayout.LayoutParams params = match(); params.setMargins(0, 0, 0, dp(5)); return params; }
    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }

    @Override
    protected void onDestroy() { network.shutdownNow(); super.onDestroy(); }
}
