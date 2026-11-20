package com.github.uruhalushia.wattscope.internal.transport.usbhost

import android.content.ComponentName
import android.content.Context
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.hardware.usb.UsbDevice
import android.os.Binder
import android.os.IBinder
import android.os.Parcel
import android.os.Process
import android.util.Log
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import rikka.shizuku.Shizuku
import kotlin.system.exitProcess

private const val USB_GRANT = IBinder.FIRST_CALL_TRANSACTION
private const val SERVICE_DESTROY = 16777115
private const val DESCRIPTOR = "io.github.UruhaLushia.wattscope.UsbPermissionService"
private const val LOG_TAG = "WattScopeUsb"
private val grantLock = Mutex()
private val permissionRequests = AtomicInteger()

internal fun shizukuAccess(allowShell: Boolean = false): ShizukuAccess = try {
    if (!Shizuku.pingBinder() || Shizuku.isPreV11() || Shizuku.getVersion() < 13) {
        ShizukuAccess.SHIZUKU_UNAVAILABLE
    } else {
        val uid = Shizuku.getUid()
        when {
            uid != 0 && !(allowShell && uid == 2000) -> ShizukuAccess.SHIZUKU_REQUIRES_ROOT
            Shizuku.checkSelfPermission() != PackageManager.PERMISSION_GRANTED -> ShizukuAccess.SHIZUKU_NOT_AUTHORIZED
            uid == 0 -> ShizukuAccess.SHIZUKU_AUTHORIZED
            else -> ShizukuAccess.SHIZUKU_SHELL_AUTHORIZED
        }
    }
} catch (failure: RuntimeException) {
    Log.d(LOG_TAG, "Could not check Shizuku availability", failure)
    ShizukuAccess.SHIZUKU_UNAVAILABLE
}

internal suspend fun requestShizukuAccess(allowShell: Boolean): ShizukuAccess = withContext(Dispatchers.Main) {
    val access = shizukuAccess(allowShell)
    if (access == ShizukuAccess.SHIZUKU_NOT_AUTHORIZED && !Shizuku.shouldShowRequestPermissionRationale()) {
        requestShizukuPermission()
        return@withContext shizukuAccess(allowShell)
    }
    access
}

internal suspend fun grantUsbPermission(context: Context, device: UsbDevice): Boolean =
    withContext(Dispatchers.IO) {
        grantLock.withLock {
            try {
                if (shizukuAccess() != ShizukuAccess.SHIZUKU_AUTHORIZED) return@withLock false
                val granted = requestGrant(context, device)
                if (granted) Log.i(LOG_TAG, "Granted USB access through Shizuku (root, UID 0)")
                granted
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (failure: Exception) {
                Log.w(LOG_TAG, "Shizuku USB grant failed", failure)
                false
            }
        }
    }

private suspend fun requestShizukuPermission() {
    val requestCode = permissionRequests.incrementAndGet()
    val result = CompletableDeferred<Unit>()
    val listener = Shizuku.OnRequestPermissionResultListener { code, _ ->
        if (code == requestCode) result.complete(Unit)
    }
    Shizuku.addRequestPermissionResultListener(listener)
    try {
        Log.i(LOG_TAG, "Requesting Shizuku authorization")
        Shizuku.requestPermission(requestCode)
        withTimeoutOrNull(45_000) { result.await() }
    } finally {
        Shizuku.removeRequestPermissionResultListener(listener)
    }
}

private suspend fun requestGrant(context: Context, device: UsbDevice): Boolean {
    val args = Shizuku.UserServiceArgs(ComponentName(context, UsbPermissionService::class.java))
        .processNameSuffix("usb_grant")
        .daemon(false)
    val connected = CompletableDeferred<IBinder?>()
    val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            connected.complete(binder)
        }

        override fun onServiceDisconnected(name: ComponentName) {
            connected.complete(null)
        }
    }
    try {
        Shizuku.bindUserService(args, connection)
        val binder = withTimeoutOrNull(4_000) { connected.await() } ?: return false
        val request = Parcel.obtain()
        val reply = Parcel.obtain()
        try {
            request.writeInterfaceToken(DESCRIPTOR)
            request.writeTypedObject(device, 0)
            if (!binder.transact(USB_GRANT, request, reply, 0)) return false
            reply.readException()
            return reply.readInt() != 0
        } finally {
            reply.recycle()
            request.recycle()
        }
    } finally {
        try {
            Shizuku.unbindUserService(args, connection, true)
        } catch (failure: RuntimeException) {
            Log.w(LOG_TAG, "Could not stop Shizuku USB grant service", failure)
        }
    }
}

class UsbPermissionService(context: Context) : Binder() {
    private val appUid = context.applicationInfo.uid

    override fun onTransact(code: Int, data: Parcel, reply: Parcel?, flags: Int): Boolean {
        val caller = getCallingUid()
        if (code == SERVICE_DESTROY && caller == Process.myUid()) exitProcess(0)
        if (code != USB_GRANT) return super.onTransact(code, data, reply, flags)
        check(Process.myUid() == 0 && caller == appUid)
        data.enforceInterface(DESCRIPTOR)
        val identity = clearCallingIdentity()
        val granted = try {
            grantAttachedDevice(data)
        } catch (failure: Exception) {
            Log.w(LOG_TAG, "Android USB permission grant failed", failure)
            false
        } finally {
            restoreCallingIdentity(identity)
        }
        reply?.writeNoException()
        reply?.writeInt(if (granted) 1 else 0)
        return true
    }

    private fun grantAttachedDevice(parcelledDevice: Parcel): Boolean {
        val systemServices = Class.forName("android.os.ServiceManager")
        val binder = systemServices.getMethod("getService", String::class.java)
            .invoke(null, Context.USB_SERVICE) as? IBinder ?: return false
        val stub = Class.forName("android.hardware.usb.IUsbManager\$Stub")
        val transaction = stub.getDeclaredField("TRANSACTION_grantDevicePermission")
            .apply { isAccessible = true }.getInt(null)
        val descriptor = binder.interfaceDescriptor ?: return false
        val request = Parcel.obtain()
        val reply = Parcel.obtain()
        try {
            request.writeInterfaceToken(descriptor)
            request.appendFrom(parcelledDevice, parcelledDevice.dataPosition(), parcelledDevice.dataAvail())
            request.writeInt(appUid)
            if (!binder.transact(transaction, request, reply, 0)) return false
            reply.readException()
            return true
        } finally {
            reply.recycle()
            request.recycle()
        }
    }
}
