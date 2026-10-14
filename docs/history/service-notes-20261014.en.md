# 历史实现与验收记录

以下为 2026-10-14 整理中文首页前的原始英文记录，保留具体参数、证据编号及历史状态。相对路径以仓库根目录为基准；当前状态以根目录 README 和集成计划为准。

---

# Agent OS Sources

## Capture management candidate

The control-only capture API accepts a caller-generated 32-character lowercase
hex ID before fetching. `POST /captures` with the same ID and exact URL returns
the retained observation; a different URL is rejected. A discarded reply can be
recovered with `GET /captures/{id}`. Concurrent first requests can perform more
than one read-only HTTP fetch, but only one complete observation commits.
URL-scoped history uses `GET /captures?url=<exact URL>&after=<capture ID>` and
returns at most 20 records. These paths are management operations, not new MCP
tools or a claim of exactly-once network fetching.

Knowledge may read `/capture-capabilities` and `/captures/{id}/bundle` using its
own service identity, but cannot initiate capture or list arbitrary history.
The bundle preserves client-retained raw bytes and extracted text with hashes;
it rejects imports exceeding 1 MiB per payload without truncating stored Sources
data. Collection time is not publication time. Raw means the HTTP response body
after any transport/content decoding, not the original network wire bytes.

Run this owner's migration before deploying this candidate: it adds
`source_captures.extraction_version`, retaining `unrecorded` for legacy captures.
Application startup never performs DDL. The legacy service remains compatible
with the additive column. Normal service readiness does not prove downstream
Knowledge import availability. The new API, migration and controlled concurrent
capture checks are local fixture evidence until recorded as a managed deployment.

Independent public-source MCP adapter. Owns only the `sources` schema's raw
content blobs and capture metadata. It retains the exact HTTP response before
returning parsed evidence, including original URL, fetch timestamp and SHA-256.
Fetch time is not publication time or the statistical period. It does not infer
facts, predict prices or decide trades.

Tools expose the operator-approved source catalog, fetch a specific URL, paginate
an immutable capture and find a literal term within an archived page/its links.
This is not a general web search engine. HTTPS allowlisted hosts only; IP/private
network targets, embedded credentials, sensitive query parameters and redirects
are restricted. No login, paid-site bypass, browser JavaScript or host shell.
Responses are bounded at 1 MiB raw / 3 MiB decoded; supported formats are text,
HTML, JSON and XML. PDF/OCR is not currently implemented. Tool content remains
untrusted evidence; retaining it does not establish factual reliability.

## Independent deployment

Go 1.27.2 / PostgreSQL 18. Run `scripts/Build.ps1` (PowerShell 7), or
`go build -mod=vendor ./cmd/sources` on Linux. Committed vendor plus fixed SDK
provenance permits one-checkout offline dependency builds with `GOWORK=off`;
no sibling repo, Node, model key or private-module CI token is needed.

Use a non-owner application database role with `search_path=sources` and distinct
service token/peer hashes (including self). The MCP transport uses the configured
Sources identity; registered connector credentials are resolved by name through
Registry. Control may download captures, while unrelated service identities
cannot bypass MCP authorization. Protect the internal network/ports with the
deployment's TLS policy. `SOURCE_ALLOWED_HOSTS` is operator configuration, never
an Agent-provided allowlist. See `deploy/service.example.json` for placeholders.

Provision the role separately. Supply DDL credentials only as
`MIGRATION_DATABASE_URL`, then run `bin/migrate --role <role>`. It creates/updates
only this schema and grants in one bounded transaction; no cross-domain import,
bootstrap or secret/config rewriting. Normal `bin/sources --config <file>` checks
readiness without DDL. Default HTTP concurrency is 4, DB pool 3. Keep raw captures,
DB and config through compatible independent release/rollback, identified by
`deploy/release-contract.json`. No automatic capture deletion is scheduled.

## Verification scope

For managed Windows recovery, `cmd/migrate --config <protected-file>` accepts
exactly `MIGRATION_DATABASE_URL` and `ROLE` in a JSON object of at most 64 KiB.
The coordinator owns the file's access controls and process lifetime. The URL
is never a command-line argument; errors do not echo configuration values.
Combining this mode with `--role` or the `MIGRATION_DATABASE_URL` environment
variable is rejected. The existing environment plus `--role` invocation remains
available for dedicated migration jobs. A process exit alone is not proof that
DDL committed: the coordinator must verify the database before releasing writers.

`go test -mod=vendor -count=1 ./...` / `go vet -mod=vendor ./...`.
With `TEST_DATABASE_URL`, tests create isolated DBs/roles, verify permissions and
DDL rollback, preserve exact source bytes and immutable pages, reject incomplete
oversized acquisition, and check Chinese UTF-8. HTTP source responses in these
tests are synthetic; they do not establish availability of a real website or
commodity coverage. Without PostgreSQL DB tests explicitly skip. CI runs Linux
race and image builds plus Windows portability; live capture/release verification
is recorded by the integration platform, not inferred from these tests.

Legacy local configurations may omit the Runtime address: this service preserves
the previous loopback default (127.0.0.1:8094) for its bounded heartbeat/client.
Where used, Registry (8096) and ToolHub (8091) defaults are also owned by the
service entrypoint. Explicit environment/config endpoints always take precedence.
Configure these addresses explicitly for containers or multiple hosts. No config
file or shared SDK role policy is rewritten by this compatibility behavior.
