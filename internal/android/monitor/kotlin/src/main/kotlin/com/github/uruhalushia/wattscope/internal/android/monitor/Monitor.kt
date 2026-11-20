package com.github.uruhalushia.wattscope.internal.android.monitor

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.util.Log
import dev.mygo.Native
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.channels.Channel

fun makeMonitor(): Monitor = AndroidMonitor()

internal object MeterRecordingToggles {
    private val channel = Channel<Unit>(capacity = 8, onBufferOverflow = BufferOverflow.DROP_OLDEST)

    fun request() {
        channel.trySend(Unit)
    }

    suspend fun awaitNext() = channel.receive()

    fun clear() {
        while (channel.tryReceive().isSuccess) {
        }
    }
}

internal fun supportsMeterIsland(context: Context): Boolean =
    Build.VERSION.SDK_INT >= 33 &&
        Settings.System.getInt(context.contentResolver, "notification_focus_protocol", 0) >= 3

private class AndroidMonitor : Monitor {
    private var active = false
    private var notificationRequested = false

    @Synchronized
    override fun update(
        power: Double,
        voltage: Double,
        current: Double,
        energyWh: Double,
        chargeMAh: Double,
        measuredSec: Double,
        language: String,
        islandLeft: String,
        streaming: Boolean,
        recording: Boolean,
        islandEnabled: Boolean,
    ) {
        MeterForegroundService.updateNotification(
            MeterTelemetry(
                power = power,
                voltage = voltage,
                current = current,
                energyWh = energyWh,
                chargeMAh = chargeMAh,
                measuredSec = measuredSec,
                language = language,
                islandLeft = islandLeft,
                streaming = streaming,
                recording = recording,
                islandEnabled = islandEnabled,
            ),
        )
        if (!active) {
            active = true
            requestNotificationPermission()
            ensure()
        }
    }

    override fun supportsIsland(): Boolean = supportsMeterIsland(Native.context)

    override suspend fun awaitRecordingToggle() = MeterRecordingToggles.awaitNext()

    @Synchronized
    override fun ensure() {
        if (!active || MeterForegroundService.isRunning()) return
        try {
            Native.context.startForegroundService(Intent(Native.context, MeterForegroundService::class.java))
        } catch (failure: RuntimeException) {
            Log.w("WattScopeMonitor", "Could not start the meter foreground service", failure)
        }
    }

    @Synchronized
    override fun stop() {
        if (!active) return
        active = false
        MeterForegroundService.clearTelemetry()
        Native.context.stopService(Intent(Native.context, MeterForegroundService::class.java))
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT < 33 || Native.context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) return
        Handler(Looper.getMainLooper()).post {
            val activity = Native.activity
            if (activity != null && !notificationRequested) {
                notificationRequested = true
                try {
                    activity.requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 0x5741)
                } catch (failure: RuntimeException) {
                    Log.w("WattScopeMonitor", "Could not request notification permission", failure)
                }
            }
        }
    }
}
