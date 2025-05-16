# Cilium 星球大战演示测试

此目录包含用于测试 Cilium 网络策略功能的脚本和配置文件，基于 Cilium 官方的 Star Wars 演示示例。

## 概述

Cilium 是一个开源软件，用于提供和透明地保护应用程序工作负载之间的网络连接和负载均衡。它使用 eBPF 技术实现高效的网络策略、可观测性和安全功能。

本测试套件演示了如何：
1. 部署示例应用程序（死星服务、TIE战机和X翼战机）
2. 测试默认情况下的网络访问
3. 应用细粒度的 L7 HTTP 网络策略
4. 验证策略是否按预期工作

## 测试内容

测试脚本 `test-starwars-demo.sh` 会执行以下操作：

1. 检查 Cilium 是否已在集群中运行
2. 部署 Star Wars 演示应用（deathstar 服务、tiefighter 和 xwing pod）
3. 测试初始网络访问状态（无策略限制）
4. 应用 L7 HTTP 网络策略，限制 tiefighter（empire 组织）只能向 deathstar 发送 POST 请求到 `/v1/request-landing` 接口
5. 进行三项测试：
   - 测试 TIE战机是否能执行允许的 POST 请求（应成功）
   - 测试 TIE战机是否无法执行被禁止的 PUT 请求（应被拒绝）
   - 测试 X翼战机（非 empire 组织）是否完全无法访问 deathstar（应超时）
6. 测试完成后清理所有资源

## 前提条件

- 正在运行的 Kubernetes 集群
- 已安装并运行的 Cilium（最低版本 1.8）
- 可用的 `kubectl` 命令，已配置连接到集群

## 使用方法

1. 确保 Cilium 已在您的集群中运行：
   ```
   kubectl get pods -n kube-system -l k8s-app=cilium
   ```

2. 运行测试脚本：
   ```
   chmod +x test-starwars-demo.sh
   ./test-starwars-demo.sh
   ```

3. 观察输出结果，验证所有测试是否通过

## 文件结构

```
tests/cilium/
├── README.md                      # 本文档
├── test-starwars-demo.sh          # 主测试脚本
└── manifests/
    └── starwars/
        ├── http-sw-app.yaml       # 应用部署配置
        └── sw_l3_l4_l7_policy.yaml # Cilium L7 网络策略
```

## 参考资料

- [Cilium Star Wars 演示文档](https://docs.cilium.io/en/latest/gettingstarted/demo/)
- [Cilium 网络策略](https://docs.cilium.io/en/latest/security/policy/)
- [Cilium HTTP 策略示例](https://docs.cilium.io/en/latest/security/policy/language/#http) 