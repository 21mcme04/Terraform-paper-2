#!/bin/bash
set -e

# Validate required variables
if [ -z "$MASTER_IP" ] || [ -z "$K3S_TOKEN" ]; then
    echo "ERROR: MASTER_IP or K3S_TOKEN is missing."
    echo "       Master IP: $MASTER_IP"
    echo "       Token Length: ${#K3S_TOKEN}"
    exit 1
fi

echo ">>> [K3s Worker] Checking existing installation..."
if command -v k3s-agent >/dev/null 2>&1; then
    echo "    K3s Agent is already installed. Skipping."
    exit 0
fi

echo ">>> [K3s Worker] Joining Cluster at $MASTER_IP..."
# Determine label
LABEL_ARG=""
if [ "$NODE_TYPE" == "database" ]; then
    LABEL_ARG="--node-label type=database"
else
    LABEL_ARG="--node-label worker=true"
fi

# Run Installer
curl -sfL https://get.k3s.io | K3S_URL=https://${MASTER_IP}:6443 K3S_TOKEN=${K3S_TOKEN} sh -s - $LABEL_ARG

echo ">>> [K3s Worker] Waiting for Agent to start..."
TIMEOUT=60
# FIX: Added 'sudo' to systemctl to avoid permission errors
while ! sudo systemctl is-active k3s-agent >/dev/null 2>&1; do
    sleep 2
    TIMEOUT=$((TIMEOUT-2))
    if [ $TIMEOUT -le 0 ]; then
        echo "ERROR: Timed out waiting for K3s Agent."
        exit 1
    fi
done

echo ">>> [K3s Worker] Joined Successfully."
