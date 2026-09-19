# Third-Party Notices

This file lists third-party software packages used by this project, their
licenses, and any usage restrictions.

## CLIProxyAPI SDK

- **Package:** github.com/router-for-me/CLIProxyAPI/v7
- **Version:** v7.3.7
- **License:** MIT
- **Usage:** Plugin ABI types, host callback definitions, schema constants.
- **Source:** https://github.com/router-for-me/CLIProxyAPI

## License Notice

This project's source code is provided under the MIT License (see LICENSE file
in the repository root).

### Code Provenance

This project is an independent clean-room implementation. The following
repositories were used **only as behavioral evidence** to understand the CPA
plugin ABI contract and Qoder protocol semantics. No source code from these
repositories was copied into this project:

- **Orchids-2api** (github.com/orchids-2/orchids-2api) — Note: this
  repository has no root license. Usage is strictly limited to observing
  public behavior (HTTP endpoints, SSE event shapes, header formats). No
  code, algorithms, or implementation patterns were derived from it.

The following MIT-licensed projects were used as **implementation pattern
references** for the CPA plugin ABI:

- **Cursor CPA Plugin** (MIT) — C ABI initialization pattern, registration
  envelope, host callback wiring.
- **KeiRouter** (MIT) — CPA plugin architecture patterns.
- **QoderGateway** (MIT) — Qoder API endpoint patterns and transport
  semantics.

All references are noted for documentation purposes. This project's
implementation is original and does not incorporate copyrighted code from
these sources.
