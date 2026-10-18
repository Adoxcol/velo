## 2026-03-31 - Fast path filename sanitization
**Learning:** Using regular expressions like `unsafeChars` and `reservedNames` for every downloaded file name in `safeName` causes allocations and overhead for standard clean file names. Adding a fast ASCII check (`isFastSafe`) bypasses regex matching and allocations for typical filenames while maintaining strict safety checks.
**Action:** Always consider fast-pathing ASCII string validation before using regular expressions on high-frequency string sanitization paths.
