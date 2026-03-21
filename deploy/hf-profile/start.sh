#!/bin/sh

CLASH_SUB_URL="${CLASH_SUB_URL:-}"

start_without_proxy() {
    echo "Starting server without proxy..."
    sed -i 's|proxy-url: "socks5://127.0.0.1:10808"|proxy-url: ""|' /CLIProxyAPI/config.yaml 2>/dev/null
    exec ./server
}

if [ -z "$CLASH_SUB_URL" ]; then
    echo "CLASH_SUB_URL not set, skipping proxy setup"
    start_without_proxy
fi

echo "Downloading Clash subscription..."
wget -q -T 30 -O /tmp/clash-sub.yaml "$CLASH_SUB_URL"

if [ $? -ne 0 ] || [ ! -s /tmp/clash-sub.yaml ]; then
    echo "WARNING: failed to download subscription"
    start_without_proxy
fi

if ! grep -q '^proxies:' /tmp/clash-sub.yaml; then
    echo "WARNING: subscription does not contain proxies"
    start_without_proxy
fi

echo "Parsing subscription..."

awk '
/^proxies:/ { found=1 }
found && /^[a-zA-Z_-]+:/ && !/^proxies:/ { found=0 }
found { print }
' /tmp/clash-sub.yaml > /tmp/proxies-block.yaml

grep '^ *- {name:' /tmp/proxies-block.yaml | sed 's/.*{name: *//;s/,.*//' > /tmp/proxy-names.txt
PROXY_COUNT=$(wc -l < /tmp/proxy-names.txt | tr -d ' ')
echo "Found $PROXY_COUNT proxy nodes"

if [ "$PROXY_COUNT" -eq 0 ]; then
    echo "WARNING: no proxy nodes found"
    start_without_proxy
fi

PROXY_LIST_FILE=/tmp/proxy-list.yaml
> "$PROXY_LIST_FILE"
while IFS= read -r pname; do
    pname=$(echo "$pname" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    [ -z "$pname" ] && continue
    echo "      - \"$pname\"" >> "$PROXY_LIST_FILE"
done < /tmp/proxy-names.txt

{
cat << 'HEADER'
socks-port: 10808
port: 10809
allow-lan: false
mode: rule
log-level: warning
ipv6: false
tcp-concurrent: true
unified-delay: true

HEADER

cat /tmp/proxies-block.yaml

cat << 'MID'

proxy-groups:
  - name: auto
    type: url-test
    proxies:
MID

cat "$PROXY_LIST_FILE"

cat << 'TAIL'
    url: http://www.gstatic.com/generate_204
    interval: 300
    tolerance: 150

rules:
  - DOMAIN,portal.qwen.ai,DIRECT
  - DOMAIN-SUFFIX,qwen.ai,auto
  - MATCH,DIRECT
TAIL
} > /CLIProxyAPI/mihomo-config.yaml

echo "=== Generated mihomo config (first 30 lines) ==="
head -30 /CLIProxyAPI/mihomo-config.yaml
echo "=== Proxy group proxies (first 5) ==="
grep -A5 'type: url-test' /CLIProxyAPI/mihomo-config.yaml
echo "==="

echo "Starting mihomo proxy (auto-select best node)..."
mihomo -d /CLIProxyAPI -f /CLIProxyAPI/mihomo-config.yaml &
MIHOMO_PID=$!

sleep 3

if kill -0 "$MIHOMO_PID" 2>/dev/null; then
    echo "mihomo started (pid=$MIHOMO_PID)"
else
    echo "WARNING: mihomo failed to start, continuing without proxy"
fi

exec ./server
