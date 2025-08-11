# Addons Requirements for OhMyKube
## 背景
- OhMyKube 是一个用于在本地开发机上快速创建真实 Kubernetes 集群的工具。为了确保 OhMyKube 能够正常工作，需要满足一些前置条件。
- 用户可能可能需要集群创建后，安装一些常用的应用，例如：metrics-server, prometheus, grafana 等。虽然这种应用可以通过kubectl apply -f 或者 helm 工具来手动安装，但是难以复用，不可能每次安装集群后，都手动或者通过脚本去执行后续必要软件的安装过程，而且这种方式无法缓存资源，新集群总是必须通过网络重新下载镜像等资源，导致集群就绪时间过长。
## 目标
- 希望有一种便捷的手段来轻松管理并快速安装应用

## 需求
- 支持应用模板的定义，应用模板可以是 helm，manifest（k8s yaml 文件） 等，kustomize 暂时不支持
- 支持应用模板的版本管理，例如： prometheus 有 v1, v2 等版本
- 支持应用模板的参数化，例如： prometheus 的 storage class，grafana 的 admin password 等
- 复用ohmykube的缓存机制，将应用模板依赖的资源缓存到本地，新集群创建时，如果本地有缓存，则直接使用缓存，否则从网络下载并缓存到本地
- ohmykube config 生成的配置文件需要有举例
- Cluster 对象中的spec和status要体现这一特性，可能需要增加一个字段来描述 addon的期望和状态
- 可通过类似以下的命令来安装应用（flag只是举例，具体实施看方案）
```bash
ohmykube up --addon-from-manifest appName1:url1 \
--addon-from-manifest appName2:url2 \
--addon-from-helm appName3:repo:chartName:version \
--addon-from-helm appName4:repo:chartName:version
# 或者通过
ohmykube up -f config.yaml
```
