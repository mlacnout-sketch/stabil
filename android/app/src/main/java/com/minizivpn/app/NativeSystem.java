package com.minizivpn.app;

public class NativeSystem {
    static {
        System.loadLibrary("system");
    }

    public static native void jniclose(int fd);
    public static native int sendfd(int tun_fd);
    public static native void exec(String cmd);
    public static native String getABI();
}
