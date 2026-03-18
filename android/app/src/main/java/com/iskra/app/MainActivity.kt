package com.iskra.app

import android.annotation.SuppressLint
import android.content.Intent
import android.os.Bundle
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import iskramobile.Iskramobile

class MainActivity : AppCompatActivity() {

    private lateinit var webView: WebView
    private var port: Int = 0

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Start Go core
        val dataDir = filesDir.absolutePath
        port = Iskramobile.start(dataDir, 0).toInt()

        if (port == 0) {
            Toast.makeText(this, "Ошибка запуска ядра", Toast.LENGTH_LONG).show()
            finish()
            return
        }

        // Start foreground service to keep alive
        startForegroundService(Intent(this, IskraService::class.java))

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
            // Pass invite link to WebView JS
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
        Iskramobile.stop()
    }
}
