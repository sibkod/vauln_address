#!/bin/bash

# Test script for Vauln Address API
# Tests rate limit headers and /api/me endpoint

API_BASE="${API_BASE:-http://localhost:8080}"
ADMIN_KEY="${ADMIN_API_KEY:-8295de2cf00f5ac7b37154c1d93f9ee99e09301e}"

echo "========================================"
echo "Testing Vauln Address API"
echo "========================================"
echo ""

# Test 1: Health check
echo "Test 1: Health Check"
curl -s -I "$API_BASE/api/health" | head -5
echo ""

# Test 2: /api/me endpoint (anonymous)
echo "Test 2: /api/me endpoint (anonymous)"
response=$(curl -s -D - "$API_BASE/api/me" -o /dev/null)
echo "$response" | grep -E "^X-RateLimit|^HTTP"
echo ""

# Test 3: /api/me full response
echo "Test 3: /api/me full response"
curl -s "$API_BASE/api/me" | jq .
echo ""

# Test 4: /api/check endpoint with rate limit headers
echo "Test 4: /api/check with rate limit headers"
response=$(curl -s -D - -X POST "$API_BASE/api/check" \
  -H "Content-Type: application/json" \
  -d '{"address":"0x742d35Cc6634C0532925a3b844Bc9e7595f8bE21","chain":"evm"}' \
  -o /dev/null)
echo "$response" | grep -E "^X-RateLimit|^HTTP"
echo ""

# Test 5: Pricing endpoint
echo "Test 5: Pricing endpoint"
curl -s "$API_BASE/api/pricing?checks=50" | jq .
echo ""

# Test 6: Chains endpoint
echo "Test 6: Supported chains"
curl -s "$API_BASE/api/chains" | jq .
echo ""

# Test 7: Admin - Add wallet with seed phrase
echo "Test 7: Admin - Add wallet WITH seed phrase"
response=$(curl -s -X POST "$API_BASE/api/admin/wallets" \
  -H "X-Admin-Key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "seed_phrase": "promote injury citizen enroll shoe dose meat easy tribe spawn cute melody",
    "addresses": {
      "evm": "0xd02f322c99c70207971c23ad01b30c58fa2a4ed1"
    },
    "status": "hacked",
    "reason": "leaked seed phrase",
    "source": "github"
  }')
echo "$response" | jq .
echo ""

# Test 8: Admin - Add wallet WITHOUT seed phrase
echo "Test 8: Admin - Add wallet WITHOUT seed phrase"
response=$(curl -s -X POST "$API_BASE/api/admin/wallets" \
  -H "X-Admin-Key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "addresses": {
      "evm": "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
      "solana": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"
    },
    "status": "vulnerable",
    "reason": "weak private key pattern",
    "source": "manual"
  }')
echo "$response" | jq .
echo ""

echo "========================================"
echo "Tests completed!"
echo "========================================"
