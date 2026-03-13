#!/bin/sh
echo "Starting xray-core proxy..."
xray run -config /CLIProxyAPI/xray-config.json &
XRAY_PID=$!

sleep 2

if kill -0 "$XRAY_PID" 2>/dev/null; then
    echo "xray-core started (pid=$XRAY_PID)"
else
    echo "WARNING: xray-core failed to start, continuing without proxy"
fi

exec ./server
