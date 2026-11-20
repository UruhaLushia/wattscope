package com.github.uruhalushia.wattscope.internal.android.monitor

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Path
import android.graphics.drawable.Icon
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import com.xzakota.hyper.notification.focus.FocusNotification
import com.xzakota.hyper.notification.focus.model.ActionInfo
import com.xzakota.hyper.notification.island.template.IslandTemplate
import org.json.JSONObject
import java.util.Locale
import kotlin.math.abs
import kotlin.math.pow
import kotlin.math.roundToInt

internal data class MeterTelemetry(
    val power: Double = 0.0,
    val voltage: Double = 0.0,
    val current: Double = 0.0,
    val energyWh: Double = 0.0,
    val chargeMAh: Double = 0.0,
    val measuredSec: Double = 0.0,
    val language: String = "",
    val islandLeft: String = "",
    val streaming: Boolean = false,
    val recording: Boolean = false,
    val islandEnabled: Boolean = false,
)

private data class PowerTier(
    val index: Int,
    val accentHex: String,
)

class MeterForegroundService : Service() {
    private var wakeLock: PowerManager.WakeLock? = null
    private var language = ""
    private var localizedContext: Context = this
    private lateinit var islandNotifier: IslandNotifier
    private val supportsIsland by lazy { supportsMeterIsland(this) }
    private val launchIntent by lazy {
        packageManager.getLaunchIntentForPackage(packageName)?.let {
            PendingIntent.getActivity(this, 0, it, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }
    }
    private val recordingIntent by lazy {
        PendingIntent.getService(
            this, 1, Intent(this, MeterForegroundService::class.java).setAction(TOGGLE_RECORDING),
            PendingIntent.FLAG_IMMUTABLE,
        )
    }

    override fun onCreate() {
        super.onCreate()
        islandNotifier = IslandNotifier(this)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == TOGGLE_RECORDING) {
            synchronized(MeterForegroundService::class.java) {
                if (instance == null) {
                    stopSelf(startId)
                } else if (pendingTelemetry.streaming) {
                    MeterRecordingToggles.request()
                }
            }
            return START_NOT_STICKY
        }
        try {
            synchronized(MeterForegroundService::class.java) {
                val data = pendingTelemetry
                localize(data.language)
                createChannel()
                val initial = notification(data)
                if (Build.VERSION.SDK_INT >= 29) {
                    startForeground(NOTIFICATION, initial, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
                } else {
                    startForeground(NOTIFICATION, initial)
                }
                instance = this
                if (showIsland(data)) islandNotifier.post(notification(data, true), true)
            }
            if (wakeLock == null) {
                wakeLock = getSystemService(PowerManager::class.java)
                    .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "$packageName:meter")
                    .apply { acquire() }
            }
        } catch (failure: RuntimeException) {
            Log.e("WattScopeMonitor", "Could not keep meter acquisition running", failure)
            stopSelf(startId)
        }
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        synchronized(MeterForegroundService::class.java) {
            if (instance === this) instance = null
        }
        islandNotifier.close()
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun localize(tag: String): Boolean {
        if (tag.isEmpty() || tag == language) return false
        val locale = when (tag) {
            "zh-Hans" -> Locale.SIMPLIFIED_CHINESE
            "zh-Hant" -> Locale.TRADITIONAL_CHINESE
            else -> Locale.forLanguageTag(tag)
        }
        localizedContext = createConfigurationContext(Configuration(resources.configuration).apply { setLocale(locale) })
        language = tag
        return true
    }

    private fun createChannel() {
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel(CHANNEL, localizedContext.getString(R.string.meter_notification_channel), NotificationManager.IMPORTANCE_LOW),
        )
    }

    private fun showIsland(data: MeterTelemetry) = data.islandEnabled && data.streaming && supportsIsland

