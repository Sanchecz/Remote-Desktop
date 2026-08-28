package ru.supportgenesis.genesisit;

import android.annotation.SuppressLint;
import android.Manifest;
import android.app.Activity;
import android.app.DownloadManager;
import android.content.ClipData;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.net.http.SslError;
import android.net.Uri;
import android.os.Bundle;
import android.os.Build;
import android.os.Environment;
import android.util.Base64;
import android.view.ViewGroup;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.SslErrorHandler;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebChromeClient;
import android.webkit.URLUtil;
import android.webkit.ValueCallback;
import android.webkit.JavascriptInterface;
import android.widget.Toast;

import java.io.ByteArrayOutputStream;

public final class MainActivity extends Activity {
    private static final String START_URL = "https://supportgenesis.ru/";
    private static final int FILE_CHOOSER_REQUEST = 1001;
    private static final int STORAGE_PERMISSION_REQUEST = 1002;
    private static final String OFFLINE_HTML = """
        <!doctype html><html lang="ru"><meta name="viewport" content="width=device-width,initial-scale=1">
        <style>html,body{height:100%;margin:0}body{display:grid;place-items:center;background:#0a0d12;color:#eef5f1;font-family:system-ui,sans-serif}.card{max-width:320px;padding:32px;text-align:center}.logo{display:block;width:64px;height:64px;margin:0 auto 22px;border-radius:16px}h1{font-size:22px;margin:0 0 10px}p{color:#91a099;font-size:14px;line-height:1.55;margin:0 0 24px}button{border:0;border-radius:10px;padding:13px 22px;background:#39e79b;color:#07110d;font-weight:700;font-size:14px}</style>
        <body><main class="card"><img class="logo" src="data:image/png;base64,REMOTEIT_ICON" alt=""><h1>Нет связи с RemoteIt</h1><p>Проверьте интернет-соединение и попробуйте открыть панель ещё раз.</p><button onclick="location.href='https://supportgenesis.ru/'">Повторить</button></main></body></html>
        """;
    private WebView webView;
    private ValueCallback<Uri[]> pendingFileChooser;
	private boolean remoteSessionActive;

