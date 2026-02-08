# Gemini Context & Project State

## Project: MiniZivpn Stable (Android Tun2Socks)

### Current Status (As of Feb 8, 2026)
- **Branch `dev-next-step`**: Contains the latest optimizations and fixes. Ready for merge to `main`.
- **Branch `routing-test`**: Contains the "BadVPN Multiplexing Client" implementation (backed up).
- **Branch `main`**: Production release v1.0.11 (Pre-Optimization).

### Key Features Implemented
1.  **UDP Improvements:**
    -   Replaced `SymmetricNAT` with **`Restricted Cone NAT`** (`Address Restricted`) in `tunnel/udp.go`.
    -   Allows P2P/VoIP compatibility by accepting packets from same IP even if source port differs.
    -   Original BadVPN integration removed from `dev-next-step` (clean state), but code exists in `routing-test`.

2.  **Performance Optimizations:**
    -   **Go Allocator:** Zero-allocation buffer pool (`*[]byte` pooling) implemented in `buffer/allocator`.
    -   **Flutter UI:** Replaced `setState` with `ValueNotifier` for real-time stats (Download/Upload speed) to prevent full page rebuilds.
    -   **Android Service:** Moved `pkill` cleanup command to background thread to prevent ANR.
    -   **Build Size:** Added `-ldflags "-s -w"` to Gomobile bind and `ndk.abiFilters 'arm64-v8a'` to Gradle.

3.  **Stability (Anti-Kill):**
    -   Added **CPU Wakelock** support (Toggle in Settings).
    -   Added **Ignore Battery Optimization** request button in Settings.
    -   Ensured Foreground Service is sticky.

4.  **DNS Robustness:**
    -   Removed ISP DNS from TCP fallback list (to avoid timeout).
    -   Using Cloudflare, Google, Quad9, and OpenDNS for reliable DoT/TCP DNS inside tunnel.

5.  **Testing:**
    -   Full unit test coverage for Go modules (`tunnel`, `allocator`, `socks5`, `engine`).
    -   Verified via `go test ./...`.

### Next Steps
- Merge `dev-next-step` -> `update-repo` -> `main`.
- Release Production APK.
- (Future) Re-integrate BadVPN if server-side issues are resolved.

### Operational Guidelines (CRITICAL)
- **Project Status:** 🚧 ONGOING. The journey is long.
- **Decision Making:** Do **NOT** decide to merge, close, or finalize the project autonomously. Await user instruction for "Next Steps".
- **Safety Protocol:**
    - **ASK FIRST:** Always ask for user confirmation before committing changes or merging branches.
    - **READ BEFORE WRITE:** You **MUST** use `read_file` to verify the current state of a file before using `write_file` or `replace`. Blind edits are strictly forbidden.
    - **No "Assuming":** Do not assume file contents or project state. Verify first.

### Technical Notes
- **BadVPN Code:** Located in `native/tun2socks/badvpn` (only in `routing-test` branch).
- **Architecture:** Flutter -> MethodChannel -> Android Service -> JNI -> Go (Tun2Socks) -> Hysteria Core (Separate Process).
- **UDP Handling:** Tun2Socks handles UDP via `handleUDPConn` -> `RestrictedNAT` -> SOCKS5 UDP Associate.
