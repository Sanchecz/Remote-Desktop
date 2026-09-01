package ru.supportgenesis.remoteit.agent;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.media.projection.MediaProjectionManager;
import android.media.projection.MediaProjectionConfig;
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
    private static final String STATE_PROJECTION_PENDING = "projectionPendingAfterNotifications";
    private final ExecutorService network = Executors.newSingleThreadExecutor();
    private LinearLayout content;
    private AgentStore store;
    private TextView state;
    private boolean projectionPendingAfterNotifications;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        projectionPendingAfterNotifications = savedInstanceState != null && savedInstanceState.getBoolean(STATE_PROJECTION_PENDING, false);
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
        scroll.setId(R.id.agent_scroll);
        scroll.setFillViewport(true);
        scroll.setClipToPadding(false);
        scroll.setPadding(0, 0, 0, dp(12));
        scroll.setBackgroundColor(Color.rgb(245, 248, 247));
        content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setGravity(Gravity.CENTER_HORIZONTAL);
        content.setPadding(dp(18), dp(24), dp(18), dp(28));
        int availableWidth = Math.max(dp(280), getResources().getDisplayMetrics().widthPixels - dp(20));
        int contentWidth = Math.min(dp(680), availableWidth);
        ScrollView.LayoutParams contentParams = new ScrollView.LayoutParams(contentWidth, ScrollView.LayoutParams.WRAP_CONTENT, Gravity.CENTER_HORIZONTAL);
        scroll.addView(content, contentParams);
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
        server.setId(R.id.enrollment_server);
        token.setId(R.id.enrollment_token);
        String clipboardToken = enrollmentTokenFromClipboard();
        if (!clipboardToken.isEmpty()) token.setText(clipboardToken);
        EditText name = input("Название устройства", Build.MODEL, InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);
        name.setId(R.id.enrollment_name);
        content.addView(server, field()); content.addView(token, field()); content.addView(name, field());
        state = text("", 13, Color.rgb(204, 55, 55), false); state.setPadding(0, dp(8), 0, dp(8)); content.addView(state, match());
        Button enroll = primary("Зарегистрировать телефон");
        enroll.setOnClickListener(view -> {
            if (token.getText().toString().trim().isEmpty() || name.getText().toString().trim().isEmpty()) {
                state.setText(R.string.enrollment_fields_required); return;
            }
            enroll.setEnabled(false); state.setTextColor(Color.rgb(91, 112, 124)); state.setText(R.string.enrollment_in_progress);
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
        boolean accessibility = RemoteControlAccessibilityService.isEnabled(this);
        boolean sharing = ScreenShareService.running();
        PermissionFlow.State flow = PermissionFlow.evaluate(accessibility, sharing);

        LinearLayout idCard = card();
        LinearLayout idHeading = horizontal();
        LinearLayout idCopy = vertical();
        idCopy.addView(text("REMOTE ID", 11, Color.rgb(91, 112, 124), true), match());
        TextView code = text(store.connectionCode, 27, Color.rgb(13, 31, 43), true);
        code.setLetterSpacing(.08f);
        code.setPadding(0, dp(5), 0, dp(2));
        idCopy.addView(code, match());
        idHeading.addView(idCopy, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));
        TextView progress = pill(flow.completed + " / " + flow.total, flow.ready ? Color.rgb(8, 143, 96) : Color.rgb(183, 112, 20));
        idHeading.addView(progress, wrap());
        idCard.addView(idHeading, match());
        idCard.addView(text("Версия Agent " + AgentStore.VERSION + " · разрешения выдаёт владелец телефона", 11, Color.rgb(91, 112, 124), false), match());
        content.addView(idCard, cardParams());

        content.addView(sectionTitle("ПОДГОТОВКА ДОСТУПА", "Два понятных шага. Android не позволяет включить их скрыто."), match());

        Button accessibilityButton = accessibility
                ? secondary("Настройки управления")
                : primary("Шаг 1 · Открыть разрешение управления");
        accessibilityButton.setOnClickListener(view -> explainAccessibilityAndOpen());
        content.addView(permissionCard(
                "1",
                "Управление телефоном",
                accessibility ? "Включено. RemoteIt сможет выполнять касания, ввод и прокрутку только во время активного сеанса." : "Откроется системная страница Android. Найдите RemoteIt, включите службу и вернитесь в приложение — статус обновится сам.",
                accessibility ? "РАЗРЕШЕНО" : "ТРЕБУЕТСЯ",
                accessibility,
                accessibilityButton
        ), cardParams());

        Button share = primary(sharing ? "Весь экран уже транслируется" : flow.canStartSharing ? "Шаг 2 · Показать весь экран" : "Сначала выполните шаг 1");
        share.setEnabled(flow.canStartSharing);
        share.setOnClickListener(view -> explainProjectionAndRequest());
        content.addView(permissionCard(
                "2",
                "Трансляция экрана",
                sharing ? "Активна. Устройство доступно в панели RemoteIt." : accessibility ? "Android покажет одно защищённое окно. Выберите «Весь экран» и нажмите «Начать»." : "Станет доступно сразу после шага 1. Это исключает сеанс, в котором экран виден, но управление случайно осталось выключено.",
                sharing ? "АКТИВНА" : "ОСТАНОВЛЕНА",
                sharing,
                share
        ), cardParams());

        LinearLayout resultCard = card();
        resultCard.setBackground(shape(flow.ready ? Color.rgb(235, 249, 242) : Color.rgb(255, 248, 232), flow.ready ? Color.rgb(166, 226, 201) : Color.rgb(240, 207, 146), 15));
        String resultTitle = flow.ready ? "Устройство готово к подключению"
                : sharing ? "Экран виден, управление выключено"
                : accessibility ? "Остался один шаг"
                : "Начните с разрешения управления";
        String resultMessage = flow.ready
                ? "Можно свернуть приложение. Постоянное уведомление Android остаётся видимым, пока экран доступен администратору."
                : sharing
                ? "Остановите показ экрана, включите шаг 1 и запустите показ заново."
                : accessibility
                ? "Нажмите «Показать весь экран» и подтвердите одно системное окно Android."
                : "Выполните шаг 1. После возврата кнопка шага 2 включится автоматически.";
        resultCard.addView(text(resultTitle, 15, flow.ready ? Color.rgb(8, 122, 83) : Color.rgb(153, 91, 11), true), match());
        TextView resultDetail = text(resultMessage, 12, Color.rgb(77, 96, 105), false);
        resultDetail.setPadding(0, dp(5), 0, 0);
        resultCard.addView(resultDetail, match());
        content.addView(resultCard, cardParams());

        Button stop = danger("Остановить доступ");
        stop.setEnabled(sharing);
        stop.setOnClickListener(view -> {
            Intent intent = new Intent(this, ScreenShareService.class);
            intent.setAction(ScreenShareService.ACTION_STOP);
            startService(intent);
            view.postDelayed(this::render, 350);
        });
        content.addView(stop, matchHeight(48));
        Button reset = textButton("Отвязать этот телефон");
        reset.setOnClickListener(view -> {
            if (ScreenShareService.running()) Toast.makeText(this, "Сначала остановите доступ", Toast.LENGTH_SHORT).show();
            else new AlertDialog.Builder(this)
                    .setTitle("Отвязать телефон?")
                    .setMessage("Remote ID и регистрация будут удалены только с этого телефона.")
                    .setNegativeButton("Отмена", null)
                    .setPositiveButton("Отвязать", (dialog, which) -> { AgentStore.clear(this); store = AgentStore.load(this); render(); })
                    .show();
        });
        content.addView(reset, matchHeight(42));
    }

    private void explainAccessibilityAndOpen() {
        new AlertDialog.Builder(this)
                .setTitle("Разрешить управление")
                .setMessage(accessibilityInstructions())
                .setNegativeButton("Не сейчас", null)
                .setPositiveButton("Открыть настройки", (dialog, which) -> openAccessibilitySettings())
                .show();
    }

    private String accessibilityInstructions() {
        String section = PermissionFlow.accessibilitySectionFor(Build.MANUFACTURER);
        return "Android откроет только официальный раздел специальных возможностей.\n\n"
                + "1. Откройте " + section + ".\n"
                + "2. Выберите «RemoteIt — удалённое управление».\n"
                + "3. Включите службу и вернитесь в RemoteIt.\n\n"
                + "Никакие другие разрешения в этом разделе включать не нужно. Статус шага обновится автоматически.";
    }

    private void openAccessibilitySettings() {
        try {
            startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS));
        } catch (Exception unavailable) {
            Toast.makeText(this, "Android не открыл настройки. Откройте: Настройки → Специальные возможности → RemoteIt", Toast.LENGTH_LONG).show();
        }
    }

    private void explainProjectionAndRequest() {
        if (!RemoteControlAccessibilityService.isEnabled(this)) {
            Toast.makeText(this, "Сначала разрешите управление в шаге 1", Toast.LENGTH_SHORT).show();
            return;
        }
        boolean notificationsPrompt = Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED;
        String promptOrder = notificationsPrompt
                ? "Android последовательно покажет два системных окна:\n1. Уведомления — разрешите, чтобы кнопка остановки всегда была видна.\n2. Показ экрана — выберите «Весь экран» и нажмите «Начать»."
                : "Android покажет одно системное окно. Выберите «Весь экран» и нажмите «Начать».";
        new AlertDialog.Builder(this)
                .setTitle("Показать экран администратору")
                .setMessage(promptOrder + "\n\nRemoteIt не может подтвердить эти окна удалённо. Остановить доступ можно здесь или через постоянное уведомление.")
                .setNegativeButton("Отмена", null)
                .setPositiveButton("Продолжить", (dialog, which) -> requestProjection())
                .show();
    }

    private void requestProjection() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            projectionPendingAfterNotifications = true;
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, NOTIFICATION_REQUEST);
            return;
        }
        launchProjectionPicker();
    }

    private void launchProjectionPicker() {
        MediaProjectionManager manager = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
        Intent projectionIntent = Build.VERSION.SDK_INT >= 34
                ? manager.createScreenCaptureIntent(MediaProjectionConfig.createConfigForDefaultDisplay())
                : manager.createScreenCaptureIntent();
        startActivityForResult(projectionIntent, PROJECTION_REQUEST);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == NOTIFICATION_REQUEST && projectionPendingAfterNotifications) {
            projectionPendingAfterNotifications = false;
            if (grantResults.length == 0 || grantResults[0] != PackageManager.PERMISSION_GRANTED) {
                Toast.makeText(this, "Уведомление не разрешено: остановить доступ можно в RemoteIt или в списке активных приложений Android", Toast.LENGTH_LONG).show();
            }
            launchProjectionPicker();
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != PROJECTION_REQUEST) return;
        if (resultCode != RESULT_OK || data == null) { Toast.makeText(this, "Трансляция не запущена", Toast.LENGTH_SHORT).show(); return; }
        if (!RemoteControlAccessibilityService.isEnabled(this)) {
            Toast.makeText(this, "Управление выключено. Выполните шаг 1 и повторите показ экрана.", Toast.LENGTH_LONG).show();
            render();
            return;
        }
        Intent service = new Intent(this, ScreenShareService.class);
        service.setAction(ScreenShareService.ACTION_START);
        service.putExtra(ScreenShareService.EXTRA_RESULT_CODE, resultCode);
        service.putExtra(ScreenShareService.EXTRA_RESULT_DATA, data);
        startForegroundService(service);
        content.postDelayed(this::render, 550);
    }

    private LinearLayout permissionCard(String number, String title, String detail, String status, boolean completed, Button action) {
        LinearLayout card = card();
        LinearLayout heading = horizontal();
        TextView marker = text(number, 14, Color.WHITE, true);
        marker.setGravity(Gravity.CENTER);
        marker.setBackground(shape(completed ? Color.rgb(8, 168, 115) : Color.rgb(87, 107, 119), completed ? Color.rgb(8, 168, 115) : Color.rgb(87, 107, 119), 10));
        heading.addView(marker, new LinearLayout.LayoutParams(dp(34), dp(34)));
        TextView titleView = text(title, 15, Color.rgb(13, 31, 43), true);
        LinearLayout.LayoutParams titleParams = new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1);
        titleParams.setMargins(dp(11), 0, dp(8), 0);
        heading.addView(titleView, titleParams);
        heading.addView(pill(status, completed ? Color.rgb(8, 143, 96) : Color.rgb(183, 112, 20)), wrap());
        card.addView(heading, match());
        TextView detailView = text(detail, 12, Color.rgb(91, 112, 124), false);
        detailView.setPadding(0, dp(12), 0, dp(10));
        detailView.setLineSpacing(0, 1.12f);
        card.addView(detailView, match());
        card.addView(action, matchHeight(48));
        return card;
    }

    private LinearLayout sectionTitle(String title, String detail) {
        LinearLayout box = vertical();
        box.setPadding(0, dp(17), 0, dp(10));
        TextView titleView = text(title, 11, Color.rgb(8, 143, 96), true);
        titleView.setLetterSpacing(.11f);
        box.addView(titleView, match());
        TextView detailView = text(detail, 12, Color.rgb(91, 112, 124), false);
        detailView.setPadding(0, dp(4), 0, 0);
        box.addView(detailView, match());
        return box;
    }

    private EditText input(String label, String hint, int type) { EditText value = new EditText(this); value.setHint(label + " · " + hint); value.setTextSize(15); value.setSingleLine(true); value.setInputType(type); value.setPadding(dp(13), 0, dp(13), 0); value.setBackgroundColor(Color.WHITE); return value; }
    private LinearLayout card() { LinearLayout card = new LinearLayout(this); card.setOrientation(LinearLayout.VERTICAL); card.setPadding(dp(16), dp(15), dp(16), dp(15)); card.setBackground(shape(Color.WHITE, Color.rgb(222, 231, 228), 15)); card.setElevation(dp(1)); return card; }
    private TextView note(String value) { TextView note = text(value, 12, Color.rgb(91, 112, 124), false); note.setPadding(0, dp(16), 0, 0); note.setGravity(Gravity.CENTER); return note; }
    private Button primary(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.WHITE); button.setTextSize(14); button.setTypeface(Typeface.DEFAULT, Typeface.BOLD); button.setAllCaps(false); button.setBackground(shape(Color.rgb(8, 168, 115), Color.rgb(8, 168, 115), 11)); return button; }
    private Button secondary(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.rgb(13, 91, 67)); button.setTextSize(14); button.setAllCaps(false); button.setBackground(shape(Color.WHITE, Color.rgb(194, 216, 207), 11)); return button; }
    private Button danger(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.rgb(190, 54, 59)); button.setTextSize(14); button.setTypeface(Typeface.DEFAULT, Typeface.BOLD); button.setAllCaps(false); button.setBackground(shape(Color.rgb(255, 247, 247), Color.rgb(239, 184, 187), 11)); return button; }
    private Button textButton(String value) { Button button = new Button(this); button.setText(value); button.setTextColor(Color.rgb(123, 137, 145)); button.setTextSize(12); button.setAllCaps(false); button.setBackgroundColor(Color.TRANSPARENT); return button; }
    private TextView text(String value, int size, int color, boolean bold) { TextView view = new TextView(this); view.setText(value); view.setTextSize(size); view.setTextColor(color); if (bold) view.setTypeface(android.graphics.Typeface.DEFAULT, android.graphics.Typeface.BOLD); return view; }
    private LinearLayout.LayoutParams match() { return new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT); }
    private LinearLayout.LayoutParams wrap() { return new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT); }
    private LinearLayout.LayoutParams matchHeight(int height) { LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(height)); params.setMargins(0, dp(5), 0, dp(5)); return params; }
    private LinearLayout.LayoutParams field() { LinearLayout.LayoutParams params = matchHeight(52); params.setMargins(0, dp(5), 0, dp(7)); return params; }
    private LinearLayout.LayoutParams cardParams() { LinearLayout.LayoutParams params = match(); params.setMargins(0, 0, 0, dp(10)); return params; }
    private LinearLayout horizontal() { LinearLayout layout = new LinearLayout(this); layout.setOrientation(LinearLayout.HORIZONTAL); layout.setGravity(Gravity.CENTER_VERTICAL); return layout; }
    private LinearLayout vertical() { LinearLayout layout = new LinearLayout(this); layout.setOrientation(LinearLayout.VERTICAL); return layout; }
    private TextView pill(String value, int color) { TextView view = text(value, 10, color, true); view.setGravity(Gravity.CENTER); view.setPadding(dp(10), dp(6), dp(10), dp(6)); view.setBackground(shape(mixWithWhite(color, .90f), mixWithWhite(color, .65f), 20)); return view; }
    private GradientDrawable shape(int fill, int stroke, int radius) { GradientDrawable drawable = new GradientDrawable(); drawable.setColor(fill); drawable.setCornerRadius(dp(radius)); drawable.setStroke(dp(1), stroke); return drawable; }
    private int mixWithWhite(int color, float whiteRatio) { float keep = 1f - whiteRatio; return Color.rgb(Math.round(Color.red(color) * keep + 255 * whiteRatio), Math.round(Color.green(color) * keep + 255 * whiteRatio), Math.round(Color.blue(color) * keep + 255 * whiteRatio)); }
    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }

    @Override
    protected void onSaveInstanceState(Bundle outState) {
        outState.putBoolean(STATE_PROJECTION_PENDING, projectionPendingAfterNotifications);
        super.onSaveInstanceState(outState);
    }

    @Override
    protected void onDestroy() { network.shutdownNow(); super.onDestroy(); }
}
