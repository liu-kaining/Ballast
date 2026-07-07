# syntax=docker/dockerfile:1
# Ballast 沙箱基础镜像：
#   - harness-agent（PTY 劫持 + 指令拦截网关）
#   - mock-opencode（稳定回归测试路径）
#   - ballast-real-k8s-runner（真实 kubectl / Kubernetes API 场景）
#   - 基础 SRE CLI（真实 kubectl + 常用诊断工具）
FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY harness-agent/go.mod harness-agent/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY harness-agent/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /harness-agent ./cmd/harness-agent

FROM alpine:3.20
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.30.6
RUN apk add --no-cache ca-certificates git bash socat jq python3 curl \
    && ln -sf /bin/bash /bin/sh \
    && addgroup -S ballast \
    && adduser -S -G ballast ballast

# 基础 SRE 工具箱：kubectl 使用真实官方二进制；Terraform/Helm 仍保留轻量占位，
# 生产镜像可在此处替换为企业批准的真实工具链。
RUN case "${TARGETARCH:-amd64}" in \
      amd64|arm64) kubectl_arch="${TARGETARCH:-amd64}" ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${kubectl_arch}/kubectl" \
    && chmod +x /usr/local/bin/kubectl \
    && kubectl version --client=true --output=yaml >/dev/null \
    && printf '#!/bin/sh\necho "[mock-terraform] $*" >&2\nexit 0\n' > /usr/local/bin/terraform && chmod +x /usr/local/bin/terraform \
    && printf '#!/bin/sh\necho "[mock-helm] $*" >&2\nexit 0\n' > /usr/local/bin/helm && chmod +x /usr/local/bin/helm

# harness-agent
COPY --from=build /harness-agent /usr/local/bin/harness-agent

# mock-opencode：模拟 opencode serve --format json 的最小 HTTP server
COPY sandbox-image/mock-opencode /usr/local/bin/mock-opencode
COPY sandbox-image/ballast-real-k8s-runner /usr/local/bin/ballast-real-k8s-runner
RUN chmod +x /usr/local/bin/mock-opencode /usr/local/bin/ballast-real-k8s-runner

# Skill 挂载点（运行时由控制面动态注入到 /workspace/.opencode/skills/）
RUN mkdir -p /workspace/.opencode/skills /workspace/.kube
COPY sandbox-image/.opencode/skills/ /workspace/.opencode/skills/
RUN chown -R ballast:ballast /workspace

WORKDIR /workspace
ENV BALLAST_SANDBOX=1
USER ballast
HEALTHCHECK --interval=5s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:9091/healthz || exit 1
# harness-agent 启动后以 mock-opencode 为受控子进程
ENTRYPOINT ["/usr/local/bin/harness-agent"]
