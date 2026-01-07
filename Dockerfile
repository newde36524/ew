
# 使用 BUILDPLATFORM 确保构建阶段使用本机架构，避免模拟性能损耗
FROM --platform=$BUILDPLATFORM golang:1.25.5 AS builder

# 接收 Docker 注入的目标平台参数
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# 复制源码
COPY . .

# 交叉编译：根据目标平台编译二进制文件
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o app .

# 安装 UPX 并压缩二进制文件
RUN apt-get update && \
    apt-get install -y upx && \
    upx --best app && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 证书阶段：从 Alpine 提取证书
FROM alpine:latest AS certs
RUN apk --no-cache add ca-certificates

# 使用固定版本（避免 latest 的不确定性）
FROM scratch

# 复制证书
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 设置工作目录
WORKDIR /app

# 从编译阶段复制二进制文件
COPY --from=builder /app/app /app/app

ENTRYPOINT ["./app"]