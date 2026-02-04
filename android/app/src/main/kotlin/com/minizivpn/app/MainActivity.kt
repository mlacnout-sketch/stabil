package com.minizivpn.app

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.EventChannel
import android.util.Log
import android.content.Intent
import android.net.VpnService
import android.os.Handler
import android.os.Looper
import android.os.Bundle

/**
 * ZIVPN Turbo Main Activity
 * Optimized for high-performance tunneling and aggressive cleanup.
 */
class MainActivity: FlutterActivity() {
    private val CHANNEL = "com.minizivpn.app/core"
    private val LOG_CHANNEL = "com.minizivpn.app/logs"
    private val REQUEST_VPN_CODE = 1
    
    private var logSink: EventChannel.EventSink? = null
    private val uiHandler = Handler(Looper.getMainLooper())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Ensure environment is clean on launch
        stopEngine()
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        
        EventChannel(flutterEngine.dartExecutor.binaryMessenger, LOG_CHANNEL).setStreamHandler(
            object : EventChannel.StreamHandler {
                override fun onListen(arguments: Any?, events: EventChannel.EventSink?) {
                    logSink = events
                    sendToLog("Logging system initialized.")
                }
                override fun onCancel(arguments: Any?) {
                    logSink = null
                }
            }
        )

        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL).setMethodCallHandler { call, result ->
            if (call.method == "startCore") {
                val ip = call.argument<String>("ip") ?: ""
                val range = call.argument<String>("port_range") ?: "6000-19999"
                val pass = call.argument<String>("pass") ?: ""
                val obfs = call.argument<String>("obfs") ?: "hu``hqb`c"
                val multiplier = call.argument<Double>("recv_window_multiplier") ?: 1.0
                val udpMode = call.argument<String>("udp_mode") ?: "tcp"
                val mtu = call.argument<Int>("mtu") ?: 1280

                // Save Config for ZivpnService
                getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
                    .edit()
                    .putString("server_ip", ip)
                    .putString("server_range", range)
                    .putString("server_pass", pass)
                    .putString("server_obfs", obfs)
                    .putFloat("multiplier", multiplier.toFloat())
                    .putString("udp_mode", udpMode)
                    .putInt("mtu", mtu)
                    .apply()

                sendToLog("Config saved. Ready to start VPN.")
                result.success("READY")
            } else if (call.method == "stopCore") {
                stopVpn()
                result.success("Stopped")
            } else if (call.method == "startVpn") {
                startVpn(result)
            } else {
                result.notImplemented()
            }
        }
    }

    private fun sendToLog(msg: String) {
        uiHandler.post {
            logSink?.success(msg)
        }
        Log.d("ZIVPN-Core", msg)
    }

    private fun startVpn(result: MethodChannel.Result) {
        val intent = VpnService.prepare(this)
        if (intent != null) {
            startActivityForResult(intent, REQUEST_VPN_CODE)
            result.success("REQUEST_PERMISSION")
            sendToLog("Requesting VPN permission...")
        } else {
            val serviceIntent = Intent(this, ZivpnService::class.java)
            serviceIntent.action = ZivpnService.ACTION_CONNECT
            startService(serviceIntent)
            result.success("STARTED")
            sendToLog("VPN Service started.")
        }
    }

    private fun stopVpn() {
        val serviceIntent = Intent(this, ZivpnService::class.java)
        serviceIntent.action = ZivpnService.ACTION_DISCONNECT
        startService(serviceIntent)
        sendToLog("VPN Service stopped.")
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQUEST_VPN_CODE) {
            if (resultCode == RESULT_OK) {
                val serviceIntent = Intent(this, ZivpnService::class.java)
                serviceIntent.action = ZivpnService.ACTION_CONNECT
                startService(serviceIntent)
                sendToLog("VPN permission granted. Starting service.")
            } else {
                sendToLog("VPN permission denied.")
            }
        }
    }

    private fun stopEngine() {
        val intent = Intent(this, ZivpnService::class.java)
        intent.action = ZivpnService.ACTION_DISCONNECT
        startService(intent)

        // Brute force cleanup for ALL instances of the cores
        try {
            val cleanupCmd = arrayOf("sh", "-c", "pkill -9 libuz; pkill -9 libload; pkill -9 libuz.so; pkill -9 libload.so")
            Runtime.getRuntime().exec(cleanupCmd).waitFor()
        } catch (e: Exception) {}
        
        sendToLog("Aggressive cleanup executed.")
    }
    
    override fun onDestroy() {
        stopEngine()
        super.onDestroy()
    }
}
