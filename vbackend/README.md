# VaulnAddress Backend (V/Veb)

Vlang backend for the VaulnAddress service using Veb framework and SQLite. Full feature parity with Go backend.

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

### Health & Info
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/chains` | List supported chains |
| GET | `/api/pricing` | Get pricing info |
| GET | `/api/packages` | Get pricing packages |
| GET | `/api/stats` | Get wallet statistics |

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/auth/nonce` | Generate authentication nonce |
| POST | `/api/auth/login` | Authenticate with wallet signature |
| GET | `/api/me` | Get current user info |
| GET | `/api/user/profile` | Get user profile |
| GET | `/api/user/balance` | Get user balance |
| GET | `/api/user/purchases` | Get purchase history |

### Wallet Check
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/check` | Check wallet vulnerability |
| GET | `/api/recent` | Get recent checks |
| GET | `/api/checks` | Get check history |

### Orders
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/orders` | Create order |
| POST | `/api/orders/:id/cancel` | Cancel order |
| POST | `/api/orders/:id/confirm` | Confirm order |
| GET | `/api/orders/verify` | Verify payment |
| POST | `/api/payment/status/:signature` | Get payment status |

### API Keys
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/api-keys` | List API keys |
| POST | `/api/api-keys` | Create API key |
| DELETE | `/api/api-keys/:id` | Delete API key |
| POST | `/api/api-keys/revoke/:id` | Revoke API key |
| POST | `/api/api-keys/renew` | Renew API key |

### Admin & Contact
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/wallets` | Add wallet(s) to database |
| POST | `/contact` | Submit contact form |

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

# Get packages
curl http://localhost:8080/api/packages

# Get stats
curl http://localhost:8080/api/stats

# Generate auth nonce
curl "http://localhost:8080/api/auth/nonce?address=0x123&chain=evm"

# Authenticate
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"address":"0x123","chain":"evm","signature":"sig","message":"msg"}'

# Check wallet
curl -X POST http://localhost:8080/api/check \
  -H "Content-Type: application/json" \
  -d '{"address":"0x123","chain":"evm"}'

# Recent checks
curl "http://localhost:8080/api/recent?limit=10"

# Create order
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{"chain":"evm","wallet_address":"0x123","checks":5}'

# Add wallet
curl -X POST http://localhost:8080/api/wallets \
  -H "Content-Type: application/json" \
  -d '{"addresses":{"evm":"0x123"},"status":"hacked","reason":"manual","source":"api"}'
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
- `api_keys` - API key storage
