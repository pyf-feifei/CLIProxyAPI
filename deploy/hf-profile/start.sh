#!/bin/sh

set -eu

CLASH_SUB_URL="${CLASH_SUB_URL:-}"
MIHOMO_WORKDIR="/CLIProxyAPI"
MIHOMO_CONFIG="$MIHOMO_WORKDIR/mihomo-config.yaml"
CLASH_SUB_FILE="/tmp/clash-sub.yaml"
PROXIES_BLOCK_FILE="/tmp/proxies-block.yaml"
PROXY_NAMES_FILE="/tmp/proxy-names.txt"
PROXY_LIST_FILE="/tmp/proxy-list.yaml"
QWEN_PROXY_URL="socks5://127.0.0.1:10808"
QWEN_PROXY_PROBE_URL="https://chat.qwen.ai/api/v1/oauth2/device/code"
QWEN_PROXY_PROBE_BODY="client_id=f0304373b74a44d2b584a3fb70ca9e56&scope=openid%20profile%20email%20model.completion&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256"
QWEN_USER_AGENT="QwenCode/0.12.0 (linux; x64)"
SUBSCRIPTION_USER_AGENT="Clash.Meta"

fail_proxy_bootstrap() {
    echo "FATAL: $1" >&2
    exit 1
}

require_proxy_url_configured() {
    if ! grep -q 'proxy-url: "socks5://127.0.0.1:10808"' /CLIProxyAPI/config.yaml; then
        fail_proxy_bootstrap "config.yaml no longer points Qwen traffic at $QWEN_PROXY_URL"
    fi
}

download_subscription() {
    echo "Downloading Clash subscription..."
    if ! curl --fail --silent --show-error --location --connect-timeout 30 --max-time 60 \
        --user-agent "$SUBSCRIPTION_USER_AGENT" \
        -H 'Accept: application/x-yaml,text/yaml,application/octet-stream,*/*' \
        -o "$CLASH_SUB_FILE" \
        "$CLASH_SUB_URL"; then
        fail_proxy_bootstrap "failed to download subscription"
    fi
    if [ ! -s "$CLASH_SUB_FILE" ]; then
        fail_proxy_bootstrap "downloaded subscription is empty"
    fi

    tr -d '\r' < "$CLASH_SUB_FILE" > "${CLASH_SUB_FILE}.normalized"
    mv "${CLASH_SUB_FILE}.normalized" "$CLASH_SUB_FILE"
}

extract_proxies_block() {
    if ! grep -q '^[[:space:]]*proxies:[[:space:]]*$' "$CLASH_SUB_FILE"; then
        fail_proxy_bootstrap "subscription does not contain a proxies section"
    fi

    awk '
/^[[:space:]]*proxies:[[:space:]]*$/ { found=1 }
found && /^[^[:space:]-][^:]*:[[:space:]]*$/ && !/^[[:space:]]*proxies:[[:space:]]*$/ { exit }
found { print }
' "$CLASH_SUB_FILE" > "$PROXIES_BLOCK_FILE"

    if [ ! -s "$PROXIES_BLOCK_FILE" ]; then
        fail_proxy_bootstrap "failed to extract proxies block from subscription"
    fi
}

extract_proxy_names() {
    : > "$PROXY_NAMES_FILE"

    grep '^ *- {name:' "$PROXIES_BLOCK_FILE" | sed 's/.*{name: *//;s/,.*//' >> "$PROXY_NAMES_FILE" || true
    grep '^ *- name:' "$PROXIES_BLOCK_FILE" | sed 's/^[[:space:]]*-[[:space:]]*name:[[:space:]]*//' >> "$PROXY_NAMES_FILE" || true

    if [ -s "$PROXY_NAMES_FILE" ]; then
        sed -i 's/[[:space:]]*#.*$//' "$PROXY_NAMES_FILE"
        sed -i 's/^"//;s/"$//' "$PROXY_NAMES_FILE"
        sed -i "s/^'//;s/'$//" "$PROXY_NAMES_FILE"
        awk 'NF && !seen[$0]++ { print }' "$PROXY_NAMES_FILE" > "${PROXY_NAMES_FILE}.dedup"
        mv "${PROXY_NAMES_FILE}.dedup" "$PROXY_NAMES_FILE"
    fi

    PROXY_COUNT=$(wc -l < "$PROXY_NAMES_FILE" | tr -d ' ')
    echo "Found $PROXY_COUNT proxy nodes"
    if [ "${PROXY_COUNT:-0}" -eq 0 ]; then
        fail_proxy_bootstrap "no proxy nodes found in subscription"
    fi
}

