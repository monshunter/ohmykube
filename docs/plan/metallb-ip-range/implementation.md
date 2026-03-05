# MetalLB IP 地址范围可配置化实施计划

> **版本**: 1.1
> **状态**: draft
> **更新日期**: 2026-03-05
> **执行模式**: sequential

**关联 Checklist**: [checklist](./implementation-checklist.md)

## 1 目标

使 MetalLB IP 地址范围可由用户通过 CLI flag 或配置文件指定，解决多集群场景下 IP 范围重叠问题，并确保该范围在集群恢复时保持一致。

## 2 背景

当前 MetalLB IP 范围在安装器中固定为控制节点所在 /24 子网的 `.200-.250`。在多个集群共享同一子网（例如 Lima 默认 `192.168.64.x`）时会导致地址池重叠。

详见 [MetalLB IP 地址范围设计文档](../../spec/metallb-ip-range.md)。

## 3 实施步骤

### 3.1 Phase 1: 配置模型扩展

在配置模型中新增 `LBAddressRange` 字段，使其可声明、可持久化、可读取。

**文件**: `pkg/config/cluster.go`, `pkg/config/config.go`, `pkg/config/template.go`

#### 3.1.1 `NetworkingConfig` 增加 `LBAddressRange` 字段

在 `pkg/config/cluster.go` 的 `NetworkingConfig` 结构体中添加：

```go
LBAddressRange string `yaml:"lbAddressRange,omitempty"`
```

#### 3.1.2 为 `Cluster` 添加线程安全 getter/setter

新增：
- `GetLBAddressRange() string`
- `SetLBAddressRange(addrRange string)`

要求：复用 `Cluster` 现有锁模型（`RLock/Lock`），避免运行期并发写入出现数据竞争。

#### 3.1.3 `Config` 结构体增加 `LBAddressRange` 字段与 setter

在 `pkg/config/config.go` 的 `Config` 结构体中添加：

```go
LBAddressRange string
```

新增：

```go
func (c *Config) SetLBAddressRange(addrRange string)
```

#### 3.1.4 更新 `NewCluster()` 映射

在 `pkg/config/cluster.go` 的 `NewCluster()` 中，将 `cfg.LBAddressRange` 映射到 `NetworkingConfig.LBAddressRange`。

#### 3.1.5 更新配置模板说明

在 `pkg/config/template.go` 的 `networking` 示例中补充 `lbAddressRange` 注释示例，确保文件配置路径与 CLI 能力一致。

### 3.2 Phase 2: CLI 入口与恢复语义

添加 `--lb-range` 参数，并明确新建/恢复集群时的优先级规则。

**文件**: `cmd/ohmykube/app/up.go`

#### 3.2.1 添加变量与 flag 注册

新增变量 `lbAddressRange string`，在 `--lb` 后注册：

```go
upCmd.Flags().StringVar(&lbAddressRange, "lb-range", "",
    `MetalLB IP address range (format: "startIP-endIP", e.g. "192.168.64.200-192.168.64.210")`)
```

#### 3.2.2 添加范围规范化与格式校验

新增 `normalizeAndValidateLBAddressRange(input string) (string, error)`：
- 接受 `startIP-endIP` 与 `startIP - endIP`
- 统一输出 `startIP - endIP`
- 校验起始/结束 IP 都是合法 IPv4
- 校验 `startIP < endIP`
- 校验两端在同一 /24 子网

#### 3.2.3 约束 flag 组合

当 `--lb-range` 非空且 `--lb != metallb` 时，立即返回错误，避免无效参数被静默忽略。

#### 3.2.4 新建集群时写入配置

当提供 `--lb-range` 时，先校验并规范化，再写入 `cfg.SetLBAddressRange(normalizedRange)`。

#### 3.2.5 恢复已有集群时的优先级

