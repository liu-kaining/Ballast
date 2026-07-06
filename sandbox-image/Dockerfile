# syntax=docker/dockerfile:1
# Ballast 沙箱基础镜像：
#   - harness-agent（PTY 劫持 + 指令拦截网关）
#   - mock-opencode（v0.1 占位，模拟 opencode serve 的最小行为）
#   - 基础 SRE CLI（kubectl/git/terraform 占位）
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY harness-agent/go.mod harness-agent/go.sum ./
RUN go mod download
COPY harness-agent/ ./
RUN CGO_ENABLED=0 go build -o /harness-agent ./cmd/harness-agent

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git bash socat jq python3 \
    && ln -sf /bin/bash /bin/sh

# 基础 SRE 工具箱占位（v0.2 替换为真实二进制或通过 MCP 注入）
RUN printf '#!/bin/sh\necho "[mock-kubectl] $*" >&2\nexit 0\n' > /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl \
    && printf '#!/bin/sh\necho "[mock-terraform] $*" >&2\nexit 0\n' > /usr/local/bin/terraform && chmod +x /usr/local/bin/terraform \
    && printf '#!/bin/sh\necho "[mock-helm] $*" >&2\nexit 0\n' > /usr/local/bin/helm && chmod +x /usr/local/bin/helm

# harness-agent
COPY --from=build /harness-agent /usr/local/bin/harness-agent

# mock-opencode：模拟 opencode serve --format json 的最小 HTTP server
COPY sandbox-image/mock-opencode /usr/local/bin/mock-opencode
RUN chmod +x /usr/local/bin/mock-opencode

# Skill 挂载点（运行时由控制面动态注入到 /workspace/.opencode/skills/）
RUN mkdir -p /workspace/.opencode/skills
COPY sandbox-image/.opencode/skills/ /workspace/.opencode/skills/

WORKDIR /workspace
ENV BALAST_SANDBOX=1
# harness-agent 启动后以 mock-opencode 为受控子进程
ENTRYPOINT ["/usr/local/bin/harness-agent"]
