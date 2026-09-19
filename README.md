# qoder-cpa

A CPA (CLIProxyAPI) native plugin for Qoder account management and chat
execution.

## Overview

This plugin registers with CPA as a schema-6 provider named `qoder`. It
provides:

- **Auth Provider:** Device OAuth flow for Qoder account login
- **Model Provider:** Dynamic model discovery from authorized Qoder accounts
- **Executor:** Chat completion forwarding to Qoder endpoints
- **Management API:** Embedded WebUI for account and model management

## Build

Requires CGO and a C compiler (gcc/clang):

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o qoder.so .
```

## Test

```bash
go test ./... -shuffle=on -count=1
```

## License

MIT — see [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