    private fun notification(data: MeterTelemetry, island: Boolean = false): Notification {
        val appTitle = localizedContext.getString(R.string.meter_notification_title)
        val builder = Notification.Builder(this, CHANNEL)
            .setSmallIcon(applicationInfo.icon)
            .setContentIntent(launchIntent)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
        if (!data.streaming) {
            return builder.setContentTitle(appTitle)
                .setContentText(localizedContext.getString(R.string.meter_notification_waiting))
                .build()
        }

        val title = "${formatPowerDigits(data.power)} W"
        val voltage = "${formatFixedDigits(data.voltage, 2)} V"
        val current = "${formatFixedDigits(abs(data.current), 2)} A"
        val voltageCurrent = "$voltage · $current"
        val energy = "${formatFixedDigits(data.energyWh, 3)} Wh"
        val charge = if (data.chargeMAh >= 1000.0) {
            "${formatFixedDigits(data.chargeMAh / 1000.0, 3)} Ah"
        } else {
            "${formatFixedDigits(data.chargeMAh, 2)} mAh"
        }
        val whAh = "$energy · $charge"
        val content = "$voltageCurrent · $whAh"

        return builder
            .setContentTitle(title)
            .setContentText(content)
            .apply {
                val recButtonTitle = localizedContext.getString(
                    if (data.recording) R.string.meter_notification_record_stop
                    else R.string.meter_notification_record_start,
                )
                val recAction = Notification.Action.Builder(null, recButtonTitle, recordingIntent).build()
                if (!island) {
                    addAction(recAction)
                    return@apply
                }

                val tier = tierForPower(data.power)
                val duration = formatElapsed(data.measuredSec)
                val durationLabel = if (data.recording) "• $duration" else duration
                val durationColorLight = if (data.recording) "#DC2626" else "#6B7280"
                val durationColorDark = if (data.recording) "#F87171" else "#9CA3AF"
                val compactLeft = when (data.islandLeft) {
                    "off" -> null
                    "current" -> current
                    "energy" -> energy
                    "charge" -> charge
                    else -> voltage
                }

                val iconKey = "cap_${tier.index}"
                val iconDrawable = capsuleBadgeIcon(tier)
                val tickerText = "$title · $voltageCurrent"
                val capsuleRight = "${formatFixedDigits(data.power, 2)} W"
                fun IslandTemplate.configureCapsule(picName: String?) {
                    islandProperty = 1
                    highlightColor = tier.accentHex
                    bigIslandArea {
                        imageTextInfoLeft {
                            type = 1
                            picInfo {
                                type = 1
                                pic = picName
                            }
                            if (compactLeft != null) {
                                textInfo {
                                    this.title = compactLeft
                                    isTitleDigit = false
                                    narrowFont = false
                                }
                            }
                        }
                        imageTextInfoRight {
                            type = 2
                            textInfo {
                                this.title = capsuleRight
                                isTitleDigit = false
                                showHighlightColor = false
                                turnAnim = false
                            }
                        }
                    }
                    smallIslandArea {
                        picInfo {
                            type = 1
                            pic = picName
                        }
                    }
                }

                val v3Extras = FocusNotification.buildV3 {
                    val capsuleIcon = createPicture(iconKey, iconDrawable)
                    val recActionKey = createAction("rec_toggle", recAction)
                    updatable = true
                    enableFloat = false
                    islandFirstFloat = false
                    ticker = tickerText
                    tickerPic = capsuleIcon
                    island { configureCapsule(capsuleIcon) }
                    highlightInfo {
                        this.title = title
                        colorTitle = "#111827"
                        colorTitleDark = "#FFFFFF"
                        this.content = durationLabel
                        colorContent = durationColorLight
                        colorContentDark = durationColorDark
                    }
                    actions {
                        add(
                            ActionInfo().apply {
                                action = recActionKey
                                actionTitle = recButtonTitle
                                actionTitleColor = if (data.recording) "#FFFFFF" else "#111827"
                                actionTitleColorDark = "#FFFFFF"
                                actionBgColor = if (data.recording) "#DC2626" else "#E5E7EB"
                                actionBgColorDark = if (data.recording) "#EF4444" else "#3A3A3C"
                            },
                        )
                    }
                    highlightInfoV3 {
                        primaryText = voltageCurrent
                        primaryColor = "#111827"
                        primaryColorDark = "#FFFFFF"
                        secondaryText = "· $whAh"
                        secondaryColor = "#4B5563"
                        secondaryColorDark = "#D1D5DB"
                        highLightText = "\u200B"
                        highLightTextColor = "#00000000"
                        highLightTextColorDark = "#00000000"
                        highLightbgColor = "#00000000"
                        highLightbgColorDark = "#00000000"
                        actionInfo {
                            action = "miui.focus.action_noop"
                            actionTitle = ""
                            actionTitleColor = "#00000000"
                            actionTitleColorDark = "#00000000"
                            actionBgColor = "#00000000"
                            actionBgColorDark = "#00000000"
                        }
                    }
                }
                // focus-api 1.3 omits ActionInfo.type and clickWithCollapse; set them so V3 renders a non-collapsing text pill button.
                v3Extras.getString("miui.focus.param")?.let { rawParam ->
                    val root = JSONObject(rawParam)
                    root.optJSONObject("param_v2")?.optJSONArray("actions")?.optJSONObject(0)?.apply {
                        put("type", 2)
                        put("clickWithCollapse", false)
                    }
                    v3Extras.putString("miui.focus.param", root.toString())
                }
                addExtras(v3Extras)
            }
            .build()
    }

