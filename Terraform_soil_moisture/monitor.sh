#!/bin/bash

echo "Timestamp,CPU_Percent,RAM_Active_MB,Temp_C" >> workstation_metrics.csv

while true; do
  TS=$(date +%s)
  
  # Calculate CPU% using top (batch mode, 1 iteration)
  CPU=$(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print 100 - $1}')
  
  # Get active RAM (used - buffers/cache) in MB
  RAM=$(free -m | awk '/Mem:/ {print $3}')
  
  # Get Temperature
  TEMP=$(ssh saathvikk@192.168.0.5 'sensors | awk "/Package id 0:/ {gsub(/[+°C]/,\"\",\$4); print \$4}"')
  
  echo "$TS,$CPU,$RAM,$TEMP" >> workstation_metrics.csv
  sleep 1
done
