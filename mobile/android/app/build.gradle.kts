plugins {
    id("com.android.application")
}

android {
    namespace = "ru.supportgenesis.genesisit"
    compileSdk = 36

    defaultConfig {
        applicationId = "ru.supportgenesis.genesisit"
        minSdk = 26
        targetSdk = 36
		versionCode = 89
		versionName = "1.0.2"
    }

	val releaseKeystore = System.getenv("GENESIS_ANDROID_KEYSTORE") ?: error("GENESIS_ANDROID_KEYSTORE is required")
	val releaseStorePassword = System.getenv("GENESIS_ANDROID_STORE_PASSWORD") ?: error("GENESIS_ANDROID_STORE_PASSWORD is required")
	val releaseKeyAlias = System.getenv("GENESIS_ANDROID_KEY_ALIAS") ?: error("GENESIS_ANDROID_KEY_ALIAS is required")
	val releaseKeyPassword = System.getenv("GENESIS_ANDROID_KEY_PASSWORD") ?: error("GENESIS_ANDROID_KEY_PASSWORD is required")

	signingConfigs {
		create("release") {
			storeFile = file(releaseKeystore)
			storePassword = releaseStorePassword
			keyAlias = releaseKeyAlias
			keyPassword = releaseKeyPassword
			enableV1Signing = true
			enableV2Signing = true
			enableV3Signing = true
		}
	}

    buildTypes {
        release {
            isMinifyEnabled = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
			signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
