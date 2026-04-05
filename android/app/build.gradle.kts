plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.iskra.app"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.iskra.app"
        minSdk = 24
        targetSdk = 34
        versionCode = 23
        versionName = "2.0-alpha"
    }

    signingConfigs {
        create("release") {
            val ksFile = file("iskra-release.jks")
            if (ksFile.exists()) {
                storeFile = ksFile
                storePassword = System.getenv("KEYSTORE_PASSWORD") ?: "iskra2026"
                keyAlias = System.getenv("KEY_ALIAS") ?: "iskra"
                keyPassword = System.getenv("KEY_PASSWORD") ?: "iskra2026"
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }

    kotlinOptions {
        jvmTarget = "1.8"
    }
}

dependencies {
    implementation("androidx.appcompat:appcompat:1.6.1")
    implementation("androidx.webkit:webkit:1.8.0")
    // iskra.aar — Go core via gomobile
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))
}
