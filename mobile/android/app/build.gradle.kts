plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

val stagingBuild = System.getenv("PINELINK_ANDROID_STAGING") == "1"

android {
    namespace = "top.sqmem.pinelink"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = if (stagingBuild) "top.sqmem.pinelink.staging" else "top.sqmem.pinelink"
        manifestPlaceholders["appLabel"] = if (stagingBuild) "Pinelink Staging" else "Pinelink"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName + if (stagingBuild) "-staging" else ""
    }

    signingConfigs {
        getByName("debug") {
            val debugKey = System.getenv("PINELINK_ANDROID_DEBUG_KEYSTORE")
            if (debugKey != null) {
                storeFile = file(debugKey)
                storePassword = "android"
                keyAlias = "androiddebugkey"
                keyPassword = "android"
            }
        }
        create("privateRelease") {
            val keyPath = System.getenv("PINELINK_ANDROID_KEYSTORE")
            if (keyPath != null) {
                storeFile = file(keyPath)
                storePassword = System.getenv("PINELINK_ANDROID_STORE_PASSWORD")
                keyAlias = System.getenv("PINELINK_ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("PINELINK_ANDROID_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
        }
        release {
            // Private signing material is supplied by the release environment.
            signingConfig = signingConfigs.getByName("privateRelease")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

flutter {
    source = "../.."
}
