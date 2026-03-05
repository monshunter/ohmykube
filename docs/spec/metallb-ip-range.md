# MetalLB IP 地址范围设计文档

> **版本**: 1.1
> **状态**: draft
> **更新日期**: 2026-03-05

## 1 概述

本文档描述 MetalLB LoadBalancer IP 地址范围可配置化设计，用于解决多集群共享子网时地址池重叠问题。

### 1.1 背景

当前 MetalLB 的 IP 地址范围在 `getMetalLBAddressRange()` 中固定为控制节点所在 /24 子网的 `.200-.250`。在多集群共享同一子网（Lima `192.168.64.x` 为典型场景）时，不同集群会获得相同地址池，造成冲突。

### 1.2 问题分析

**根因代码**: `pkg/addons/plugins/lb/metallb.go`

```go
func (m *MetalLBInstaller) getMetalLBAddressRange() (string, error) {
    ipParts := strings.Split(m.controllerIP, ".")
    prefix := strings.Join(ipParts[:3], ".")
    startIP := 200
    endIP := 250
    return fmt.Sprintf("%s.%d - %s.%d", prefix, startIP, prefix, endIP), nil
}
```

**问题复现**:
- Cluster-1: Master IP = `192.168.64.100` -> Range = `192.168.64.200 - 192.168.64.250`
- Cluster-2: Master IP = `192.168.64.105` -> Range = `192.168.64.200 - 192.168.64.250`（重复）

## 2 设计目标

- **用户可配置**: 支持 CLI flag（`--lb-range`）和配置文件字段（`lbAddressRange`）
- **声明式持久化**: 范围作为 `ClusterSpec.Networking` 的一部分写入 `cluster.yaml`
- **默认行为稳定**: 未提供范围时仍自动推导，避免引入额外迁移步骤
- **单一规则**: CLI 与安装器遵循一致的范围规范化和校验规则

## 3 非目标

- 不做全网段冲突探测或 IP 可达性探测
- 不做跨子网自动规划
- 不引入额外迁移流程

## 4 架构设计

### 4.1 配置流转路径

```
CLI/config file (lbAddressRange)
       |
       v
normalize + validate
       |
       v
Config.LBAddressRange
       |
       v
ClusterSpec.Networking.LBAddressRange  <-- 持久化
       |
       v
MetalLBInstaller.addressRange
       |
       v
getMetalLBAddressRange()
       |-- 有值 -> 校验并使用
       |-- 空值 -> 基于 controllerIP 自动推导
       v
allocatedRange (规范化结果)
       |
       v
IPAddressPool YAML -> apply
```

### 4.2 配置文件格式

```yaml
apiVersion: ohmykube.dev/v1alpha1
kind: Cluster
metadata:
  name: my-cluster
spec:
  networking:
    loadbalancer: metallb
    lbAddressRange: "192.168.64.200-192.168.64.210"
```

### 4.3 CLI 用法

```bash
# 显式指定范围
ohmykube up --lb metallb --lb-range "192.168.64.200-192.168.64.210"

# 不指定时自动推导
ohmykube up --lb metallb
```

## 5 接口定义

### 5.1 配置层

`pkg/config/cluster.go` — `NetworkingConfig` 新增字段：

```go
type NetworkingConfig struct {
    ProxyMode      string `yaml:"proxyMode,omitempty"`
    CNI            string `yaml:"cni,omitempty"`
    PodSubnet      string `yaml:"podSubnet,omitempty"`
    ServiceSubnet  string `yaml:"serviceSubnet,omitempty"`
    LoadBalancer   string `yaml:"loadbalancer,omitempty"`
    LBAddressRange string `yaml:"lbAddressRange,omitempty"`
}
```

`Cluster` 新增线程安全方法：

```go
func (c *Cluster) GetLBAddressRange() string
func (c *Cluster) SetLBAddressRange(addrRange string)
```

`pkg/config/config.go` — `Config` 新增字段与 setter：

```go
type Config struct {
    // ...
    LB             string
    LBAddressRange string
    // ...
}

func (c *Config) SetLBAddressRange(addrRange string)
```

### 5.2 CLI 与恢复语义

`cmd/ohmykube/app/up.go`：
- 新增 `--lb-range`
- 当 `--lb-range` 非空且 `--lb != metallb` 时立即报错
- 恢复已有集群时遵循“持久化值优先”：
  - 若已持久化范围，则忽略冲突的 CLI 输入并告警
  - 若未持久化范围且 CLI 提供了值，则写回集群配置

### 5.3 安装器与 Addon Manager

`pkg/addons/plugins/lb/metallb.go`：

```go
type MetalLBInstaller struct {
    sshRunner      interfaces.SSHRunner
    controllerNode string
    controllerIP   string
    addressRange   string
    allocatedRange string
    Version        string
    manifestURL    string
}

func NewMetalLBInstaller(
    sshRunner interfaces.SSHRunner,
    controllerNode, controllerIP, addressRange string,
) *MetalLBInstaller

func (m *MetalLBInstaller) GetAllocatedRange() string
```

`pkg/addons/addons.go`：
- `InstallLB()` 传入 `m.Cluster.GetLBAddressRange()`
- 当集群未指定范围时，将 `GetAllocatedRange()` 写回 `Cluster`

## 6 数据结构与校验规则

### 6.1 输入与规范化格式

支持输入：
- `startIP-endIP`
- `startIP - endIP`

统一规范化输出为：
- `startIP - endIP`

### 6.2 校验规则

1. 解析为两个 IPv4 地址
2. 起始地址严格小于结束地址
3. 两端地址在同一 /24 子网

校验应同时覆盖：
- CLI 输入路径（前置失败）
- 安装器执行路径（防御性兜底，覆盖配置文件/恢复路径）

## 7 错误处理

| 场景 | 处理方式 |
|------|----------|
| `--lb-range` 与非 `metallb` 组合 | 立即报错并终止 |
| `--lb-range` 格式非法 | 立即报错并终止 |
| 自动推导时 `controllerIP` 非法 | 安装阶段报错并终止 |
| 用户指定范围与网络冲突 | 不做自动探测，交由用户负责 |

## 8 影响范围

| 文件 | 变更 |
|------|------|
| `pkg/config/cluster.go` | `NetworkingConfig` 新增字段，新增 `Get/SetLBAddressRange` |
| `pkg/config/config.go` | `Config` 新增字段与 setter |
| `pkg/config/template.go` | 配置模板新增 `lbAddressRange` 示例 |
| `cmd/ohmykube/app/up.go` | 新增 `--lb-range` 和恢复语义处理 |
| `pkg/addons/plugins/lb/metallb.go` | 支持用户范围、统一校验、暴露分配结果 |
| `pkg/addons/addons.go` | 传递范围配置并持久化自动推导结果 |
| `pkg/addons/plugins/lb/metallb_test.go` | 新增安装器范围逻辑测试 |
| `cmd/ohmykube/app/up_test.go` | 新增 CLI 语义与参数组合测试 |

## 9 关联文档

- [Addon 系统设计](./addon-system-design.md)
- [项目需求设计](./project-requirements-design.md)
