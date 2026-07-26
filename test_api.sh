#!/bin/bash

# Test script for Vauln Address API
# Tests rate limit headers and /api/me endpoint

API_BASE="${API_BASE:-http://localhost:8080}"

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

echo "========================================"
echo "Tests completed!"
echo "========================================"
