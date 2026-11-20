package com.github.uruhalushia.wattscope.internal.transport.usbhost

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.Context
import android.content.pm.ServiceInfo
import android.content.res.Configuration
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import java.util.Locale

class MeterForegroundService : Service() {
    private var wakeLock: PowerManager.WakeLock? = null
    private var language = ""
    private var localizedContext: Context = this

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        try {
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(NotificationChannel(CHANNEL, localizedContext.getString(R.string.meter_notification_channel), NotificationManager.IMPORTANCE_LOW))
            val notification = notification(localizedContext.getString(R.string.meter_notification_text))
            if (Build.VERSION.SDK_INT >= 29) {
                startForeground(NOTIFICATION, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
            } else {
                startForeground(NOTIFICATION, notification)
            }
            if (wakeLock == null) {
                wakeLock = getSystemService(PowerManager::class.java)
                    .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "$packageName:meter")
                    .apply { acquire() }
            }
            synchronized(MeterForegroundService::class.java) { instance = this }
        } catch (failure: RuntimeException) {
            Log.e("WattScopeUsb", "Could not keep meter acquisition running", failure)
            stopSelf(startId)
        }
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        synchronized(MeterForegroundService::class.java) { instance = null }
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun notification(text: String): Notification {
        val launch = packageManager.getLaunchIntentForPackage(packageName)
        return Notification.Builder(this, CHANNEL)
            .setSmallIcon(applicationInfo.icon)
            .setContentTitle(localizedContext.getString(R.string.meter_notification_title))
            .setContentText(text)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .apply {
                if (launch != null) setContentIntent(PendingIntent.getActivity(this@MeterForegroundService, 0, launch, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            }
            .build()
    }

    companion object {
        const val CHANNEL = "meter_acquisition"
        const val NOTIFICATION = 1001
        private var instance: MeterForegroundService? = null

        fun isRunning(): Boolean = synchronized(MeterForegroundService::class.java) { instance != null }

        fun updateNotification(text: String, language: String) = synchronized(MeterForegroundService::class.java) {
            instance?.let {
                if (language.isNotEmpty() && it.language != language) {
                    val locale = when (language) {
                        "zh-Hans" -> Locale.SIMPLIFIED_CHINESE
                        "zh-Hant" -> Locale.TRADITIONAL_CHINESE
                        else -> Locale.forLanguageTag(language)
                    }
                    it.localizedContext = it.createConfigurationContext(Configuration(it.resources.configuration).apply { setLocale(locale) })
                    it.language = language
                    it.getSystemService(NotificationManager::class.java).createNotificationChannel(
                        NotificationChannel(CHANNEL, it.localizedContext.getString(R.string.meter_notification_channel), NotificationManager.IMPORTANCE_LOW),
                    )
                }
                val content = text.ifEmpty { it.localizedContext.getString(R.string.meter_notification_waiting) }
                it.getSystemService(NotificationManager::class.java).notify(NOTIFICATION, it.notification(content))
            }
        }
    }
}
