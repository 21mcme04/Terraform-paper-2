#!/bin/bash
# Do not set -e here because we want to force clean even if something fails

echo ">>> [Cleanup] Starting Uninstallation..."

if [ -f /usr/local/bin/k3s-uninstall.sh ]; then
    echo "    Running Master Uninstaller..."
    sudo /usr/local/bin/k3s-uninstall.sh >/dev/null 2>&1
elif [ -f /usr/local/bin/k3s-agent-uninstall.sh ]; then
    echo "    Running Agent Uninstaller..."
    sudo /usr/local/bin/k3s-agent-uninstall.sh >/dev/null 2>&1
else
    echo "    No uninstaller found. Attempting manual cleanup..."
fi

# Force cleanup of Rancher directories
echo "    Removing data directories..."
sudo rm -rf /var/lib/rancher/k3s
sudo rm -rf /etc/rancher/k3s
sudo rm -rf /var/lib/cni
sudo rm -rf /var/log/pods
sudo rm -rf /var/log/containers

echo ">>> [Cleanup] Node Cleaned."
