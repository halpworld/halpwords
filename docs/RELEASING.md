# Releasing Halpwords

Releases are built by GitHub Actions from a version tag. Everything below
the first section is set up once.

## Cutting a release

1. Make sure `main` is green in CI, and try a build: `make check`, then
   `make serve-web` or `make bundle-mac` and play a floor.
2. Add a section for the version to [CHANGELOG.md](../CHANGELOG.md), headed
   `## v1.2.3`. It becomes the release notes.
3. Tag and push:

   ```sh
   git tag -a v1.2.3 -m "Halpwords v1.2.3"
   git push origin v1.2.3
   ```

The [Release workflow](../.github/workflows/release.yml) then:

- runs the tests;
- builds the Linux, Windows (x64 and Arm) and web versions on Linux, and the
  universal macOS app on macOS;
- publishes a GitHub release with the archives and `SHA256SUMS.txt`;
- puts the web version on GitHub Pages.

To check the builds without publishing anything, run the workflow by hand
from the Actions tab (**Run workflow**). Its archives are in the run's
artifacts.

The version comes from `git describe`, so a build from a tag says `v1.2.3`
on the title screen, and anything else says where it was built from.

## One-time setup

### GitHub Pages

In the repository's **Settings → Pages**, set **Source** to **GitHub
Actions**. The web version is then at `https://<owner>.github.io/halpwords/`.

### Signing and notarising the macOS app

Without this, the app is signed just for the Mac that runs it, and macOS
asks players to right-click and choose **Open** the first time. With it,
the app opens like any other.

You need an [Apple Developer](https://developer.apple.com/programs/)
account. Add these repository secrets (**Settings → Secrets and variables →
Actions**):

| Secret | What |
|---|---|
| `MACOS_CERT_P12` | Your **Developer ID Application** certificate and key, exported from Keychain Access as a `.p12`, then base64-encoded: `base64 -i cert.p12 \| pbcopy` |
| `MACOS_CERT_PASSWORD` | The password you gave the `.p12` |
| `MACOS_SIGN_IDENTITY` | The certificate's name, such as `Developer ID Application: Jane Doe (AB12CD34EF)` |
| `APPLE_ID` | The Apple ID of the developer account |
| `APPLE_TEAM_ID` | The team ID, such as `AB12CD34EF` |
| `APPLE_APP_PASSWORD` | An [app-specific password](https://support.apple.com/102654) for that Apple ID |

With the first three the app is signed; with all six it is also notarised
and stapled.

To sign locally, pass the identity to make:

```sh
make bundle-mac MACOS_SIGN_IDENTITY="Developer ID Application: Jane Doe (AB12CD34EF)"
```

### Windows

The Windows builds carry the icon and version, added by
[go-winres](https://github.com/tc-hib/go-winres) (fetched by `go run` the
first time). They are not code-signed, so SmartScreen may warn about them
until they are well known; signing needs a code-signing certificate and is
not set up.
