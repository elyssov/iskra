package com.iskra.app

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.View
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import iskramobile.Iskramobile
import java.io.File

class MainActivity : AppCompatActivity() {

    private var webView: WebView? = null
    private var port: Int = 0
    private val mainHandler = Handler(Looper.getMainLooper())

    companion object {
        private const val TAG = "Iskra"
        private const val START_TIMEOUT_MS = 30_000L
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Show splash screen immediately
        setContentView(R.layout.activity_splash)

        // Request notification permission for Android 13+
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED) {
                ActivityCompat.requestPermissions(this,
                    arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
            }
        }

        // Set up timeout watchdog
        val timeoutRunnable = Runnable {
            if (port == 0) {
                showError("Таймаут запуска ядра (30 сек). Попробуйте перезапустить приложение.")
            }
        }
        mainHandler.postDelayed(timeoutRunnable, START_TIMEOUT_MS)

        // Start Go core on background thread to avoid ANR
        Thread {
            try {
                val dataDir = filesDir.absolutePath
                val result = Iskramobile.start(dataDir, 0)
                val startedPort = result.toInt()

                mainHandler.removeCallbacks(timeoutRunnable)

                if (startedPort == 0) {
                    runOnUiThread { showError("Ядро вернуло порт 0") }
                    return@Thread
                }

                runOnUiThread {
                    port = startedPort
                    onCoreReady()
                }
            } catch (e: Exception) {
                Log.e(TAG, "Failed to start Go core", e)
                mainHandler.removeCallbacks(timeoutRunnable)
                runOnUiThread {
                    showError("Ошибка запуска: ${e.message}")
                }
            }
        }.apply {
            name = "iskra-core-init"
            isDaemon = true
            start()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun onCoreReady() {
        Log.i(TAG, "Go core ready on port $port")

        // Start foreground service to keep alive
        try {
            val serviceIntent = Intent(this, IskraService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(serviceIntent)
            } else {
                startService(serviceIntent)
            }
        } catch (e: Exception) {
            Log.e(TAG, "Failed to start foreground service", e)
        }

        // Setup WebView and replace splash
        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.databaseEnabled = true
            settings.allowFileAccess = false
            settings.allowContentAccess = false
            webViewClient = WebViewClient()
            webChromeClient = WebChromeClient()
            addJavascriptInterface(UpdateBridge(), "IskraUpdate")
        }

        setContentView(webView)
        webView?.loadUrl("http://localhost:$port")

        // Handle any pending deep link
        handleIntent(intent)
    }

    private fun showError(message: String) {
        Log.e(TAG, "Startup error: $message")

        val subtitle = findViewById<TextView>(R.id.splash_subtitle)
        val progress = findViewById<ProgressBar>(R.id.splash_progress)

        subtitle?.text = message
        subtitle?.setTextColor(0xFFDC2626.toInt()) // red
        progress?.visibility = View.GONE
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme == "iskra") {
            val link = data.toString()
            webView?.evaluateJavascript(
                "if(window._handleDeepLink) window._handleDeepLink('$link')",
                null
            )
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (webView?.canGoBack() == true) {
            webView?.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        mainHandler.removeCallbacksAndMessages(null)
        try {
            Iskramobile.stop()
        } catch (e: Exception) {
            Log.e(TAG, "Error stopping core", e)
        }
    }

    // JS bridge: allows WebView to trigger APK installation
    inner class UpdateBridge {
        @JavascriptInterface
        fun installApk(filename: String): Boolean {
            return try {
                val file = File(filesDir, filename)
                if (!file.exists()) {
                    Log.e(TAG, "APK not found: ${file.absolutePath}")
                    return false
                }
                val uri = FileProvider.getUriForFile(
                    this@MainActivity,
                    "${packageName}.fileprovider",
                    file
                )
                val intent = Intent(Intent.ACTION_VIEW).apply {
                    setDataAndType(uri, "application/vnd.android.package-archive")
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                    addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                }
                startActivity(intent)
                true
            } catch (e: Exception) {
                Log.e(TAG, "Install APK failed", e)
                false
            }
        }
    }
}
