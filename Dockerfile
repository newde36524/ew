
# 使用 BUILDPLATFORM 确保构建阶段使用本机架构，避免模拟性能损耗
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

# 接收 Docker 注入的目标平台参数
ARG TARGETOS
ARG TARGETARCH

# 安装构建依赖和 UPX
RUN apk add --no-cache git upx

WORKDIR /app

# 复制源码
COPY . .

# 交叉编译：根据目标平台编译二进制文件
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOPROXY=https://goproxy.cn,direct go build -ldflags="-s -w" -o app .

# 使用 UPX 压缩二进制文件
RUN upx --best app

# 使用固定版本（避免 latest 的不确定性）
FROM alpine:3.19

# 安装 CA 证书并更新证书库
RUN apk add --no-cache ca-certificates && update-ca-certificates

# 设置工作目录
WORKDIR /app

# 从编译阶段复制二进制文件
COPY --from=builder /app/app /app/app
COPY ./chn_ip_v6.txt /app/chn_ip_v6.txt
COPY ./chn_ip.txt /app/chn_ip.txt

ENTRYPOINT ["./app"]