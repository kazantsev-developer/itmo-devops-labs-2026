#!/bin/bash
NAMESPACE="shop"
INTERVAL="${INTERVAL:-4}"

trap 'echo "Империус снят"; exit 0' INT

while true; do
  PODS=$(kubectl get pods -n "$NAMESPACE" \
    -l 'app in (api-server,worker)' \
    -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)

  if [ -z "$PODS" ]; then
    sleep "$INTERVAL"
    continue
  fi

  POD=$(echo "$PODS" | tr ' ' '\n' | awk 'BEGIN{srand()} {a[NR]=$0} END{print a[int(rand()*NR)+1]}')
  echo "[$(date +%H:%M:%S)] Imperio: $POD"
  kubectl delete pod -n "$NAMESPACE" "$POD" --grace-period=0 --force 2>/dev/null

  sleep "$INTERVAL"
done