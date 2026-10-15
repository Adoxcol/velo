# Code signing and automated releases

Releases are built by [.github/workflows/release.yml](../.github/workflows/release.yml) when a version
tag is pushed. Code signing through [SignPath Foundation](https://signpath.org) (free for open-source
projects) switches on by itself once the secrets below exist; until then releases are built unsigned.

## Making a release

1. Add a `## <version> — <date>` section to `CHANGELOG.md` (the release notes are taken from it),
   then run `go run ./tools/changelog` so the website's changelog page matches.
2. Commit, then tag and push:

   ```
   git tag v0.4.1
   git push origin main v0.4.1
   ```

3. GitHub Actions tests, builds, signs (when set up) and publishes the release with the installer,
   portable zip, extension packages and checksums. Installed copies of Velo then update themselves.

To try a build without publishing: Actions → Release → Run workflow, and enter a version. The files
are kept as a workflow artifact.

## How signing works

The installer embeds the programs, so signing happens in two stages:

1. `velo.exe`, `velo-host.exe` and `veloctl.exe` are built and sent to SignPath
   (artifact configuration `programs`).
2. The installer is built around the signed programs, then sent to SignPath itself
   (artifact configuration `installer`). The portable zip contains the signed programs too.

Each stage checks the returned files carry a valid signature before the release is published.

## Setting up SignPath

1. **Apply** to SignPath Foundation's open-source program at https://signpath.org (Velo qualifies:
   MIT license, public repository, releases built on GitHub Actions). They review the project.
2. Once accepted, in SignPath:
   - Link the predefined **GitHub.com trusted build system** to the project, and install the
     **SignPath GitHub App** on the `Adoxcol/velo` repository.
   - Create (or check) the project with the slug **`velo`**.
   - Add two **artifact configurations**, pasting the files from this repository:
     - slug **`programs`**: [.signpath/artifact-configurations/programs.xml](../.signpath/artifact-configurations/programs.xml)
     - slug **`installer`**: [.signpath/artifact-configurations/installer.xml](../.signpath/artifact-configurations/installer.xml)
   - Use the signing policy SignPath gives you for releases. The workflow expects the slug
     **`release-signing`**; if yours differs, set the repository variable below.
   - Create an **API token** for a CI user that may submit to that policy.
3. In GitHub (repository → Settings → Secrets and variables → Actions):
   - Secret **`SIGNPATH_API_TOKEN`**: the API token.
   - Variable **`SIGNPATH_ORGANIZATION_ID`**: your SignPath organization ID.
   - Optional variables **`SIGNPATH_PROJECT_SLUG`** and **`SIGNPATH_POLICY_SLUG`** if you didn't use
     `velo` and `release-signing`.

The next tag you push is signed. SignPath Foundation may require someone to approve each signing
request in their web app; the workflow waits up to an hour for that.

## Notes

- Even signed, a brand-new release can still show SmartScreen's warning until it builds download
  reputation; signing makes that happen much faster.
- With SignPath Foundation the certificate is issued to "SignPath Foundation", and Velo's listing
  on signpath.org says which project it signs for.
- `tools/release.ps1 -Version x.y.z` still builds a full release locally (unsigned).
