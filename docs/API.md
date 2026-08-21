# Vauln Address API Documentation

## Overview

API for checking wallet addresses against a database of compromised/hacked wallets. Supports multi-chain: EVM, Bitcoin, Solana, Sui, and Tron.

**Base URL:** `/api`

**Note:** This API uses wallet address as the primary user identifier (not numeric user IDs).

## Authentication

Most endpoints require Web3 authentication. After successful authentication, you'll receive a JWT token containing your wallet address.

### Web3 Authentication Flow

1. **Get Nonce** - `GET /api/auth/nonce?address={address}&chain={chain}`
2. **Sign Message** - Sign the returned message with your wallet
3. **Authenticate** - `POST /api/auth/login` with signature

---

## Endpoints

### Health Check

**GET** `/api/health`

Check if the API is running.

**Response:**
```json
{
  "status": "ok",
  "service": "vauln-address-api",
  "time": "2024-01-01T00:00:00Z"
}
```

---

### Get Supported Chains

**GET** `/api/chains`

Returns list of supported blockchain networks.

**Response:**
```json
{
  "chains": [
    {"name": "EVM", "id": "evm", "example": "0x742d35Cc..."},
    {"name": "Bitcoin", "id": "btc", "example": "bc1qxy2kg..."},
    {"name": "Solana", "id": "solana", "example": "7EcDhSYGx..."},
    {"name": "Sui", "id": "sui", "example": "0x8a1c4cd2..."},
    {"name": "Tron", "id": "tron", "example": "TJK5M5kKx..."}
  ]
}
```

---

### Get Pricing

**GET** `/api/pricing?checks={count}`

Returns pricing for different payment methods. Default is 10 checks if not specified. Maximum is 100.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| checks | int | 10 | Number of checks (1-100) |

**Response:**
```json
{
  "checks": 50,
  "price_per_check_usd": 0.10,
  "payment_methods": [
    {"currency": "usdc", "price_usd": 5.0},
    {"currency": "usdt", "price_usd": 5.0},
    {"currency": "eth", "price_usd": 5.0, "token_amount": 0.0025},
    {"currency": "sui", "price_usd": 5.0, "token_amount": 5.0, "has_discount": true, "discount_label": "50% OFF"}
  ]
}
```

**Pricing:**
- $0.10 per check
- 40% discount for 500+ checks
- 50% discount for 1000+ checks

---

### Get Pricing Packages

**GET** `/api/packages`

Returns pre-defined pricing packages for display on the frontend. Prices are calculated server-side.

**Response:**
```json
{
  "packages": [
    {
      "id": "starter",
      "name": "Starter",
      "checks": 50,
      "price_usd": 5.0,
      "price_sol": 0.01,
      "discount_percent": 0,
      "discount_label": "",
      "popular": false
    },
    {
      "id": "pro",
      "name": "Pro",
      "checks": 200,
      "price_usd": 20.0,
      "price_sol": 0.01,
      "discount_percent": 0,
      "discount_label": "",
      "popular": true
    },
    {
      "id": "enterprise",
      "name": "Enterprise",
      "checks": 1000,
      "price_usd": 50.0,
      "price_sol": 0.01,
      "discount_percent": 50,
      "discount_label": "50% OFF",
      "popular": false
    }
  ],
  "payment_address": "7bMD8B3a3yDj7JMBQZYse7x4FqNKLNmEACSUitKxVNXJ",
  "price_per_check": 0.10,
  "network": "devnet"
}
```

---

### Get Recent Checks

**GET** `/api/recent`

Returns recent wallet checks (public feed).

