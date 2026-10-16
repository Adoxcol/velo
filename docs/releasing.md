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
