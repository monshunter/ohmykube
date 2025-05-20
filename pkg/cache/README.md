# Cache 功能增强
## 背景
- 初始化节点需要通过网络安装一堆工具，不仅消耗网络流量，而且由于网络不稳定，可能会中途流产，导致初始化失败
- 初始化k8s集群需要通过网络下载一系列镜像，不仅消耗网络，而且由于网络不稳定，可能会中途流产，导致初始化失败
## 目标
- 初始化节点或者k8s集群，尽量离线化，减少网络依赖
## 方案
- 在本地电脑缓存需要使用的二进制包、镜像，在创建好节点后，直接将这些二进制包和镜像上传到目标节点，辅助完成进一步的节点vm初始化和k8s节点初始化
- cache 负责管理这些二进制文件和镜像文件，负责下载、保存、上传、更新

## 实现功能
OhMyKube的缓存模块支持以下功能：

### 通用缓存管理
- 支持本地缓存二进制文件和容器镜像
- 提供统一的文件操作接口（下载、检查、列出、删除等）
- 自动创建和管理缓存目录结构

### 二进制文件缓存
- 从网络下载二进制工具到本地缓存
- 检查二进制文件版本信息
- 将本地缓存的二进制文件上传到目标节点
- 设置二进制文件的可执行权限

### 容器镜像缓存
- 从Docker Hub等仓库拉取镜像并保存到本地
- 将镜像导入到目标节点
- 管理镜像名称和文件名的映射关系

### 缓存管理器
- 统一管理二进制和镜像缓存
- 支持批量操作（下载所有所需二进制、拉取所有所需镜像）
- 一键准备节点所需的所有资源
- 清理特定类型的缓存
- 获取缓存使用情况统计

## 使用示例

```go
// 创建缓存管理器
manager, err := cache.NewManager()
if err != nil {
    // 处理错误
}

// 初始化缓存目录
if err := manager.Initialize(); err != nil {
    // 处理错误
}

// 下载所需的二进制文件
binaries := map[string]string{
    "kubectl": "https://dl.k8s.io/release/v1.27.0/bin/linux/amd64/kubectl",
    "kubeadm": "https://dl.k8s.io/release/v1.27.0/bin/linux/amd64/kubeadm",
}
if err := manager.DownloadRequiredBinaries(binaries); err != nil {
    // 处理错误
}

// 拉取所需的镜像
images := []string{
    "k8s.gcr.io/kube-apiserver:v1.27.0",
    "k8s.gcr.io/kube-controller-manager:v1.27.0",
}
if err := manager.PullRequiredImages(images); err != nil {
    // 处理错误
}

// 准备节点所需的资源
nodeName := "worker-1"
targetDir := "/usr/local/bin"
if err := manager.PrepareNode(nodeName, []string{"kubectl", "kubeadm"}, images, targetDir); err != nil {
    // 处理错误
}

// 查看缓存信息
info, err := manager.GetCacheInfo()
if err != nil {
    // 处理错误
}
fmt.Printf("缓存信息: %+v\n", info)
```

## 缓存目录结构
缓存文件保存在用户主目录的`.ohmykube/cache`目录下，具体结构如下：

```
~/.ohmykube/cache/
  └── binary/       # 二进制文件缓存
      ├── kubectl
      ├── kubeadm
      └── ...
  └── image/        # 镜像文件缓存
      ├── k8s.gcr.io_kube-apiserver_v1.27.0.tar
      ├── k8s.gcr.io_kube-controller-manager_v1.27.0.tar
      └── ...
```