**Response:**
```json
{
  "checks": [
    {
      "id": 1,
      "address": "0x742d35Cc...",
      "chain": "evm",
      "status": "safe",
      "checked_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

---

### Get Wallet Report

**GET** `/api/report?address={address}&chain={chain}`

Detailed report for an address **found in the database**. Access is granted only
after the same requester checks the address via `POST /api/check`:

- **Authenticated users** (JWT, checked from their wallet address) keep reports forever.
- **Anonymous users** (identified by IP) can open the report for **24 hours** after
  the check; after that the report is deleted and the endpoint returns `REPORT_EXPIRED`.

**Response (found, 200):**
```json
{
  "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1",
  "chain": "evm",
  "found": true,
  "status": "hacked",
  "reason": "leaked private key",
  "details": "The private key of this wallet is publicly available to everyone. ...",
  "source": "github",
  "has_pk": true,
  "has_seed": false,
  "leaks": [
    { "key_type": "private_key", "source": "github_leak", "discovered_at": "2024-01-01T00:00:00Z" }
  ],
  "transactions": {
    "address": "0x742d35...",
    "tx_count": 12,
    "amount": 96.7142,
    "currency": "ETH",
    "status": "hacked",
    "children": [ { "address": "0x6dd8...", "tx_count": 1, "amount": 54.5901, "currency": "ETH", "status": "unknown" } ]
  },
  "expires_at": "2024-01-02T00:00:00Z",
  "created_at": "2024-01-01T00:00:00Z"
}
```

`transactions` is a deterministic tree of outgoing transfers. Child wallet
statuses are resolved against the database; addresses not present in it are
classified as `unknown` or `potential_hacker`. `expires_at` is only present
for anonymous requests.

**Errors:**
- `400 INVALID_REQUEST / INVALID_CHAIN / INVALID_ADDRESS` - missing or invalid params
- `403 REPORT_NOT_AVAILABLE` - check the address first
- `403 REPORT_EXPIRED` - anonymous report is older than 24 hours
- `404 NOT_FOUND` - address not found in the database

---

### Make Report Public

**POST** `/api/report/share` 🔒 (authentication required)

Mints a public share link for a report. Only **authenticated users** can make
reports public - anonymous users receive `401`. The user must have checked the
address before.

**Request:**
```json
{ "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", "chain": "evm" }
```

**Response:**
```json
{
  "share_id": "00000000-1a2b-3c4d-9e8f-a1b2c3d4e5f6",
  "share_url": "/report/00000000-1a2b-3c4d-9e8f-a1b2c3d4e5f6"
}
```

The `share_id` is a UUID-like token bound to the user's check record (HMAC-signed,
no extra storage). Anyone holding the link can open the report.

**Errors:** `401 UNAUTHORIZED` - not authenticated; `403 REPORT_NOT_AVAILABLE`;
`404 NOT_FOUND`.

---

### Get Shared Report

**GET** `/api/report/shared/:id`

Opens a publicly shared report by its UUID token. No authentication, no 24h
expiry - the link itself is the capability. Response is the same as
`GET /api/report` plus `"public": true` and without `expires_at`.

**Errors:** `404 INVALID_SHARE` - invalid or unknown token; `404 NOT_FOUND` -
address no longer in the database.

---

## Drainer Scanner & Monitoring

The Solana drainer scanner (`solana_scan.py`) reports every detection to the
backend; findings feed the live monitoring page and the report evidence chain.

### Ingest Scan Finding (admin)

**POST** `/api/admin/scanner/findings`

Headers: `X-Admin-Key: <ADMIN_API_KEY>`.

```json
{
  "chain": "solana",
  "signature": "5Kd4...",
  "slot": 123456789,
  "verdict": "DRAINER",
  "indicators": ["P2_FULL_BALANCE_SWEEP", "P3_UNKNOWN_PROGRAM"],
  "victim_address": "VictimWallet...",
  "hacker_address": "HackerWallet...",
  "amount_sol": 1.5,
  "programs": ["EtrnLzg..."],
  "source": "watch"
}
```

Stores the finding (deduplicated by `signature`). For `DRAINER` verdicts the
victim is registered in the wallets table as `drained` and the hacker as
`hacker` (source `solana_scan`), unless already present.

**Response:** `{ "id": 1, "inserted": true, "victim_added": true, "hacker_added": true }`

### Monitor Findings

**GET** `/api/monitor/findings?limit=20&after_id=0`

Public live feed of scanner findings. Without `after_id` returns the latest
rows newest-first; with `after_id` returns only newer rows ascending (used by
the live page for incremental polling).

### Monitor Stats

**GET** `/api/monitor/stats`

Aggregate counters: total findings, DRAINER/SUSPICIOUS counts, distinct
victim/hacker counts and total swept SOL.

### Get Captcha

**GET** `/api/captcha`

Issues a one-time captcha challenge for the drainer report form:
`{ "captcha_id": "...", "image": "data:image/svg+xml;base64,..." }`.
Challenges live in memory for 10 minutes and are single-use.

### Submit Drainer Report

**POST** `/api/drainer-reports`

Public (no auth) user report about a drainer theft; a valid captcha is
mandatory. The report is stored and forwarded to the team Telegram bot
(`TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID`).

```json
{
  "tx_signature": "5Kd4...",
  "chain": "solana",
  "site_url": "https://scam-site.example",
  "description": "what happened",
  "captcha_id": "uuid from GET /api/captcha",
  "captcha_answer": "ABCDE"
}
```

**Errors:** `403 CAPTCHA_INVALID` - wrong/expired/reused captcha answer (fetch
a new one); `400 INVALID_REQUEST` / `400 INVALID_CHAIN`.

### Report evidence chain

`GET /api/report` (and shared reports) now include an `evidence` array: an
ordered list of reasons the wallet carries its status - registry listing, key
leaks and scanner indicators (P1..P5) with the transaction each indicator was
detected in, the counterparty and the swept amount.

---

## Authentication Endpoints

### Get Nonce

**GET** `/api/auth/nonce?address={address}&chain={chain}`

Generates a nonce for Web3 authentication.

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| address | string | Yes | Wallet address |
| chain | string | Yes | Chain: `evm`, `btc`, `solana`, `sui`, `tron` |

**Response:**
```json
{
  "nonce": "abc123...",
  "message": "Sign this message to authenticate with Vauln Address.\n\nNonce: abc123...\nTimestamp: 1704067200"
}
```

**Errors:**
- `400 INVALID_REQUEST` - Missing address or chain
- `400 INVALID_CHAIN` - Unsupported chain

---

### Authenticate

**POST** `/api/auth/login`

Verifies signature and returns JWT token.

**Headers:**
- `Content-Type: application/json`

**Request Body:**
```json
{
  "address": "0x742d35Cc...",
  "chain": "evm",
  "signature": "0x...",
  "message": "Sign this message to authenticate with Vauln Address..."
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 86400,
  "user": {
    "wallet_address": "0x742d35Cc...",
    "chain": "evm",
    "balance": 100,
    "is_premium": true
  }
}
```

**Note:** User response no longer contains numeric `id` field - users are identified by wallet address.

**Errors:**
- `400 INVALID_REQUEST` - Invalid request body
- `401 AUTH_FAILED` - Signature verification failed

---

### Get User Profile

**GET** `/api/user/profile`

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
{
  "user": {
    "wallet_address": "0x742d35Cc...",
    "chain": "evm",
    "balance": 100,
    "is_premium": true,
    "created_at": "2024-01-01T00:00:00Z",
    "last_login_at": "2024-01-01T00:00:00Z"
  }
}
```

