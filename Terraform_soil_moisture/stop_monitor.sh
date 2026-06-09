# Stop local monitor on the workstation
pkill -f monitor.sh

# Stop remote monitors on the Raspberry Pis
ssh pi1@192.168.0.204 "pkill -f monitor.sh"
ssh pi2@192.168.0.237 "pkill -f monitor.sh"
ssh pi3@192.168.0.76 "pkill -f monitor.sh"

echo "All monitors successfully stopped!"
