## 2026-03-30 - Empty Token Bypass with ConstantTimeCompare
**Vulnerability:** `subtle.ConstantTimeCompare([]byte(header), []byte(token))` evaluates to 1 when both inputs are empty slices `[]byte("")`.
**Learning:** In Go, `subtle.ConstantTimeCompare` compares byte slices and returns 1 if their lengths match and contents match. Empty slices match this condition, meaning unauthenticated HTTP requests with missing `X-Velo-Token` headers would pass authorization checks if the application's expected token was empty or unset.
**Prevention:** Always explicitly check that both tokens (request token and expected token) are non-empty before calling `subtle.ConstantTimeCompare`.
