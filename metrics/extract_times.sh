#!/bin/bash

# Ensure a log file was passed as an argument
if [ -z "$1" ]; then
    echo "Usage: ./extract_times.sh <logfile.log>"
    exit 1
fi

FILE=$1

awk '
{
    # CRITICAL FIX: Strip invisible ANSI color codes (like \e[32m)
    # This prevents the script from reading color codes as "minutes"
    gsub(/\x1B\[[0-9;]*[a-zA-Z]/, "");
}

# Function to convert formats like "1m41s", "45s", or "1m51.947s" into pure seconds
function to_secs(t) {
    min=0; sec=0;
    if (t ~ /m/) {
        split(t, a, "m");
        min = a[1];
        sec_str = a[2];
    } else {
        sec_str = t;
    }
    gsub(/s/, "", sec_str);
    return min * 60 + sec_str;
}

BEGIN {
    master = 0; worker_max = 0; workloads = 0; daemon_max = 0; total_real = 0;
    mode = "UNKNOWN"
}

# Auto-detect if this is an apply log or destroy log
/Creation complete after/ { mode = "APPLY"; keyword = "Creation complete after" }
/Destruction complete after/ { mode = "DESTROY"; keyword = "Destruction complete after" }

# Extract Terraform resource timings
(mode != "UNKNOWN") && $0 ~ keyword {
    split($0, parts, keyword " ");
    split(parts[2], time_parts, " ");
    t_val = to_secs(time_parts[1]);

    if ($0 ~ /k3s_master/) { master = t_val; }
    else if ($0 ~ /k3s_workers/) { if (t_val > worker_max) worker_max = t_val; }
    else if ($0 ~ /deploy_k3s_stack/) { workloads = t_val; }
    else if ($0 ~ /fog_systemd_service\.daemon/) { if (t_val > daemon_max) daemon_max = t_val; }
}

# Extract Bash "time" wrapper output
/^real[ \t]+/ {
    total_real = to_secs($2);
}

END {
    # Calculate totals and initialisation overhead
    crit_path = master + worker_max + workloads + daemon_max;
    init_time = total_real - crit_path;
    
    # Safety catch in case the "time" wrapper output was missing
    if (init_time < 0) init_time = 0;

    printf "\n=== %s TIMING REPORT ===\n", mode
    printf "%-45s %8.3f s\n", "1. Graph & Provider Initialisation:", init_time
    printf "%-45s %8.3f s\n", "2. Infrastructure: Master (Sequential):", master
    printf "%-45s %8.3f s\n", "3. Infrastructure: Workers (Parallel Max):", worker_max
    printf "%-45s %8.3f s\n", "4. K3s Workloads (Sequential):", workloads
    printf "%-45s %8.3f s\n", "5. System Daemons (Parallel Max):", daemon_max
    printf "--------------------------------------------------------\n"
    printf "%-45s %8.3f s\n", "Sum of Critical Path:", crit_path
    printf "========================================================\n"
    printf "%-45s %8.3f s\n\n", "TOTAL DEPLOYMENT TIME (Real):", total_real
}
' "$FILE"
