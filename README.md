# Wallet Checker Server

A V Language web server for checking wallet addresses against a database of compromised wallets.

## Features

- **HTTP Server** (port 8080) - Serves the frontend UI and REST API
- **IP Rate Limiting** - Maximum 3 checks per IP per hour (configurable)
- **Demo Data** - 10 demo wallets with various security statuses
- **REST API** - Check wallets via HTTP requests
- **No external dependencies** - Self-contained binary

## Running

```bash
# Compile
v .

# Run
./vauln_address
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `MAX_CHECKS` | 3 | Max checks per IP |
| `RATE_LIMIT_TTL` | 3600 | Rate limit TTL in seconds |

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Serve the frontend UI |
| `GET /api/health` | Health check |
| `GET /api/stats` | Get statistics (hacked/vulnerable/safe counts) |
| `GET /api/wallets` | Get all demo wallets |
| `GET /api/recent` | Get recent wallet checks |
| `GET /api/rate-limit` | Check remaining API calls for your IP |
| `GET /api/wallet/<address>` | Check specific wallet (rate limited) |

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
- **net module** - TCP/HTTP server
- **Best Practices**: No globals, state passed via struct, proper error handling