    @Override
    @SuppressLint("SetJavaScriptEnabled")
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().setStatusBarColor(Color.WHITE);
        getWindow().setNavigationBarColor(Color.WHITE);
        getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR);
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);

        webView = new WebView(this);
        webView.setLayoutParams(new ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        webView.setBackgroundColor(Color.rgb(10, 13, 18));
        webView.setOverScrollMode(View.OVER_SCROLL_NEVER);
        setContentView(webView);
        applySystemBars();

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(false);
        settings.setAllowUniversalAccessFromFileURLs(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(true);
        settings.setJavaScriptCanOpenWindowsAutomatically(false);
        settings.setSafeBrowsingEnabled(true);
        settings.setSupportZoom(false);
        settings.setBuiltInZoomControls(false);
        settings.setDisplayZoomControls(false);
        settings.setTextZoom(100);
        settings.setUseWideViewPort(true);
        settings.setLoadWithOverviewMode(false);
        settings.setUserAgentString(settings.getUserAgentString() + " RemoteIt-Android/1.0.8");
		webView.addJavascriptInterface(new RemoteItAndroidBridge(), "RemoteItAndroid");

        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        cookies.setAcceptThirdPartyCookies(webView, false);

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri uri = request.getUrl();
                if (isAllowed(uri)) {
                    return false;
                }
                if ("https".equalsIgnoreCase(uri.getScheme())) {
                    startActivity(new Intent(Intent.ACTION_VIEW, uri));
                }
                return true;
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) {
                    showOfflinePage();
                }
            }

            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                handler.cancel();
                showOfflinePage();
            }
        });
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (pendingFileChooser != null) {
                    pendingFileChooser.onReceiveValue(null);
                }
                pendingFileChooser = callback;
                try {
                    Intent chooser = params.createIntent();
                    chooser.addCategory(Intent.CATEGORY_OPENABLE);
                    chooser.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
                    startActivityForResult(chooser, FILE_CHOOSER_REQUEST);
                    return true;
                } catch (RuntimeException error) {
                    pendingFileChooser = null;
                    callback.onReceiveValue(null);
                    Toast.makeText(MainActivity.this, "Не удалось открыть выбор файлов", Toast.LENGTH_LONG).show();
                    return true;
                }
            }
        });
        webView.setDownloadListener((url, userAgent, contentDisposition, mimeType, contentLength) -> {
            Uri uri = Uri.parse(url);
            if (isAllowed(uri)) {
                enqueueDownload(url, userAgent, contentDisposition, mimeType);
            }
        });

        if (savedInstanceState == null) {
            webView.loadUrl(START_URL);
        } else {
            webView.restoreState(savedInstanceState);
        }
    }

	private final class RemoteItAndroidBridge {
		@JavascriptInterface
		public void setRemoteSessionActive(boolean active) {
			runOnUiThread(() -> {
				remoteSessionActive = active;
				applySystemBars();
			});
		}

		@JavascriptInterface
		public void hideKeyboard() {
			runOnUiThread(() -> {
				InputMethodManager keyboard = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
				if (keyboard != null && webView != null) {
					keyboard.hideSoftInputFromWindow(webView.getWindowToken(), 0);
				}
			});
		}
	}

	private void applySystemBars() {
		boolean immersive = remoteSessionActive && getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
		if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
			getWindow().setDecorFitsSystemWindows(!immersive);
			WindowInsetsController controller = getWindow().getInsetsController();
			if (controller != null) {
				if (immersive) {
					controller.hide(WindowInsets.Type.statusBars() | WindowInsets.Type.navigationBars());
					controller.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
				} else {
					controller.show(WindowInsets.Type.statusBars() | WindowInsets.Type.navigationBars());
					controller.setSystemBarsAppearance(
						WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS,
						WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS
					);
				}
			}
		} else {
			getWindow().getDecorView().setSystemUiVisibility(immersive
				? View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY | View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_STABLE
				: View.SYSTEM_UI_FLAG_VISIBLE);
		}
	}

	@Override
	public void onConfigurationChanged(Configuration newConfig) {
		super.onConfigurationChanged(newConfig);
		applySystemBars();
		if (webView != null) {
			webView.requestLayout();
			webView.invalidate();
			webView.post(() -> webView.evaluateJavascript("requestAnimationFrame(()=>{window.dispatchEvent(new Event('resize'));window.dispatchEvent(new Event('orientationchange'));});", null));
			// System bars change the usable viewport after the configuration callback.
			// Send a second resize after insets settle so the remote canvas fills the
			// final landscape area instead of keeping the portrait dimensions.
			webView.postDelayed(() -> {
				webView.requestLayout();
				webView.invalidate();
				webView.evaluateJavascript("window.dispatchEvent(new Event('resize'));", null);
			}, 180);
		}
	}

	@Override
	protected void onResume() {
		super.onResume();
		applySystemBars();
	}

	@Override
	public void onWindowFocusChanged(boolean hasFocus) {
		super.onWindowFocusChanged(hasFocus);
		if (hasFocus) {
			applySystemBars();
		}
	}

    private void enqueueDownload(String url, String userAgent, String contentDisposition, String mimeType) {
        if (Build.VERSION.SDK_INT <= Build.VERSION_CODES.P && checkSelfPermission(Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.WRITE_EXTERNAL_STORAGE}, STORAGE_PERMISSION_REQUEST);
            Toast.makeText(this, "Разрешите сохранение и повторите скачивание", Toast.LENGTH_LONG).show();
            return;
        }
        String fileName = URLUtil.guessFileName(url, contentDisposition, mimeType);
        DownloadManager.Request request = new DownloadManager.Request(Uri.parse(url));
        String cookie = CookieManager.getInstance().getCookie(url);
        if (cookie != null && !cookie.trim().isEmpty()) {
            request.addRequestHeader("Cookie", cookie);
        }
        if (userAgent != null && !userAgent.trim().isEmpty()) {
            request.addRequestHeader("User-Agent", userAgent);
        }
        if (mimeType != null && !mimeType.trim().isEmpty()) {
            request.setMimeType(mimeType);
        }
        request.setTitle(fileName);
        request.setDescription("RemoteIt — скачивание файла");
        request.setAllowedOverMetered(true);
        request.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
        request.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, fileName);
        DownloadManager manager = (DownloadManager) getSystemService(DOWNLOAD_SERVICE);
        manager.enqueue(request);
        Toast.makeText(this, "Скачивание началось", Toast.LENGTH_SHORT).show();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == FILE_CHOOSER_REQUEST && pendingFileChooser != null) {
            Uri[] result = selectedUris(resultCode, data);
            pendingFileChooser.onReceiveValue(result);
            pendingFileChooser = null;
        }
    }

    private static Uri[] selectedUris(int resultCode, Intent data) {
        if (resultCode != RESULT_OK || data == null) {
            return null;
        }
        ClipData clipData = data.getClipData();
        if (clipData != null && clipData.getItemCount() > 0) {
            Uri[] result = new Uri[clipData.getItemCount()];
            for (int index = 0; index < clipData.getItemCount(); index++) {
                result[index] = clipData.getItemAt(index).getUri();
            }
            return result;
        }
        Uri selected = data.getData();
        return selected == null ? null : new Uri[]{selected};
    }

    private static boolean isAllowed(Uri uri) {
        if (!"https".equalsIgnoreCase(uri.getScheme()) || uri.getHost() == null) {
            return false;
        }
        String host = uri.getHost().toLowerCase();
        return host.equals("supportgenesis.ru") || host.equals("www.supportgenesis.ru");
    }

    private void showOfflinePage() {
        Bitmap logo = BitmapFactory.decodeResource(getResources(), R.drawable.app_icon);
        ByteArrayOutputStream stream = new ByteArrayOutputStream();
        logo.compress(Bitmap.CompressFormat.PNG, 100, stream);
        String html = OFFLINE_HTML.replace("REMOTEIT_ICON", Base64.encodeToString(stream.toByteArray(), Base64.NO_WRAP));
        webView.loadDataWithBaseURL(START_URL, html, "text/html", "UTF-8", null);
    }

    @Override
    protected void onSaveInstanceState(Bundle outState) {
        webView.saveState(outState);
        super.onSaveInstanceState(outState);
    }

    @Override
    public void onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onDestroy() {
        if (pendingFileChooser != null) {
            pendingFileChooser.onReceiveValue(null);
            pendingFileChooser = null;
        }
        if (webView != null) {
            webView.loadUrl("about:blank");
            webView.stopLoading();
            webView.setWebViewClient(null);
            webView.destroy();
            webView = null;
        }
        super.onDestroy();
    }
}
