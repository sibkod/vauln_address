# Wallet Checker Server

A V Language web server with HTTP API and WebSocket support for checking wallet addresses against a database of compromised wallets.

## Features

- **HTTP Server** (port 8080) - FastHTTP-based REST API
- **WebSocket Server** (port 8081) - Real-time wallet checks
- **IP Rate Limiting** - Maximum 3 checks per IP per hour (configurable)
- **Demo Data** - 10 demo wallets with various security statuses
- **Thread-safe** - Mutex-protected shared state
- **HTML Frontend** - Served from `./templates/index.html`

## Project Structure

```
vauln_address/
├── main.v           # Entry point
├── app/
│   ├── models.v     # Data models (Config, AppState, Wallet, demo data)
│   └── handlers.v   # HTTP & WebSocket handlers
├── templates/
│   └── index.html   # Frontend UI
└── README.md
```

## Running

```bash
# Compile (requires -enable-globals for global state)
v -enable-globals .

# Run
./vauln_address
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `WS_PORT` | 8081 | WebSocket server port |
| `MAX_CHECKS` | 3 | Max checks per IP |
| `RATE_LIMIT_TTL` | 3600 | Rate limit TTL in seconds |

## API Endpoints (HTTP)

| Endpoint | Description |
|----------|-------------|
| `GET /` | Serve the frontend UI |
| `GET /api/health` | Health check |
| `GET /api/stats` | Get statistics (hacked/vulnerable/safe counts) |
| `GET /api/wallets` | Get all demo wallets |
| `GET /api/rate-limit` | Check remaining API calls for your IP |
| `GET /api/wallet/<address>` | Check specific wallet (rate limited) |

## WebSocket API (port 8081)

Connect to `ws://localhost:8081` and send JSON messages:

```json
// Ping/pong
{"type":"ping"}
{"type":"pong"}

// Check wallet (rate limited)
{"type":"check_wallet", "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb"}

// Get stats
{"type":"get_stats"}

// Get wallets
{"type":"get_wallets"}

// Get rate limit status
{"type":"get_rate_limit"}
```

## Example Usage

```bash
# Check stats
curl http://localhost:8080/api/stats

# Check wallet
curl http://localhost:8080/api/wallet/0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb

# Check rate limit status
curl http://localhost:8080/api/rate-limit
```

## Rate Limiting

- **Limit**: 3 checks per IP address (configurable via `MAX_CHECKS`)
- **TTL**: 1 hour (configurable via `RATE_LIMIT_TTL`)
- **Storage**: In-memory map (auto-expires)
- **Response**: HTTP 429 with `Retry-After` header when exceeded

## Tech Stack

- **V Language** - Fast, compiled language
- **fasthttp** - High-performance HTTP server
- **net.websocket** - WebSocket server
- **sync** - Mutex for thread-safe state access
- **Best Practices**: Module separation, global state with mutex protection
