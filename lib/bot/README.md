# usecode-mcp

MCP server that operates usecode agent: it wraps the `usecode-agent-api` HTTP endpoints as MCP tools so an
AI agent can log in and act as a usecode agent client (OTP login, session lookup, logout).

## Run

`usecode-mcp` needs a running `usecode-agent-api` to talk to. Start it first, in a separate terminal:

```sh
cd ../api && uv sync && USECODE_AGENT_NODE_NAME=api-1 uv run usecode-agent-api   # http://localhost:8000
```

Then start the bot:

```sh
uv sync
uv run usecode-mcp   # starts an MCP server over stdio
```

By default the bot spreads its requests round-robin over both Caddy load balancers from
`deploy/compose.yml` (`https://localhost:4430/api` and `https://localhost:4431/api`), failing
over to the other if one can't be reached. Set `USECODE_MCP_API_BASE_URL` to pin it to a single
address instead — a remote deployment, or a bare `usecode-agent-api` with no Caddy in front (see
Configuration below).

## Configuration

Copy `.env.example` to `.env` and adjust as needed.

- `USECODE_MCP_API_BASE_URLS` — the Caddy load balancers to spread requests over, as JSON
  (default `["https://localhost:4430/api", "https://localhost:4431/api"]`, matching
  `deploy/compose.yml`). Requests rotate over them, and one that can't reach an endpoint is
  retried against the next. This is the tier *above* Caddy's own load balancing: Caddy already
  spreads requests over `api-1`/`api-2`, but a client pinned to one Caddy goes down with it.
- `USECODE_MCP_API_BASE_URL` — single-endpoint override. When set it *replaces* the list above,
  so the bot talks to exactly that one address.
- `USECODE_MCP_API_KEY` — optional api_key (from `verify_otp`) used as the default
  `X-API-Key` for tools that accept an `api_key` argument, so an agent already tied to one
  usecode agent account doesn't have to pass it on every call.
- `USECODE_MCP_REQUEST_TIMEOUT_SECONDS` — HTTP timeout for calls to `usecode-agent-api` (default `10`).
- `USECODE_MCP_COMPOSE_FILE` — path to the compose file used by `ensure_running` /
  `logs_commands` (default `deploy/compose.yml` at the root of this checkout).
- `USECODE_MCP_CONTAINER_CLI` — `podman` (default, uses `podman-compose`, matching
  `deploy/push.sh`/`deploy/pull.sh`) or `docker` (uses `docker compose`).

## Tools

- `request_otp(phone)` — request a login code for a phone number.
- `verify_otp(phone, code)` — verify the code and get back an `api_key`.
- `me(api_key=None)` — look up the phone tied to an `api_key`.
- `logout(api_key=None)` — revoke an `api_key`.
- `health()` — check that usecode agent is reachable, reporting every configured endpoint
  rather than just the next one in the rotation (so a single dead load balancer is visible
  instead of being silently failed over). Each reachable entry names the API node that
  answered it.
- `ensure_running()` — start usecode agent locally via `deploy/compose.yml` (building images
  first) if it isn't already running (no-op otherwise).
- `stop()` — stop usecode agent locally by tearing down the `deploy/compose.yml` stack.
- `logs_commands()` — print the `podman-compose`/`docker compose` commands (with an
  absolute `-f` path, so they work from any directory) to follow logs for each service (and all
  of them combined).
- `model_options()` — list the configurable fields for kick-starting the AI model
  container (llama-server), with defaults and allowed options.
- `model_status()` — check whether the AI model container is running on the usecode-agent-api
  host.
- `model_start(image=None, hf_repo=None, device=None, ngl=None, alias=None, ctx_size=None, host=None, port=None)`
  — kick-start the AI model container on the usecode-agent-api host. Defaults to
  `ggml-org/Qwen3-0.6B-GGUF:Q4_0` on device `Vulkan0`, alias `local-model`, 32768-token context,
  `127.0.0.1:8080`; pass only the fields you want to override.
- `model_stop()` — stop the running AI model container.

## Add to Claude Code

```sh
claude mcp add usecode -- uv run --directory /path/to/usecode/lib/bot usecode-mcp
```

