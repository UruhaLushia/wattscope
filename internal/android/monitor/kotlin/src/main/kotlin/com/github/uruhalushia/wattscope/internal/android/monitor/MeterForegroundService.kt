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
import android.view.View
import android.widget.RemoteViews
import com.xzakota.hyper.notification.focus.FocusNotification
import com.xzakota.hyper.notification.focus.template.CustomFocusTemplate
import com.xzakota.hyper.notification.focus.template.CustomFocusTemplateV3
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
    val deviceLabel: String = "",
    val language: String = "",
    val islandLeft: String = "",
    val streaming: Boolean = false,
    val recording: Boolean = false,
    val islandEnabled: Boolean = false,
)

private data class PowerTier(
    val index: Int,
    val accentHex: String,
    val accentLightTextHex: String,
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

    override fun onCreate() {
        super.onCreate()
        islandNotifier = IslandNotifier(this)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
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

        val powerDigits = formatPowerDigits(data.power)
        val title = "$powerDigits W"
        val voltageCurrent = String.format(Locale.US, "%.2f V · %.2f A", data.voltage, abs(data.current))
        val energy = String.format(Locale.US, "%.3f Wh", data.energyWh)
        val charge = if (data.chargeMAh >= 1000.0) {
            String.format(Locale.US, "%.3f Ah", data.chargeMAh / 1000.0)
        } else {
            String.format(Locale.US, "%.1f mAh", data.chargeMAh)
        }
        val whAh = "$energy · $charge"
        val content = "$voltageCurrent · $whAh"

        return builder
            .setContentTitle(title)
            .setContentText(content)
            .apply {
                if (island) {
                    val tier = tierForPower(data.power)
                    val deviceTitle = data.deviceLabel.trim().ifEmpty { appTitle }
                    val duration = formatElapsed(data.measuredSec)
                    val recLabel = localizedContext.getString(R.string.meter_notification_recording)
                    val compactLeft = when (data.islandLeft) {
                        "off" -> null
                        "current" -> "${formatFixedDigits(abs(data.current), 2)} A"
                        "energy" -> "${formatFixedDigits(data.energyWh, 3)} Wh"
                        "charge" -> if (data.chargeMAh >= 1000.0) {
                            "${formatFixedDigits(data.chargeMAh / 1000.0, 3)} Ah"
                        } else {
                            "${formatFixedDigits(data.chargeMAh, 2)} mAh"
                        }
                        else -> "${formatFixedDigits(data.voltage, 2)} V"
                    }
                    fun bindCardViews(layoutRes: Int): RemoteViews = RemoteViews(packageName, layoutRes).apply {
                        setTextViewText(R.id.island_power_value, powerDigits)
                        setTextViewText(R.id.island_va, voltageCurrent)
                        setTextViewText(R.id.island_whah, whAh)
                        setTextViewText(R.id.island_device, deviceTitle)
                        setTextViewText(R.id.island_time, duration)
                        setTextViewText(R.id.island_rec_badge, "● $recLabel")
                        setViewVisibility(R.id.island_rec_badge, if (data.recording) View.VISIBLE else View.GONE)
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

                    val customBundle = FocusNotification.buildCustomV2 {
                        val capsuleIcon = createPicture(iconKey, iconDrawable)
                        val darkRv = bindCardViews(R.layout.meter_island_expanded)
                        val lightRv = bindCardViews(R.layout.meter_island_card_light)
                        updatable = true
                        enableFloat = false
                        ticker = tickerText
                        tickerPic = capsuleIcon
                        createRemoteViews(CustomFocusTemplateV3.LAYOUT_ISLAND_EXPAND, darkRv)
                        createRemoteViews(CustomFocusTemplate.LAYOUT_NIGHT, darkRv)
                        createRemoteViews(CustomFocusTemplate.LAYOUT, lightRv)
                    }
                    // focus-api 1.3's buildBundle() casts custom templates to CustomV2 and drops
                    // CustomFocusTemplateV3.island, while CustomV3 omits @SerialName("param_v2").
                    val customV3 = FocusNotification.createCustomV3 {
                        val capsuleIcon = createPicture(iconKey, iconDrawable)
                        updatable = true
                        enableFloat = false
                        islandFirstFloat = false
                        ticker = tickerText
                        tickerPic = capsuleIcon
                        island { configureCapsule(capsuleIcon) }
                    }
                    val customV3Root = JSONObject(customV3.getParamJSON())
                    val customV3Json = (customV3Root.optJSONObject("param_v2") ?: customV3Root.getJSONObject("param")).toString()

                    val v3Extras = FocusNotification.buildV3 {
                        val capsuleIcon = createPicture(iconKey, iconDrawable)
                        updatable = true
                        enableFloat = false
                        islandFirstFloat = false
                        ticker = tickerText
                        tickerPic = capsuleIcon
                        island { configureCapsule(capsuleIcon) }
                        baseInfo {
                            type = 2
                            this.title = title
                            colorTitle = "#111827"
                            colorTitleDark = "#FFFFFF"
                            this.content = content
                            colorContent = "#374151"
                            colorContentDark = "#E5E7EB"
                        }
                        hintInfo {
                            type = 1
                            this.title = deviceTitle
                            colorTitle = "#6B7280"
                            colorTitleDark = "#9CA3AF"
                            this.content = duration
                            colorContent = tier.accentLightTextHex
                            colorContentDark = tier.accentHex
                        }
                    }
                    v3Extras.putAll(customBundle)
                    v3Extras.putString("miui.focus.param.custom", customV3Json)
                    addExtras(v3Extras)
                }
            }
            .build()
    }

    companion object {
        const val CHANNEL = "meter_acquisition"
        const val NOTIFICATION = 1001
        private var instance: MeterForegroundService? = null
        private var pendingTelemetry = MeterTelemetry()

        private val tiers = arrayOf(
            // 0: Idle / trickle (< 1 W) — Muted slate & soft frost
            PowerTier(0, "#CBD5E1", "#475569"),
            // 1: Normal charge (1 .. 17.9 W) — Subtle sage tint on matte dark
            PowerTier(1, "#A7F3D0", "#047857"),
            // 2: Fast PD charge (18 .. 44.9 W) — Restrained cool steel-blue on matte dark
            PowerTier(2, "#BFDBFE", "#1D4ED8"),
            // 3: Super-fast high power (>= 45 W) — Muted warm sand-amber on matte dark
            PowerTier(3, "#FDE68A", "#B45309"),
        )
        private val capsuleIconCache = arrayOfNulls<Icon>(tiers.size)

        private fun tierForPower(power: Double): PowerTier = when {
            power >= 45.0 -> tiers[3]
            power >= 18.0 -> tiers[2]
            power >= 1.0 -> tiers[1]
            else -> tiers[0]
        }

        private fun formatPowerDigits(power: Double): String = when {
            power >= 100.0 -> String.format(Locale.US, "%.1f", power)
            else -> String.format(Locale.US, "%.2f", power)
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
