package ru.supportgenesis.remoteit.agent;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.graphics.Bitmap;
import android.graphics.drawable.Icon;
import android.graphics.PixelFormat;
import android.graphics.Rect;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.Image;
import android.media.ImageReader;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.util.DisplayMetrics;
import android.view.WindowManager;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

public final class ScreenShareService extends Service {
    static final String ACTION_START = "ru.supportgenesis.remoteit.agent.START";
    static final String ACTION_STOP = "ru.supportgenesis.remoteit.agent.STOP";
    static final String EXTRA_RESULT_CODE = "resultCode";
    static final String EXTRA_RESULT_DATA = "resultData";
    private static final String CHANNEL = "remoteit_active_access";
    private static final int NOTIFICATION_ID = 2601;
    private static final AtomicBoolean RUNNING = new AtomicBoolean(false);

    private final ScheduledExecutorService controlExecutor = Executors.newSingleThreadScheduledExecutor();
    private final ExecutorService frameExecutor = Executors.newSingleThreadExecutor();
    private final ExecutorService inputExecutor = Executors.newSingleThreadExecutor();
    private final AtomicBoolean frameInFlight = new AtomicBoolean(false);
    private final AtomicBoolean inputInFlight = new AtomicBoolean(false);
    private final AtomicReference<Offer> offer = new AtomicReference<>();
    private AgentStore store;
    private MediaProjection projection;
    private VirtualDisplay virtualDisplay;
    private ImageReader imageReader;
    private HandlerThread imageThread;
    private Handler imageHandler;
    private long lastFrameNanos;
    private int captureWidth;
    private int captureHeight;

    static boolean running() { return RUNNING.get(); }

    @Override
    public void onCreate() {
        super.onCreate();
        store = AgentStore.load(this);
        createNotificationChannel();
    }

