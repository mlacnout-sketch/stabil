package com.minizivpn.app

import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log
import android.app.PendingIntent
import android.app.Service
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationCompat
import android.content.pm.ServiceInfo
import java.net.InetAddress
import java.util.LinkedList
import androidx.annotation.Keep
import java.io.File
import org.json.JSONObject

import java.io.BufferedReader
import java.io.InputStreamReader

import android.os.PowerManager

/**
 * ZIVPN TunService
 * Handles the VpnService interface and integrates with tun2socks via JNI.
 */
@Keep
class ZivpnService : VpnService() {

    companion object {
        const val ACTION_CONNECT = "com.minizivpn.app.CONNECT"
        const val ACTION_DISCONNECT = "com.minizivpn.app.DISCONNECT"
        const val ACTION_LOG = "com.minizivpn.app.LOG"
        const val CHANNEL_ID = "ZIVPN_SERVICE_CHANNEL"
        const val NOTIFICATION_ID = 1
    }

    private var vpnInterface: ParcelFileDescriptor? = null
    private val processes = mutableListOf<Process>()
    private var wakeLock: PowerManager.WakeLock? = null

    private fun logToApp(msg: String) {
        val intent = Intent(ACTION_LOG)
        intent.putExtra("message", msg)
        sendBroadcast(intent)
        Log.d("ZIVPN-Core", msg)
    }

    private fun captureProcessLog(process: Process, name: String) {
        Thread {
            try {
                val reader = BufferedReader(InputStreamReader(process.inputStream))
                var line: String?
                while (reader.readLine().also { line = it } != null) {
                    logToApp("[$name] $line")
                }
            } catch (e: Exception) {
                logToApp("[$name] Log stream closed: ${e.message}")
            }
        }.start()
        
        Thread {
            try {
                val reader = BufferedReader(InputStreamReader(process.errorStream))
                var line: String?
                while (reader.readLine().also { line = it } != null) {
                    logToApp("[$name-ERR] $line")
                }
            } catch (e: Exception) {}
        }.start()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_CONNECT) {
             startForegroundService()
        }
        
