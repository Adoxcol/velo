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
   portable zip, extension packages and checksums. Installed copies of Velo then update themselves.

To try a build without publishing: Actions → Release → Run workflow, and enter a version. The files
are kept as a workflow artifact.

## winget

After the release is published, the workflow sends the new version to winget as a pull request to
`microsoft/winget-pkgs`. It needs a repository secret **`WINGET_TOKEN`**: a classic GitHub token
(Settings → Developer settings → Tokens (classic)) with the `public_repo` scope. Without it the step
is skipped. The first version, 0.4.2, was submitted by hand from `packaging/winget`.
