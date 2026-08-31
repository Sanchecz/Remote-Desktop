package ru.supportgenesis.remoteit.agent;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

final class AgentStore {
    static final String VERSION = "1.0.33";
    private static final String PREFS = "remoteit_agent";

    final String serverUrl;
    final String deviceName;
    final String deviceId;
    final String deviceSecret;
    final String desktopSecret;
    final String connectionCode;

    private AgentStore(String serverUrl, String deviceName, String deviceId, String deviceSecret, String desktopSecret, String connectionCode) {
        this.serverUrl = trimServer(serverUrl);
        this.deviceName = deviceName;
        this.deviceId = deviceId;
        this.deviceSecret = deviceSecret;
        this.desktopSecret = desktopSecret;
        this.connectionCode = connectionCode;
    }

    static AgentStore load(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return new AgentStore(
            prefs.getString("server", "https://supportgenesis.ru"),
            prefs.getString("name", android.os.Build.MODEL),
            prefs.getString("device_id", ""),
            prefs.getString("device_secret", ""),
            prefs.getString("desktop_secret", ""),
            prefs.getString("connection_code", "")
        );
    }

    boolean enrolled() {
        return !deviceId.isBlank() && !deviceSecret.isBlank() && !desktopSecret.isBlank();
    }

    static AgentStore enroll(Context context, String serverUrl, String token, String deviceName) throws Exception {
        String server = trimServer(serverUrl);
        if (!server.startsWith("https://")) throw new IOException("Сервер должен использовать HTTPS");
        JSONObject body = new JSONObject();
        body.put("token", token.trim());
        body.put("name", deviceName.trim());
        body.put("hostname", android.os.Build.MODEL);
        body.put("os", "Android");
        body.put("osVersion", android.os.Build.VERSION.RELEASE);
        body.put("arch", android.os.Build.SUPPORTED_ABIS.length > 0 ? android.os.Build.SUPPORTED_ABIS[0] : "android");
        body.put("agentVersion", VERSION);
        body.put("installMode", "user");
        body.put("privileged", false);
        JSONObject response = requestJSON(server + "/api/agent/enroll", "POST", body, null, null);
        AgentStore store = new AgentStore(server, deviceName.trim(), response.getString("deviceId"), response.getString("deviceSecret"), response.getString("desktopSecret"), response.getString("connectionCode"));
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString("server", store.serverUrl)
            .putString("name", store.deviceName)
            .putString("device_id", store.deviceId)
            .putString("device_secret", store.deviceSecret)
            .putString("desktop_secret", store.desktopSecret)
            .putString("connection_code", store.connectionCode)
            .apply();
        return store;
    }

    static void clear(Context context) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().clear().apply();
    }

    JSONObject requestDeviceJSON(String path, String method, JSONObject body) throws Exception {
        return requestJSON(serverUrl + path, method, body, deviceId, "Device " + deviceSecret);
    }

    JSONObject requestDesktopJSON(String path, String method, JSONObject body) throws Exception {
        return requestJSON(serverUrl + path, method, body, deviceId, "Desktop " + desktopSecret);
    }

    HttpURLConnection openDesktop(String path, String method) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(serverUrl + path).openConnection();
        connection.setRequestMethod(method);
        connection.setConnectTimeout(10_000);
        connection.setReadTimeout(25_000);
        connection.setUseCaches(false);
        connection.setRequestProperty("X-Genesis-Device-Id", deviceId);
        connection.setRequestProperty("Authorization", "Desktop " + desktopSecret);
        connection.setRequestProperty("User-Agent", "RemoteIt-Android-Agent/" + VERSION);
        return connection;
    }

    void heartbeat() throws Exception {
        JSONObject body = new JSONObject();
        body.put("name", deviceName);
        body.put("hostname", android.os.Build.MODEL);
        body.put("os", "Android");
        body.put("osVersion", android.os.Build.VERSION.RELEASE);
        body.put("arch", android.os.Build.SUPPORTED_ABIS.length > 0 ? android.os.Build.SUPPORTED_ABIS[0] : "android");
        body.put("agentVersion", VERSION);
        body.put("currentUser", "Android user");
        body.put("installMode", "user");
        body.put("privileged", false);
        requestDeviceJSON("/api/agent/heartbeat", "POST", body);
    }

    static byte[] readAll(InputStream input, int limit) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[8192];
        int total = 0;
        for (int read; (read = input.read(buffer)) >= 0;) {
            total += read;
            if (total > limit) throw new IOException("Ответ сервера слишком большой");
            output.write(buffer, 0, read);
        }
        return output.toByteArray();
    }

    private static JSONObject requestJSON(String endpoint, String method, JSONObject body, String deviceId, String authorization) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL(endpoint).openConnection();
        connection.setRequestMethod(method);
        connection.setConnectTimeout(10_000);
        connection.setReadTimeout(25_000);
        connection.setUseCaches(false);
        connection.setRequestProperty("Accept", "application/json");
        connection.setRequestProperty("User-Agent", "RemoteIt-Android-Agent/" + VERSION);
        if (deviceId != null) connection.setRequestProperty("X-Genesis-Device-Id", deviceId);
        if (authorization != null) connection.setRequestProperty("Authorization", authorization);
        if (body != null) {
            byte[] payload = body.toString().getBytes(StandardCharsets.UTF_8);
            connection.setDoOutput(true);
            connection.setFixedLengthStreamingMode(payload.length);
            connection.setRequestProperty("Content-Type", "application/json");
            try (OutputStream output = connection.getOutputStream()) { output.write(payload); }
        }
        int status = connection.getResponseCode();
        InputStream stream = status >= 200 && status < 300 ? connection.getInputStream() : connection.getErrorStream();
        byte[] payload = stream == null ? new byte[0] : readAll(stream, 512 * 1024);
        connection.disconnect();
        String text = new String(payload, StandardCharsets.UTF_8);
        if (status < 200 || status >= 300) {
            try { throw new IOException(new JSONObject(text).optString("error", "HTTP " + status)); }
            catch (org.json.JSONException ignored) { throw new IOException("HTTP " + status); }
        }
        return text.isBlank() ? new JSONObject() : new JSONObject(text);
    }

    private static String trimServer(String value) {
        String result = value == null ? "" : value.trim();
        while (result.endsWith("/")) result = result.substring(0, result.length() - 1);
        return result;
    }
}
