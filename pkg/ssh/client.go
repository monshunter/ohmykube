package ssh

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client 是SSH客户端的封装
type Client struct {
	Host     string
	Port     string
	User     string
	Password string
	PrivKey  string
	client   *ssh.Client
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
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(c.Host, c.Port)
	var err error
	var client *ssh.Client
	for range 5 {
		client, err = ssh.Dial("tcp", addr, config)
		if err != nil {
			fmt.Printf("ssh连接 %s 失败: %v, 正在重试...\n", addr, err)
			time.Sleep(3 * time.Second)
			continue
		}
		c.client = client
		fmt.Printf("ssh连接 %s 成功\n", addr)
		return nil
	}
	return fmt.Errorf("ssh连接失败: %w", err)
}

// Close 关闭SSH连接
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// RunCommand 在SSH服务器上执行命令
func (c *Client) RunCommand(command string) (string, error) {
	if c.client == nil {
		if err := c.Connect(); err != nil {
			return "", err
		}
		defer c.Close()
	}

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return "", fmt.Errorf("执行命令失败: %w, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// TransferFile 将本地文件传输到远程服务器
func (c *Client) TransferFile(localPath, remotePath string) error {
	if c.client == nil {
		if err := c.Connect(); err != nil {
			return err
		}
		defer c.Close()
	}

	// 读取本地文件
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("读取本地文件失败: %w", err)
	}

	// 创建SCP会话
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	// 创建远程文件
	cmd := fmt.Sprintf("cat > %s", remotePath)
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建标准输入管道失败: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("启动SCP会话失败: %w", err)
	}

	// 写入文件内容
	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("写入文件数据失败: %w", err)
	}
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("传输文件等待完成失败: %w", err)
	}

	return nil
}