恢复逻辑遵循：
- 若集群已持久化 `LBAddressRange`，以持久化值为准
- 若用户同时提供不同 `--lb-range`，记录 warning 并忽略覆盖
- 若集群无持久化值且用户提供了 `--lb-range`，写入 `cls.SetLBAddressRange(normalizedRange)`

### 3.3 Phase 3: MetalLBInstaller 改造

安装器需对最终使用的范围进行防御性校验，覆盖配置文件路径和恢复路径。

**文件**: `pkg/addons/plugins/lb/metallb.go`

#### 3.3.1 扩展 `MetalLBInstaller` 结构体

新增字段：

```go
addressRange   string // 用户输入/持久化读取的范围
allocatedRange string // 最终实际使用的范围（规范化后）
```

#### 3.3.2 更新 `NewMetalLBInstaller` 签名

```go
func NewMetalLBInstaller(
    sshRunner interfaces.SSHRunner,
    controllerNode, controllerIP, addressRange string,
) *MetalLBInstaller
```

#### 3.3.3 安装器内部增加校验函数

在安装器内部增加范围规范化与校验函数（与 CLI 规则一致）。

#### 3.3.4 修改 `getMetalLBAddressRange()`

行为调整：
- `addressRange` 非空：先校验+规范化后返回
- `addressRange` 为空：按控制节点 IP 自动推导，再进行相同校验并返回规范化结果

#### 3.3.5 在 `Install()` 中记录分配结果

调用 `getMetalLBAddressRange()` 后将结果存入 `m.allocatedRange`，用于上层持久化。

#### 3.3.6 添加 `GetAllocatedRange()`

```go
func (m *MetalLBInstaller) GetAllocatedRange() string
```

### 3.4 Phase 4: Addon Manager 集成

将范围配置传递到安装器，并在自动推导场景下持久化最终结果。

**文件**: `pkg/addons/addons.go`

#### 3.4.1 更新 `InstallLB()` 调用

传入 `m.Cluster.GetLBAddressRange()` 给 `NewMetalLBInstaller`。

#### 3.4.2 持久化自动推导结果

安装成功后：

```go
if m.Cluster.GetLBAddressRange() == "" {
    m.Cluster.SetLBAddressRange(metallbInstaller.GetAllocatedRange())
}
```

要求：使用 `Cluster` setter，避免直接写 `Spec` 导致并发访问风险。

### 3.5 Phase 5: 测试与回归验证

覆盖用户输入路径、自动推导路径、恢复语义与编译回归。

**文件**: `pkg/addons/plugins/lb/metallb_test.go`, `cmd/ohmykube/app/up_test.go`, `pkg/config/*_test.go`

#### 3.5.1 安装器：用户指定范围

验证用户输入在合法时被规范化并直接使用。

#### 3.5.2 安装器：自动推导范围

验证基于 `controllerIP` 自动计算并输出规范化范围。

#### 3.5.3 安装器：非法输入错误

验证非法 `controllerIP` 与非法 `addressRange` 都能返回明确错误。

#### 3.5.4 CLI：参数组合与恢复优先级

验证：
- `--lb-range` 与非 `metallb` 组合时报错
- 恢复集群时“持久化值优先”规则

#### 3.5.5 配置映射与持久化

验证 `Config -> Cluster` 映射、`Get/SetLBAddressRange` 行为和序列化路径。

#### 3.5.6 编译与测试命令

- `go build ./...`
- `go test ./pkg/config/...`
- `go test ./pkg/addons/...`
- `go test ./cmd/ohmykube/app/...`

## 4 风险与应对

| 风险 | 应对措施 |
|------|----------|
| CLI 与安装器校验规则漂移 | 使用同一规范化/校验规则，并在测试中覆盖 |
| 用户误用 `--lb-range`（未启用 metallb） | CLI 前置失败，避免静默忽略 |
| 并发流程中直接修改 `Cluster.Spec` 引发竞态 | 通过新增线程安全 setter 写入范围 |

## 5 关联文档

- [MetalLB IP 地址范围设计](../../spec/metallb-ip-range.md)
