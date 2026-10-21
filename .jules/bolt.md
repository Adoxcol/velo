## 2026-03-31 - Fast path filename sanitization
**Learning:** Using regular expressions like `unsafeChars` and `reservedNames` for every downloaded file name in `safeName` causes allocations and overhead for standard clean file names. Adding a fast ASCII check (`isFastSafe`) bypasses regex matching and allocations for typical filenames while maintaining strict safety checks.
**Action:** Always consider fast-pathing ASCII string validation before using regular expressions on high-frequency string sanitization paths.

## 2026-04-01 - Pre-parse DASH URL templates before loop expansion
**Learning:** Running `regexp.ReplaceAllStringFunc` and `regexp.FindStringSubmatch` on every segment URL during DASH manifest expansion executes regular expressions thousands of times per stream download. Pre-parsing templates into token chunks (`dashTemplate`) once outside the expansion loop reduces segment URL generation time to simple `strings.Builder` formatting (~188 ns/op, 64 B/op).
**Action:** Pre-compile or pre-parse URL templates outside high-frequency segment expansion loops when handling manifest-based video downloads (DASH/HLS).
