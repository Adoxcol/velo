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
