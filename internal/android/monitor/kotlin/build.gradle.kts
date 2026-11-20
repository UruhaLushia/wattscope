// The Kotlin of the native module monitor: an Android library, which mygo
// build adds to the app's Gradle build. The build sets its SDK levels, Java
// and Kotlin, and its dependency on MyGo's host library (dev.mygo), which
// brings coroutines: add only what the implementation uses.
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.github.uruhalushia.wattscope.internal.android.monitor"
    defaultConfig {
        consumerProguardFiles("consumer-rules.pro")
    }
}

dependencies {
    implementation("com.xzakota.hyper.notification:focus-api:1.3")
    implementation("dev.rikka.shizuku:api:13.1.5")
}
