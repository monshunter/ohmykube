package cluster

const (
	RoleMaster = "master"
	RoleWorker = "worker"
)

type Resource struct {
	CPU    int
	Memory int
	Disk   int
}

// Config stores cluster configuration
type Config struct {
	Parallel          int
	LauncherType      string
	Image             string
	Template          string
	Name              string
	Master            Resource
	Workers           []Resource
	KubernetesVersion string
	ProxyMode         string
}

func NewConfig(name string, workers int, proxyMode string, masterResource Resource, workerResource Resource) *Config {
	c := &Config{
		Name:    name,
		Master:  masterResource,
		Workers: make([]Resource, workers),
	}
	for i := range workers {
		c.Workers[i] = workerResource
	}
	c.ProxyMode = proxyMode
	return c
}

func (c *Config) SetParallel(parallel int) {
	c.Parallel = parallel
}

func (c *Config) SetLauncherType(launcherType string) {
	c.LauncherType = launcherType
}

func (c *Config) SetKubernetesVersion(k8sVersion string) {
	c.KubernetesVersion = k8sVersion
}

func (c *Config) SetImage(image string) {
	c.Image = image
}

func (c *Config) SetTemplate(template string) {
	c.Template = template
}
