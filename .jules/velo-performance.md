## 2026-10-25 — Update Check Network Latency & Lifecycle Optimization

**Problem:** Update checks took up to 2 minutes when GitHub or network endpoints were slow/unresponsive, and update status displayed "Checking…" continuously during multi-megabyte installer downloads.

**Root Cause:**
1. `update.Latest` context timeout was set to 2 minutes and HTTP transport response header timeout to 60 seconds.
2. `update.Latest` made two sequential HTTP GET requests (release JSON + `.sha256` asset) on every check before checking if the tag was newer.
3. `u.checking` state in `cmd/velo/updater.go` remained `true` during both metadata checking and installer downloading (`update.Download`), forcing UI to display "Checking…" instead of download progress.
4. Downloaded installer files matching SHA256 were not reused upon app restart or repeated checks.

**Evidence:** Code analysis in `internal/update/update.go` and `cmd/velo/updater.go`, unit test verifications in `updater_test.go` and `update_test.go`.

**Resolution:**
1. Reduced `update.Latest` timeout to 15s and added explicit HTTP transport timeouts (15s response header, 10s TLS).
2. Deferred `.sha256` asset fetching in `update.Latest` to `update.Download`, reducing version check requests to 1 single HTTP call.
3. Decoupled `u.checking` state from `update.Download` in `cmd/velo/updater.go`, allowing status to display "Downloading Velo X.Y.Z…" correctly.
4. Added SHA256 verification of existing files in destination directory in `update.Download` to avoid re-downloading existing installers.

**Future Guidance:** Separate metadata check phase from heavy binary asset downloading phase, and ensure network calls on UI pathways use bounded timeouts.

## 2026-10-26 — UI Thread Freezes, HTTP Connection Pooling & Polling DOM Efficiency

**Problem:** Application UI experienced intermittent freezes during update checks, link probing, settings saving, file export, and playlist expansion. Additionally, update checks recreated HTTP transports on every invocation, and 1-second status polling triggered full DOM innerHTML rebuilds.

**Root Cause:**
1. Long-running and I/O-bound functions (`probe`, `exportLinks`, `pendingReleaseNotes`, `installUpdate`, `setSettings`, `deleteFile`, `expandPlaylist`, `installVideoTools`) were bound synchronously using `w.Bind`, blocking the WebView2 UI message thread.
2. `u.check(a)` created new `http.Client` and `http.Transport` instances on every check, preventing TCP/TLS connection reuse and causing DNS/handshake latency.
3. Rapid repeated update requests could trigger duplicate checks.
4. `refresh()` in `ui.html` called `render()` every second unconditionally, discarding and re-creating list card DOM elements even when download state was unchanged.

**Evidence:** Analysis of `cmd/velo/ui.go` webview bindings, `cmd/velo/updater.go` HTTP client lifecycle, `cmd/velo/ui.html` polling loop, and unit test `TestUpdaterDeduplication`.

**Resolution:**
1. Converted I/O-bound and long-running webview bindings to `bindWindowAsync` and enhanced `asyncbind_windows.go` to handle multi-argument asynchronous invocations cleanly.
2. Reused a thread-safe `defaultUpdateClient` with connection pooling (`MaxIdleConns: 10`, `IdleConnTimeout: 90s`).
3. Added `u.busy` lock guard to deduplicate concurrent update check/download operations.
4. Implemented state key comparison in `ui.html` `render()` to skip DOM re-renders during status polling when state is unchanged.

**Future Guidance:** Always execute file I/O, network requests, and external tool invocations asynchronously off the webview thread, reuse HTTP connection pools for background services, and avoid un-memoized DOM innerHTML replacement in polling loops.
