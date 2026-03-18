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
        versionCode = 9
        versionName = "0.1.9-alpha"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
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