    companion object {
        const val CHANNEL = "meter_acquisition"
        const val NOTIFICATION = 1001
        private const val TOGGLE_RECORDING = "toggle_recording"

        private var instance: MeterForegroundService? = null
        private var pendingTelemetry = MeterTelemetry()

        private val tiers = arrayOf(
            // 0: Idle / trickle (< 1 W) — Muted slate & soft frost
            PowerTier(0, "#CBD5E1"),
            // 1: Normal charge (1 .. 17.9 W) — Subtle sage tint on matte dark
            PowerTier(1, "#A7F3D0"),
            // 2: Fast PD charge (18 .. 44.9 W) — Restrained cool steel-blue on matte dark
            PowerTier(2, "#BFDBFE"),
            // 3: Super-fast high power (>= 45 W) — Muted warm sand-amber on matte dark
            PowerTier(3, "#FDE68A"),
        )
        private val capsuleIconCache = arrayOfNulls<Icon>(tiers.size)

        private fun tierForPower(power: Double): PowerTier = when {
            power >= 45.0 -> tiers[3]
            power >= 18.0 -> tiers[2]
            power >= 1.0 -> tiers[1]
            else -> tiers[0]
        }

        private fun formatPowerDigits(power: Double): String {
            val p = abs(power)
            return when {
                p + 0.005 >= 100.0 -> String.format(Locale.US, "%.1f", power)
                else -> String.format(Locale.US, "%.2f", power)
            }
        }

        private fun formatFixedDigits(value: Double, maxDecimals: Int): String {
            val v = abs(value)
            var decimals = maxDecimals
            var threshold = 10.0
            while (decimals > 0 && v + 0.5 * 10.0.pow(-decimals) >= threshold) {
                decimals--
                threshold *= 10.0
            }
            return String.format(Locale.US, "%.${decimals}f", value)
        }

        private fun formatElapsed(sec: Double): String {
            val total = sec.roundToInt().coerceAtLeast(0)
            val h = total / 3600
            val m = (total % 3600) / 60
            val s = total % 60
            return when {
                h > 0 -> String.format(Locale.US, "%dh%02dm", h, m)
                m > 0 -> String.format(Locale.US, "%dm%02ds", m, s)
                else -> String.format(Locale.US, "%ds", s)
            }
        }

        @Synchronized
        private fun capsuleBadgeIcon(tier: PowerTier): Icon {
            capsuleIconCache[tier.index]?.let { return it }
            val size = 64
            val bmp = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
            val canvas = Canvas(bmp)
            val boltTint = Color.parseColor(tier.accentHex)
            drawLightningBolt(canvas, size.toFloat(), 0.72f, boltTint)
            return Icon.createWithBitmap(bmp).also { capsuleIconCache[tier.index] = it }
        }

        private fun drawLightningBolt(canvas: Canvas, size: Float, scale: Float, fillColor: Int) {
            val s = size * scale
            val ox = (size - s) * 0.5f
            val oy = (size - s) * 0.5f
            fun x(nx: Float) = ox + (nx / 24f) * s
            fun y(ny: Float) = oy + (ny / 24f) * s
            val path = Path().apply {
                moveTo(x(13.5f), y(2f))
                lineTo(x(4.5f), y(13.8f))
                lineTo(x(11.3f), y(13.8f))
                lineTo(x(10.2f), y(22f))
                lineTo(x(19.5f), y(10.2f))
                lineTo(x(12.7f), y(10.2f))
                close()
            }
            val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
                style = Paint.Style.FILL
                color = fillColor
            }
            canvas.drawPath(path, paint)
        }

        fun isRunning(): Boolean = synchronized(MeterForegroundService::class.java) { instance != null }

        internal fun clearTelemetry() = synchronized(MeterForegroundService::class.java) {
            pendingTelemetry = MeterTelemetry()
            MeterRecordingToggles.clear()
        }

        internal fun updateNotification(data: MeterTelemetry) = synchronized(MeterForegroundService::class.java) {
            pendingTelemetry = data
            instance?.let { service ->
                if (service.localize(data.language)) service.createChannel()
                val island = service.showIsland(data)
                service.islandNotifier.post(service.notification(data, island), island)
            }
        }
    }
}
