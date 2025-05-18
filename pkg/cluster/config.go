package cluster

import (
	"fmt"
)

const (
	RoleMaster = "master"
	RoleWorker = "worker"
)

// Node stores node configuration
type Node struct {
	Name string
	Resource
}

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
	Master            Node
	Workers           []Node
	KubernetesVersion string
}

func NewConfig(name string, workers int, masterResource Resource, workerResource Resource) *Config {
	c := &Config{
		Name:    name,
		Master:  Node{},
		Workers: make([]Node, workers),
	}
	for i := range workers {
		c.Workers[i].Name = c.GetWorkerVMName(i)
		c.Workers[i].Resource = workerResource
	}
	c.Master.Name = c.GetMasterVMName(0)
	c.Master.Resource = masterResource
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

func (c *Config) GetMasterVMName(index int) string {
	return fmt.Sprintf("%smaster-%d", c.Prefix(), index)
}

func (c *Config) GetWorkerVMName(index int) string {
	return fmt.Sprintf("%sworker-%d", c.Prefix(), index)
}

func (c *Config) Prefix() string {
	return fmt.Sprintf("%s-", c.Name)
}
