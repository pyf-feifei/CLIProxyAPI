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

RUN apk add --no-cache tzdata unzip

# Download xray-core for linux amd64
RUN wget -O /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip" \
    && unzip /tmp/xray.zip -d /usr/local/bin xray \
    && chmod +x /usr/local/bin/xray \
    && rm /tmp/xray.zip

RUN mkdir /CLIProxyAPI

COPY --from=builder ./app/server /CLIProxyAPI/server

COPY config.example.yaml /CLIProxyAPI/config.example.yaml

# Use example config as default so the app can start (e.g. Hugging Face Spaces)
RUN cp /CLIProxyAPI/config.example.yaml /CLIProxyAPI/config.yaml
# Route traffic through local xray SOCKS5 proxy
RUN sed -i 's|proxy-url: ""|proxy-url: "socks5://127.0.0.1:10808"|' /CLIProxyAPI/config.yaml

COPY xray-config.json /CLIProxyAPI/xray-config.json
COPY start.sh /CLIProxyAPI/start.sh
RUN chmod +x /CLIProxyAPI/start.sh

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai
# Cloud deploy mode: allow startup without config, stand by for configuration
ENV DEPLOY=cloud

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./start.sh"]
