#!/bin/bash
if [ "$1" = "events" ]; then
    while true; do
        if [ -f /tmp/podman_event_trigger ]; then
            cat /tmp/podman_event_trigger
            rm /tmp/podman_event_trigger
        fi
        sleep 0.1
    done
elif [ "$1" = "inspect" ]; then
    echo '[{"HostPort": 9090, "ContainerPort": 80, "Protocol": "tcp"}]'
fi
