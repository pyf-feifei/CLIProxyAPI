FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./server ./cmd/server/

FROM alpine:3.22.0

RUN apk add --no-cache tzdata gzip

# Download mihomo (Clash Meta) for linux amd64 and use it as the only HF Qwen OAuth proxy path.
RUN set -ex && \
    MIHOMO_DL_URL=$(wget -qO- "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest" | \
      grep '"browser_download_url"' | grep 'mihomo-linux-amd64' | grep '\.gz"' | grep -v compatible | head -1 | \
      sed 's/.*"browser_download_url": *"//;s/".*//') && \
    echo "Downloading mihomo from: $MIHOMO_DL_URL" && \
    wget -O /tmp/mihomo.gz "$MIHOMO_DL_URL" && \
    gunzip /tmp/mihomo.gz && \
    chmod +x /tmp/mihomo && \
    mv /tmp/mihomo /usr/local/bin/mihomo && \
    rm -f /tmp/mihomo.gz

RUN mkdir /CLIProxyAPI

COPY --from=builder ./app/server /CLIProxyAPI/server

COPY config.example.yaml /CLIProxyAPI/config.example.yaml

# HF deploys use config.yaml as a single proxy source of truth for Qwen OAuth.
RUN cp /CLIProxyAPI/config.example.yaml /CLIProxyAPI/config.yaml
RUN sed -i 's|proxy-url: ""|proxy-url: "socks5://127.0.0.1:10808"|' /CLIProxyAPI/config.yaml

COPY start.sh /CLIProxyAPI/start.sh
RUN chmod +x /CLIProxyAPI/start.sh

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai
ENV DEPLOY=cloud
# Set CLASH_SUB_URL to a Clash subscription URL to enable the HF-only Qwen proxy bootstrap.
# Leave it empty to disable the proxy and restore direct traffic in config.yaml before startup.

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./start.sh"]