        when (intent?.action) {
            ACTION_CONNECT -> {
                connect()
                return START_STICKY
            }
            ACTION_DISCONNECT -> {
                disconnect()
                return START_NOT_STICKY
            }
        }
        return START_NOT_STICKY
    }
    
    private fun startForegroundService() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ZIVPN Service Channel",
                NotificationManager.IMPORTANCE_LOW
            )
            val manager = getSystemService(NotificationManager::class.java)
            manager?.createNotificationChannel(channel)
        }

        val pendingIntent = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("MiniZIVPN Running")
            .setContentText("VPN Service is active")
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()

        if (Build.VERSION.SDK_INT >= 34) {
             startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
             startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
             startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun connect() {
        if (vpnInterface != null) return

        Log.i("ZIVPN-Tun", "Initializing ZIVPN (BadVPN C engine)...")
        
        val prefs = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
        
        val ip = prefs.getString("server_ip", "") ?: ""
        val range = prefs.getString("server_range", "") ?: ""
        val pass = prefs.getString("server_pass", "") ?: ""
        val obfs = prefs.getString("server_obfs", "") ?: ""
        val upMbps = prefs.getString("up_mbps", "15") ?: "15"
        val downMbps = prefs.getString("down_mbps", "20") ?: "20"
        val multiplier = prefs.getFloat("multiplier", 1.0f)
        val mtu = prefs.getInt("mtu", 1500)
        val logLevel = prefs.getString("log_level", "info") ?: "info"
        val coreCount = prefs.getInt("core_count", 4)
        val useWakelock = prefs.getBoolean("cpu_wakelock", false)

        if (useWakelock) {
            val powerManager = getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "MiniZivpn::CoreWakelock")
            wakeLock?.acquire()
            logToApp("CPU Wakelock acquired")
        }

        // 1. START HYSTERIA & LOAD BALANCER
        try {
            startPdnsd()
            startCores(ip, range, pass, obfs, upMbps, downMbps, multiplier.toDouble(), coreCount, logLevel)
        } catch (e: Exception) {
            Log.e("ZIVPN-Tun", "Failed to start cores: ${e.message}")
            stopSelf()
            return
        }

        val pendingIntent = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        // 2. Build VPN Interface
        val builder = Builder()
        builder.setSession("MiniZivpn")
        builder.setConfigureIntent(pendingIntent)
        builder.setMtu(mtu)
        
        // ZIVPN Style IP/Route
        builder.addAddress("169.254.1.2", 24)
        builder.addDnsServer("1.1.1.1")
        builder.addDnsServer("8.8.8.8")
        
        try {
            builder.addRoute("0.0.0.0", 0)
        } catch (e: Exception) {
            val subnets = listOf(
                "0.0.0.0" to 1, "128.0.0.0" to 1
            )
            for ((addr, mask) in subnets) {
                try { builder.addRoute(addr, mask) } catch (ex: Exception) {}
            }
        }

        try {
            builder.addDisallowedApplication(packageName)
        } catch (e: Exception) {}

        try {
            vpnInterface = builder.establish()
            val fd = vpnInterface?.fd ?: return

            Log.i("ZIVPN-Tun", "VPN Interface established. FD: $fd")

            /**
             * BadVPN Tun2Socks Parameter Explanation:
             * --netif-ipaddr: Virtual IP address for the VPN interface (169.254.1.2).
             * --netif-netmask: Netmask for the virtual interface.
             * --socks-server-addr: Local SOCKS5 proxy server address (Port 7777, Load Balancer).
             * --tunmtu: Maximum Transmission Unit size (usually 1500).
             * --tunfd: File Descriptor of the TUN interface created by Android VpnService.
             * --sock: Path to the control socket file (used for status/control).
             * --loglevel: Log verbosity level (3 = info).
             * --udpgw-remote-server-addr: BadVPN UDP Gateway server address (127.0.0.1:7300).
             * --dnsgw: DNS gateway address (169.254.1.1:8091, served by pdnsd).
             */
            val libDir = applicationInfo.nativeLibraryDir
            val libTun = File(libDir, "libtun2socks.so").absolutePath
            val sockPath = File(filesDir, "sock_path").absolutePath
            
            val tunCmd = arrayListOf(
                libTun,
                "--netif-ipaddr", "169.254.1.2",
                "--netif-netmask", "255.255.255.0",
                "--socks-server-addr", "127.0.0.1:7777",
                "--tunmtu", mtu.toString(),
                "--tunfd", fd.toString(),
                "--loglevel", "3",
                "--udpgw-remote-server-addr", "127.0.0.1:7300",
                "--sock", sockPath,
                "--dnsgw", "169.254.1.1:8091"
            )
            
            logToApp("Starting Engine: libtun2socks.so...")
            val tunPb = ProcessBuilder(tunCmd)
            tunPb.directory(filesDir)
            tunPb.environment()["LD_LIBRARY_PATH"] = libDir
            tunPb.redirectErrorStream(true)
            
            val tunProc = tunPb.start()
            processes.add(tunProc)
            captureProcessLog(tunProc, "Tun2Socks-C")
            
            logToApp("BadVPN Engine Started successfully.")
            prefs.edit().putBoolean("flutter.vpn_running", true).apply()

        } catch (e: Throwable) {
            Log.e("ZIVPN-Tun", "Error starting VPN: ${e.message}")
            stopSelf()
        }
    }

    private fun startPdnsd() {
        val libDir = applicationInfo.nativeLibraryDir
        val libPdnsd = File(libDir, "libpdnsd.so").absolutePath
        val confFile = File(filesDir, "pdnsd.conf")
        
        val confContent = """
            global {
                perm_cache=1024;
                cache_dir="${filesDir.absolutePath}";
                server_port = 8091;
                server_ip = 169.254.1.1;
                query_method=tcp_only;
                min_ttl=15m;
                max_ttl=1w;
                timeout=10;
                daemon=off;
            }
            server {
                label= "google1";
                ip = 8.8.8.8;
                port = 53;
                uptest = none;
            }
            server {
                label= "google2";
                ip = 8.8.4.4;
                port = 53;
                uptest = none;
            }
        """.trimIndent()
        
        confFile.writeText(confContent)
        
        val pdnsdCmd = arrayListOf(libPdnsd, "-v9", "-c", confFile.absolutePath)
        val pdnsdPb = ProcessBuilder(pdnsdCmd)
        pdnsdPb.directory(filesDir)
        pdnsdPb.environment()["LD_LIBRARY_PATH"] = libDir
        pdnsdPb.redirectErrorStream(true)
        
        val p = pdnsdPb.start()
        processes.add(p)
        captureProcessLog(p, "PDNSD")
        logToApp("DNS Gateway active on 169.254.1.1:8091")
    }

    private fun startCores(ip: String, range: String, pass: String, obfs: String, upMbps: String, downMbps: String, multiplier: Double, coreCount: Int, logLevel: String) {
        val libDir = applicationInfo.nativeLibraryDir
        val libUz = File(libDir, "libuz.so").absolutePath
        val libLoad = File(libDir, "libload.so").absolutePath
        
        val baseConn = 131072
        val baseWin = 327680
        val dynamicConn = (baseConn * multiplier).toInt()
        val dynamicWin = (baseWin * multiplier).toInt()
        
        val ports = (0 until coreCount).map { 20080 + it }
        val tunnelTargets = mutableListOf<String>()

        // Map log level for Hysteria
        val hyLogLevel = when(logLevel) {
            "silent" -> "disable"
            "error" -> "error"
            "debug" -> "debug"
            else -> "info"
        }

        /**
         * Hysteria Core Parameters (libuz.so):
         * -s [obfs]: Obfuscation password.
         * --config [json_string]: Complete configuration in JSON format.
         * 
         * JSON Config Fields:
         * "server": Remote server address (IP:Range).
         * "obfs": Obfuscation password.
         * "auth": Authentication password.
         * "up"/"down": Bandwidth limits.
         * "socks5": Local SOCKS5 listener config.
         * "udpgw": Local UDPGW listener config (port 7300).
         * "insecure": Allow self-signed certificates.
         */
        for (port in ports) {
            val hyConfig = JSONObject()
            hyConfig.put("server", "$ip:$range")
            hyConfig.put("obfs", obfs)
            hyConfig.put("auth", pass)
            hyConfig.put("loglevel", hyLogLevel)
            hyConfig.put("up", "$upMbps mbps")
            hyConfig.put("down", "$downMbps mbps")
            
            val socks5Json = JSONObject()
            socks5Json.put("listen", "127.0.0.1:$port")
            hyConfig.put("socks5", socks5Json)
            
            // ENABLE UDPGW SERVER in Hysteria Core
            val udpgwJson = JSONObject()
            udpgwJson.put("listen", "127.0.0.1:7300")
            hyConfig.put("udpgw", udpgwJson)
            
            hyConfig.put("insecure", true)
            hyConfig.put("recvwindowconn", dynamicConn)
            hyConfig.put("recvwindow", dynamicWin)
            
            val hyCmd = arrayListOf(libUz, "-s", obfs, "--config", hyConfig.toString())
            
            logToApp("Starting Hysteria Core on port $port...")
            val hyPb = ProcessBuilder(hyCmd)
            hyPb.directory(filesDir)
            hyPb.environment()["LD_LIBRARY_PATH"] = libDir
            hyPb.redirectErrorStream(true)
            
            val p = hyPb.start()
            processes.add(p)
            captureProcessLog(p, "Hysteria-$port")
            tunnelTargets.add("127.0.0.1:$port")
        }
        
        logToApp("Waiting for cores to warm up...")
        Thread.sleep(1500)

        /**
         * Load Balancer Parameters (libload.so):
         * -lport [port]: Port to listen for combined SOCKS5 traffic (7777).
         * -tunnel [target1] [target2] ...: List of Hysteria SOCKS5 endpoints to balance.
         */
        val lbCmd = mutableListOf(libLoad, "-lport", "7777", "-tunnel")
        lbCmd.addAll(tunnelTargets)
        
        logToApp("Starting Load Balancer on port 7777...")
        val lbPb = ProcessBuilder(lbCmd)
        lbPb.directory(filesDir)
        lbPb.environment()["LD_LIBRARY_PATH"] = libDir
        lbPb.redirectErrorStream(true)
        
        val lbProcess = lbPb.start()
        processes.add(lbProcess)
        captureProcessLog(lbProcess, "LoadBalancer")
        logToApp("Load Balancer active.")
    }

    private fun disconnect() {
        Log.i("ZIVPN-Tun", "Stopping VPN and cores...")
        
        if (wakeLock?.isHeld == true) {
            wakeLock?.release()
            logToApp("CPU Wakelock released")
        }
        wakeLock = null
        
        // Remove mobile.Mobile.stop() as we don't use the AAR anymore

        processes.forEach { 
            try {
                if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
                    it.destroyForcibly()
                } else {
                    it.destroy()
                }
            } catch(e: Exception){} 
        }
        processes.clear()

        // Optimized: Run cleanup in background to prevent ANR
        Thread {
            try {
                val sockPath = File(filesDir, "sock_path")
                if (sockPath.exists()) sockPath.delete()
                
                // Brute force kill ZIVPN binaries
                val cleanupCmd = arrayOf("sh", "-c", "pkill -9 libuz; pkill -9 libload; pkill -9 libtun2socks; pkill -9 libpdnsd; pkill -9 libuz.so; pkill -9 libload.so; pkill -9 libtun2socks.so; pkill -9 libpdnsd.so")
                Runtime.getRuntime().exec(cleanupCmd).waitFor()
            } catch (e: Exception) {}
        }.start()

        vpnInterface?.close()
        vpnInterface = null
        
        val prefs = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
        prefs.edit().putBoolean("flutter.vpn_running", false).apply()
        
        stopForeground(true)
        stopSelf()
    }

    override fun onDestroy() {
        disconnect()
        super.onDestroy()
    }
}