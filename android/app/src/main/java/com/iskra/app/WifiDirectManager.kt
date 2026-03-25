package com.iskra.app

import android.annotation.SuppressLint
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.wifi.p2p.WifiP2pConfig
import android.net.wifi.p2p.WifiP2pDevice
import android.net.wifi.p2p.WifiP2pInfo
import android.net.wifi.p2p.WifiP2pManager
import android.net.wifi.p2p.nsd.WifiP2pDnsSdServiceInfo
import android.net.wifi.p2p.nsd.WifiP2pDnsSdServiceRequest
import android.os.Looper
import android.util.Log
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/**
 * WiFi Direct mesh manager for Iskra.
 *
 * Each device simultaneously:
 * 1. ADVERTISES itself via DNS-SD service ("_iskra._tcp")
 * 2. DISCOVERS other Iskra devices nearby
 * 3. CONNECTS to found peers (first come first served)
 * 4. Tells Go backend the peer IP via HTTP POST
 * 5. Go handles TCP mesh sync as usual
 */
@SuppressLint("MissingPermission")
class WifiDirectManager(private val context: Context) {

    companion object {
        private const val TAG = "IskraP2P"
        private const val SERVICE_TYPE = "_iskra._tcp"
        private const val SERVICE_NAME = "iskra_mesh"
        private const val SCAN_INTERVAL_MS = 20_000L // 20 seconds
        private const val CONNECT_TIMEOUT_SEC = 12L
    }

    private var manager: WifiP2pManager? = null
    private var channel: WifiP2pManager.Channel? = null
    private var goPort: Int = 0
    private val running = AtomicBoolean(false)
    private val connectedPeers = mutableSetOf<String>() // MAC addresses we already synced
    private val discoveredDevices = mutableMapOf<String, String>() // MAC -> device name

