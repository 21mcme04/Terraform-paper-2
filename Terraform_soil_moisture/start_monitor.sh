# Start local monitor on the workstation
nohup ./monitor.sh > /dev/null 2>&1 &

# Start remote monitors on the Raspberry Pis
ssh pi1@192.168.0.204 "nohup ./monitor.sh > /dev/null 2>&1 &"
ssh pi2@192.168.0.237 "nohup ./monitor.sh > /dev/null 2>&1 &"
ssh pi3@192.168.0.76 "nohup ./monitor.sh > /dev/null 2>&1 &"

echo "All monitors (workstation + 3 Pis) successfully started in the background!"
