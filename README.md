# enoch-edge

Wallet-facing edge proxy for the [Enoch](../enoch) federated Bitcoin
L2. Mirrors the bitcoind / electrs / wallet pattern: operators serve
raw protocol primitives, **edge** does the wallet conveniences.

## Split with the operator

| Concern | Lives in |
|---|---|
| UTXO state, Bitcoin Script execution, ledger | operator |
| State-root signing, slashing, fraud claims | operator |
| Bridge custody / agent signing | operator + bridge agents |
| `/info`, `/state_roots/latest` self-describe | operator |
| **Wallet API versioning (`/v1/*`)** | **edge** |
| **Address-format normalization** (`enoch1`/`bc1q`/`tb1q`/`bcrt1q`) | **edge** |
| **Per-address transaction history index** | **edge** |
| **SSE event multiplexing to wallets** | **edge** |
| **Caching, rate limiting, CORS** | **edge** |
| **Cross-operator agreement / fan-out** (later) | **edge** |

The operator stays narrow. Wallet-facing surface evolves at the edge,
where response shapes can change without touching protocol code.

## Stack

- Go 1.22 + [`go-chi/chi/v5`](https://github.com/go-chi/chi)
- Multi-stage `Dockerfile` (golang:1.22-alpine → alpine:3.20)
- Standalone `docker-compose.yml`
- Talks to the operator over plain HTTP (`OPERATOR_URL` env)

## Run

```bash
# bring up the enoch operator stack first (separate repo)
cd ../enoch && make up

# then edge
cd ../enoch-edge
make build
make up
make curl-health
make curl-info
```

By default edge reaches the operator on `http://host.docker.internal:8080`.
To skip the host bridge and put edge on the same docker network as
the operator, see the commented `networks:` block in
`docker-compose.yml`.

## Endpoints (initial)

| Method | Path | Notes |
|---|---|---|
| GET | `/v1/health` | local liveness, does not call upstream |
| GET | `/v1/info` | proxies operator `/info`, wraps with edge metadata |

More to come (utxos, balance, address_history, events, fee oracle)
as the iOS wallet PoC needs them.
