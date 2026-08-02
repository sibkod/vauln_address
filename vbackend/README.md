# VaulnAddress Backend (V/Veb)

Vlang backend for the VaulnAddress service using Veb framework and SQLite.

## Requirements

- V compiler (recent version)
- Veb framework (included in vlib)
- SQLite3 development headers

## Project Structure

```
vbackend/
├── src/
│   └── main.v      # Main application (all code in single file)
├── vauln-address-api  # Compiled binary
├── vauln_address.db   # SQLite database (created on first run)
└── README.md
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/chains` | List supported chains |
| GET | `/api/pricing` | Get pricing info |
| GET | `/api/stats` | Get wallet statistics |
| POST | `/api/check/:chain/:address` | Check if wallet is registered |
| POST | `/api/wallets` | Add wallet(s) to database |
| POST | `/api/order` | Create order |
| POST | `/contact` | Contact form |

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `vauln_address.db` | Path to SQLite database file |

## Build & Run

```bash
# Build
v -prod -o vauln-address-api src/main.v

# Run
./vauln-address-api
```

## Example API Calls

```bash
# Health check
curl http://localhost:8080/api/health

# List chains
curl http://localhost:8080/api/chains

# Get pricing
curl http://localhost:8080/api/pricing

# Get stats
curl http://localhost:8080/api/stats

# Check wallet (EVM)
curl -X POST http://localhost:8080/api/check/evm/0x123...

# Add wallet
curl -X POST http://localhost:8080/api/wallets \
  -H "Content-Type: application/json" \
  -d '{"addresses":{"evm":"0x123..."},"status":"clean","reason":"manual","source":"api"}'

# Create order
curl -X POST http://localhost:8080/api/order \
  -H "Content-Type: application/json" \
  -d '{"chain":"evm","address":"0x123...","checks_count":5,"currency":"usd"}'
```

## Development

```bash
# Run in development mode
v run src/main.v
```

## Database

SQLite database is automatically created on first run with tables:
- `wallets` - Registered wallet addresses
- `users` - User accounts with balance tracking  
- `orders` - Order records
- `check_history` - Historical check records
