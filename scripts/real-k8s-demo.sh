#!/usr/bin/env bash
set -euo pipefail

root="${BALLAST_REAL_K8S_ROOT:-/tmp/ballast/real-k8s}"
container="${BALLAST_REAL_K8S_CONTAINER:-ballast-real-k8s}"
api_port="${BALLAST_REAL_K8S_PORT:-6445}"
image="${BALLAST_REAL_K8S_IMAGE:-rancher/k3s:v1.30.6-k3s1}"
namespace="${BALLAST_KUBE_NAMESPACE:-ballast-demo}"
deployment="${BALLAST_KUBE_TARGET_DEPLOYMENT:-crashloop-demo}"
selector="${BALLAST_KUBE_TARGET_SELECTOR:-app=crashloop-demo}"
fix_configmap="${BALLAST_KUBE_FIX_CONFIGMAP:-crashloop-demo-config}"

raw_kubeconfig="${root}/kubeconfig.raw.yaml"
host_kubeconfig="${root}/kubeconfig.host.yaml"
sandbox_kubeconfig="${root}/kubeconfig.sandbox.yaml"

usage() {
  cat <<USAGE
Usage: $0 [start|seed|status|env|stop]

start   Start local k3s, rewrite kubeconfigs, and seed CrashLoopBackOff demo.
seed    Re-apply the broken CrashLoopBackOff demo workload.
status  Show current demo Kubernetes resources.
env     Print docker compose environment needed for real-k8s Ballast mode.
stop    Remove the local k3s container.
USAGE
}

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

rewrite_kubeconfigs() {
  python3 - "$raw_kubeconfig" "$host_kubeconfig" "$sandbox_kubeconfig" "$api_port" <<'PY'
from pathlib import Path
import sys

raw, host, sandbox, port = sys.argv[1:]
content = Path(raw).read_text(encoding="utf-8")
host_content = content.replace("https://127.0.0.1:6443", f"https://127.0.0.1:{port}")
host_content = host_content.replace("https://localhost:6443", f"https://127.0.0.1:{port}")
sandbox_content = content.replace("https://127.0.0.1:6443", f"https://host.docker.internal:{port}")
sandbox_content = sandbox_content.replace("https://localhost:6443", f"https://host.docker.internal:{port}")
Path(host).write_text(host_content, encoding="utf-8")
Path(sandbox).write_text(sandbox_content, encoding="utf-8")
PY
}

wait_for_k3s() {
  local deadline=$((SECONDS + 120))
  until [[ -s "$raw_kubeconfig" ]]; do
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for ${raw_kubeconfig}" >&2
      docker logs "$container" >&2 || true
      exit 1
    fi
    sleep 1
  done
  rewrite_kubeconfigs
  until kubectl --kubeconfig "$host_kubeconfig" get nodes -o name 2>/dev/null | grep -q .; do
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for k3s API" >&2
      docker logs "$container" >&2 || true
      exit 1
    fi
    sleep 1
  done
  kubectl --kubeconfig "$host_kubeconfig" wait --for=condition=Ready node --all --timeout=120s
}

start() {
  require docker
  require kubectl
  require python3
  mkdir -p "$root"
  if docker ps -a --format '{{.Names}}' | grep -qx "$container"; then
    if ! docker ps --format '{{.Names}}' | grep -qx "$container"; then
      docker start "$container" >/dev/null
    fi
  else
    docker run -d \
      --name "$container" \
      --privileged \
      -p "127.0.0.1:${api_port}:6443" \
      -v "${root}:/output" \
      "$image" \
      server \
      --disable=traefik \
      --disable=servicelb \
      --write-kubeconfig=/output/kubeconfig.raw.yaml \
      --write-kubeconfig-mode=644 \
      --tls-san=127.0.0.1 \
      --tls-san=host.docker.internal >/dev/null
  fi
  wait_for_k3s
  seed
  env_output
}

seed() {
  require kubectl
  if [[ ! -s "$host_kubeconfig" ]]; then
    if [[ -s "$raw_kubeconfig" ]]; then
      rewrite_kubeconfigs
    else
      echo "missing kubeconfig; run: $0 start" >&2
      exit 1
    fi
  fi

  kubectl --kubeconfig "$host_kubeconfig" apply -f - <<YAML
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${deployment}
  namespace: ${namespace}
  labels:
    app: crashloop-demo
spec:
  strategy:
    type: Recreate
  replicas: 1
  selector:
    matchLabels:
      app: crashloop-demo
  template:
    metadata:
      labels:
        app: crashloop-demo
    spec:
      containers:
        - name: app
          image: busybox:1.36
          imagePullPolicy: IfNotPresent
          command:
            - sh
            - -c
            - |
              if [ "\${APP_CONFIG_READY}" != "true" ]; then
                echo "Error: configmap ${fix_configmap} not ready"
                exit 1
              fi
              echo "config loaded; app is healthy"
              sleep 3600
          env:
            - name: APP_CONFIG_READY
              value: "false"
YAML

  kubectl --kubeconfig "$host_kubeconfig" -n "$namespace" rollout restart "deployment/${deployment}" >/dev/null
  sleep 5
  status
}

status() {
  require kubectl
  kubectl --kubeconfig "$host_kubeconfig" get ns "$namespace" >/dev/null
  kubectl --kubeconfig "$host_kubeconfig" -n "$namespace" get deploy,pods -l "$selector" -o wide
}

env_output() {
  cat <<ENV

Real K8s demo is ready.

Use these variables when starting Ballast:

export BALLAST_RUNNER_COMMAND=/usr/local/bin/ballast-real-k8s-runner
export BALLAST_KUBECONFIG=${sandbox_kubeconfig}
export BALLAST_REAL_K8S_ROOT=${root}
export BALLAST_KUBE_NAMESPACE=${namespace}
export BALLAST_KUBE_TARGET_SELECTOR=${selector}
export BALLAST_KUBE_TARGET_DEPLOYMENT=${deployment}
export BALLAST_KUBE_FIX_CONFIGMAP=${fix_configmap}

Then restart the stack:

docker compose up -d --build sandbox-image ballast-server web
ENV
}

stop() {
  require docker
  docker rm -f "$container" >/dev/null 2>&1 || true
  echo "stopped ${container}"
}

case "${1:-start}" in
  start) start ;;
  seed) seed ;;
  status) status ;;
  env) env_output ;;
  stop) stop ;;
  -h|--help|help) usage ;;
  *) usage; exit 1 ;;
esac