Pass config as env vars with `-e` if not using `.env`, e.g. to point at a non-default API:

```sh
claude mcp add usecode \
  -e USECODE_MCP_API_BASE_URL=http://localhost:8000 \
  -- uv run --directory /path/to/usecode/lib/bot usecode-mcp
```

Verify it's registered and reachable with `claude mcp list`, then check `health` from
within a session. To remove it: `claude mcp remove usecode-mcp`.

## Add to GitHub Copilot

In VS Code, Copilot Chat's agent mode picks up MCP servers from a workspace `.vscode/mcp.json`
(create it if it doesn't exist):

```json
{
  "servers": {
    "usecode-mcp": {
      "type": "stdio",
      "command": "uv",
      "args": ["run", "--directory", "/path/to/usecode/lib/bot", "usecode-mcp"],
      "env": {
        "USECODE_MCP_API_BASE_URL": "http://localhost:8000"
      }
    }
  }
}
```

(`env` is optional — omit it and use `.env` instead if you prefer.) VS Code shows a `Start`
codelens above the `usecode` entry; click it, or run **MCP: List Servers** from the Command
Palette and start it from there. Restart/reload the server the same way after editing this repo's
code, same as the Claude Code workflow below.

For the [Copilot CLI](https://docs.github.com/copilot/how-tos/copilot-cli) (`copilot`):

```sh
copilot mcp add usecode -- uv run --directory /path/to/usecode/lib/bot usecode-mcp
```

Add env vars with `--env` if not using `.env`:

```sh
copilot mcp add usecode \
  --env USECODE_MCP_API_BASE_URL=http://localhost:8000 \
  -- uv run --directory /path/to/usecode/lib/bot usecode-mcp
```

This writes to `~/.copilot/mcp-config.json` (user scope, available in every session). Verify with
`copilot mcp list` / `copilot mcp get usecode`. To remove it: `copilot mcp remove usecode`.

### Other MCP clients

For clients that read raw JSON config (e.g. `mcpServers` in a config file):

```json
{
  "mcpServers": {
    "usecode": {
      "command": "uv",
      "args": ["run", "--directory", "/path/to/usecode/lib/bot", "usecode-mcp"]
    }
  }
}
```

## Developing

Tools live in `src/usecode_mcp/server.py`, HTTP calls to `usecode-agent-api` live in
`src/usecode_mcp/client.py` (including the round-robin/failover logic every call goes through —
`UsecodeAgentClient._request`). To add a new tool: add a method to `UsecodeAgentClient` for the endpoint,
then wrap it with an `@mcp.tool()` function in `server.py` — the function's docstring becomes the
tool description shown to agents, so keep it accurate.

Make sure `usecode-agent-api` is running locally (see Run above) so there's something to talk to.

### Try changes without a client

`mcp[cli]` ships an interactive inspector — it reloads your code each run, so this is the fastest
way to try a tool change:

```sh
uv run mcp dev src/usecode_mcp/server.py
```

Or call tool functions directly in a one-off script/REPL (they're plain async functions):

```sh
uv run python -c "
import asyncio
from usecode_mcp.server import health
print(asyncio.run(health()))
"
```

### Try changes through Claude Code

Since `usecode` is registered to run via `uv run --directory .../lib/bot usecode-mcp`, edits
to the source take effect on the next process start — no reinstall needed. After editing:

- Run `/mcp` in Claude Code and reconnect `usecode`, or restart the session, to pick up the
  change.
- In VS Code Copilot Chat, use the `Restart` codelens above the `usecode` entry in
  `.vscode/mcp.json` (or **MCP: List Servers** → restart from the Command Palette).
- In the Copilot CLI, start a new session (`copilot`); it spawns the server process fresh each
  session, so there's no separate restart step.
- If you changed `pyproject.toml` (new dependency, etc.), run `uv sync` first.

### Tests

There's no test suite yet; validate manually via the inspector or a script like the one above,
covering both the success path and the error path (e.g. call `me` with a bogus `api_key`
and confirm you get back `{"error": ..., "status_code": 401}` rather than an exception).
