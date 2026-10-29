# Security Policy

`bitfinex-go` is an **unofficial** Bitfinex API v2 client — not affiliated with, endorsed
by, or sponsored by Bitfinex.

## Reporting a vulnerability

Please report security issues **privately** to the maintainer:

- Igor Sazonov — <sovletig@gmail.com> (GitHub: [@tigusigalpa](https://github.com/tigusigalpa))

Do **not** open a public issue for suspected vulnerabilities. Include affected versions,
reproduction details, and whether any credentials or funds could be impacted.
You will get an acknowledgement and a coordinated disclosure timeline.

## Supported versions

| Version | Supported |
| ------- | --------- |
| latest 1.x | ✅ |
| older minors | last minor only |

## Security design notes (what this library guarantees)

- Credentials are redacted from all stringers/serializers and never logged, embedded in
  errors, or written to fixtures. Request signing bytes are produced once and never
  re-serialized after signing.
- HTTPS/WSS endpoints by default; insecure schemes only via explicit test/proxy options.
- The default nonce provider is process-local; multi-process deployments MUST inject a
  distributed `NonceProvider` or use separate API keys — a repeated/decreasing nonce
  rejects requests but a leaked key does not forgive them.
- Money never traverses `float64`; request bodies carrying secrets (transfer/withdrawal/
  deposit) are never included in errors or logs.
- Dead-man switch (`dms: 4`) and WS `filter` are strictly opt-in.
- Dependencies are minimal (`gorilla/websocket` only); CI runs govulncheck, CodeQL,
  dependency review and a secret scan.

## Recommendations for users

- Prefer **read-only** API keys where possible; separate keys per process.
- Keep secrets in a secret manager/env, never in source or tests.
- Pin the module version and review the CHANGELOG before upgrading.
