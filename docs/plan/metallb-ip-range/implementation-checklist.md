# MetalLB IP 地址范围可配置化 Checklist

> **版本**: 1.1
> **状态**: draft
> **更新日期**: 2026-03-05

**关联计划**: [实施计划](./implementation.md)

## 1 Phase 1: 配置模型扩展

- [ ] 1.1 `NetworkingConfig` 增加 `LBAddressRange` 字段 (`pkg/config/cluster.go`)
- [ ] 1.2 添加 `GetLBAddressRange()` / `SetLBAddressRange(addrRange string)`（线程安全）(`pkg/config/cluster.go`)
- [ ] 1.3 `Config` 结构体增加 `LBAddressRange` 字段和 `SetLBAddressRange()` (`pkg/config/config.go`)
- [ ] 1.4 更新 `NewCluster()` 映射 `LBAddressRange` (`pkg/config/cluster.go`)
- [ ] 1.5 更新配置模板注释，补充 `lbAddressRange` 示例 (`pkg/config/template.go`)

## 2 Phase 2: CLI 入口与恢复语义

- [ ] 2.1 添加 `lbAddressRange` 变量和 `--lb-range` flag (`cmd/ohmykube/app/up.go`)
- [ ] 2.2 添加 `normalizeAndValidateLBAddressRange()`（支持 `start-end` 与 `start - end`）(`cmd/ohmykube/app/up.go`)
- [ ] 2.3 增加 `--lb-range` 与 `--lb` 组合约束（非 `metallb` 时报错）(`cmd/ohmykube/app/up.go`)
- [ ] 2.4 新建集群时将规范化范围写入 `Config` (`cmd/ohmykube/app/up.go`)
- [ ] 2.5 恢复集群时实现“持久化值优先，冲突参数 warning”语义 (`cmd/ohmykube/app/up.go`)

## 3 Phase 3: MetalLBInstaller 改造

- [ ] 3.1 扩展 `MetalLBInstaller` 增加 `addressRange` / `allocatedRange` (`pkg/addons/plugins/lb/metallb.go`)
- [ ] 3.2 更新 `NewMetalLBInstaller` 签名增加 `addressRange` 参数 (`pkg/addons/plugins/lb/metallb.go`)
- [ ] 3.3 增加安装器内部范围规范化与校验函数 (`pkg/addons/plugins/lb/metallb.go`)
- [ ] 3.4 修改 `getMetalLBAddressRange()`：用户值优先，自动推导兜底，并统一校验 (`pkg/addons/plugins/lb/metallb.go`)
- [ ] 3.5 在 `Install()` 中记录 `allocatedRange` (`pkg/addons/plugins/lb/metallb.go`)
- [ ] 3.6 添加 `GetAllocatedRange()` 方法 (`pkg/addons/plugins/lb/metallb.go`)

## 4 Phase 4: Addon Manager 集成

- [ ] 4.1 `InstallLB()` 传递 `LBAddressRange` 到 `NewMetalLBInstaller` (`pkg/addons/addons.go`)
- [ ] 4.2 安装成功后持久化自动推导范围到 Cluster（使用 setter）(`pkg/addons/addons.go`)
- [ ] 4.3 保持 LB condition 状态流转行为不回归 (`pkg/addons/addons.go`)

## 5 Phase 5: 测试与回归验证

- [ ] 5.1 测试用户指定范围的规范化与直通行为 (`pkg/addons/plugins/lb/metallb_test.go`)
- [ ] 5.2 测试自动推导范围行为 (`pkg/addons/plugins/lb/metallb_test.go`)
- [ ] 5.3 测试非法 `controllerIP` 和非法 `addressRange` 错误处理 (`pkg/addons/plugins/lb/metallb_test.go`)
- [ ] 5.4 测试 `--lb-range` 与 `--lb` 参数组合约束及恢复优先级 (`cmd/ohmykube/app/up_test.go`)
- [ ] 5.5 测试 `Config -> Cluster` 映射和 `Get/SetLBAddressRange` (`pkg/config/*_test.go`)
- [ ] 5.6 `go build ./...` 编译通过
- [ ] 5.7 `go test ./pkg/config/...` 通过
- [ ] 5.8 `go test ./pkg/addons/...` 通过
- [ ] 5.9 `go test ./cmd/ohmykube/app/...` 通过
