## 2026-03-30 - Empty Token Bypass with ConstantTimeCompare
**Vulnerability:** `subtle.ConstantTimeCompare([]byte(header), []byte(token))` evaluates to 1 when both inputs are empty slices `[]byte("")`.
**Learning:** In Go, `subtle.ConstantTimeCompare` compares byte slices and returns 1 if their lengths match and contents match. Empty slices match this condition, meaning unauthenticated HTTP requests with missing `X-Velo-Token` headers would pass authorization checks if the application's expected token was empty or unset.
**Prevention:** Always explicitly check that both tokens (request token and expected token) are non-empty before calling `subtle.ConstantTimeCompare`.

## 2026-10-26 - Cross-Host Credential Leakage on Link Refresh
**Vulnerability:** When a download link is refreshed to a new URL on a different host, failing to clear `User` and `Pass` alongside `Cookies` leaks HTTP Basic Auth credentials to the new host in the `Authorization` header.
**Learning:** `refreshLink` updated `req.URL` and cleared `req.Cookies` on host change, but retained `req.User` and `req.Pass`. When enqueued, the HTTP engine sent Basic Auth credentials to the untrusted new host.
**Prevention:** Always clear all sensitive credentials (`Cookies`, `User`, `Pass`) whenever a request is redirected or updated across host boundaries (`hostOf(req.URL) != hostOf(newURL)`).

## 2026-11-12 - Cross-Host Credential Leakage on Origin Restart
**Vulnerability:** When a download request fails with 401/403 and restarts from `Origin` on a different host, retaining `User` and `Pass` credentials leaks HTTP Basic Auth credentials across host boundaries in the `Authorization` header.
**Learning:** `openFirst` updated `r.URL` and `r.Cookies` to `r.Origin` and `r.OriginCookies`, but retained `r.User` and `r.Pass`. If `r.Origin` points to a third-party host or different domain, basic auth credentials intended for `r.URL` were sent to the origin server.
**Prevention:** Clear `r.User` and `r.Pass` whenever restarting a request from `Origin` across host boundaries (`hostOf(r.URL) != hostOf(r.Origin)`).
