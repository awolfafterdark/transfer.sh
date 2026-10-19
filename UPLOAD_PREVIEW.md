# Upload, preview and direct-download sharing

The default homepage now includes a responsive file picker, drag and drop, upload progress,
and a results card for each successful upload. Every card contains the preview and download
URLs and copy buttons. API bodies remain plain-text raw file URLs, including multipart POSTs.
`X-View-URL` and `X-Direct-URL` headers are additive (repeated in multipart upload order).

## Routes

| Route | Behavior |
| --- | --- |
| `PUT /filename`, `POST /` | Original upload backend and plain-text URL response |
| `/view/token/filename` | Dedicated HTML preview |
| `/get/token/filename` | Complete original file, attachment disposition |
| `/inline/token/filename` | Inline original media; active documents served as plain text |
| `/token/filename` | Original raw-file download, independent of browser Accept headers |

All routes use the same stored file. Previewing checks existing expiry and download-limit
metadata and calls `storage.Head`/`storage.Get`; it does not assume a local storage directory.
Opening a preview page does not consume a download. Loading embedded media through `/inline/`
does consume an existing download allowance, just like other file retrievals.

Text reads stop at 256 KiB plus one byte to detect truncation. JSON is formatted when the
whole document fits; incomplete JSON is source. CSV previews stop at 200 rows. Python and
JavaScript use a small local lexical highlighter. HTML, SVG and XML are escaped source.
Raster images, audio, video and PDF use native browser viewers; PDF support varies by browser.
ZIP and unknown binary files show metadata and a download button. All download buttons target
the original `/get/` route, including when previews are truncated.

New uploads record a UTC timestamp in the existing metadata record. Existing records without
one omit the upload time. Expiration is displayed only when `MaxDate` is supplied. Setting
`TRANSFER_TEMPORARY_HOSTING=true` adds the runner-termination notice.

Assets are embedded for ordinary `go build` usage. The hosting workflow supplies
`--web-path server/web` to use the checked-in frontend without a separate web server.
Custom web paths must contain `index.html`, `styles/share.css`, and `scripts/share.js`.

## GitHub Actions hosting

The fork did not contain a hosting workflow when these changes were prepared. The added
`.github/workflows/host-transfer.yml` checks out this fork, clones official transfer.sh at
`f040f28df9fd139e7717af7eaa4c055870ab8339`, overlays the modified server files and frontend,
builds, starts transfer.sh and ngrok, and runs local and public API/browser integration tests.
The overlay fails if the official checkout differs, so dependency upgrades require a review
of the pinned version and the modified Go files together.

Configure the repository secret `NGROK_AUTHTOKEN` with the token for the reserved domain.
The repository variable `NGROK_DOMAIN` can override the default
`nondurable-tabetha-cellular.ngrok-free.dev`. No token is stored in source.

The exact requested cron `0 */5 * * *`, `RUNTIME_MINUTES: "330"`, `--provider local`, and
`--basedir "$RUNNER_TEMP/transfer-data"` are retained. The cron runs at 00:00, 05:00, 10:00,
15:00 and 20:00 UTC (the overnight interval is four hours). Runs may be delayed by GitHub.
A subsequent run cancels the preceding host to release the same ngrok domain, so a scheduled
replacement can interrupt the 330-minute hold. Storage disappears with that runner. This
setup does not provide an uninterrupted handoff or persistent files.

No ProxyForge repository or its five-minute upload schedule is modified. Its uploader remains
compatible. Its README publishing code must generate both links itself, for example:

```python
from urllib.parse import urlsplit, urlunsplit
raw = upload_response.strip()
u = urlsplit(raw)
if u.scheme not in ("https", "http") or not u.netloc or len(u.path.split("/")) != 3:
    raise ValueError("Unexpected upload URL")
view_url = urlunsplit((u.scheme, u.netloc, "/view" + u.path, "", ""))
direct_url = urlunsplit((u.scheme, u.netloc, "/get" + u.path, "", ""))
```

## Verification

`.github/workflows/share-integration.yml` builds the actual server and runs Go tests,
stdlib Python black-box API tests, and Playwright desktop/mobile browser tests without
requiring ngrok credentials. The hosting workflow repeats the API and browser tests through
the public tunnel before announcing readiness.

Local equivalents (Go toolchain and Chromium required):

```sh
gofmt -w server/share.go server/share_test.go server/server.go server/handlers.go
go vet ./...
go test ./...
go build -o /tmp/transfer .
mkdir -p /tmp/transfer-data /tmp/transfer-tmp
/tmp/transfer --provider local --basedir /tmp/transfer-data --temp-path /tmp/transfer-tmp --listener 127.0.0.1:8080 --web-path "$PWD/server/web"
# In another shell:
python3 tests/api_test.py
npm install --prefix /tmp/transfer-browser playwright@1.56.1
/tmp/transfer-browser/node_modules/.bin/playwright install --with-deps chromium
NODE_PATH=/tmp/transfer-browser/node_modules node tests/browser.cjs
```
