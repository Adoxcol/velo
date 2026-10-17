# Making releases

Releases are built by [.github/workflows/release.yml](../.github/workflows/release.yml) when a version
tag is pushed.

## Making a release

1. Add a `## <version> — <date>` section to `CHANGELOG.md` (the release notes are taken from it),
   then run `go run ./tools/changelog` so the website's changelog page matches.
2. Commit, then tag and push:

   ```
   git tag v0.4.1
   git push origin main v0.4.1
   ```

3. GitHub Actions tests, builds and publishes the release with the installer,
   portable zip, extension packages and checksums. Linux and macOS jobs then test
   the portable packages natively and attach amd64/arm64 CLI archives with checksums.
   Wait for every release job to finish before announcing availability. Installed
   Windows copies can update themselves; portable and CLI users download manually.

To try a build without publishing: Actions → Release → Run workflow, and enter a version. The files
are kept as a workflow artifact.

## winget

After the release is published, the workflow sends the new version to winget as a pull request to
`microsoft/winget-pkgs`. It needs a repository secret **`WINGET_TOKEN`**: a classic GitHub token
(Settings → Developer settings → Tokens (classic)) with the `public_repo` scope. Without it the step
is skipped. The first version, 0.4.2, was submitted by hand from `packaging/winget`.

## Website download links

The release workflow publishes both versioned CLI archives and stable aliases:
`veloctl-linux-amd64.tar.gz`, `veloctl-linux-arm64.tar.gz`,
`veloctl-darwin-amd64.tar.gz` and `veloctl-darwin-arm64.tar.gz`.
Each platform's checksum file includes both names. Website `/download/linux-x64`,
`/download/linux-arm64`, `/download/macos-x64` and `/download/macos-arm64`
redirect to these aliases on the latest GitHub release. Keep these names in sync
with `vercel.json`, the README and `docs/cli.md` when changing packaging.

The public homepage is deployed from `site/` through Vercel when main changes.
Update its feature claims when platform support changes; the browser extension
currently requires the Windows desktop app.
