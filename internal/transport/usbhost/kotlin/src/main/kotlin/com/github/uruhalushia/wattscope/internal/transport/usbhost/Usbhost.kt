package com.github.uruhalushia.wattscope.internal.transport.usbhost

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.hardware.usb.UsbDevice
import android.hardware.usb.UsbDeviceConnection
import android.hardware.usb.UsbEndpoint
import android.hardware.usb.UsbInterface
import android.hardware.usb.UsbManager
import android.os.Build
import dev.mygo.Native
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withTimeoutOrNull

fun makeHost(): Host = AndroidHost()

private class AndroidHost : Host {
    private data class Connection(
        val path: String,
        val usb: UsbDeviceConnection,
        val iface: UsbInterface,
        val input: UsbEndpoint,
        val output: UsbEndpoint?,
    )

    private val context get() = Native.context
    private val manager get() = checkNotNull(context.getSystemService(UsbManager::class.java))
    private val handles = ConcurrentHashMap<Long, Connection>()
    private val nextHandle = AtomicLong()

    override fun devices(vendor: Long, product: Long): List<String> =
        manager.deviceList.values
            .filter { it.vendorId.toLong() == vendor && it.productId.toLong() == product }
            .map { it.deviceName }
            .sorted()

    override fun shizukuStatus(): ShizukuAccess = rootShizukuAccess()

    override suspend fun authorizeShizuku(): ShizukuAccess = requestShizukuAccess()

    override suspend fun open(path: String, interfaceNumber: Long, input: Long, output: Long, prompt: Boolean): Long {
        val device = manager.deviceList[path] ?: throw UsbhostException(UsbhostError.DISCONNECTED)
        val iface = (0 until device.interfaceCount).asSequence().map(device::getInterface)
            .firstOrNull { it.id.toLong() == interfaceNumber && it.alternateSetting == 0 }
            ?: error("USB interface $interfaceNumber is unavailable")
        val endpoints = (0 until iface.endpointCount).asSequence().map(iface::getEndpoint).toList()
        val inputEndpoint = endpoints.firstOrNull { it.address.toLong() == input }
            ?: error("USB input endpoint $input is unavailable")
        val outputEndpoint = if (output < 0) {
            null
        } else {
            endpoints.firstOrNull { it.address.toLong() == output }
                ?: error("USB output endpoint $output is unavailable")
        }
        val rootAuthorized = rootShizukuAccess() == ShizukuAccess.SHIZUKU_AUTHORIZED
        val canPrompt = prompt && !rootAuthorized
        val permitted = manager.hasPermission(device) || if (rootAuthorized) {
            grantUsbPermission(context, device) && manager.hasPermission(device)
        } else {
            canPrompt && requestPermission(device)
        }
        if (!permitted) {
            if (!manager.deviceList.containsKey(path)) {
                throw UsbhostException(UsbhostError.DISCONNECTED)
            }
            if (!canPrompt) {
                val reason = if (rootAuthorized) "Shizuku could not grant USB access" else "USB access requires authorization"
                throw UsbhostException(UsbhostError.PERMISSION_REQUIRED, reason)
            }
            throw UsbhostException(UsbhostError.PERMISSION, "Allow USB access to the meter")
        }
        currentCoroutineContext().ensureActive()
        val connection = manager.openDevice(device) ?: throw UsbhostException(UsbhostError.DISCONNECTED)
        try {
            check(connection.claimInterface(iface, true)) { "Could not claim USB interface $interfaceNumber" }
            currentCoroutineContext().ensureActive()
            val handle = nextHandle.incrementAndGet()
            handles[handle] = Connection(path, connection, iface, inputEndpoint, outputEndpoint)
            return handle
        } catch (failure: Throwable) {
            connection.close()
            throw failure
        }
    }

    private suspend fun requestPermission(device: UsbDevice): Boolean {
        currentCoroutineContext().ensureActive()
        val action = "${context.packageName}.USB_PERMISSION.${UUID.randomUUID()}"
        val granted = CompletableDeferred<Boolean>()
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(context: Context, intent: Intent) {
                if (intent.action == action) granted.complete(manager.hasPermission(device))
            }
        }
        if (Build.VERSION.SDK_INT >= 33) {
            context.registerReceiver(receiver, IntentFilter(action), Context.RECEIVER_NOT_EXPORTED)
        } else {
            context.registerReceiver(receiver, IntentFilter(action))
        }
        val pending = PendingIntent.getBroadcast(
            context, 0, Intent(action).setPackage(context.packageName), PendingIntent.FLAG_IMMUTABLE,
        )
        try {
            manager.requestPermission(device, pending)
            return withTimeoutOrNull(60_000) { granted.await() } ?: run {
                if (!manager.deviceList.containsKey(device.deviceName)) {
                    throw UsbhostException(UsbhostError.DISCONNECTED)
                }
                throw UsbhostException(UsbhostError.PERMISSION, "Timed out waiting for USB permission")
            }
        } finally {
            pending.cancel()
            context.unregisterReceiver(receiver)
        }
    }

    private fun connection(handle: Long): Connection =
        handles[handle] ?: throw UsbhostException(UsbhostError.DISCONNECTED)

    override fun read(handle: Long, size: Long, timeoutMS: Long): ByteArray {
        val connection = connection(handle)
        val buffer = ByteArray(size.toInt())
        val count = connection.usb.bulkTransfer(connection.input, buffer, buffer.size, timeoutMS.toInt())
        if (count < 0 && !manager.deviceList.containsKey(connection.path)) {
            throw UsbhostException(UsbhostError.DISCONNECTED, "USB device ${connection.path} is no longer attached")
        }
        return buffer.copyOf(count.coerceAtLeast(0))
    }

    override fun write(handle: Long, data: ByteArray, timeoutMS: Long): Long {
        val connection = connection(handle)
        val endpoint = requireNotNull(connection.output)
        val count = connection.usb.bulkTransfer(endpoint, data, data.size, timeoutMS.toInt())
        if (count < 0 && !manager.deviceList.containsKey(connection.path)) {
            throw UsbhostException(UsbhostError.DISCONNECTED, "USB device ${connection.path} is no longer attached")
        }
        return count.toLong()
    }

    override fun close(handle: Long) {
        val connection = handles.remove(handle) ?: return
        connection.usb.releaseInterface(connection.iface)
        connection.usb.close()
    }
}
