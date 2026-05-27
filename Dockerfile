# syntax=docker/dockerfile:1

FROM node:22-alpine AS web-builder
WORKDIR /src/web
# 固定 pnpm 版本，避免 Docker 构建时被 Corepack 自动切到新版本并触发新的安装策略。
RUN corepack enable && corepack prepare pnpm@10.0.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web ./
RUN pnpm run build

FROM golang:1.26-alpine AS server-builder
WORKDIR /src/server
RUN apk add --no-cache ca-certificates
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mx-mail-api .

FROM alpine:3.22
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=server-builder /out/mx-mail-api /app/mx-mail-api
COPY --from=web-builder /src/web/dist /app/web/dist
COPY API.md /app/API.md
COPY docker/config.yaml /app/config.yaml
ENV CONFIG_PATH=/app/config.yaml
EXPOSE 8080 2525
CMD ["/app/mx-mail-api"]
