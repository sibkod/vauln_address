# Wallet Checker Server

A V Language web server with WebSocket support for checking wallet addresses against a database of compromised wallets.

## Features

- **HTTP Server** (port 8080) - Serves the frontend UI and API
- **WebSocket Server** (port 8081) - Real-time wallet checks
- **Demo Data** - 15 demo wallets with various security statuses
- **REST API** - Check wallets via HTTP requests

## Running

```bash
# Compile
v -enable-globals .

# Run
./vauln_address
```

## API Endpoints

- `GET /` - Serve the frontend UI
- `GET /api/health` - Health check
- `GET /api/stats` - Get statistics (hacked/vulnerable/safe counts)
- `GET /api/wallets` - Get all demo wallets
- `GET /api/recent` - Get recent wallet checks
- `GET /api/wallet/<address>` - Check specific wallet

## Example Usage

```bash
# Check wallet
curl http://localhost:8080/api/wallet/0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb

# Get stats
curl http://localhost:8080/api/stats

# Get recent checks
curl http://localhost:8080/api/recent
```

## WebSocket Messages

Connect to `ws://localhost:8081` and send JSON messages:

```json
{"type": "ping"}
{"type": "check_wallet", "address": "0x..."}
```

## Tech Stack

- **V Language** - Fast, compiled language
- **net module** - TCP/HTTP/WebSocket server
- **No external dependencies** - Self-contained binary
