## 2026-03-31 - Fast path filename sanitization
**Learning:** Using regular expressions like `unsafeChars` and `reservedNames` for every downloaded file name in `safeName` causes allocations and overhead for standard clean file names. Adding a fast ASCII check (`isFastSafe`) bypasses regex matching and allocations for typical filenames while maintaining strict safety checks.
**Action:** Always consider fast-pathing ASCII string validation before using regular expressions on high-frequency string sanitization paths.

## 2026-04-01 - Pre-parse DASH URL templates before loop expansion
**Learning:** Running `regexp.ReplaceAllStringFunc` and `regexp.FindStringSubmatch` on every segment URL during DASH manifest expansion executes regular expressions thousands of times per stream download. Pre-parsing templates into token chunks (`dashTemplate`) once outside the expansion loop reduces segment URL generation time to simple `strings.Builder` formatting (~188 ns/op, 64 B/op).
**Action:** Pre-compile or pre-parse URL templates outside high-frequency segment expansion loops when handling manifest-based video downloads (DASH/HLS).

## 2026-04-02 - Buffered segment concatenation with buffer reuse
**Learning:** Calling `io.Copy(out, in)` inside a loop over hundreds or thousands of HLS/DASH media segment files allocates a new 32KB buffer on every iteration and performs unbuffered `os.File` write syscalls per chunk. Wrapping `out` in a `bufio.Writer` (256KB) and copying with `io.CopyBuffer` using a single pre-allocated 64KB buffer eliminates per-part buffer allocations and reduces write syscalls, improving concatenation speed by ~43% (7.92 ms vs 13.99 ms per 100 parts).
**Action:** When concatenating multiple files into a single output file, wrap the destination in a `bufio.Writer` and pass a pre-allocated buffer to `io.CopyBuffer`.

## 2026-04-03 - Compare snapshot slice before JSON serialization in history persistence
**Learning:** Calling `json.MarshalIndent(a.snapshot(), "", " ")` on high-frequency background dispatch ticks (every 15s or on event nudges) allocates hundreds of kilobytes and executes expensive string formatting even when no downloads changed. Storing the previous `histEntry` slice (`a.lastSnap`) and checking slice equality (`histEqual`) before marshaling eliminates `json.MarshalIndent` overhead on unchanged state checks, reducing execution time by ~5.9x (0.30 ms vs 1.76 ms for 500 items) and reducing memory allocations by ~47%.
**Action:** When persisting application state periodically or on change triggers, perform field-level slice comparison against previous snapshot structs to bypass JSON serialization entirely when state hasn't changed.
