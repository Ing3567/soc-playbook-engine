# SOC Playbook Engine

Converts SOC Standard Operating Procedures (text or `.docx`) into interactive, executable flowcharts with Sandbox/Live execution, manual anti-bot handoff, and BPMN 2.0 / OASIS CACAO / Markdown export.

## Architecture

- **Frontend (`frontend/`)**: React 19, TypeScript, Vite, Tailwind CSS v4, `@xyflow/react` + `@dagrejs/dagre`
- **Backend (`backend/`)**: Go (Gin + MongoDB driver), LLM parser (Ollama / Hugging Face) with indentation fallback, Sandbox & SSRF-guarded Live HTTP executors, BPMN/CACAO/Markdown exporters
- **Database**: MongoDB 7 (`playbooks` and `tools` collections)

## Quick Start (Docker Compose)

1. Create a `.env` file at the project root (same level as `backend/` and `frontend/`, gitignored) with at least MongoDB credentials and any optional API keys:
   ```env
   MONGO_INITDB_ROOT_USERNAME=socadmin
   MONGO_INITDB_ROOT_PASSWORD=change_me_local_dev
   ALLOW_LIVE_EXECUTION=false
   ```
2. Build and start all services:
   ```bash
   docker compose up --build
   ```
3. Open `http://localhost:5173` (API health check: `http://localhost:8080/api/health`).

## Local Development

### Backend
```bash
cd backend
go run .
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

### Running Tests
```bash
cd backend && go test -v ./...
cd frontend && npm run lint && npm run build
```

## Execution Modes & Security

- **Sandbox (default)**: Runs deterministic mock responses without external network calls.
- **Live HTTP (`ALLOW_LIVE_EXECUTION=true`)**: Opt-in mode for `http-api` tools. Blocks loopback, private, link-local, and cloud metadata IP ranges (SSRF guard) and optionally restricts destinations via `LIVE_ALLOWED_HOSTS`.
- **Tool Credentials**: Stored as environment variable references (`ToolAuth.env_var`, restricted to tool credential names — system/infra env vars are blocked) or browser session-only keys (`runtime_auth`); secrets are never persisted in playbooks or returned in execution logs.
- **Manual / Anti-bot Nodes**: Tools with `command_type=manual` pause automated runs so an analyst can inspect the target in a browser and record a verdict (`clean`, `suspicious`, or `done`).

## Environment Variables

Place `.env` at the repository root (next to `backend/` and `frontend/`):

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Backend HTTP port |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Comma-separated CORS allowed origins |
| `TRUSTED_PROXIES` | *(none)* | Comma-separated proxy IPs/CIDRs trusted for `X-Forwarded-For` |
| `MONGO_INITDB_ROOT_USERNAME` | `socadmin` | MongoDB root username (Docker Compose) |
| `MONGO_INITDB_ROOT_PASSWORD` | `change_me_local_dev` | MongoDB root password (Docker Compose) |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection string |
| `MONGO_DB` | `soc_platform` | MongoDB database name |
| `LLM_PROVIDER` | `ollama` | `ollama` or `huggingface` |
| `LLM_ENDPOINT` | `http://localhost:11434` | LLM API base URL |
| `LLM_MODEL` | `qwen2.5:3b` | Model name for SOP parsing |
| `HF_TOKEN` | — | Hugging Face API token (if `LLM_PROVIDER=huggingface`) |
| `ALLOW_LIVE_EXECUTION` | `false` | Enables Live HTTP step execution |
| `LIVE_ALLOWED_HOSTS` | — | Comma-separated hostname allowlist for Live HTTP mode |
