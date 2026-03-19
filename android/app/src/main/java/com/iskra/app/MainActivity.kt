package com.iskra.app

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import iskramobile.Iskramobile

class MainActivity : AppCompatActivity() {

    private lateinit var webView: WebView
    private var port: Int = 0

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Request notification permission for Android 13+
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED) {
                ActivityCompat.requestPermissions(this,
                    arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
            }
        }

        // Start Go core
        try {
            val dataDir = filesDir.absolutePath
            port = Iskramobile.start(dataDir, 0).toInt()
        } catch (e: Exception) {
            Log.e("Iskra", "Failed to start Go core", e)
            Toast.makeText(this, "Ошибка запуска: ${e.message}", Toast.LENGTH_LONG).show()
            port = 0
        }

        if (port == 0) {
            Toast.makeText(this, "Ошибка запуска ядра", Toast.LENGTH_LONG).show()
            finish()
            return
        }

        // Start foreground service to keep alive
        try {
            val serviceIntent = Intent(this, IskraService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(serviceIntent)
            } else {
                startService(serviceIntent)
            }
        } catch (e: Exception) {
            Log.e("Iskra", "Failed to start foreground service", e)
        }

        // Setup WebView
        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.databaseEnabled = true
            settings.allowFileAccess = false
            settings.allowContentAccess = false
            webViewClient = WebViewClient()
            webChromeClient = WebChromeClient()
        }

        setContentView(webView)
        webView.loadUrl("http://localhost:$port")

        // Handle iskra:// deep links
        handleIntent(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme == "iskra") {
            val link = data.toString()
            webView.evaluateJavascript(
                "if(window._handleDeepLink) window._handleDeepLink('$link')",
                null
            )
        }
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        try {
            Iskramobile.stop()
        } catch (e: Exception) {
            Log.e("Iskra", "Error stopping core", e)
        }
    }
}
