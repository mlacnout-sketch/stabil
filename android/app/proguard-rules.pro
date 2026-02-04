# Flutter standard rules
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.** { *; }
-keep class io.flutter.util.** { *; }
-keep class io.flutter.view.** { *; }
-keep class io.flutter.** { *; }
-keep class io.flutter.plugins.** { *; }

# Keep our VPN Service and its native methods
-keep class com.minizivpn.app.ZivpnService {
    native <methods>;
    public <methods>;
}

# Keep MainActivity native communication
-keep class com.minizivpn.app.MainActivity {
    public <methods>;
}

# General native method preservation
-keepclasseswithmembernames class * {
    native <methods>;
}

# Google Play Core
-keep class com.google.android.play.core.** { *; }
-dontwarn com.google.android.play.core.**
