#!/bin/bash
if [ "$1" = "events" ]; then
    while true; do
        if [ -f /tmp/docker_event_trigger ]; then
            cat /tmp/docker_event_trigger
            rm /tmp/docker_event_trigger
        fi
        sleep 0.1
    done
elif [ "$1" = "inspect" ]; then
    echo '{"80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}]}'
fi
