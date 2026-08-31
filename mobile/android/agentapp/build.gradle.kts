plugins {
    id("com.android.application")
}

import org.gradle.api.tasks.compile.JavaCompile

val releaseKeystore = providers.environmentVariable("GENESIS_ANDROID_KEYSTORE").orNull
val releaseStorePassword = providers.environmentVariable("GENESIS_ANDROID_STORE_PASSWORD").orNull
val releaseKeyAlias = providers.environmentVariable("GENESIS_ANDROID_KEY_ALIAS").orNull
val releaseKeyPassword = providers.environmentVariable("GENESIS_ANDROID_KEY_PASSWORD").orNull
val releaseSigningConfigured = listOf(releaseKeystore, releaseStorePassword, releaseKeyAlias, releaseKeyPassword).all { !it.isNullOrBlank() }
val releaseTaskRequested = gradle.startParameter.taskNames.any { it.contains("release", ignoreCase = true) }
if (releaseTaskRequested && !releaseSigningConfigured) {
    throw GradleException("Release signing requires the RemoteIt Android signing environment")
}

android {
    namespace = "ru.supportgenesis.remoteit.agent"
    compileSdk = 36

    defaultConfig {
        applicationId = "ru.supportgenesis.remoteit.agent"
        minSdk = 26
        targetSdk = 36
        versionCode = 2
        versionName = "1.0.27"
    }

    signingConfigs {
        if (releaseSigningConfigured) {
            create("release") {
                storeFile = file(requireNotNull(releaseKeystore))
                storePassword = requireNotNull(releaseStorePassword)
                keyAlias = requireNotNull(releaseKeyAlias)
                keyPassword = requireNotNull(releaseKeyPassword)
                enableV1Signing = true
                enableV2Signing = true
                enableV3Signing = true
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (releaseSigningConfigured) signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

tasks.withType<JavaCompile>().configureEach {
    options.compilerArgs.add("-Xlint:deprecation")
}
