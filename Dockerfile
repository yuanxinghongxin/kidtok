# syntax=docker/dockerfile:1

# ---- 构建阶段：静态编译（纯 Go，无 cgo，前端已 embed 进二进制） ----
FROM golang:1.22-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/qimeng-feed ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 app \
    && mkdir -p /data && chown app:app /data

WORKDIR /app
COPY --from=build /out/qimeng-feed /usr/local/bin/qimeng-feed

# 配置优先走环境变量（QIMENG_ALIST_URL / QIMENG_ALIST_USERNAME / QIMENG_ALIST_PASSWORD ...）
# 网盘配置落盘到 /data/config.json，挂载该目录即可持久化
ENV QIMENG_SERVER_ADDR=:8080 \
    QIMENG_CONFIG_JSON=/data/config.json \
    TZ=Asia/Shanghai

USER app
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/qimeng-feed"]