build_proxy_group_list() {
    : > "$PROXY_LIST_FILE"
    while IFS= read -r pname; do
        pname=$(echo "$pname" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        [ -z "$pname" ] && continue
        echo "      - \"$pname\"" >> "$PROXY_LIST_FILE"
    done < "$PROXY_NAMES_FILE"

    if [ ! -s "$PROXY_LIST_FILE" ]; then
        fail_proxy_bootstrap "proxy group list is empty after parsing subscription"
    fi
}

write_mihomo_config() {
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

        cat "$PROXIES_BLOCK_FILE"

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
    } > "$MIHOMO_CONFIG"

    echo "=== Generated mihomo config (first 30 lines) ==="
    sed -n '1,30p' "$MIHOMO_CONFIG"
    echo "=== Proxy group proxies (first 5) ==="
    grep -A5 'type: url-test' "$MIHOMO_CONFIG" || true
    echo "==="
}

validate_mihomo_config() {
    echo "Validating mihomo config..."
    if ! mihomo -t -f "$MIHOMO_CONFIG" >/tmp/mihomo-validate.log 2>&1; then
        cat /tmp/mihomo-validate.log >&2 || true
        fail_proxy_bootstrap "mihomo config validation failed"
    fi
}

start_mihomo() {
    echo "Starting mihomo proxy (auto-select best node)..."
    mihomo -d "$MIHOMO_WORKDIR" -f "$MIHOMO_CONFIG" >/tmp/mihomo.log 2>&1 &
    MIHOMO_PID=$!

    sleep 3

    if ! kill -0 "$MIHOMO_PID" 2>/dev/null; then
        cat /tmp/mihomo.log >&2 || true
        fail_proxy_bootstrap "mihomo failed to start"
    fi

    echo "mihomo started (pid=$MIHOMO_PID)"
}

probe_qwen_proxy() {
    echo "Probing Qwen OAuth endpoint through mihomo..."
    HTTP_CODE="$(
        curl --silent --show-error --output /tmp/qwen-proxy-check.out --write-out '%{http_code}' --max-time 20 \
            --socks5-hostname 127.0.0.1:10808 \
            -X POST "$QWEN_PROXY_PROBE_URL" \
            -H 'Content-Type: application/x-www-form-urlencoded' \
            -H 'Accept: application/json' \
            -H 'Accept-Language: zh-CN,zh;q=0.9,en;q=0.8' \
            -H 'Origin: vscode-file://vscode-app' \
            -H 'Referer: https://chat.qwen.ai/' \
            -H "User-Agent: $QWEN_USER_AGENT" \
            -H "X-Dashscope-Useragent: $QWEN_USER_AGENT" \
            -H 'x-request-id: hf-proxy-probe' \
            --data "$QWEN_PROXY_PROBE_BODY" || printf '000'
    )"

    case "$HTTP_CODE" in
        200)
            echo "Qwen OAuth probe succeeded"
            ;;
        400|401|403)
            echo "Qwen OAuth probe reached upstream (HTTP $HTTP_CODE)"
            ;;
        *)
            cat /tmp/qwen-proxy-check.out >&2 || true
            kill "$MIHOMO_PID" 2>/dev/null || true
            fail_proxy_bootstrap "Qwen OAuth probe failed through mihomo (HTTP $HTTP_CODE)"
            ;;
    esac
}

if [ -z "$CLASH_SUB_URL" ]; then
    fail_proxy_bootstrap "CLASH_SUB_URL is required for HF Qwen OAuth deployment"
fi

require_proxy_url_configured
download_subscription
extract_proxies_block
extract_proxy_names
build_proxy_group_list
write_mihomo_config
validate_mihomo_config
start_mihomo
probe_qwen_proxy

exec ./server
