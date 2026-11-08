# 多阶段构建轻量化容器镜像
FROM golang:1.27-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/nga main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /root
COPY --from=builder /app/nga /usr/local/bin/nga

# 持久化配置与 SQLite 缓存目录
VOLUME ["/root/.config/nga-cli"]

ENTRYPOINT ["nga"]
CMD ["doctor"]
