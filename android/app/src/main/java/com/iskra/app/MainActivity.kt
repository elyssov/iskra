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
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebViewClient
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import android.net.wifi.WifiManager
import iskramobile.Iskramobile
import java.io.File

class MainActivity : AppCompatActivity() {

    private var webView: WebView? = null
    private var port: Int = 0
    private val mainHandler = Handler(Looper.getMainLooper())
    private var multicastLock: WifiManager.MulticastLock? = null
    private var wifiDirectManager: WifiDirectManager? = null

    companion object {
        private const val TAG = "Iskra"
        private const val START_TIMEOUT_MS = 30_000L
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Catch native crashes for diagnostics
        Thread.setDefaultUncaughtExceptionHandler { thread, throwable ->
            Log.e(TAG, "Uncaught exception in ${thread.name}", throwable)
            try {
                File(filesDir, "crash.log").writeText(
                    "Thread: ${thread.name}\n${throwable.stackTraceToString()}"
                )
            } catch (_: Exception) {}
        }

        // Show splash screen immediately
        setContentView(R.layout.activity_splash)

        // Request permissions for notifications, location, and WiFi Direct
        val permsNeeded = mutableListOf<String>()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED) {
                permsNeeded.add(Manifest.permission.POST_NOTIFICATIONS)
            }
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.NEARBY_WIFI_DEVICES)
                != PackageManager.PERMISSION_GRANTED) {
                permsNeeded.add(Manifest.permission.NEARBY_WIFI_DEVICES)
            }
        }
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION)
            != PackageManager.PERMISSION_GRANTED) {
            permsNeeded.add(Manifest.permission.ACCESS_FINE_LOCATION)
        }
        if (permsNeeded.isNotEmpty()) {
            ActivityCompat.requestPermissions(this, permsNeeded.toTypedArray(), 1001)
        }

        // Set up timeout watchdog
        val timeoutRunnable = Runnable {
            if (port == 0) {
                showError("Таймаут запуска ядра (30 сек). Попробуйте перезапустить приложение.")
            }
        }
        mainHandler.postDelayed(timeoutRunnable, START_TIMEOUT_MS)

        // Acquire MulticastLock — required for LAN mesh discovery on Android
        try {
            val wifiManager = applicationContext.getSystemService(WIFI_SERVICE) as WifiManager
            multicastLock = wifiManager.createMulticastLock("iskra-mesh").apply {
                setReferenceCounted(false)
                acquire()
            }
            Log.i(TAG, "MulticastLock acquired for LAN mesh")
        } catch (e: Exception) {
            Log.w(TAG, "MulticastLock failed: ${e.message}")
        }

        // Initialize Wi-Fi Direct for mesh wave protocol
        try {
            wifiDirectManager = WifiDirectManager(this).apply { initialize() }
            Log.i(TAG, "Wi-Fi Direct manager initialized for mesh")
        } catch (e: Exception) {
            Log.w(TAG, "Wi-Fi Direct init failed: ${e.message}")
        }

        // Start Go core on background thread to avoid ANR (with retry)
        Thread {
            try {
                val dataDir = filesDir.absolutePath
                var startedPort = 0
                for (attempt in 1..3) {
                    val result = Iskramobile.start(dataDir, 0)
                    startedPort = result.toInt()
                    if (startedPort != 0) break
                    Log.w(TAG, "Core returned port 0, attempt $attempt/3")
                    Iskramobile.stop()
                    Thread.sleep(1000)
                }

                mainHandler.removeCallbacks(timeoutRunnable)

                if (startedPort == 0) {
                    runOnUiThread { showError("Ядро не запустилось после 3 попыток. Перезапустите приложение.") }
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
            settings.saveFormData = false
            // Disable autofill to prevent keyboard from learning mnemonic words
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                importantForAutofill = android.view.View.IMPORTANT_FOR_AUTOFILL_NO
            }
            webViewClient = object : WebViewClient() {
                override fun onReceivedError(view: WebView?, request: WebResourceRequest?, error: WebResourceError?) {
                    if (request?.isForMainFrame == true) {
                        Log.w(TAG, "WebView load error, retrying in 1s...")
                        mainHandler.postDelayed({ view?.reload() }, 1000)
                    }
                }
            }
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
            multicastLock?.release()
        } catch (_: Exception) {}
        try {
            wifiDirectManager?.destroy()
        } catch (_: Exception) {}
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
