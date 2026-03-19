# Apple Binary Signing & Notarization

Notes on making macOS release binaries trusted by Gatekeeper so users don't get "unidentified developer" warnings.

## Background

macOS Gatekeeper blocks unsigned binaries downloaded from the internet. For CLI tools distributed as pre-built binaries (like our GitHub release artifacts), this means users must manually bypass the warning. Proper signing and notarization eliminates this friction.

`go install` is not viable for distributing with embedded web assets since it only has Go source files — the npm-built `web/dist/client/` artifacts are not available, so the embed directive picks up an empty directory.

## Requirements

- **Apple Developer Program membership** ($99/year) — individual or organization account
- **Developer ID Application certificate** — used for code signing
- **App-specific password** — for notarization API access
- **macOS CI runner** — `codesign` and `xcrun notarytool` only run on macOS

There is no free tier or open-source exemption for Apple code signing.

## GitHub Actions Implementation

### Runner Change

The current release workflow uses `ubuntu-latest` for all targets. Darwin builds must move to `macos-latest` since signing tools are macOS-only. Split the matrix:

- Linux targets: `runs-on: ubuntu-latest`
- macOS targets: `runs-on: macos-latest`

### Required Secrets

| Secret | Description |
|--------|-------------|
| `APPLE_CERTIFICATE_P12` | Base64-encoded `.p12` export of the Developer ID Application certificate |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the `.p12` file |
| `APPLE_DEVELOPER_ID` | Signing identity string (e.g., `Developer ID Application: Name (TEAMID)`) |
| `APPLE_ID` | Apple ID email used for notarization |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_APP_PASSWORD` | App-specific password for the Apple ID |

### Workflow Steps (Darwin builds only)

```yaml
- name: Import signing certificate
  if: startsWith(matrix.goos, 'darwin')
  env:
    CERTIFICATE_P12: ${{ secrets.APPLE_CERTIFICATE_P12 }}
    CERTIFICATE_PASSWORD: ${{ secrets.APPLE_CERTIFICATE_PASSWORD }}
  run: |
    echo "$CERTIFICATE_P12" | base64 -d > cert.p12
    security create-keychain -p "" build.keychain
    security import cert.p12 -k build.keychain -P "$CERTIFICATE_PASSWORD" -T /usr/bin/codesign
    security set-key-partition-list -S apple-tool:,apple: -k "" build.keychain
    security list-keychains -s build.keychain

- name: Sign binary
  if: startsWith(matrix.goos, 'darwin')
  run: |
    codesign --force --options runtime --sign "${{ secrets.APPLE_DEVELOPER_ID }}" dist/scion

- name: Notarize binary
  if: startsWith(matrix.goos, 'darwin')
  env:
    APPLE_ID: ${{ secrets.APPLE_ID }}
    APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
    APPLE_APP_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}
  run: |
    zip scion.zip dist/scion
    xcrun notarytool submit scion.zip \
      --apple-id "$APPLE_ID" \
      --team-id "$APPLE_TEAM_ID" \
      --password "$APPLE_APP_PASSWORD" \
      --wait
```

The `--options runtime` flag enables the hardened runtime, which is required for notarization.

## Alternatives (No Apple Developer Account)

These don't eliminate Gatekeeper warnings but reduce user friction:

- **Homebrew tap** — users installing via `brew install` bypass Gatekeeper since Homebrew strips the quarantine attribute. Many open-source Go CLI projects use this approach.
- **Manual bypass** — instruct users to run `xattr -d com.apple.quarantine ./scion` after downloading, or right-click and select Open.
- **`go install`** — builds locally so Gatekeeper is not involved, but cannot embed web assets (see Background above).

## Recommendation

For alpha/pre-release: start with a Homebrew tap and document `xattr -d` as a fallback. Add proper signing + notarization when the project matures and the annual fee is justified.
