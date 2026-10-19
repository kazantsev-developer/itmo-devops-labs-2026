#!/bin/bash

DURATION="${DURATION:-5m}"
TARGET="${TARGET:-http://api.shop:8080/order}"

echo "Load: $TARGET, duration: $DURATION"

kubectl run hey-client-$(date +%s) \
  --image=williamyeh/hey:latest \
  --namespace=shop \
  --restart=Never \
  --rm \
  -it \
  -- \
  -z "$DURATION" \
  -q 10 \
  -c 5 \
  -m POST \
  -H "Content-Type: application/json" \
  -d '{"description":"chaos-load"}' \
  "$TARGET"