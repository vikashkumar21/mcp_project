# MCP Research Chatbot

A learning project built while following an MCP (Model Context Protocol) tutorial. It implements a full MCP ecosystem in Go — a custom research MCP server, a multi-server MCP chatbot client backed by Anthropic Claude, and a deployable remote server — all demonstrating the three core MCP primitives: **Tools**, **Resources**, and **Prompts**.

---

## What It Does

### Core Features

| Feature | Description |
|---|---|
| 🔧 **MCP Tools** | Claude autonomously calls `search_papers` and `extract_info` to research academic topics on arXiv |
| 📂 **MCP Resources** | Browse locally cached papers by topic using `@folders` or `@<topic>` resource URIs |
| 💬 **MCP Prompts** | Generate structured LLM prompts via `/prompts` and `/prompt <name>` slash commands |
| 🤖 **Multi-Server Client** | Connects simultaneously to multiple MCP servers (local stdio + remote HTTP) loaded from a JSON config |
| 🌐 **Remote HTTP Server** | A deployable HTTP server exposing the same research tools over Streamable HTTP and SSE transports |
| 🗂️ **arXiv Integration** | Fetches real papers from the arXiv API and caches them locally as JSON for offline use |
| 🧪 **Simulation Mode** | Runs a full demo of the MCP tool loop without needing an Anthropic API key |

### Agentic Tool Loop

When you ask a question (e.g., *"Find 3 papers on RAG"*), the chatbot:
1. Sends your query to Claude (Anthropic Messages API)
2. Claude decides to call `search_papers` — the call is routed to the research MCP server
3. The server queries arXiv, stores results locally, and returns paper IDs
4. Claude calls `extract_info` on a paper ID — again routed via MCP
5. Claude synthesizes the results and responds to you

This full agentic loop runs up to **10 turns** per query.

### Chat Commands

| Command | What it does |
|---|---|
| `<any text>` | Sends query to Claude for agentic research |
| `@folders` | Lists all locally cached research topics |
| `@<topic>` | Shows all cached papers for that topic (e.g., `@llm_interpretability`) |
| `/prompts` | Lists all available prompt templates from connected servers |
| `/prompt <name> [key=value ...]` | Executes a named prompt template (e.g., `/prompt generate_summary topic=rag`) |
| `quit` | Exits the chatbot |

---

## Tech Stack

| Layer | Technology |
|---|---|
| **Language** | Go 1.25 |
| **MCP SDK** | [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) v1.8.0 |
| **LLM** | Anthropic Claude (`claude-sonnet-4-6`) via direct HTTP to the Messages API |
| **External Data** | arXiv API (Atom/XML feed) for academic paper search |
| **Transports** | stdio (local), Streamable HTTP, SSE (remote) |
| **Third-party MCP Servers** | `@modelcontextprotocol/server-filesystem` (Node/npx), `mcp-server-fetch` (Python/uvx) |
| **Deployment** | Docker + Render.com (`render.yaml`) |

---

## Project Structure

```
mcp_project/
├── main.go                        # Entrypoint: loads config, starts MCPChatBot
├── server_config.json             # Declares which MCP servers to connect to
├── Dockerfile                     # Builds remote_server for container deployment
├── render.yaml                    # Render.com deployment spec
│
├── cmd/
│   ├── research_server/main.go    # Local stdio MCP server (runs as subprocess)
│   └── remote_server/main.go      # HTTP MCP server (Streamable HTTP + SSE)
│
├── pkg/
│   ├── chatbot/
│   │   ├── mcp_chatbot.go         # MCPChatBot: multi-server client, chat loop, tool routing
│   │   ├── chatbot.go             # Simple single-server chatbot (direct tool execution)
│   │   ├── anthropic.go           # Lightweight Anthropic Messages API client
│   │   └── tools.go               # Tool schemas + local tool dispatcher (search_papers, extract_info)
│   │
│   ├── papers/
│   │   ├── papers.go              # arXiv API search + local JSON caching (SearchPapers, ExtractInfo)
│   │   └── resources.go           # MCP resource formatters (GetAvailableFolders, GetTopicPapers)
│   │
│   └── research/
│       └── server.go              # MCP server factory: registers tools, resources, prompts
│
└── papers/                        # Local cache of fetched arXiv papers (JSON per topic)
    ├── ai_engineering/
    ├── llm_interpretability/
    ├── quantum_computing/
    └── rag_-_index_to_generate/
```

---

