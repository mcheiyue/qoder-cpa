# qoder-cpa

CPA plugin for Qoder: device OAuth, dynamic models, chat execution.

## Commands

- `go test ./... -shuffle=on -count=1` — run tests
- `CGO_ENABLED=1 go build -buildmode=c-shared -o qoder.so .` — build plugin

## Architecture

Root `package main` contains:
- `main.go` — method dispatch, host callback placeholder
- `cabi.go` — CGO FFI layer (build-tagged `cgo`)
- `registration.go` — schema-6 registration with qoder capabilities
- `envelope.go` — ok/error envelope helpers
- `*_test.go` — registration, handler, and ABI tests

## Constraints

- Schema 6, ABI 1, plugin ID `qoder`
- Capabilities: auth_provider, model_provider, executor, management_api
- Executor scope: oauth
- Input/output formats: chat-completions
- No model_router, no request_interceptor
- No business logic in Todo 1 (skeleton only)
