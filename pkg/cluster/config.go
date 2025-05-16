package cluster

import (
	"fmt"
)

const (
	RoleMaster = "master"
	RoleWorker = "worker"
)

// Node 保存节点配置
type Node struct {
	Name string
	Resource
}

type Resource struct {
	CPU    int
	Memory int
	Disk   int
}

// Config 保存集群配置
type Config struct {
	Image      string
	Name       string
	Master     Node
	Workers    []Node
	K8sVersion string
}

func NewConfig(name string, k8sVersion string, workers int, masterResource Resource, workerResource Resource) *Config {
	c := &Config{
		Name:       name,
		Master:     Node{},
		Workers:    make([]Node, workers),
		K8sVersion: k8sVersion,
	}
	for i := range workers {
		c.Workers[i].Name = c.GetWorkerVMName(i)
		c.Workers[i].Resource = workerResource
	}
	c.Master.Name = c.GetMasterVMName(0)
	c.Master.Resource = masterResource
	return c
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
