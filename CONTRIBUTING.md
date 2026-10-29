# Contributing

Thanks for considering a contribution to `bitfinex-go` — an **unofficial** Bitfinex API v2
client (not affiliated with, endorsed by, or sponsored by Bitfinex).

## Ground rules

1. **Wire contract = official docs.** The only source of truth for endpoints, positional
   array indices, headers and codes is <https://docs.bitfinex.com>. When you add or fix an
   endpoint, update `docs/endpoint-coverage.md` (status, audit date) in the same PR.
2. **No secrets, ever.** No credentials, signatures, nonces or `bfx-*` headers in code,
   tests, fixtures, logs, issues or CI. Fixtures must be sanitized/deterministic.
3. **Money is never float64.** Use `types.Decimal` everywhere; decode via
   `internal/wire` (`json.Decoder` + `UseNumber`).
4. **Writes are never retried.** New write endpoints get `Idempotent: false` and typed
   request validation. High-impact writes keep the `Confirm: true` opt-in pattern.
5. **Errors are typed.** Use the `types` error family; match with `errors.Is`/`As`.

## Development

```bash
go build ./...
go test ./...
go test -race ./...
gofmt -l .          # must be empty
go vet ./...
```

- Keep public API documented; no stubs masquerading as endpoint support — use `deferred`
  in the coverage doc instead.
- Add deterministic unit tests (`httptest`/fakes) for every endpoint group: one success
  fixture + one error fixture minimum. Fuzz new decoders.
- Positional decoders: validate documented MINIMUM length, tolerate unknown tail fields.

## Commits / PRs

- Conventional, concise messages focused on *why*.
- CI runs: build, tests (+race), fuzz smoke, gofmt, vet, lint, vuln check. Keep it green.

## Releasing (maintainers)

- Tag `vX.Y.Z`; never rewrite published tags. Release CI repeats the full gate.
