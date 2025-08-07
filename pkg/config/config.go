package config

const (
	RoleMaster = "master"
	RoleWorker = "worker"
)

type HookPhase string

const (
	HookPreSystemInit  HookPhase = "pre-system-init"
	HookPostSystemInit HookPhase = "post-system-init"
	HookPreK8sInit     HookPhase = "pre-k8s-init"
	HookPostK8sInit    HookPhase = "post-k8s-init"
)

type Resource struct {
	CPU    int
	Memory int
	Disk   int
}

// NodeMetadata defines metadata that can be applied to nodes
type NodeMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Taints      []NodeTaint       `json:"taints,omitempty"`
}

// NodeTaint represents a Kubernetes node taint (used for CLI parsing)
type NodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// CustomInitConfig defines custom initialization configuration for nodes
type CustomInitConfig struct {
	// Hook scripts to run at different initialization phases
	Hooks CustomInitHooks `json:"hooks,omitempty"`
	// Files to upload to nodes
	Files []FileUpload `json:"files,omitempty"`
}

// CustomInitHooks defines scripts to run at different phases
type CustomInitHooks struct {
	PreSystemInit  []string `json:"preSystemInit,omitempty"`  // Before system update
	PostSystemInit []string `json:"postSystemInit,omitempty"` // After system update, before K8s
	PreK8sInit     []string `json:"preK8sInit,omitempty"`     // Before K8s components install
	PostK8sInit    []string `json:"postK8sInit,omitempty"`    // After all initialization
}

// FileUpload defines a file to upload to nodes
type FileUpload struct {
	LocalPath  string `json:"localPath"`  // Local file path
	RemotePath string `json:"remotePath"` // Remote destination path
	Mode       string `json:"mode"`       // File permissions (e.g., "0644")
	Owner      string `json:"owner"`      // File owner (e.g., "root:root")
}

// Config stores cluster configuration
type Config struct {
	Parallel          int
	Provider          string // lima, alicloud, gke, aws, tke
	Template          string // template or file
	Name              string
	Master            Resource
	Workers           []Resource
	KubernetesVersion string // k8s version
	ProxyMode         string
	CNI               string
	CSI               string
	LB                string
	UpdateSystem      bool
	OutputFormat      string
	// Node metadata configurations
	MasterMetadata NodeMetadata
	WorkerMetadata NodeMetadata
	// Custom initialization configurations
	MasterCustomInit CustomInitConfig
	WorkerCustomInit CustomInitConfig
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

func (c *Config) SetProvider(provider string) {
	c.Provider = provider
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

// SetMasterMetadata sets metadata for master nodes
func (c *Config) SetMasterMetadata(metadata NodeMetadata) {
	c.MasterMetadata = metadata
}

// SetWorkerMetadata sets metadata for worker nodes
func (c *Config) SetWorkerMetadata(metadata NodeMetadata) {
	c.WorkerMetadata = metadata
}

// AddMasterLabel adds a label to master nodes
func (c *Config) AddMasterLabel(key, value string) {
	if c.MasterMetadata.Labels == nil {
		c.MasterMetadata.Labels = make(map[string]string)
	}
	c.MasterMetadata.Labels[key] = value
}

// AddWorkerLabel adds a label to worker nodes
func (c *Config) AddWorkerLabel(key, value string) {
	if c.WorkerMetadata.Labels == nil {
		c.WorkerMetadata.Labels = make(map[string]string)
	}
	c.WorkerMetadata.Labels[key] = value
}

// AddMasterAnnotation adds an annotation to master nodes
func (c *Config) AddMasterAnnotation(key, value string) {
	if c.MasterMetadata.Annotations == nil {
		c.MasterMetadata.Annotations = make(map[string]string)
	}
	c.MasterMetadata.Annotations[key] = value
}

// AddWorkerAnnotation adds an annotation to worker nodes
func (c *Config) AddWorkerAnnotation(key, value string) {
	if c.WorkerMetadata.Annotations == nil {
		c.WorkerMetadata.Annotations = make(map[string]string)
	}
	c.WorkerMetadata.Annotations[key] = value
}

// AddMasterTaint adds a taint to master nodes
func (c *Config) AddMasterTaint(taint NodeTaint) {
	c.MasterMetadata.Taints = append(c.MasterMetadata.Taints, taint)
}

// AddWorkerTaint adds a taint to worker nodes
func (c *Config) AddWorkerTaint(taint NodeTaint) {
	c.WorkerMetadata.Taints = append(c.WorkerMetadata.Taints, taint)
}

// SetMasterCustomInit sets custom initialization configuration for master nodes
func (c *Config) SetMasterCustomInit(customInit CustomInitConfig) {
	c.MasterCustomInit = customInit
}

// SetWorkerCustomInit sets custom initialization configuration for worker nodes
func (c *Config) SetWorkerCustomInit(customInit CustomInitConfig) {
	c.WorkerCustomInit = customInit
}

// AddMasterHookScript adds a hook script for master nodes at specified phase
func (c *Config) AddMasterHookScript(phase HookPhase, script string) {
	switch phase {
	case HookPreSystemInit:
		c.MasterCustomInit.Hooks.PreSystemInit = append(c.MasterCustomInit.Hooks.PreSystemInit, script)
	case HookPostSystemInit:
		c.MasterCustomInit.Hooks.PostSystemInit = append(c.MasterCustomInit.Hooks.PostSystemInit, script)
	case HookPreK8sInit:
		c.MasterCustomInit.Hooks.PreK8sInit = append(c.MasterCustomInit.Hooks.PreK8sInit, script)
	case HookPostK8sInit:
		c.MasterCustomInit.Hooks.PostK8sInit = append(c.MasterCustomInit.Hooks.PostK8sInit, script)
	}
}

// AddWorkerHookScript adds a hook script for worker nodes at specified phase
func (c *Config) AddWorkerHookScript(phase HookPhase, script string) {
	switch phase {
	case HookPreSystemInit:
		c.WorkerCustomInit.Hooks.PreSystemInit = append(c.WorkerCustomInit.Hooks.PreSystemInit, script)
	case HookPostSystemInit:
		c.WorkerCustomInit.Hooks.PostSystemInit = append(c.WorkerCustomInit.Hooks.PostSystemInit, script)
	case HookPreK8sInit:
		c.WorkerCustomInit.Hooks.PreK8sInit = append(c.WorkerCustomInit.Hooks.PreK8sInit, script)
	case HookPostK8sInit:
		c.WorkerCustomInit.Hooks.PostK8sInit = append(c.WorkerCustomInit.Hooks.PostK8sInit, script)
	}
}

func (c *Config) AddMasterFileUpload(file FileUpload) {
	c.MasterCustomInit.Files = append(c.MasterCustomInit.Files, file)
}

func (c *Config) AddWorkerFileUpload(file FileUpload) {
	c.WorkerCustomInit.Files = append(c.WorkerCustomInit.Files, file)
}
