#!/bin/sh

set -eu

CLASH_SUB_URL="${CLASH_SUB_URL:-}"
MIHOMO_WORKDIR="/CLIProxyAPI"
MIHOMO_CONFIG="$MIHOMO_WORKDIR/mihomo-config.yaml"
CLASH_SUB_FILE="/tmp/clash-sub.yaml"
PROXIES_BLOCK_FILE="/tmp/proxies-block.yaml"
PROXY_NAMES_FILE="/tmp/proxy-names.txt"
QWEN_PROXY_URL="socks5://127.0.0.1:10808"
QWEN_PROXY_PROBE_URL="https://chat.qwen.ai/api/v1/oauth2/device/code"
QWEN_PROXY_PROBE_BODY="client_id=f0304373b74a44d2b584a3fb70ca9e56&scope=openid%20profile%20email%20model.completion&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256"
QWEN_USER_AGENT="QwenCode/0.12.0 (linux; x64)"
SUBSCRIPTION_USER_AGENT="Clash.Meta"
QWEN_SELECTED_PROXY=""
MIHOMO_PID=""

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

extract_inline_proxy_name() {
    line="$1"
    value=${line#*name:}
    value=$(printf '%s' "$value" | sed 's/^[[:space:]]*//')

    case "$value" in
        \"*)
            value=${value#\"}
            value=${value%%\"*}
            ;;
        \'*)
            value=${value#\'}
            value=${value%%\'*}
            ;;
        *)
            value=${value%%,*}
            value=$(printf '%s' "$value" | sed 's/[[:space:]]*$//')
            ;;
    esac

    printf '%s\n' "$value"
}

extract_proxy_names() {
    : > "$PROXY_NAMES_FILE"

    while IFS= read -r line; do
        extract_inline_proxy_name "$line" >> "$PROXY_NAMES_FILE"
    done <<EOF
$(grep '^ *- {name:' "$PROXIES_BLOCK_FILE" || true)
EOF

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

yaml_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

write_mihomo_config() {
    selected_proxy_name=$(yaml_escape "$1")

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

        cat <<MID

proxy-groups:
  - name: qwen
    type: select
    proxies:
      - "$selected_proxy_name"

rules:
  - DOMAIN,portal.qwen.ai,DIRECT
  - DOMAIN-SUFFIX,qwen.ai,qwen
  - MATCH,DIRECT
MID
    } > "$MIHOMO_CONFIG"

    echo "=== Generated mihomo config (first 30 lines) ==="
    sed -n '1,30p' "$MIHOMO_CONFIG"
    echo "=== Qwen proxy group ==="
    grep -A5 'type: select' "$MIHOMO_CONFIG" || true
    echo "==="
}

validate_mihomo_config() {
    echo "Validating mihomo config..."
    if ! mihomo -t -f "$MIHOMO_CONFIG" >/tmp/mihomo-validate.log 2>&1; then
        cat /tmp/mihomo-validate.log >&2 || true
        fail_proxy_bootstrap "mihomo config validation failed"
    fi
}

stop_mihomo() {
    if [ -n "$MIHOMO_PID" ] && kill -0 "$MIHOMO_PID" 2>/dev/null; then
        kill "$MIHOMO_PID" 2>/dev/null || true
        wait "$MIHOMO_PID" 2>/dev/null || true
    fi
    MIHOMO_PID=""
}

start_mihomo() {
    echo "Starting mihomo proxy..."
    mihomo -d "$MIHOMO_WORKDIR" -f "$MIHOMO_CONFIG" >/tmp/mihomo.log 2>&1 &
    MIHOMO_PID=$!

    sleep 3

    if ! kill -0 "$MIHOMO_PID" 2>/dev/null; then
        cat /tmp/mihomo.log >&2 || true
        MIHOMO_PID=""
        fail_proxy_bootstrap "mihomo failed to start"
    fi

    echo "mihomo started (pid=$MIHOMO_PID)"
}

probe_qwen_proxy() {
    HTTP_CODE="$({
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
            --data "$QWEN_PROXY_PROBE_BODY"
    } || printf '000')"

    case "$HTTP_CODE" in
        200)
            echo "Qwen OAuth probe succeeded"
            return 0
            ;;
        400|401|403)
            echo "Qwen OAuth probe reached upstream (HTTP $HTTP_CODE)"
            return 0
            ;;
        *)
            cat /tmp/qwen-proxy-check.out >&2 || true
            echo "Qwen OAuth probe failed through mihomo (HTTP $HTTP_CODE)" >&2
            return 1
            ;;
    esac
}

probe_qwen_proxy_for_candidate() {
    candidate_name="$1"

    echo "Probing Qwen OAuth with candidate proxy: $candidate_name"
    write_mihomo_config "$candidate_name"
    validate_mihomo_config
    start_mihomo

    if probe_qwen_proxy; then
        QWEN_SELECTED_PROXY="$candidate_name"
        return 0
    fi

    stop_mihomo
    return 1
}

select_working_qwen_proxy() {
    while IFS= read -r candidate_name; do
        candidate_name=$(echo "$candidate_name" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        [ -z "$candidate_name" ] && continue

        if probe_qwen_proxy_for_candidate "$candidate_name"; then
            echo "Locked Qwen OAuth traffic to proxy node: $candidate_name"
            return 0
        fi
    done < "$PROXY_NAMES_FILE"

    fail_proxy_bootstrap "no working proxy node could reach Qwen OAuth"
}

if [ -z "$CLASH_SUB_URL" ]; then
    fail_proxy_bootstrap "CLASH_SUB_URL is required for HF Qwen OAuth deployment"
fi

require_proxy_url_configured
download_subscription
extract_proxies_block
extract_proxy_names
select_working_qwen_proxy

echo "Qwen OAuth proxy selection complete: $QWEN_SELECTED_PROXY"
exec ./server