    @Override
    @SuppressWarnings("deprecation")
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) { stopSelfSafely(); return START_NOT_STICKY; }
        if (intent == null || !ACTION_START.equals(intent.getAction()) || !store.enrolled()) return START_NOT_STICKY;
        Notification notification = buildNotification("Трансляция запущена · ожидаем подключение администратора");
        if (Build.VERSION.SDK_INT >= 34) startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION);
        else startForeground(NOTIFICATION_ID, notification);
        int resultCode = intent.getIntExtra(EXTRA_RESULT_CODE, 0);
        Intent resultData = Build.VERSION.SDK_INT >= 33 ? intent.getParcelableExtra(EXTRA_RESULT_DATA, Intent.class) : intent.getParcelableExtra(EXTRA_RESULT_DATA);
        if (resultCode == 0 || resultData == null) { stopSelfSafely(); return START_NOT_STICKY; }
        try {
            MediaProjectionManager manager = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
            projection = manager.getMediaProjection(resultCode, resultData);
            projection.registerCallback(new MediaProjection.Callback() { @Override public void onStop() { stopSelfSafely(); } }, new Handler(getMainLooper()));
            imageThread = new HandlerThread("RemoteItCapture"); imageThread.start(); imageHandler = new Handler(imageThread.getLooper());
            createCaptureSurface();
            RUNNING.set(true);
            controlExecutor.scheduleWithFixedDelay(this::refreshOffer, 0, 350, TimeUnit.MILLISECONDS);
            controlExecutor.scheduleWithFixedDelay(this::heartbeat, 0, 20, TimeUnit.SECONDS);
            controlExecutor.scheduleWithFixedDelay(this::ensureCaptureGeometry, 2, 2, TimeUnit.SECONDS);
            controlExecutor.scheduleWithFixedDelay(this::pollInputs, 100, 100, TimeUnit.MILLISECONDS);
        } catch (RuntimeException error) {
            stopSelfSafely();
        }
        return START_NOT_STICKY;
    }

    private synchronized void createCaptureSurface() {
        int[] geometry = desiredGeometry();
        captureWidth = geometry[0]; captureHeight = geometry[1];
        if (imageReader != null) imageReader.close();
        imageReader = ImageReader.newInstance(captureWidth, captureHeight, PixelFormat.RGBA_8888, 2);
        imageReader.setOnImageAvailableListener(this::onImageAvailable, imageHandler);
        int density = getResources().getDisplayMetrics().densityDpi;
        if (virtualDisplay == null) {
            virtualDisplay = projection.createVirtualDisplay("RemoteIt", captureWidth, captureHeight, density, DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, imageReader.getSurface(), null, imageHandler);
        } else {
            virtualDisplay.resize(captureWidth, captureHeight, density);
            virtualDisplay.setSurface(imageReader.getSurface());
        }
    }

    private void ensureCaptureGeometry() {
        if (!RUNNING.get()) return;
        if (!RemoteControlAccessibilityService.isEnabled(this)) {
            stopSelfSafely();
            return;
        }
        int[] geometry = desiredGeometry();
        if (geometry[0] != captureWidth || geometry[1] != captureHeight) imageHandler.post(this::createCaptureSurface);
    }

    @SuppressWarnings("deprecation")
    private int[] desiredGeometry() {
        WindowManager window = (WindowManager) getSystemService(WINDOW_SERVICE);
        int width, height;
        if (Build.VERSION.SDK_INT >= 30) {
            Rect bounds = window.getMaximumWindowMetrics().getBounds(); width = bounds.width(); height = bounds.height();
        } else {
            DisplayMetrics metrics = new DisplayMetrics(); window.getDefaultDisplay().getRealMetrics(metrics); width = metrics.widthPixels; height = metrics.heightPixels;
        }
        int maxWidth = 1600;
        if (width > maxWidth) { height = Math.max(1, Math.round(height * (maxWidth / (float) width))); width = maxWidth; }
        return new int[]{Math.max(1, width), Math.max(1, height)};
    }

    private void onImageAvailable(ImageReader reader) {
        Image image = reader.acquireLatestImage();
        if (image == null) return;
        Offer current = offer.get();
        int fps = current == null ? 0 : current.targetFps == 0 ? 15 : Math.min(30, current.targetFps);
        long now = System.nanoTime();
        if (fps == 0 || now - lastFrameNanos < 1_000_000_000L / fps || !frameInFlight.compareAndSet(false, true)) { image.close(); return; }
        lastFrameNanos = now;
        frameExecutor.execute(() -> encodeAndSend(image, current, fps));
    }

    private void encodeAndSend(Image image, Offer current, int fps) {
        Bitmap padded = null; Bitmap frame = null;
        try {
            Image.Plane plane = image.getPlanes()[0];
            ByteBuffer buffer = plane.getBuffer();
            int pixelStride = plane.getPixelStride();
            int rowStride = plane.getRowStride();
            int paddedWidth = captureWidth + Math.max(0, rowStride - pixelStride * captureWidth) / pixelStride;
            padded = Bitmap.createBitmap(paddedWidth, captureHeight, Bitmap.Config.ARGB_8888);
            padded.copyPixelsFromBuffer(buffer);
            frame = Bitmap.createBitmap(padded, 0, 0, captureWidth, captureHeight);
            ByteArrayOutputStream encoded = new ByteArrayOutputStream(512 * 1024);
            frame.compress(Bitmap.CompressFormat.JPEG, fps >= 30 ? 82 : 88, encoded);
            sendFrame(current.id, encoded.toByteArray(), captureWidth, captureHeight);
        } catch (Exception ignored) {
        } finally {
            image.close();
            if (frame != null) frame.recycle();
            if (padded != null && padded != frame) padded.recycle();
            frameInFlight.set(false);
        }
    }

    private void sendFrame(String sessionId, byte[] jpeg, int width, int height) throws Exception {
        HttpURLConnection connection = store.openDesktop("/api/desktop/agent/sessions/" + sessionId + "/frame", "POST");
        connection.setDoOutput(true);
        connection.setFixedLengthStreamingMode(jpeg.length);
        connection.setRequestProperty("Content-Type", "image/jpeg");
        connection.setRequestProperty("X-RemoteIt-Width", Integer.toString(width));
        connection.setRequestProperty("X-RemoteIt-Height", Integer.toString(height));
        try (OutputStream output = connection.getOutputStream()) { output.write(jpeg); }
        int status = connection.getResponseCode();
        connection.disconnect();
        if (status == 409) offer.set(null);
        else if (status < 200 || status >= 300) throw new java.io.IOException("frame HTTP " + status);
    }

    private void refreshOffer() {
        if (!RUNNING.get()) return;
        try {
            HttpURLConnection connection = store.openDesktop("/api/desktop/agent/session", "GET");
            int status = connection.getResponseCode();
            if (status == 204) { offer.set(null); connection.disconnect(); updateNotification("Трансляция активна · ожидаем администратора"); return; }
            InputStream input = status >= 200 && status < 300 ? connection.getInputStream() : connection.getErrorStream();
            byte[] payload = input == null ? new byte[0] : AgentStore.readAll(input, 128 * 1024);
            connection.disconnect();
            if (status == 200) {
                JSONObject json = new JSONObject(new String(payload, StandardCharsets.UTF_8));
                Offer next = new Offer(json.getString("id"), json.optBoolean("controlEnabled"), json.optInt("targetFps"));
                Offer previous = offer.getAndSet(next);
                if (previous == null || !previous.id.equals(next.id)) updateNotification("Администратор подключён · экран передаётся");
            }
        } catch (Exception ignored) { }
    }

    private void pollInputs() {
        Offer current = offer.get();
        if (!RUNNING.get() || current == null || !current.control || !inputInFlight.compareAndSet(false, true)) return;
        inputExecutor.execute(() -> {
            try {
                JSONObject payload = store.requestDesktopJSON("/api/desktop/agent/sessions/" + current.id + "/inputs", "GET", null);
                JSONArray events = payload.optJSONArray("events");
                if (events != null) for (int index = 0; index < events.length(); index++) {
                    JSONObject wrapped = events.optJSONObject(index);
                    JSONObject event = wrapped == null ? null : wrapped.optJSONObject("event");
                    if (event != null) RemoteControlAccessibilityService.dispatch(event);
                }
            } catch (Exception ignored) {
            } finally { inputInFlight.set(false); }
        });
    }

    private void heartbeat() { if (RUNNING.get()) try { store.heartbeat(); } catch (Exception ignored) { } }

    private void createNotificationChannel() {
        NotificationChannel channel = new NotificationChannel(CHANNEL, "Активный удалённый доступ", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("Показывается, пока RemoteIt передаёт экран телефона");
        ((NotificationManager) getSystemService(NOTIFICATION_SERVICE)).createNotificationChannel(channel);
    }

    private Notification buildNotification(String text) {
        Intent activity = new Intent(this, MainActivity.class);
        PendingIntent open = PendingIntent.getActivity(this, 0, activity, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Intent stopIntent = new Intent(this, ScreenShareService.class); stopIntent.setAction(ACTION_STOP);
        PendingIntent stop = PendingIntent.getService(this, 1, stopIntent, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder builder = new Notification.Builder(this, CHANNEL);
        Notification.Action stopAction = new Notification.Action.Builder(Icon.createWithResource(this, android.R.drawable.ic_menu_close_clear_cancel), "Остановить", stop).build();
        return builder.setSmallIcon(ru.supportgenesis.remoteit.agent.R.drawable.remoteit_agent_icon).setContentTitle("RemoteIt Agent").setContentText(text).setOngoing(true).setOnlyAlertOnce(true).setContentIntent(open).addAction(stopAction).build();
    }

    private void updateNotification(String text) { ((NotificationManager) getSystemService(NOTIFICATION_SERVICE)).notify(NOTIFICATION_ID, buildNotification(text)); }

    private synchronized void stopSelfSafely() {
        if (!RUNNING.getAndSet(false) && projection == null) { stopSelf(); return; }
        offer.set(null);
        if (imageReader != null) { imageReader.close(); imageReader = null; }
        if (virtualDisplay != null) { virtualDisplay.release(); virtualDisplay = null; }
        MediaProjection current = projection; projection = null;
        if (current != null) current.stop();
        if (imageThread != null) { imageThread.quitSafely(); imageThread = null; }
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf();
    }

    @Override public void onDestroy() { stopSelfSafely(); controlExecutor.shutdownNow(); frameExecutor.shutdownNow(); inputExecutor.shutdownNow(); super.onDestroy(); }
    @Override public IBinder onBind(Intent intent) { return null; }

    private static final class Offer {
        final String id;
        final boolean control;
        final int targetFps;
        Offer(String id, boolean control, int targetFps) { this.id = id; this.control = control; this.targetFps = targetFps; }
    }
}