    private var connectionInfo: WifiP2pInfo? = null
    private var connectionLatch: CountDownLatch? = null

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(ctx: Context?, intent: Intent?) {
            when (intent?.action) {
                WifiP2pManager.WIFI_P2P_CONNECTION_CHANGED_ACTION -> {
                    manager?.requestConnectionInfo(channel) { info ->
                        connectionInfo = info
                        if (info?.groupFormed == true) {
                            Log.i(TAG, "P2P group formed: owner=${info.isGroupOwner}, host=${info.groupOwnerAddress?.hostAddress}")
                            connectionLatch?.countDown()
                        }
                    }
                }
                WifiP2pManager.WIFI_P2P_THIS_DEVICE_CHANGED_ACTION -> {
                    val device = intent.getParcelableExtra<WifiP2pDevice>(WifiP2pManager.EXTRA_WIFI_P2P_DEVICE)
                    Log.i(TAG, "This device: ${device?.deviceName} (${device?.status})")
                }
            }
        }
    }

    fun initialize() {
        manager = context.getSystemService(Context.WIFI_P2P_SERVICE) as? WifiP2pManager
        channel = manager?.initialize(context, Looper.getMainLooper(), null)

        val filter = IntentFilter().apply {
            addAction(WifiP2pManager.WIFI_P2P_STATE_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_PEERS_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_CONNECTION_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_THIS_DEVICE_CHANGED_ACTION)
        }
        context.registerReceiver(receiver, filter)
        Log.i(TAG, "WiFi Direct initialized")
    }

    /**
     * Start the mesh loop: advertise + discover + connect + notify Go.
     * Call from background thread.
     */
    fun startMeshLoop(port: Int) {
        goPort = port
        if (running.getAndSet(true)) return

        // Step 1: Register our service (advertise "I'M HERE!")
        registerService()

        // Step 2: Set up service discovery listeners
        setupServiceDiscovery()

        // Step 3: Start the scan-connect loop
        Thread {
            Log.i(TAG, "Mesh loop started (Go port=$port)")
            while (running.get()) {
                try {
                    discoverAndConnect()
                } catch (e: Exception) {
                    Log.e(TAG, "Mesh loop error: ${e.message}")
                }
                Thread.sleep(SCAN_INTERVAL_MS)
            }
        }.start()
    }

    /**
     * Register DNS-SD service so other Iskra devices can find us.
     * This is the "I'M HERE!" broadcast.
     */
    private fun registerService() {
        val record = mapOf(
            "port" to goPort.toString(),
            "app" to "iskra",
            "ver" to "1"
        )
        val serviceInfo = WifiP2pDnsSdServiceInfo.newInstance(
            SERVICE_NAME, SERVICE_TYPE, record
        )

        manager?.addLocalService(channel, serviceInfo, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.i(TAG, "Service registered: $SERVICE_NAME ($SERVICE_TYPE) port=$goPort")
            }
            override fun onFailure(reason: Int) {
                Log.e(TAG, "Service registration failed: reason=$reason")
            }
        })
    }

    /**
     * Set up DNS-SD discovery response listeners.
     * When another Iskra device is found, we store it.
     */
    private fun setupServiceDiscovery() {
        // TXT record listener — receives service metadata (port, app name)
        val txtListener = WifiP2pManager.DnsSdTxtRecordListener { _, record, device ->
            val app = record["app"] ?: ""
            if (app == "iskra") {
                Log.i(TAG, "Found Iskra device: ${device.deviceName} (${device.deviceAddress})")
                discoveredDevices[device.deviceAddress] = device.deviceName
            }
        }

        // Service response listener — receives service type
        val serviceListener = WifiP2pManager.DnsSdServiceResponseListener { _, _, device ->
            Log.d(TAG, "Service response from ${device.deviceName}")
            discoveredDevices[device.deviceAddress] = device.deviceName
        }

        manager?.setDnsSdResponseListeners(channel, serviceListener, txtListener)

        // Add service discovery request
        val request = WifiP2pDnsSdServiceRequest.newInstance()
        manager?.addServiceRequest(channel, request, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.i(TAG, "Service discovery request added")
            }
            override fun onFailure(reason: Int) {
                Log.e(TAG, "Service request failed: reason=$reason")
            }
        })
    }

    /**
     * One cycle: discover services, connect to new peers, notify Go.
     */
    private fun discoverAndConnect() {
        // Trigger service discovery
        val scanLatch = CountDownLatch(1)
        manager?.discoverServices(channel, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.d(TAG, "Service discovery started")
                // Give it time to collect responses
                Thread.sleep(5000)
                scanLatch.countDown()
            }
            override fun onFailure(reason: Int) {
                Log.w(TAG, "Service discovery failed: reason=$reason")
                scanLatch.countDown()
            }
        })
        scanLatch.await(10, TimeUnit.SECONDS)

        // Try to connect to each discovered device we haven't synced with yet
        val newPeers = discoveredDevices.keys.filter { it !in connectedPeers }
        for (mac in newPeers) {
            Log.i(TAG, "Connecting to ${discoveredDevices[mac]} ($mac)")
            val peerIP = connectToPeer(mac)
            if (peerIP != null) {
                Log.i(TAG, "Connected! Peer IP: $peerIP — notifying Go")
                connectedPeers.add(mac)
                notifyGoBackend(peerIP)

                // Stay connected briefly for sync, then disconnect for next peer
                Thread.sleep(3000)
                disconnect()
                Thread.sleep(1000)
            }
        }
    }

    /**
     * Connect to a specific peer by MAC address.
     * Returns the peer's IP address in the P2P group.
     */
    private fun connectToPeer(deviceAddress: String): String? {
        connectionInfo = null
        connectionLatch = CountDownLatch(1)

        val config = WifiP2pConfig().apply {
            this.deviceAddress = deviceAddress
        }

        manager?.connect(channel, config, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.i(TAG, "Connection initiated to $deviceAddress")
            }
            override fun onFailure(reason: Int) {
                Log.e(TAG, "Connection failed to $deviceAddress: reason=$reason")
                connectionLatch?.countDown()
            }
        })

        connectionLatch?.await(CONNECT_TIMEOUT_SEC, TimeUnit.SECONDS)

        val info = connectionInfo ?: return null
        if (!info.groupFormed) return null

        // The group owner's IP is always known
        // If we're the owner, the client connects to us — Go will see the incoming TCP connection
        // If we're the client, we need the owner's IP to connect
        return info.groupOwnerAddress?.hostAddress
    }

    /**
     * Tell Go backend about a discovered peer IP so it can do TCP mesh sync.
     */
    private fun notifyGoBackend(peerIP: String) {
        try {
            val url = URL("http://127.0.0.1:$goPort/api/mesh/add-peer")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            conn.connectTimeout = 3000
            conn.readTimeout = 3000
            conn.outputStream.write("""{"ip":"$peerIP"}""".toByteArray())
            val code = conn.responseCode
            Log.i(TAG, "Notified Go about peer $peerIP: HTTP $code")
            conn.disconnect()
        } catch (e: Exception) {
            Log.e(TAG, "Failed to notify Go: ${e.message}")
        }
    }

    private fun disconnect() {
        manager?.removeGroup(channel, object : WifiP2pManager.ActionListener {
            override fun onSuccess() { Log.d(TAG, "Disconnected from P2P group") }
            override fun onFailure(reason: Int) { Log.w(TAG, "Disconnect failed: $reason") }
        })
        connectionInfo = null
    }

    fun stop() {
        running.set(false)
        try {
            manager?.clearLocalServices(channel, null)
            manager?.clearServiceRequests(channel, null)
            manager?.stopPeerDiscovery(channel, null)
        } catch (_: Exception) {}
    }

    fun destroy() {
        stop()
        try {
            context.unregisterReceiver(receiver)
        } catch (_: Exception) {}
    }
}
