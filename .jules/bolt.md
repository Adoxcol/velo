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

## 2026-04-07 - Pre-parse origin page host in batch link processing
**Learning:** Calling `sameHost(u, b.Page)` inside the batch insertion loop in `addBatch` repeatedly parsed `b.Page` with `url.Parse` for every batch URL. Pre-parsing the host string `pageHost := hostOf(b.Page)` once outside the loop reduces `addBatch` execution time by ~43% (1.76 ms vs 3.13 ms per 500 items) and eliminates N redundant `url.Parse` heap allocations per batch.
**Action:** Pre-parse invariant URL hostnames outside URL iteration loops when comparing domain/host equality across batch collections.

## 2026-04-08 - Fast-path percent escape check before url.PathUnescape
**Learning:** Calling `url.PathUnescape` on every extracted URL path base during row rendering and filename extraction executes unescaping logic and heap string formatting even when no percent escapes (`%XX`) are present in typical filenames. Fast-pathing with `strings.IndexByte(b, '%') != -1` skips unescape processing for standard filenames, improving `urlPathName` execution speed by ~23% (373.9 ns vs 486.2 ns per batch of 5 URLs).
**Action:** Always check for `%` before calling `url.PathUnescape` or `url.QueryUnescape` on high-frequency URL path and query string processing paths.

## 2026-04-09 - Fast manual string scanning for HLS tag attributes
**Learning:** Parsing key-value pairs in HLS stream tag lines (e.g. `#EXT-X-STREAM-INF:BANDWIDTH=...`, `#EXT-X-KEY:...`) using `regexp.FindAllStringSubmatch` creates significant regex execution overhead and multiple heap slice allocations per tag. Replacing regexp matching with linear string scanning (`strings.IndexByte` / `strings.LastIndexAny`) speeds up `attrs()` execution time by ~8.1x (547.9 ns vs 4,439 ns) and cuts memory allocations by 52% (336 B vs 699 B per call).
**Action:** Replace regular expressions with linear string slicing when parsing structured key=value tag lines in stream manifests or protocol headers.

## 2026-10-30 - Replace regex in ISO 8601 duration parsing with direct linear string scanning
**Learning:** Using `regexp.FindStringSubmatch` to parse ISO 8601 duration strings (`PT1H2M3.5S`, `P1DT1S`) in DASH manifest loading creates regex evaluation overhead and heap slice allocations for submatches. Replacing regexp matching with a single-pass linear string scanner reduces execution time from ~807 ns/op to ~68.8 ns/op (~11.7x faster) and completely eliminates heap allocations (0 B/op).
**Action:** Replace regular expression submatch extractions with linear pointer string scanning when parsing standard structured strings (like ISO 8601 durations).

## 2026-11-06 - Zero-allocation slicing and linear hex scanning in ParseChecksum
**Learning:** Calling `strings.Fields(s)`, `hex.DecodeString(s)`, and unconditional `strings.ToLower(s)` to parse and validate checksum strings causes multiple heap allocations and buffer copies per checksum check. Replacing `strings.Fields` with index-based whitespace slicing (`strings.IndexAny(s, " \t\r\n")`), validating hex characters with a single-pass ASCII scan, and conditionally lowercasing only when uppercase runes are found cuts latency from ~2,035 ns/op to ~832 ns/op (~59% faster) and drops allocations from 9 allocs/op (272 B) to 1 alloc/op (64 B).
**Action:** Use index slicing instead of `strings.Fields` when extracting single leading tokens, and prefer single-pass ASCII validation over `hex.DecodeString` when only verifying string hex validity.

## 2026-11-09 - Reuse parsed URL and lazily split path in Downloadable and VideoPage
**Learning:** In `Downloadable(rawURL)`, calling `url.Parse(rawURL)` followed immediately by `VideoPage(rawURL)` re-parsed the exact same URL string. In addition, `VideoPage` unconditionally called `strings.Split` on URL paths before checking if the domain belonged to a video service, creating heap slice allocations for every non-video URL. Passing the pre-parsed `*url.URL` directly into `videoPageURL(u)` and evaluating domain matches before path splitting cuts `Downloadable` evaluation time by ~39% (7,585 ns/op vs 12,371 ns/op per batch of 10 URLs) and reduces heap allocations by over 45% (16 allocs/op / 1,952 B vs 33 allocs/op / 3,584 B).
**Action:** Pass pre-parsed `*url.URL` pointers to subordinate URL inspection helpers rather than raw strings, and check target hostnames before performing path slicing or splitting operations.

## 2026-11-10 - Batch lock acquisitions and preallocate slice in history snapshot
**Learning:** In `a.snapshot()`, calling `d.State()` and `d.Info()` separately for every download item in `a.items` acquired and released the `Download.mu` mutex twice per item (1,000 lock/unlock operations for 500 items). Adding `d.StateInfo()` to retrieve status, error, path, and total size in a single critical section halves mutex acquisitions, and pre-allocating `out := make([]histEntry, 0, capacity)` eliminates dynamic slice growth reallocations, cutting snapshot allocation overhead and reducing lock contention across background dispatch cycles.
**Action:** Combine multi-field getters into a single batched accessor when reading multiple protected fields from synchronised objects in iteration loops, and pre-allocate destination slice capacities when item counts are known.