## Prerequisites

- **Go 1.21+** — [Download](https://go.dev/dl/)
- **Node.js + npx** — for the `@modelcontextprotocol/server-filesystem` MCP server
- **Python + uvx** — for the `mcp-server-fetch` MCP server (`pip install uv` then `uvx` is available)
- **Anthropic API Key** — [Get one here](https://console.anthropic.com/) *(optional — simulation mode works without it)*

---

## How to Run

### 1. Clone and Install Dependencies

```bash
git clone <repo-url>
cd mcp_project
go mod download
```

### 2. Build the Local Research Server

The chatbot spawns `./research_server` as a subprocess. Build it first:

```bash
go build -o research_server ./cmd/research_server
```

### 3. (Optional) Set Your Anthropic API Key

Create a `.env` file in the project root:

```bash
echo 'ANTHROPIC_API_KEY="sk-ant-..."' > .env
```

Or export it directly:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

> **Without an API key**, the chatbot runs in **Simulation Mode**, which demonstrates the full MCP tool loop (Resources → Prompts → Tools) using real MCP server calls — great for exploring the project without a paid API key.

### 4. Run the Chatbot

```bash
go run main.go
```

On startup, it connects to all servers defined in `server_config.json`:
- `research` — local `./research_server` subprocess (stdio)
- `filesystem` — `npx @modelcontextprotocol/server-filesystem .` (reads local files)
- `fetch` — `uvx mcp-server-fetch` (fetches URLs)

**Example session:**

```
Connecting to MCP Servers defined in server_config.json...

Connected to research with tools: [search_papers extract_info]
Connected to filesystem with tools: [read_file write_file list_directory ...]
Connected to fetch with tools: [fetch]

Connected to Anthropic Claude via multi-server MCP!

MCP Chatbot Started!
Type your queries, '@folders' / '@<topic>' for resources, '/prompts' or '/prompt <name>' for prompts, or 'quit' to exit.

Query: Find 2 papers on transformer attention mechanisms
Calling tool search_papers with args map[max_results:2 topic:transformer attention mechanisms]
Calling tool extract_info with args map[paper_id:2405.12345]
Here are two recent papers on transformer attention mechanisms: ...

Query: @folders
# Available Topics
- ai_engineering
- llm_interpretability
- quantum_computing

Query: /prompts
Available Prompts:
- generate_summary: Generate a comprehensive summary of research papers on a specific topic.
  Arguments: topic (required)

Query: /prompt generate_summary topic=llm_interpretability
[Prompt Template Generated]: Please provide a thorough research summary...
<Claude responds with analysis>

Query: quit
Goodbye!
```

---

## Running the Remote HTTP Server

The remote server exposes the same research MCP tools over HTTP for remote clients.

### Locally

```bash
go run ./cmd/remote_server
```

Endpoints available:
- `http://localhost:8080/mcp` — Streamable HTTP (modern MCP standard)
- `http://localhost:8080/sse` — SSE (backwards-compatible)
- `http://localhost:8080/health` — Health check

### Connect as a Remote Server

Add a remote entry to `server_config.json`:

```json
{
  "mcpServers": {
    "remote-research": {
      "url": "http://localhost:8080/mcp",
      "transport": "streamable-http"
    }
  }
}
```

### With Docker

```bash
docker build -t mcp-research-server .
docker run -p 8080:8080 mcp-research-server
```

### Deploy to Render.com

The `render.yaml` is pre-configured for [Render.com](https://render.com) deployment. Just connect your repo and it will build and deploy `cmd/remote_server` automatically.

---

## Running Tests

```bash
go test ./...
```

Tests cover:
- `pkg/chatbot` — Anthropic client, tool dispatching, MCP chatbot multi-server routing
- `pkg/papers` — arXiv search, local caching, paper extraction
- `cmd/research_server` — MCP server tool/resource/prompt registration
- `cmd/remote_server` — HTTP transport and health endpoint

---

## MCP Primitives Demonstrated

| MCP Primitive | Implementation |
|---|---|
| **Tool** | `search_papers(topic, max_results)` — searches arXiv and caches results |
| **Tool** | `extract_info(paper_id)` — retrieves cached paper metadata |
| **Resource (static)** | `papers://folders` — lists all locally cached topics |
| **Resource (dynamic)** | `papers://{topic}` — returns markdown-formatted paper details for a topic |
| **Prompt** | `generate_summary(topic)` — generates a structured research prompt for Claude |
