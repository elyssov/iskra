package com.iskra.app

import android.annotation.SuppressLint
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.wifi.WifiManager
import android.net.wifi.p2p.WifiP2pConfig
import android.net.wifi.p2p.WifiP2pDevice
import android.net.wifi.p2p.WifiP2pInfo
import android.net.wifi.p2p.WifiP2pManager
import android.os.Looper
import android.util.Log
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * Manages Wi-Fi Direct for Iskra mesh wave protocol.
 *
 * This class handles:
 * - Peer discovery (finding other Iskra devices)
 * - P2P connection (one becomes Group Owner, other is client)
 * - Bidirectional sync via TCP over the P2P link
 * - Disconnect after sync
 *
 * Called from Go via gomobile JNI bridge.
 */
@SuppressLint("MissingPermission")
class WifiDirectManager(private val context: Context) {

    companion object {
        private const val TAG = "IskraWifiDirect"
        private const val ISKRA_SERVICE_NAME = "iskra"
    }

    private var manager: WifiP2pManager? = null
    private var channel: WifiP2pManager.Channel? = null
    private val discoveredPeers = mutableListOf<WifiP2pDevice>()
    private var connectionInfo: WifiP2pInfo? = null
    private var isInitialized = false

    private var discoveryLatch: CountDownLatch? = null
    private var connectionLatch: CountDownLatch? = null

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                WifiP2pManager.WIFI_P2P_PEERS_CHANGED_ACTION -> {
                    // Peers list updated
                    manager?.requestPeers(channel) { peerList ->
                        discoveredPeers.clear()
                        discoveredPeers.addAll(peerList.deviceList)
                        Log.i(TAG, "Discovered ${discoveredPeers.size} peers")
                        discoveryLatch?.countDown()
                    }
                }
                WifiP2pManager.WIFI_P2P_CONNECTION_CHANGED_ACTION -> {
                    // Connection state changed
                    manager?.requestConnectionInfo(channel) { info ->
                        connectionInfo = info
                        if (info?.groupFormed == true) {
                            Log.i(TAG, "P2P connected: isOwner=${info.isGroupOwner}, host=${info.groupOwnerAddress?.hostAddress}")
                            connectionLatch?.countDown()
                        }
                    }
                }
            }
        }
    }

    fun initialize() {
        if (isInitialized) return

        manager = context.getSystemService(Context.WIFI_P2P_SERVICE) as? WifiP2pManager
        channel = manager?.initialize(context, Looper.getMainLooper(), null)

        val filter = IntentFilter().apply {
            addAction(WifiP2pManager.WIFI_P2P_STATE_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_PEERS_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_CONNECTION_CHANGED_ACTION)
            addAction(WifiP2pManager.WIFI_P2P_THIS_DEVICE_CHANGED_ACTION)
        }
        context.registerReceiver(receiver, filter)
        isInitialized = true
        Log.i(TAG, "Wi-Fi Direct initialized")
    }

    /**
     * Check if device is connected to an infrastructure Wi-Fi network.
     */
    fun isWifiConnected(): Boolean {
        val wifiManager = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        val info = wifiManager.connectionInfo
        return info?.networkId != -1
    }

    /**
     * Scan for nearby Wi-Fi Direct peers.
     * Blocks for up to timeoutSec seconds.
     * Returns list of discovered device addresses.
     */
    fun scanPeers(timeoutSec: Int): List<String> {
        if (manager == null || channel == null) {
            Log.w(TAG, "Not initialized")
            return emptyList()
        }

        discoveredPeers.clear()
        discoveryLatch = CountDownLatch(1)

        manager?.discoverPeers(channel, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.i(TAG, "Discovery started")
            }
            override fun onFailure(reason: Int) {
                Log.e(TAG, "Discovery failed: reason=$reason")
                discoveryLatch?.countDown()
            }
        })

        discoveryLatch?.await(timeoutSec.toLong(), TimeUnit.SECONDS)
        manager?.stopPeerDiscovery(channel, null)

        return discoveredPeers.map { it.deviceAddress }
    }

    /**
     * Connect to a specific peer by MAC address.
     * Returns the Group Owner IP address for TCP sync.
     */
    fun connectToPeer(deviceAddress: String): String? {
        if (manager == null || channel == null) return null

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

        // Wait for connection (up to 15 seconds)
        connectionLatch?.await(15, TimeUnit.SECONDS)

        val info = connectionInfo
        return info?.groupOwnerAddress?.hostAddress
    }

    /**
     * Disconnect from current P2P group.
     */
    fun disconnect() {
        manager?.removeGroup(channel, object : WifiP2pManager.ActionListener {
            override fun onSuccess() {
                Log.i(TAG, "Disconnected from P2P group")
            }
            override fun onFailure(reason: Int) {
                Log.w(TAG, "Disconnect failed: reason=$reason")
            }
        })
        connectionInfo = null
    }

    /**
     * Cleanup on destroy.
     */
    fun destroy() {
        try {
            context.unregisterReceiver(receiver)
        } catch (_: Exception) {}
        manager?.stopPeerDiscovery(channel, null)
    }
}
