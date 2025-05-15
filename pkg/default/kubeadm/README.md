# Kubeadm 配置系统

本模块提供了 Kubeadm 配置的管理功能，包括默认配置的提供、配置文件的加载、合并和生成。

## 结构设计

配置系统将 Kubeadm 配置分为四个主要部分：

1. `InitConfiguration` - 初始化配置
2. `ClusterConfiguration` - 集群配置
3. `KubeletConfiguration` - Kubelet 配置
4. `KubeProxyConfiguration` - KubeProxy 配置

这些配置可以单独获取和设置，也可以合并成一个完整的配置文件。

## 主要功能

### 获取默认配置

```go
// 获取完整的默认配置
config := kubeadm.GetDefaultConfig()
```

### 加载配置文件

```go
// 从文件加载配置
config, err := kubeadm.LoadFromFile("/path/to/config.yaml")
if err != nil {
    // 处理错误
}
```

### 合并多个配置

```go
// 合并多个配置文件
configs := []string{
    "/path/to/config1.yaml",
    "/path/to/config2.yaml",
}
mergedConfig, err := kubeadm.LoadAndMergeConfigs(configs)
if err != nil {
    // 处理错误
}
```

### 生成配置文件

```go
// 生成配置并保存到临时文件
configPath, err := kubeadm.GenerateKubeadmConfig(
    "1.33.0",         // Kubernetes 版本
    "10.244.0.0/16",  // Pod CIDR
    "10.96.0.0/12",   // Service CIDR
    "/path/to/custom-config.yaml", // 可选的自定义配置
)
if err != nil {
    // 处理错误
}
// 使用生成的配置文件
// ...
// 完成后删除临时文件
os.Remove(configPath)
```

## 在 OhMyKube 中的使用

在 OhMyKube 中，您可以通过 `KubeadmConfig` 对象的 `SetCustomConfig` 方法设置自定义配置文件：

```go
kubeadmConfig := kubeadm.NewKubeadmConfig(mpClient, "1.33.0", "master-node")
kubeadmConfig.SetCustomConfig("/path/to/your-custom-config.yaml")
```

这样在初始化集群时，自定义配置将与默认配置合并使用。

## 配置优先级

当合并配置时，自定义配置会覆盖默认配置。具体规则如下：

1. 对于简单值（如字符串、数字、布尔值），自定义配置会完全替换默认配置
2. 对于嵌套map/对象，会递归合并，同名键的值按上述规则处理
3. 对于数组/列表，自定义配置会完全替换默认配置的相应数组 