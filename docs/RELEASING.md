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
- puts the web version on GitHub Pages, at
  <https://halpworld.github.io/halpwords/>.

To check the builds without publishing anything, run the workflow by hand
from the Actions tab (**Run workflow**). Its archives are in the run's
artifacts.

The version comes from `git describe`, so a build from a tag says `v1.2.3`
on the title screen, and anything else says where it was built from.

## One-time setup

### GitHub Pages

In the repository's **Settings → Pages**, set **Source** to **GitHub
Actions**. The web version is then at
<https://halpworld.github.io/halpwords/>. Don't set a custom domain here, or
on the organisation's own `halpworld.github.io` site: that would redirect
this address before players' saves have moved (see below).

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

## The web game's address

Where players are sent to play in a browser is `move.WebURL` in
[internal/move/move.go](../internal/move/move.go). The README and this file
link to it, and `make test` fails if they don't agree. Today it is the
GitHub Pages address, <https://halpworld.github.io/halpwords/>. It is moving
to `play.halpwords.com` (halpwords-server W1.16).

A browser keeps local storage per address, so the saves at the old address
(the hero, word memory, own word lists, Hall of Fame and settings) can't be
seen at the new one. So the old address will serve a small *We've moved*
page, [web/moved](../web/moved) (`make build-moved` builds it into
`dist/moved`). It reads the saves and opens the new address with them in
the URL fragment (`#import=…`, compressed), which browsers never send to a
server. The game there takes them in once, asks before replacing progress
that browser already has, and clears the fragment. A save too big for one
address (over 900,000 characters once compressed, which is rare, as a
browser only holds about 5 MB) goes in parts: the game fetches each part
from the old page in turn. The AI helper's keys (`ai.json`) stay behind,
since an address can end up in the browser's history; players set them up
again. The format is described in `internal/move`.

The proposed release workflow for after the move is
[docs/cutover/release.yml](cutover/release.yml). It pushes the web game to
the `halpworld/play` repository, whose Pages site has the custom domain
`play.halpwords.com`, and puts the *We've moved* page on this repository's
Pages site, only once the new address has the game.

### Cut-over checklist

For a person to do, in this order. The old address keeps serving the game
until step 6.

1. **DNS** (halpwords-server W0.10): a `CNAME` record from
   `play.halpwords.com` to `halpworld.github.io`. Check the `CAA` records
   allow Let's Encrypt, which GitHub Pages uses.
2. **The new site.** Create the public repository `halpworld/play` with a
   `README` on `main`. In its **Settings → Pages**, set **Source** to
   **Deploy from a branch**, `main`, `/ (root)`; set **Custom domain** to
   `play.halpwords.com`, wait for the certificate, and tick **Enforce
   HTTPS**. Verify the domain for the organisation (**Organisation settings
   → Pages → Add a domain**) so no one else can claim it.
3. **The deploy key.** `ssh-keygen -t ed25519 -f play-deploy -N ""`. Add
   `play-deploy.pub` to `halpworld/play` as a deploy key with write access,
   and the private key `play-deploy` to this repository as the Actions
   secret `PLAY_DEPLOY_KEY`. Delete both files.
4. **A rehearsal**, before the old address changes. Put the web game on
   the new site by hand (`make build-web`, then push `dist/web` with a
   `CNAME` file containing `play.halpwords.com` to `halpworld/play`) and
   check <https://play.halpwords.com/> plays. Then, in a browser that has
   played at <https://halpworld.github.io/halpwords/>, open that address,
   open the browser's developer console and paste in the whole of
   `web/moved/moved.js`: it does what the moved page will do. Check that
   the hero, word memory, an own word list, the Hall of Fame and the
   settings arrive; that the game asks first when the new address already
   has progress; and that the address bar loses its `#import=…`.
5. **The code.** On a branch: set `WebURL = PlayURL` in
   `internal/move/move.go`; change the web game links in `README.md` and at
   the top of this file to <https://play.halpwords.com/> (`make test` says
   which are left); copy `docs/cutover/release.yml` over
   `.github/workflows/release.yml` (this needs someone who may change
   workflows); add a CHANGELOG line. Merge.
6. **Tag a release.** The workflow puts the game on
   `play.halpwords.com` first, then the *We've moved* page on
   `halpworld.github.io/halpwords/`. From then on every old link lands on
   the moved page, which hands over any saves and opens the new address:
   old links redirect only once the saves have gone with them.
7. **Check it live**: in a browser that played at the old address, open
   <https://halpworld.github.io/halpwords/> and check the progress arrives
   at <https://play.halpwords.com/>. Tell players (release notes, the
   website).
8. **Keep the moved page up** for at least a year. Never set a custom
   domain on this repository's Pages site: that turns the old address into
   a plain redirect, and any saves still there are lost to their players.
   halpwords-server's API allows only `https://play.halpwords.com` for
   CORS, so check that is in its configuration too.
