package com.github.uruhalushia.wattscope.internal.android.monitor

import android.app.Notification
import android.app.NotificationManager
import android.content.ComponentName
import android.content.Context
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.os.Binder
import android.os.IBinder
import android.os.Parcel
import android.os.Process
import android.util.Log
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.atomic.AtomicBoolean
import rikka.shizuku.Shizuku
import kotlin.system.exitProcess

private const val DESCRIPTOR = "wattscope.island.network"
private const val BEGIN = IBinder.FIRST_CALL_TRANSACTION
private const val END = IBinder.FIRST_CALL_TRANSACTION + 1
private const val DESTROY = 16777115

// Keep network changes in the Shizuku process so its watchdog can restore them
// even if the app is killed between posting the notification and restoring them.
class IslandNetworkService(private val context: Context) : Binder() {
    private val appUid = context.applicationInfo.uid
    private val executor = Executors.newSingleThreadScheduledExecutor()
    private var restore: (() -> Unit)? = null
    private var watchdog: ScheduledFuture<*>? = null

    @Synchronized
    override fun onTransact(code: Int, data: Parcel, reply: Parcel?, flags: Int): Boolean {
        val caller = getCallingUid()
        if (code == DESTROY && caller == Process.myUid()) {
            restoreNetwork()
            executor.shutdownNow()
            exitProcess(0)
        }
        if (code != BEGIN && code != END) return super.onTransact(code, data, reply, flags)
        check(caller == appUid && (Process.myUid() == 0 || Process.myUid() == 2000))
        data.enforceInterface(DESCRIPTOR)
        val identity = clearCallingIdentity()
        val ok = try {
            if (code == BEGIN) blockNetwork() else restoreNetwork()
            true
        } catch (failure: Exception) {
            Log.w("WattScopeIsland", "Could not change Xiaomi service networking", failure)
            try { restoreNetwork() } catch (_: Exception) { }
            false
        } finally {
            restoreCallingIdentity(identity)
        }
        reply?.writeNoException()
        reply?.writeInt(if (ok) 1 else 0)
        return true
    }

    private fun blockNetwork() {
        restoreNetwork()
        val uid = context.packageManager.getPackageUid("com.xiaomi.xmsf", 0)
        val manager = Class.forName("android.os.ServiceManager").getMethod("getService", String::class.java)
            .invoke(null, Context.CONNECTIVITY_SERVICE) as IBinder
        val stub = Class.forName("android.net.IConnectivityManager\$Stub")
        val api = Class.forName("android.net.IConnectivityManager")
        val connectivity = stub.getMethod("asInterface", IBinder::class.java).invoke(null, manager)
        val getChain = api.getMethod("getFirewallChainEnabled", Int::class.javaPrimitiveType)
        val setChain = api.getMethod("setFirewallChainEnabled", Int::class.javaPrimitiveType, Boolean::class.javaPrimitiveType)
        val getRule = api.getMethod("getUidFirewallRule", Int::class.javaPrimitiveType, Int::class.javaPrimitiveType)
        val setRule = api.getMethod("setUidFirewallRule", Int::class.javaPrimitiveType, Int::class.javaPrimitiveType, Int::class.javaPrimitiveType)
        val chain = 9 // FIREWALL_CHAIN_OEM_DENY_3
        val wasEnabled = getChain.invoke(connectivity, chain) as Boolean
        val oldRule = getRule.invoke(connectivity, chain, uid) as Int
        restore = {
            try {
                setRule.invoke(connectivity, chain, uid, oldRule)
            } finally {
                if (!wasEnabled) setChain.invoke(connectivity, chain, false)
            }
        }
        watchdog = executor.schedule({ synchronized(this) { restoreNetwork() } }, 2, TimeUnit.SECONDS)
        setRule.invoke(connectivity, chain, uid, 2)
        if (!wasEnabled) setChain.invoke(connectivity, chain, true)
    }

    private fun restoreNetwork() {
        watchdog?.cancel(false)
        watchdog = null
        val action = restore ?: return
        try {
            action()
            restore = null
        } catch (failure: Exception) {
            Log.e("WattScopeIsland", "Could not restore Xiaomi service networking; retrying", failure)
            watchdog = executor.schedule({ synchronized(this) { try { restoreNetwork() } catch (_: Exception) { } } }, 1, TimeUnit.SECONDS)
            throw failure
        }
    }
}

internal class IslandNotifier(private val context: Context) {
    private val executor = Executors.newSingleThreadExecutor()
    private var pending: Pair<Notification, Boolean>? = null
    private var scheduled = false
    @Volatile private var closed = false
    @Volatile private var binder: IBinder? = null
    private val binding = AtomicBoolean()
    private val args = Shizuku.UserServiceArgs(ComponentName(context, IslandNetworkService::class.java))
        .processNameSuffix("island_network").daemon(false)
    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, service: IBinder) { binder = service }
        override fun onServiceDisconnected(name: ComponentName) { binder = null; binding.set(false) }
    }

    @Synchronized
    fun post(notification: Notification, island: Boolean) {
        if (closed) return
        if (island && binder == null && binding.compareAndSet(false, true)) {
            try {
                if (Shizuku.pingBinder() && Shizuku.checkSelfPermission() == PackageManager.PERMISSION_GRANTED) {
                    Shizuku.bindUserService(args, connection)
                } else {
                    binding.set(false)
                }
            } catch (failure: RuntimeException) {
                binding.set(false)
                Log.w("WattScopeIsland", "Could not bind Shizuku service", failure)
            }
        }
        pending = notification to island
        if (!scheduled) {
            scheduled = true
            executor.execute {
                while (true) {
                    val (next, isIsland) = takePending() ?: break
                    publish(next, isIsland)
                }
            }
        }
    }

    @Synchronized
    private fun takePending(): Pair<Notification, Boolean>? {
        val next = pending
        pending = null
        if (next == null) scheduled = false
        return next
    }

    private fun publish(notification: Notification, island: Boolean) {
        val remote = if (island) binder else null
        var blocked = false
        try {
            if (remote != null) {
                try {
                    blocked = transact(remote, BEGIN)
                } catch (failure: Exception) {
                    Log.w("WattScopeIsland", "Could not temporarily block Xiaomi service networking", failure)
                }
            }
            val posted = synchronized(this) {
                if (closed) false else {
                    context.getSystemService(NotificationManager::class.java).notify(MeterForegroundService.NOTIFICATION, notification)
                    true
                }
            }
            if (blocked && posted) Thread.sleep(300)
        } catch (failure: Exception) {
            Log.w("WattScopeIsland", "Could not post island notification", failure)
        } finally {
            if (blocked && remote != null) {
                try {
                    transact(remote, END)
                } catch (failure: Exception) {
                    Log.w("WattScopeIsland", "Could not restore Xiaomi service networking", failure)
                }
            }
        }
    }

    @Synchronized
    fun close() {
        closed = true
        pending = null
        executor.execute {
            if (binding.get()) {
                try { Shizuku.unbindUserService(args, connection, true) } catch (_: RuntimeException) { }
            }
        }
        executor.shutdown()
    }

    private fun transact(remote: IBinder, code: Int): Boolean {
        val data = Parcel.obtain()
        val reply = Parcel.obtain()
        try {
            data.writeInterfaceToken(DESCRIPTOR)
            check(remote.transact(code, data, reply, 0))
            reply.readException()
            return reply.readInt() != 0
        } finally {
            data.recycle()
            reply.recycle()
        }
    }
}
