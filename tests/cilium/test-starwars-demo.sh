#!/bin/bash

# 测试脚本: Cilium 星球大战演示
# 此脚本实现了在Kubernetes集群上部署Cilium星球大战演示应用
# 并测试L3/L4和L7网络策略

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST_DIR="${SCRIPT_DIR}/manifests/starwars"

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# 辅助函数
log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

wait_for_pods_ready() {
  log_info "等待所有Pod准备就绪..."
  kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=deathstar --timeout=120s
  kubectl wait --for=condition=ready pod/tiefighter --timeout=60s
  kubectl wait --for=condition=ready pod/xwing --timeout=60s
  log_info "所有Pod已就绪"
}

cleanup() {
  log_info "清理资源..."
  kubectl delete -f "${MANIFEST_DIR}/http-sw-app.yaml" --ignore-not-found=true
  kubectl delete -f "${MANIFEST_DIR}/sw_l3_l4_l7_policy.yaml" --ignore-not-found=true
  log_info "清理完成"
}

# 注册清理函数
trap cleanup EXIT

# 检查cilium是否正在运行
if ! kubectl get pods -n kube-system -l k8s-app=cilium | grep -q Running; then
  log_error "Cilium未在集群中运行，请先安装Cilium"
  exit 1
fi

# 部署应用
log_info "部署星球大战示例应用..."
kubectl apply -f "${MANIFEST_DIR}/http-sw-app.yaml"

# 等待Pod准备就绪
wait_for_pods_ready

# 检查不带策略时的初始访问情况
log_info "测试初始访问情况（无网络策略）..."
log_info "TIE战机访问死星的端口:"
if kubectl exec tiefighter -- curl -s -XPUT deathstar.default.svc.cluster.local/v1/exhaust-port | grep -q "deathstar exploded"; then
  log_info "成功: TIE战机可以访问死星的排气口端口（这个安全漏洞需要被修复）"
else
  log_error "失败: TIE战机无法访问死星排气口端口"
  exit 1
fi


# 应用L7网络策略
log_info "应用L7网络策略..."
kubectl apply -f "${MANIFEST_DIR}/sw_l3_l4_l7_policy.yaml"

# 等待策略生效
log_info "等待策略生效..."
sleep 10

# 测试网络策略有效性
log_info "测试L7网络策略有效性..."

# 测试1: TIE战机应该可以执行允许的POST请求
log_info "测试1: TIE战机执行允许的POST请求"
if kubectl exec tiefighter -- curl -s -XPOST deathstar.default.svc.cluster.local/v1/request-landing | grep -q "Ship landed"; then
  log_info "测试1通过: TIE战机可以发送允许的POST请求"
else
  log_error "测试1失败: TIE战机无法发送允许的POST请求"
  exit 1
fi

# 测试2: TIE战机不应该能执行被禁止的PUT请求
log_info "测试2: TIE战机执行被禁止的PUT请求"
RESPONSE=$(kubectl exec tiefighter -- curl -s -XPUT deathstar.default.svc.cluster.local/v1/exhaust-port)
if [[ "$RESPONSE" == "Access denied" ]]; then
  log_info "测试2通过: TIE战机无法发送被禁止的PUT请求"
else
  log_error "测试2失败: TIE战机仍然可以发送被禁止的PUT请求"
  exit 1
fi

# 测试3: X翼战机不属于empire组织，应该被完全阻止访问
log_info "测试3: X翼战机访问死星"
if timeout 5 kubectl exec xwing -- curl -s -XPOST deathstar.default.svc.cluster.local/v1/request-landing; then
  log_error "测试3失败: X翼战机能够访问死星"
  exit 1
else
  log_info "测试3通过: X翼战机无法访问死星"
fi

# 显示Cilium网络策略
log_info "当前生效的Cilium网络策略:"
kubectl describe ciliumnetworkpolicies

log_info "星球大战演示测试全部通过!"
exit 0 