**Errors:**
- `401 UNAUTHORIZED` - Missing or invalid token
- `404 NOT_FOUND` - User not found

---

### Get Current User (Me)

**GET** `/api/me`

Returns comprehensive user information including balance, rate limits, and authentication status. This endpoint is designed for polling from the frontend to keep user data up-to-date.

**Headers:**
- `Authorization: Bearer {token}` (optional - works for both authenticated and anonymous users)

**Response (Authenticated):**
```json
{
  "wallet_address": "0x742d35Cc...",
  "chain": "evm",
  "balance": 100,
  "purchased_balance": 100,
  "rate_limit_remaining": 95,
  "rate_limit_used": 5,
  "rate_limit_limit": 100,
  "is_premium": true,
  "is_authenticated": true,
  "created_at": "2024-01-01T00:00:00Z",
  "last_login_at": "2024-01-01T00:00:00Z"
}
```

**Response (Anonymous):**
```json
{
  "wallet_address": null,
  "chain": null,
  "balance": 95,
  "purchased_balance": 0,
  "rate_limit_remaining": 95,
  "rate_limit_used": 5,
  "rate_limit_limit": 100,
  "is_premium": false,
  "is_authenticated": false
}
```

**Response Headers:**
- `X-RateLimit-Limit` - Maximum requests allowed
- `X-RateLimit-Remaining` - Requests remaining
- `X-RateLimit-Used` - Requests used
- `X-RateLimit-Reset` - Unix timestamp for reset
- `X-RateLimit-Source` - `ip` or `balance`
- `X-Balance-Available` - Available balance (for authenticated users)

