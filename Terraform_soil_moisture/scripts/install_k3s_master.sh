#!/bin/bash
set -e

echo ">>> [K3s Master] Checking existing installation..."
if command -v k3s >/dev/null 2>&1; then
    echo "    K3s is already installed. Skipping."
    exit 0
fi

echo ">>> [K3s Master] Installing Control Plane..."
# The K3s installer handles sudo internally, but we pipe to sh explicitly
curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644

echo ">>> [K3s Master] Enabling Service..."
sudo systemctl enable k3s

echo ">>> [K3s Master] Waiting for Node Token..."
TIMEOUT=120
# FIX: Added 'sudo test -f' because 'pi' user cannot read /var/lib/rancher
while ! sudo test -f /var/lib/rancher/k3s/server/node-token; do
    sleep 2
    TIMEOUT=$((TIMEOUT-2))
    if [ $TIMEOUT -le 0 ]; then
        echo "ERROR: Timed out waiting for K3s token."
        exit 1
    fi
done

echo ">>> [K3s Master] Ready."
