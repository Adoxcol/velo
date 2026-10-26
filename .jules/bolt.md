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

## 2026-04-04 - Pre-parse base URL before segment URL resolution in HLS/DASH manifest expansion
**Learning:** Calling `url.Parse(base)` inside loops over hundreds or thousands of HLS or DASH media segments repeatedly parses the exact same base URL string, causing thousands of redundant allocations per stream download. Pre-parsing the base URL into a `*url.URL` object (`baseURL`) once outside segment expansion loops and passing it to `resolveURL(baseURL, ref)` reduces segment parsing time by ~21.6% (2.06 ms vs 2.63 ms per 1,000 segments) and saves 1,000 allocations (~144 KB) per playlist parse.
**Action:** Always pre-parse static base URLs into `*url.URL` structs outside high-frequency relative URL resolution loops when parsing stream manifests (HLS/DASH).

## 2026-04-05 - Avoid fixed large scanner buffer allocations and fast-path comment lines in HLS playlist parsing
**Learning:** Initializing `bufio.Scanner` with a fixed large buffer (`make([]byte, 1<<20)`) allocates 1 MB of heap upfront on every HLS playlist parse regardless of playlist size. Initializing `sc.Buffer` with a small initial capacity buffer (`make([]byte, 0, 4096)`) and a large max limit (`1<<20`), combined with filtering `#` comment lines on raw byte slices before converting to strings, reduces parse execution time by ~45% (1.10 ms vs 2.01 ms per 1,000 segments) and cuts heap allocations by ~71% (0.43 MB vs 1.49 MB).
**Action:** Use small initial buffer capacities with large max limits when configuring `bufio.Scanner` for files with variable line lengths, and filter comments on raw byte slices before converting lines to heap strings.

## 2026-04-06 - Fast path lowercasing in CategoryOf for file extension lookup
**Learning:** Calling `strings.ToLower` on every file extension in `CategoryOf` allocated heap memory even when extensions were already lowercase (which is the case for >95% of files in download queues and UI row rendering loops). Checking `unicode.IsUpper` before calling `strings.ToLower` on `ext[1:]` avoids heap allocations and string formatting overhead while preserving full Unicode lowercasing support.
**Action:** When performing map lookups by lowercased key strings, check for uppercase runes first to bypass `strings.ToLower` heap allocations for already lowercase keys.
