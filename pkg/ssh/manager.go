package ssh

import (
	"fmt"
	"maps"
	"os"
	"sync"
	"time"

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
	// 添加健康检查和自动重连相关字段
	healthCheckInterval time.Duration
	stopHealthCheck     chan struct{}
	healthCheckActive   bool
}

// NewSSHManager 创建新的SSH管理器
func NewSSHManager(cluster *cluster.Cluster, sshConfig *SSHConfig) *SSHManager {
	manager := &SSHManager{
		cluster:             cluster,
		sshConfig:           sshConfig,
		clients:             make(map[string]*Client),
		healthCheckInterval: 30 * time.Second,
		stopHealthCheck:     make(chan struct{}),
	}

	// 启动健康检查
	manager.StartHealthCheck()

	return manager
}

// StartHealthCheck 启动SSH连接的健康检查
func (sm *SSHManager) StartHealthCheck() {
	sm.mutex.Lock()
	if sm.healthCheckActive {
		sm.mutex.Unlock()
		return
	}
	sm.healthCheckActive = true
	sm.mutex.Unlock()

	go func() {
		ticker := time.NewTicker(sm.healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sm.checkAllConnections()
			case <-sm.stopHealthCheck:
				return
			}
		}
	}()
}

// StopHealthCheck 停止健康检查
func (sm *SSHManager) StopHealthCheck() {
	sm.mutex.Lock()
	if !sm.healthCheckActive {
		sm.mutex.Unlock()
		return
	}
	sm.healthCheckActive = false
	sm.mutex.Unlock()

	close(sm.stopHealthCheck)
}

// checkAllConnections 检查所有连接的健康状态
func (sm *SSHManager) checkAllConnections() {
	sm.mutex.RLock()
	clientsCopy := make(map[string]*Client)
	maps.Copy(clientsCopy, sm.clients)
	sm.mutex.RUnlock()

	for name, client := range clientsCopy {
		// 检查连接
		err := client.Connect()
		if err != nil {
			fmt.Printf("节点 %s 的SSH连接检查失败: %v\n", name, err)
			// 移除无效客户端
			sm.mutex.Lock()
			delete(sm.clients, name)
			sm.mutex.Unlock()
		}
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
	client, exists := sm.clients[nodeName]
	sm.mutex.RUnlock()

	if !exists {
		ip := sm.GetIP(nodeName)
		if ip == "" {
			return nil, false
		}

		// 创建新客户端
		sm.mutex.Lock()
		// 双重检查锁定
		client, exists = sm.clients[nodeName]
		if exists {
			sm.mutex.Unlock()
			return client, true
		}

		client = NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
		err := client.Connect()
		if err != nil {
			sm.mutex.Unlock()
			return nil, false
		}
		sm.clients[nodeName] = client
		sm.mutex.Unlock()
		return client, true
	}

	return client, true
}

func (sm *SSHManager) CreateClient(nodeName string, ip string) (*Client, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 检查是否已存在
	if client, exists := sm.clients[nodeName]; exists {
		client.Close()
		delete(sm.clients, nodeName)
	}

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
	sm.StopHealthCheck()

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for nodeName, client := range sm.clients {
		client.Close()
		delete(sm.clients, nodeName)
	}
}
