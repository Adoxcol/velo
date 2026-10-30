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

## 2026-10-27 — Update Info Async RPC & UI Status Display Precedence Optimization

**Problem:** JS calls to `updateInfo()` on the Settings page executed synchronously on the WebView2 window thread, causing window freezes when opening or remaining on Settings during background update checks or downloads. Furthermore, update checking status displayed "Checking…" continuously during installer downloads due to JS status precedence logic, and redundant 1.5s interval timers ran indefinitely.

**Root Cause:**
1. `w.Bind("updateInfo", ...)` was bound synchronously, blocking the main WebView2 window thread when acquiring locks or serving IPC requests from JavaScript.
2. In `cmd/velo/ui.html`, `showUpdate()` evaluated `u.checking ? "Checking…" : (u.status || "Not checked yet.")`, overriding `u.status` (e.g. `"Downloading Velo 1.2.3…"`) with `"Checking…"` whenever `u.checking` was true.
3. Unbounded `setInterval` loops ran every 1.5s in JS (`maybeShowNews`), needlessly waking the event loop when no pending release notes existed.
4. Default HTTP transport lacked explicit dial timeouts for update checks, allowing stalled network connections or unreachable IPs to hang dial attempts.

**Evidence:** Inspection of `cmd/velo/ui.go`, `cmd/velo/ui.html`, and `cmd/velo/updater.go`. Verified with `TestUpdaterDownloadStatus` and `TestUpdaterNetworkTimeout` unit tests.

**Resolution:**
1. Bound `updateInfo` asynchronously using `bindWindowAsync` so IPC calls execute in background goroutines off the UI window thread.
2. Fixed status text precedence in `showUpdate()` to prioritize `u.status` over generic checking indicators, displaying accurate download/check progress.
3. Added `showingUpdate` guard in JS to prevent concurrent overlapping `showUpdate()` calls and removed redundant `setInterval(maybeShowNews, 1500)` loop.
4. Configured 5s dial and 5s TLS handshake timeouts on `defaultUpdateClient` transport in `updater.go`.

**Future Guidance:** Ensure IPC getter functions called by periodic UI refresh loops are bound asynchronously off the window thread, prioritize explicit status strings over boolean flags in UI views, and bound network dial timeouts explicitly.

## 2026-10-28 — Segment Write Lock Decoupling, Async RPCs & Toast Notification Responsiveness

**Problem:** Download engine experienced mutex lock contention across parallel connection workers during segment file writes. In the UI, periodic status queries (`appStatus`, `debugInfo`, `refreshLink`, `pauseAll`, `resumeAll`, `clearDone`) and system notification calls ran synchronously on main threads, risking UI freezes.

**Root Cause:**
1. `copySeg` in `internal/engine/engine.go` held `d.mu.Lock()` during synchronous `f.WriteAt` disk writes, causing high mutex contention across 16+ segment workers.
2. `toast.Show` called `Shell_NotifyIconW` directly on the caller thread, blocking background goroutines if Windows Shell RPCs stalled.
3. Webview bindings `appStatus`, `debugInfo`, `refreshLink`, `pauseAll`, `resumeAll`, and `clearDone` were bound synchronously with `w.Bind`, executing on the main WebView2 window message thread.
4. `saveHistory()` used `json.MarshalIndent`, allocating extra formatting buffers, and `restore()` ran glob searches on non-existent directories.

**Evidence:** Benchmarks, code analysis in `internal/engine/engine.go`, `cmd/velo/notify.go`, `cmd/velo/ui.go`, `cmd/velo/history.go`, and `cmd/velo/ui.html`.

**Resolution:**
1. Decoupled file I/O from `d.mu` in `copySeg` by reserving range offsets under lock and running `f.WriteAt` un-locked with error rollback.
2. Dispatched `Shell_NotifyIconW` balloon creation asynchronously on a background goroutine in `toast.Show`.
3. Converted `appStatus`, `debugInfo`, `refreshLink`, `pauseAll`, `resumeAll`, and `clearDone` to `bindWindowAsync`.
4. Replaced `json.MarshalIndent` with `json.Marshal` in `saveHistory()` and guarded `restore()` directory scanning with `os.Stat`.

**Future Guidance:** Always reserve offset ranges under lock and perform file I/O outside mutex locks; wrap Windows Shell notification RPCs in async goroutines, and bind periodic UI RPC handlers using `bindWindowAsync`.

## 2026-10-29 — Complete WebView Async Binding Migration & Update Metadata Caching

**Problem:** Cancelling or removing downloads, adding links, queueing items, setting speed limits, querying video tool status, or reading clipboard text caused window freezes because their WebView2 IPC handlers executed synchronously on the main UI window message thread. Additionally, rapid manual update checks sent redundant HTTP requests to GitHub Releases API.

**Root Cause:**
1. `remove`, `addDownload`, `add`, `pause`, `resume`, `setQueue`, `reorder`, `setSpeedLimit`, `pasteText`, `videoToolsStatus`, `loadBatchLinks`, and `toggleOpenOnDone` were bound using synchronous `w.Bind`. In particular, `remove()` called `a.cond().Wait()` while waiting for running downloads to cancel, completely stalling the WebView2 UI message loop.
2. `videoToolsStatus()` executed executable file stats and PATH lookups synchronously on the UI thread when polled every 700ms during video tool installation.
3. `updater.check` executed fresh HTTP GET requests to GitHub Releases API on every check invocation without short-term caching.

**Evidence:** Code inspection of `cmd/velo/ui.go` and `cmd/velo/updater.go`; verified with `TestUpdaterCacheTTL` and unit tests in `cmd/velo/updater_test.go`.

**Resolution:**
1. Converted all remaining I/O, lock-heavy, Win32 API, and process-aware WebView2 RPC handlers (`remove`, `addDownload`, `add`, `pause`, `resume`, `setQueue`, `reorder`, `setSpeedLimit`, `pasteText`, `videoToolsStatus`, `loadBatchLinks`, `toggleOpenOnDone`) to `bindWindowAsync` with custom argument unmarshalers (`setQueueArgs`, `reorderArgs`, `loadBatchLinksArgs`).
2. Implemented a 60-second TTL cache (`updateCacheTTL`) for update release metadata in `updater.go` to eliminate redundant GitHub API calls on rapid manual re-checks.

**Future Guidance:** Ensure all webview handlers performing file I/O, sync condition waiting, Win32 system APIs, or environment lookups execute asynchronously off the UI thread via `bindWindowAsync`, and cache external API queries with short TTLs.
