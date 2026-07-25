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

---

## Testing

Run tests:
```bash
go test ./internal/handlers/...
```
