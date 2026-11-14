# 采购公告采集与更正追溯平台 —— 多阶段构建
# 两个构建目标：app（主服务+前端）、sample（多来源样本站点）

# ---------- Go 构建 ----------
FROM golang:1.26-bookworm AS backend
ENV GOPROXY=https://goproxy.cn,direct \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN mkdir -p /out && \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    go build -trimpath -ldflags="-s -w" -o /out/sampleserver ./cmd/sampleserver

# ---------- 前端构建 ----------
FROM node:22-bookworm AS frontend
ENV npm_config_registry=https://registry.npmmirror.com
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 主服务镜像（Gin API + 内嵌迁移 + 托管前端产物） ----------
FROM debian:bookworm-slim AS app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/server /app/server
COPY --from=frontend /build/dist /app/frontend/dist
ENV HTTP_ADDR=:8080 \
    FRONTEND_DIST=/app/frontend/dist
EXPOSE 8080
CMD ["/app/server"]

# ---------- 样本站点镜像（页码/游标/原址修订三来源） ----------
FROM debian:bookworm-slim AS sample
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/sampleserver /app/sampleserver
EXPOSE 9090
CMD ["/app/sampleserver", "-addr", ":9090"]
