package config

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
	Template          string
	Name              string
	Master            Resource
	Workers           []Resource
	KubernetesVersion string
	ProxyMode         string
	CNI               string
	CSI               string
	LB                string
	UpdateSystem      bool
	OutputFormat      string
}

func NewConfig(name string, workers int, proxyMode string, masterResource Resource, workerResource Resource) *Config {
	c := &Config{
		Name:    name,
		Master:  masterResource,
		Workers: make([]Resource, workers),
		CNI:     "flannel",                // Default to flannel
		CSI:     "local-path-provisioner", // Default to local-path-provisioner
		LB:      "metallb",                // Default to metallb
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

func (c *Config) SetTemplate(template string) {
	c.Template = template
}

func (c *Config) SetCNIType(cniType string) {
	c.CNI = cniType
}

func (c *Config) SetCSIType(csiType string) {
	c.CSI = csiType
}

func (c *Config) SetLBType(lbType string) {
	c.LB = lbType
}

func (c *Config) SetUpdateSystem(updateSystem bool) {
	c.UpdateSystem = updateSystem
}
