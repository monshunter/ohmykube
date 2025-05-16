package ssh

import (
	"fmt"
	"os"
	"sync"

	"github.com/monshunter/ohmykube/pkg/cluster"
)

type SSHConfig struct {
	Password      string
	SSHKeyFile    string
	SSHPubKeyFile string
	sshKey        string
	sshPubKey     string
}

func NewSSHConfig(password string, sshKeyFile string, sshPubKeyFile string) (*SSHConfig, error) {
	sshConfig := &SSHConfig{
		Password:      password,
		SSHKeyFile:    sshKeyFile,
		SSHPubKeyFile: sshPubKeyFile,
	}
	err := sshConfig.init()
	if err != nil {
		return nil, err
	}
	return sshConfig, nil
}

func (c *SSHConfig) init() error {
	sshKeyContent, err := os.ReadFile(c.SSHKeyFile)
	if err != nil {
		return fmt.Errorf("读取SSH私钥文件失败: %w", err)
	}
	c.sshKey = string(sshKeyContent)
	sshPubKeyContent, err := os.ReadFile(c.SSHPubKeyFile)
	if err != nil {
		return fmt.Errorf("读取SSH公钥文件失败: %w", err)
	}
	c.sshPubKey = string(sshPubKeyContent)
	return nil
}

func (c *SSHConfig) GetSSHKey() string {
	return c.sshKey
}

func (c *SSHConfig) GetSSHPubKey() string {
	return c.sshPubKey
}

// SSHManager 管理SSH客户端及其生命周期
type SSHManager struct {
	cluster   *cluster.Cluster
	sshConfig *SSHConfig
	clients   map[string]*Client
	mutex     sync.RWMutex
}

// NewSSHManager 创建新的SSH管理器
func NewSSHManager(cluster *cluster.Cluster, sshConfig *SSHConfig) *SSHManager {
	return &SSHManager{
		cluster:   cluster,
		sshConfig: sshConfig,
		clients:   make(map[string]*Client),
	}
}

func (sm *SSHManager) GetIP(nodeName string) string {
	node := sm.cluster.GetNodeByName(nodeName)
	if node == nil {
		return ""
	}
	return node.ExtraInfo.IP
}

// GetClient 获取SSH客户端
func (sm *SSHManager) GetClient(nodeName string) (*Client, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	client, exists := sm.clients[nodeName]
	if !exists {
		ip := sm.GetIP(nodeName)
		client = NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
		err := client.Connect()
		if err != nil {
			return nil, false
		}
		sm.clients[nodeName] = client
		return client, true
	}
	return client, true
}

func (sm *SSHManager) CreateClient(nodeName string, ip string) (*Client, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	client := NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
	err := client.Connect()
	if err != nil {
		return nil, err
	}
	sm.clients[nodeName] = client
	return client, nil
}

// RunCommand 在指定节点上执行命令
func (sm *SSHManager) RunCommand(nodeName, command string) (string, error) {
	client, exists := sm.GetClient(nodeName)
	if !exists {
		return "", fmt.Errorf("节点 %s 的SSH客户端不存在", nodeName)
	}

	return client.RunCommand(command)
}

// CloseClient 关闭指定节点的SSH客户端
func (sm *SSHManager) CloseClient(nodeName string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if client, exists := sm.clients[nodeName]; exists {
		client.Close()
		delete(sm.clients, nodeName)
	}
}

// CloseAllClients 关闭所有SSH客户端
func (sm *SSHManager) CloseAllClients() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for nodeName, client := range sm.clients {
		client.Close()
		delete(sm.clients, nodeName)
	}
}
