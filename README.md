# qoder-cpa

A CPA (CLIProxyAPI) native plugin for Qoder account management and chat
execution.

## Overview

This plugin registers with CPA as a schema-6 provider named `qoder`. It
provides:

- **Auth Provider:** Device OAuth flow for Qoder account login
- **Model Provider:** Dynamic model discovery from authorized Qoder accounts
- **Executor:** Chat completion forwarding to Qoder endpoints
- **Management API:** Embedded WebUI for account and transport-profile management

## Management

After loading the plugin in CPA, open the registered Qoder resource from the
CPA management page. The page uses CPA's native Qoder OAuth endpoint for login
and the plugin routes below for redacted account state and profile changes:

- `GET /v0/management/qoder/accounts`
- `POST /v0/management/qoder/accounts/profile`

The plugin never exposes access or refresh tokens in the account response. A
profile update reads the selected Qoder credential through CPA's host callback,
changes only `transport_profile`, and saves it back through CPA.

## Build

Requires CGO and a C compiler (gcc/clang):

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o qoder.so .
```

## Test

```bash
go test ./... -shuffle=on -count=1
```

Release builds are produced by GitHub Actions from version tags. Do not build
the release artifact on the VPS.

## Qualification

Use `tools/qoder-qualify.py` only with an explicitly authorized test account.
It performs three probes (models, chat completions, and Responses), prints
redacted short samples, and does not save credentials or response bodies.

## License

MIT — see [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
