# go-toolbox

Small, dependency-free helpers shared by [sebrandon1](https://github.com/sebrandon1)'s Go
projects. Every package here exists because the same code had been written independently in
at least two repositories.

The core packages use only the Go standard library. Packages that need a dependency say so
and live behind their own import path.

## Packages

| Package | Import | Purpose | External deps |
|---|---|---|---|
| `httpx` | `github.com/sebrandon1/go-toolbox/httpx` | HTTP client construction, request/response helpers, bounded body reads, typed HTTP errors, `Retry-After` parsing | none |
| `retry` | `github.com/sebrandon1/go-toolbox/retry` | Context-aware backoff (`Retry`) and an HTTP retry policy with body replay, jitter and `Retry-After` support (`HTTP`) | none |
| `cliout` | `github.com/sebrandon1/go-toolbox/cliout` | JSON output, aligned tables, output-format validation | none |
| `cobrautil` | `github.com/sebrandon1/go-toolbox/cobrautil` | Signal-aware `Execute`, version plumbing, standard shell-completion command | `spf13/cobra` |
| `yamlx` | `github.com/sebrandon1/go-toolbox/yamlx` | YAML marshal/print and file loading | `gopkg.in/yaml.v3` |
| `confkit` | `github.com/sebrandon1/go-toolbox/confkit` | Environment accessors, duration/home helpers, `KEY=VALUE` config load/update, atomic writes | none |
| `jsonstore` | `github.com/sebrandon1/go-toolbox/jsonstore` | Atomic JSON file store with optional TTL, directory store with hashed keys, file hashing | none |

## Install

```bash
go get github.com/sebrandon1/go-toolbox@latest
```

## Design rules

- A helper is only here if at least two repositories independently implemented it.
- Domain models, auth/token-refresh protocols, pagination schemes, database layers and
  scrapers stay in their own repositories.
- Core packages must not add dependencies to their consumers.
- API stability is pre-1.0: breaking changes are allowed but tagged.

## Development

```bash
make test        # go test ./... -race
make lint        # golangci-lint
make coverage    # coverage summary
```

## License

Apache-2.0. See [LICENSE](LICENSE).
