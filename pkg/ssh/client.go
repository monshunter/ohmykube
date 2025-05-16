package ssh

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client 是SSH客户端的封装
type Client struct {
	Host      string
	Port      string
	User      string
	Password  string
	PrivKey   string
	client    *ssh.Client
	mu        sync.Mutex
	connected bool
}

// NewClient 创建一个新的SSH客户端
func NewClient(host, port, user, password, privKey string) *Client {
	return &Client{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		PrivKey:  privKey,
	}
}

// Connect 连接到SSH服务器
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已经连接，直接返回
	if c.connected && c.client != nil {
		// 检查连接是否仍然有效
		if err := c.testConnection(); err == nil {
			return nil
		}
		// 连接无效，关闭并重新连接
		c.client.Close()
		c.client = nil
		c.connected = false
	}

	var auth []ssh.AuthMethod

	// 如果提供了密码，使用密码认证
	if c.Password != "" {
		auth = append(auth, ssh.Password(c.Password))
	}

	// 如果提供了私钥，使用私钥认证
	if c.PrivKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.PrivKey))
		if err != nil {
			return fmt.Errorf("解析SSH私钥失败: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User:            c.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second, // 增加超时时间
	}

	addr := net.JoinHostPort(c.Host, c.Port)
	var err error
	var client *ssh.Client
	maxRetries := 5
	for i := range maxRetries {
		client, err = ssh.Dial("tcp", addr, config)
		if err != nil {
			if i < maxRetries-1 {
				fmt.Printf("ssh连接 %s 失败: %v, 正在重试(%d/%d)...\n", addr, err, i+1, maxRetries)
				time.Sleep(3 * time.Second)
				continue
			}
			return fmt.Errorf("ssh连接失败: %w", err)
		}
		c.client = client
		c.connected = true
		fmt.Printf("ssh连接 %s 成功\n", addr)
		return nil
	}
	return fmt.Errorf("ssh连接失败: %w", err)
}

// testConnection 测试连接是否有效
func (c *Client) testConnection() error {
	if c.client == nil {
		return fmt.Errorf("SSH client is nil")
	}

	// 创建一个临时会话来测试连接
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	session.Close()
	return nil
}

// Close 关闭SSH连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		c.connected = false
		return c.client.Close()
	}
	return nil
}

// RunCommand 在SSH服务器上执行命令，包含重试机制
func (c *Client) RunCommand(command string) (string, error) {
	maxRetries := 3
	var lastErr error
	var output string

	for i := 0; i < maxRetries; i++ {
		// 确保连接
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		// 创建会话
		session, err := c.client.NewSession()
		if err != nil {
			lastErr = fmt.Errorf("创建SSH会话失败: %w", err)
			// 连接可能已断开，强制重连
			c.mu.Lock()
			if c.client != nil {
				c.client.Close()
				c.client = nil
			}
			c.connected = false
			c.mu.Unlock()

			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		// 确保会话关闭
		defer session.Close()

		// 执行命令
		out, err := session.CombinedOutput(command)
		if err != nil {
			lastErr = fmt.Errorf("执行命令失败: %w, 输出: %s", err, string(out))
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		output = string(out)
		return output, nil
	}

	return "", lastErr
}

// TransferFile 将本地文件传输到远程服务器
func (c *Client) TransferFile(localPath, remotePath string) error {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// 确保连接
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// 读取本地文件
		data, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("读取本地文件失败: %w", err)
		}

		// 创建SCP会话
		session, err := c.client.NewSession()
		if err != nil {
			lastErr = fmt.Errorf("创建SSH会话失败: %w", err)
			// 连接可能已断开，强制重连
			c.mu.Lock()
			if c.client != nil {
				c.client.Close()
				c.client = nil
			}
			c.connected = false
			c.mu.Unlock()

			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}
		defer session.Close()

		// 创建远程文件
		cmd := fmt.Sprintf("cat > %s", remotePath)
		stdin, err := session.StdinPipe()
		if err != nil {
			lastErr = fmt.Errorf("创建标准输入管道失败: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		if err := session.Start(cmd); err != nil {
			lastErr = fmt.Errorf("启动SCP会话失败: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// 写入文件内容
		if _, err := stdin.Write(data); err != nil {
			lastErr = fmt.Errorf("写入文件数据失败: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}
		stdin.Close()

		if err := session.Wait(); err != nil {
			lastErr = fmt.Errorf("传输文件等待完成失败: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		return nil
	}

	return lastErr
}
