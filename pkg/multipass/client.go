package multipass

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client 是 Multipass 命令行工具的封装
type Client struct {
	// 可以添加配置选项
	Image     string
	Password  string
	SSHKey    string
	SSHPubKey string
}

// NewClient 创建一个新的 Multipass 客户端
func NewClient(image, password, sshKey, sshPubKey string) (*Client, error) {
	// 确保 multipass 已安装
	if err := checkMultipassInstalled(); err != nil {
		return nil, err
	}
	return &Client{
		Image:     image,
		Password:  password,
		SSHKey:    sshKey,
		SSHPubKey: sshPubKey,
	}, nil
}

// checkMultipassInstalled 检查 multipass 命令是否存在
func checkMultipassInstalled() error {
	_, err := exec.LookPath("multipass")
	if err != nil {
		return fmt.Errorf("multipass 命令没有找到，请先安装 Multipass: %w", err)
	}
	return nil
}

func (c *Client) InfoVM(name string) error {
	cmd := exec.Command("multipass", "info", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("获取虚拟机信息失败: %w, 错误信息: %s", err, stderr.String())
	}
	return nil
}

// CreateVM 创建一个新的虚拟机
func (c *Client) CreateVM(name, cpus, memory, disk string) error {
	cmd := exec.Command("multipass", "launch", "--name", name, "--cpus", cpus, "--memory", memory, "--disk", disk, c.Image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("创建虚拟机失败: %w, 错误信息: %s", err, stderr.String())
	}
	return c.InitAuth(name)
}

func (c *Client) InitAuth(name string) error {
	// 设置ubuntu和root用户密码，并启用SSH密码认证
	// 原始命令: multipass exec "$VM_NAME" -- bash -c "echo 'ubuntu:$NEW_PASSWORD' | sudo chpasswd && echo 'root:$NEW_PASSWORD' | sudo chpasswd && echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd"
	passwordCmd := fmt.Sprintf("echo 'ubuntu:%s' | sudo chpasswd && echo 'root:%s' | sudo chpasswd && echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd", c.Password, c.Password)
	if _, err := c.ExecCommand(name, passwordCmd); err != nil {
		return fmt.Errorf("设置密码和SSH配置失败: %w", err)
	}

	// 创建.ssh目录并设置权限
	// 原始命令: multipass exec "$VM_NAME" -- sudo bash -c "mkdir -p /home/ubuntu/.ssh /root/.ssh && chmod 700 /home/ubuntu/.ssh /root/.ssh && chown ubuntu:ubuntu /home/ubuntu/.ssh"
	sshDirCmd := `sudo bash -c "mkdir -p /home/ubuntu/.ssh /root/.ssh && chmod 700 /home/ubuntu/.ssh /root/.ssh && chown ubuntu:ubuntu /home/ubuntu/.ssh"`
	if _, err := c.ExecCommand(name, sshDirCmd); err != nil {
		return fmt.Errorf("创建SSH目录失败: %w", err)
	}

	// 添加SSH公钥到authorized_keys
	if c.SSHPubKey != "" {
		// 原始命令: multipass exec "$VM_NAME" -- bash -c "echo '$SSH_PUB_KEY' >> /home/ubuntu/.ssh/authorized_keys"
		ubuntuKeyCmd := fmt.Sprintf("echo '%s' >> /home/ubuntu/.ssh/authorized_keys", c.SSHPubKey)
		if _, err := c.ExecCommand(name, ubuntuKeyCmd); err != nil {
			return fmt.Errorf("添加ubuntu用户SSH公钥失败: %w", err)
		}

		// 原始命令: multipass exec "$VM_NAME" -- sudo bash -c "echo '$SSH_PUB_KEY' >> /root/.ssh/authorized_keys"
		rootKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' >> /root/.ssh/authorized_keys\"", c.SSHPubKey)
		if _, err := c.ExecCommand(name, rootKeyCmd); err != nil {
			return fmt.Errorf("添加root用户SSH公钥失败: %w", err)
		}
	}

	// 复制SSH私钥
	if c.SSHKey != "" {
		// 原始命令: multipass exec "$VM_NAME" -- bash -c "echo '$SSH_KEY' > /home/ubuntu/.ssh/id_rsa && chmod 600 /home/ubuntu/.ssh/id_rsa && chown ubuntu:ubuntu /home/ubuntu/.ssh/id_rsa"
		ubuntuPrivKeyCmd := fmt.Sprintf("echo '%s' > /home/ubuntu/.ssh/id_rsa && chmod 600 /home/ubuntu/.ssh/id_rsa && chown ubuntu:ubuntu /home/ubuntu/.ssh/id_rsa", c.SSHKey)
		if _, err := c.ExecCommand(name, ubuntuPrivKeyCmd); err != nil {
			return fmt.Errorf("配置ubuntu用户SSH私钥失败: %w", err)
		}

		// 原始命令: multipass exec "$VM_NAME" -- sudo bash -c "echo '$SSH_KEY' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa"
		rootPrivKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa\"", c.SSHKey)
		if _, err := c.ExecCommand(name, rootPrivKeyCmd); err != nil {
			return fmt.Errorf("配置root用户SSH私钥失败: %w", err)
		}
	}

	return nil
}

// DeleteVM 删除一个虚拟机
func (c *Client) DeleteVM(name string) error {
	// 停止虚拟机
	cmd := exec.Command("multipass", "stop", name, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("停止虚拟机失败: %w, 错误信息: %s", err, stderr.String())
	}

	cmd = exec.Command("multipass", "delete", name)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("删除虚拟机失败: %w, 错误信息: %s", err, stderr.String())
	}

	// 清理虚拟机
	cmd = exec.Command("multipass", "purge")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("清理虚拟机资源失败: %w", err)
	}

	return nil
}

// ExecCommand 在指定的虚拟机上执行命令
func (c *Client) ExecCommand(vmName, command string) (string, error) {
	cmd := exec.Command("multipass", "exec", vmName, "--", "bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("在虚拟机 %s 上执行命令失败: %w, 错误信息: %s", vmName, err, stderr.String())
	}

	return stdout.String(), nil
}

// ListVMs 列出所有虚拟机
func (c *Client) ListVMs(prefix string) ([]string, error) {
	cmd := exec.Command("multipass", "list", "--format", "csv")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("列出虚拟机失败: %w, 错误信息: %s", err, stderr.String())
	}

	lines := strings.Split(stdout.String(), "\n")
	var vms []string

	// 跳过标题行
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) > 0 {
			vms = append(vms, parts[0])
		}
	}

	return vms, nil
}

// TransferFile 将本地文件传输到虚拟机
func (c *Client) TransferFile(localPath, vmName, remotePath string) error {
	cmd := exec.Command("multipass", "transfer", localPath, fmt.Sprintf("%s:%s", vmName, remotePath))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("传输文件到虚拟机失败: %w, 错误信息: %s", err, stderr.String())
	}

	return nil
}