**Errors:**
- Returns 200 OK for all requests (works for anonymous users)

---

## Payment Endpoints

### Create Order

**POST** `/api/orders`

Creates a new payment order for checks.

**Headers:**
- `Content-Type: application/json`
- `Authorization: Bearer {token}`

**Request Body:**
```json
{
  "checks": 50,
  "chain": "solana",
  "wallet_address": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"
}
```

**Response:**
```json
{
  "order_id": "uuid-here",
  "checks_count": 50,
  "total_usd": 5.0,
  "amount": "0.0100",
  "payment_address": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV",
  "due_date": "2024-01-01T00:30:00Z",
  "status": "pending"
}
```

**Errors:**
- `401 UNAUTHORIZED` - Authentication required
- `400 INVALID_REQUEST` - Invalid request body
- `400 INVALID_CHECKS` - Invalid checks count

---

### Confirm Order

**POST** `/api/orders/:id/confirm`

Confirms payment by providing transaction signature or message signature.

**Headers:**
- `Authorization: Bearer {token}`

**Query Parameters:**
- `tx_signature` - Solana transaction signature (optional, legacy)

**Request Body (optional):**
```json
{
  "signature": "0x...",
  "message": "...",
  "wallet_address": "..."
}
```

**Response:**
```json
{
  "status": "completed",
  "balance": 150,
  "message": "Order confirmed successfully"
}
```

**Errors:**
- `401 UNAUTHORIZED` - Authentication required
- `404 NOT_FOUND` - Order not found
- `403 FORBIDDEN` - Order doesn't belong to user (verified by wallet address)

---

### Verify Payment

**GET** `/api/orders/verify`

Verifies payment for the current user's pending order.

**Headers:**
- `Authorization: Bearer {token}`

**Query Parameters:**
- `tx_signature` - Transaction signature

**Response:**
```json
{
  "message": "payment verified",
  "order_id": "uuid-here",
  "checks_added": 50,
  "status": "completed"
}
```

---

### Get Payment Status

**POST** `/api/payment/status/:signature`

Checks Solana transaction status and credits balance if confirmed.

**Headers:**
- `Authorization: Bearer {token}`

**Path Parameters:**
- `signature` - Solana transaction signature

**Response:**
```json
{
  "status": "confirmed",
  "confirmed": true,
  "balance": 150,
  "message": "Payment confirmed! 50 checks added."
}
```

**Status Values:**
- `pending` - Transaction is processing
- `confirmed` - Transaction confirmed, checks credited
- `already_claimed` - Already credited
- `failed` - Transaction failed on chain

**Errors:**
- `401 UNAUTHORIZED` - Authentication required
- `400 INVALID_REQUEST` - Missing signature
- `503 RPC_ERROR` - Solana RPC error

---

## Wallet Checking

### Check Wallet

**POST** `/api/check`

Checks if a wallet address is in the compromised database.

**Headers:**
- `Content-Type: application/json`
- `Authorization: Bearer {token}` (optional - rate limited without auth)

**Request Body:**
```json
{
  "address": "0x742d35Cc...",
  "chain": "evm"
}
```

**Response (Safe):**
```json
{
  "address": "0x742d35Cc...",
  "chain": "evm",
  "status": "not_found",
  "found": false,
  "has_pk": false,
  "has_seed": false,
  "balance_left": 95
}
```

**Response (Compromised):**
```json
{
  "address": "0x742d35Cc...",
  "chain": "evm",
  "status": "hacked",
  "found": true,
  "has_pk": true,
  "has_seed": false,
  "balance_left": 94
}
```

