# Creative Edit MCP Service Requirements

## Background

Multica supports configuring a Streamable HTTP MCP connection in
Settings > Integrations > Workspace MCP. The server uses that connection to
call `create_creative_job` and `get_creative_job`.

The workspace MCP connection is a backend integration, not an agent runtime
MCP configuration. Agent runtime MCP remains managed by each agent's
`mcp_config`.

## Goal

Add `services/creative-mcp/`, a deployable Creative Edit MCP service. The
first version may use a replaceable mock backend for local development and
CI, while keeping an adapter boundary for a real editing provider.

## MCP Protocol

- Expose `POST /mcp` using MCP Streamable HTTP protocol version `2025-03-26`.
- Support `initialize`, `notifications/initialized`, `tools/list`, and
  `tools/call`.
- Return `Mcp-Session-Id` during initialization and validate it on subsequent
  requests.
- Support an environment-configured authentication header, such as
  `Authorization: Bearer ...`; reject unauthenticated requests with HTTP 401.
- `tools/list` must include `create_creative_job` and `get_creative_job`.

## Tool Contracts

### `create_creative_job`

Input:

- `job_id` and `idempotency_key`
- `prompt`
- `dynamic_rules`
- `candidates[]`, including `id`, `source_url`, and creative metadata

Requirements:

- Repeated calls with the same `idempotency_key` return the same external job.
- Return MCP `structuredContent` with `job_id`, `status`, `stage`, `progress`,
  `poll_after_ms`, and a sanitized `process_data` object when process telemetry
  is available.
- Supported states are `queued`, `running`, `completed`, and `failed`.

### `get_creative_job`

Input: `job_id`.

Requirements:

- A pending job returns its status, progress, stage, and next polling delay.
- A completed job returns `variants[]`. Each variant includes `candidate_id`,
  `variant_index`, `title`, `description`, `qc_status`, and `assets[]`.
- Each asset includes `width`, `height`, `label`, `asset_url`, `content_type`,
  and optional `storage_key`.
- A failed job returns `status: "failed"` and `error_message`.

## Engineering Requirements

- Follow the Node ESM, Dockerfile, and test patterns in `services/crawler-worker`.
- Persist jobs outside process memory so `get_creative_job` still works after
  the MCP service restarts. Use a dedicated PostgreSQL table or the upstream
  provider's durable job API.
- Do not log authentication headers or return credentials in tool responses.
- `process_data` must never contain API keys, authentication headers, service
  URLs, raw credentials, absolute file paths, or signed asset URLs. If actual
  token or cost data is unavailable, return `not_available` rather than an
  estimate.
- Add Docker Compose wiring and a README covering environment variables,
  authentication, endpoint configuration, and local startup.
- The mock backend must provide a controllable `queued -> running -> completed`
  flow and stable test asset URLs.

## Acceptance Criteria

1. A workspace MCP connection can be configured and verified from Multica.
2. An issue with selected creative material can create a creative job.
3. Multica polls `get_creative_job` and persists returned variants and assets.
   It also persists the latest `process_data` object without erasing a previous
   snapshot when a later poll omits the field.
4. An MCP service restart does not make submitted jobs unqueryable.
5. Tests cover unauthenticated access, unknown tools, invalid sessions, and
   failed jobs.
6. Service tests and relevant Go tests pass.

## Non-Goals

- Do not change agent runtime `mcp_config`.
- Do not introduce arbitrary workspace MCP capabilities or arbitrary tool names.
- Do not send Credential Broker cookies, browser state, or ciphertext to the
  MCP service.

## Open Dependency

Real image editing requires a selected upstream provider and its API
credentials. Without that input, implement the MCP protocol, durable jobs,
and mock end-to-end integration only; do not claim real image generation.
