package containerd

const (
	DOCKER_MIRROR = `
server = "https://docker.io"

[host."https://registry.cn-hangzhou.aliyuncs.com"]
  capabilities = ["pull", "resolve"]
`
	K8S_MIRROR = `
server = "https://registry.k8s.io"

[host."https://k8s.m.daocloud.io"]
  capabilities = ["pull", "resolve"]
`
	QUAY_MIRROR = `
server = "https://quay.io"

[host."https://quay.mirrors.ustc.edu.cn"]
  capabilities = ["pull", "resolve"]
`
)

type Mirror struct {
	Name   string
	Config string
}

func Mirrors() []Mirror {
	return []Mirror{
		{
			Name:   "docker.io",
			Config: DOCKER_MIRROR,
		},
		{
			Name:   "registry.k8s.io",
			Config: K8S_MIRROR,
		},
		{
			Name:   "quay.io",
			Config: QUAY_MIRROR,
		},
	}
}