**Status Values:**
- `not_found` / `safe` - Address not in database
- `hacked` - Address found, private key leaked
- `vulnerable` - Address flagged as vulnerable
- `hacker` - Address belongs to hacker
- `drained` - Address has been drained

**Errors:**
- `400 INVALID_REQUEST` - Missing address or chain
- `400 INVALID_CHAIN` - Unsupported chain
- `400 INVALID_ADDRESS` - Invalid address format

---

## Contact

### Submit Contact Form

**POST** `/api/contact`

Submit a contact message.

**Request Body:**
```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "message": "I have a question about your service..."
}
```

**Response:**
```json
{
  "message": "contact form submitted successfully"
}
```

**Errors:**
- `400 INVALID_REQUEST` - Missing or invalid fields

---

## API Key Management

### List API Keys

**GET** `/api/api-keys`

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
{
  "keys": [
    {
      "id": 1,
      "wallet_address": "0x742d35Cc...",
      "key_prefix": "vkn_abc1",
      "name": "My API Key",
      "created_at": "2024-01-01T00:00:00Z",
      "expires_at": "2025-01-01T00:00:00Z",
      "is_revoked": false,
      "last_used_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Note:** API keys are now associated with wallet address instead of user ID.

---

### Create API Key

**POST** `/api/api-keys`

**Headers:**
- `Authorization: Bearer {token}`
- `Content-Type: application/json`

**Request Body:**
```json
{
  "name": "Production Key",
  "expires_in": 365
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Descriptive name (1-100 chars) |
| expires_in | int | No | Days until expiration (0 = never) |

**Response:**
```json
{
  "message": "API key created successfully. Store this key securely - it will not be shown again.",
  "api_key": "vkn_abc123def456..."
}
```

**Warning:** The full API key is only shown once. Store it securely.

---

### Delete API Key

**DELETE** `/api/api-keys/:id`

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
{
  "message": "API key deleted successfully"
}
```

---

### Revoke API Key

**POST** `/api/api-keys/revoke/:id`

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
{
  "message": "API key revoked successfully"
}
```

---

### Renew API Key

**POST** `/api/api-keys/renew`

Renew an API key via Web3 signature.

**Headers:**
- `Authorization: Bearer {token}`
- `Content-Type: application/json`

**Request Body:**
```json
{
  "address": "0x742d35Cc...",
  "chain": "evm",
  "signature": "0x...",
  "message": "...",
  "key_id": 1
}
```

**Response:**
```json
{
  "message": "API key renewed successfully. Store this new key securely - it will not be shown again.",
  "api_key": "vkn_newkey123..."
}
```

---

## Error Response Format

All errors follow this format:

```json
{
  "error": "Human readable error message",
  "code": "ERROR_CODE",
  "details": "Additional details (optional)"
}
```

**Common Error Codes:**
- `INVALID_REQUEST` - Malformed request
- `INVALID_CHAIN` - Unsupported blockchain
- `INVALID_ADDRESS` - Invalid address format
- `UNAUTHORIZED` - Authentication required
- `FORBIDDEN` - Access denied
- `NOT_FOUND` - Resource not found
- `DB_ERROR` - Database error
- `RPC_ERROR` - Blockchain RPC error

---

## Rate Limiting

- **Unauthenticated:** 100 requests per 15 minutes per IP
- **Authenticated:** Based on user's balance (checks remaining)

### Rate Limit Response Headers

All API responses include rate limit headers:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed in current window |
| `X-RateLimit-Remaining` | Requests remaining in current window |
| `X-RateLimit-Used` | Requests used in current window |
| `X-RateLimit-Reset` | Unix timestamp when the rate limit resets |
| `X-RateLimit-Source` | Source of rate limiting: `ip` or `balance` |
| `X-Balance-Available` | Available balance for authenticated users (if using balance) |

**Example Response Headers:**
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Used: 5
X-RateLimit-Reset: 1751347200
X-RateLimit-Source: balance
X-Balance-Available: 50
```

---

## Testing

Run tests:
```bash
go test ./internal/handlers/...
```
