#!/bin/bash

NAMESPACE="shop"
PG_HOST="shop-postgres-rw"
PG_PORT="5432"

probe() {
  local label=$1
  local target=$2
  local port=$3
  local expected=$4
  local name="probe-$(date +%s%N)"

  local out
  out=$(kubectl run "$name" \
    --image=busybox:1.36 \
    --namespace="$NAMESPACE" \
    --labels="app=$label" \
    --restart=Never \
    --rm -i \
    --command -- sh -c "nc -zvw3 $target $port" 2>&1) || true

  local actual="DENY"
  echo "$out" | grep -qE "open|succeeded" && actual="ALLOW"

  local status="PASS"
  [ "$actual" != "$expected" ] && status="FAIL"

  printf "%-12s -> %-22s expected=%-5s actual=%-5s %s\n" \
    "$label" "$target:$port" "$expected" "$actual" "$status"
}

probe "api-server" "$PG_HOST" "$PG_PORT" "ALLOW"
probe "worker"     "$PG_HOST" "$PG_PORT" "ALLOW"
probe "stranger"   "$PG_HOST" "$PG_PORT" "DENY"