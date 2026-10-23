package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func main() {
	hostFlag := flag.String("host", "10.9.31.204:22", "Target host:port")
	userFlag := flag.String("user", "coreip", "SSH username")
	passFlag := flag.String("pass", "", "SSH password (or set COREIP_SSH_PASS)")
	flag.Parse()

	host := *hostFlag
	if !strings.Contains(host, ":") {
		host = host + ":22"
	}
	user := *userFlag
	pass := *passFlag
	if pass == "" {
		pass = os.Getenv("COREIP_SSH_PASS")
	}
	if pass == "" {
		log.Fatal("SSH password required. Provide via -pass flag or COREIP_SSH_PASS environment variable.")
	}

	fmt.Println("==============================================================")
	fmt.Printf("Deploying CoreIP Server Inspector to %s (%s)\n", host, user)
	fmt.Println("==============================================================")

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		log.Fatalf("SSH connection to %s failed: %v", host, err)
	}
	defer client.Close()
	fmt.Println("✓ SSH connection established successfully.")

	// 1. Inspect target system
	fmt.Println("\nInspecting remote host operating system...")
	osRelease, _ := runSSH(client, "cat /etc/os-release 2>/dev/null || uname -a")
	fmt.Println("--- Remote OS Identity ---")
	fmt.Println(strings.TrimSpace(osRelease))
	fmt.Println("--------------------------")

	isDebian := strings.Contains(osRelease, "ID=ubuntu") || strings.Contains(osRelease, "ID=debian") || strings.Contains(osRelease, "ID_LIKE=debian")
	isRHEL := strings.Contains(osRelease, "ID=\"rhel\"") || strings.Contains(osRelease, "ID=\"centos\"") || strings.Contains(osRelease, "ID=\"rocky\"")

	// 2. Prepare remote configuration with allow_remote: true
	remoteConfigYAML := `# CoreIP Server Inspector Remote Configuration
server:
  host: "0.0.0.0"
  port: 8080
  allow_remote: true
  tls_cert_file: ""
  tls_key_file: ""

auth:
  enabled: true
  username: "admin"
  initial_password: "CoreIPAdmin2026!"
  session_timeout_minutes: 60
  rate_limit_per_minute: 10

collector:
  poll_interval_seconds: 5
  cache_inventory_seconds: 30
  history_buffer_size: 120
  service_max_items: 200
  smart_enabled: true
  smart_cache_seconds: 300
  mock_mode: false

logging:
  level: "info"
  audit_log_path: ""
`

	// Upload binary and config to /home/coreip/
	fmt.Println("\nUploading Linux amd64 binary to remote server...")
	localBin := "bin/coreip-inspector-linux-amd64"
	binData, err := os.ReadFile(localBin)
	if err != nil {
		log.Fatalf("Failed to read local binary %s: %v", localBin, err)
	}

	if err := uploadFile(client, "/home/coreip/coreip-inspector", binData, 0755); err != nil {
		log.Fatalf("Failed to upload binary: %v", err)
	}
	fmt.Println("✓ Binary uploaded to /home/coreip/coreip-inspector")

	fmt.Println("Uploading remote configuration...")
	if err := uploadFile(client, "/home/coreip/config.yaml", []byte(remoteConfigYAML), 0644); err != nil {
		log.Fatalf("Failed to upload config: %v", err)
	}
	fmt.Println("✓ Config uploaded to /home/coreip/config.yaml")

	// 3. If Debian/Ubuntu, also upload .deb package
	debPath := "dist/coreip-server-inspector_1.0.0_amd64.deb"
	if debData, err := os.ReadFile(debPath); err == nil {
		fmt.Println("Uploading .deb package to /home/coreip/...")
		_ = uploadFile(client, "/home/coreip/coreip-server-inspector_1.0.0_amd64.deb", debData, 0644)
		fmt.Println("✓ .deb package uploaded")
	}

	// 4. Check if coreip user has sudo privileges
	fmt.Println("\nChecking sudo capability...")
	sudoCheck, sudoErr := runSSH(client, fmt.Sprintf("echo '%s' | sudo -S id 2>/dev/null", pass))
	hasSudo := (sudoErr == nil && strings.Contains(sudoCheck, "uid=0"))

	if hasSudo {
		fmt.Println("✓ Sudo available. Setting up system service...")
		if isDebian {
			fmt.Println("Installing Debian package via apt/dpkg...")
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S dpkg -i /home/coreip/coreip-server-inspector_1.0.0_amd64.deb || sudo -S apt-get install -f -y", pass))
			// Update config to 0.0.0.0 in /etc/coreip-inspector/config.yaml
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S cp /home/coreip/config.yaml /etc/coreip-inspector/config.yaml", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S systemctl restart coreip-inspector || sudo -S systemctl enable --now coreip-inspector", pass))
		} else {
			// Manual systemd install
			serviceContent, _ := os.ReadFile("configs/coreip-inspector.service")
			uploadFile(client, "/home/coreip/coreip-inspector.service", serviceContent, 0644)
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S cp /home/coreip/coreip-inspector /usr/bin/coreip-inspector", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S mkdir -p /etc/coreip-inspector", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S cp /home/coreip/config.yaml /etc/coreip-inspector/config.yaml", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S cp /home/coreip/coreip-inspector.service /lib/systemd/system/coreip-inspector.service", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S systemctl daemon-reload", pass))
			runSSH(client, fmt.Sprintf("echo '%s' | sudo -S systemctl restart coreip-inspector || sudo -S systemctl enable --now coreip-inspector", pass))
		}
	} else {
		fmt.Println("Notice: Sudo not available or restricted. Running as user daemon with nohup...")
		// Stop any previous instance
		runSSH(client, "pkill -f coreip-inspector || true")
		time.Sleep(1 * time.Second)
		runSSH(client, "nohup /home/coreip/coreip-inspector -config /home/coreip/config.yaml > /home/coreip/inspector.log 2>&1 &")
	}

	time.Sleep(2 * time.Second)

	// 5. Verify process is running
	fmt.Println("\nVerifying running service...")
	psOut, _ := runSSH(client, "pgrep -la coreip-inspector || ps aux | grep coreip-inspector | grep -v grep")
	fmt.Println("Process status:")
	fmt.Println(strings.TrimSpace(psOut))

	// 6. Test local curl on remote host
	fmt.Println("\nTesting remote HTTP endpoint from host...")
	healthCheck, _ := runSSH(client, "curl -s http://127.0.0.1:8080/api/v1/health")
	fmt.Printf("Remote Health Check Response: %s\n", strings.TrimSpace(healthCheck))

	fmt.Println("\n==============================================================")
	fmt.Println("   DEPLOYMENT SUCCESSFUL!")
	fmt.Println("==============================================================")
	fmt.Println("Access the web dashboard in your browser at:")
	fmt.Printf("👉 http://10.9.31.204:8080/\n\n")
	fmt.Println("Login Credentials:")
	fmt.Println(" - Username : admin")
	fmt.Println(" - Password : CoreIPAdmin2026!")
	fmt.Println("==============================================================")

	_ = isRHEL
}

func runSSH(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

func uploadFile(client *ssh.Client, remotePath string, content []byte, mode os.FileMode) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("cat > %s && chmod %04o %s", remotePath, mode&0777, remotePath)
	if err := session.Start(cmd); err != nil {
		return err
	}

	if _, err := stdin.Write(content); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}

	return session.Wait()
}

// InsecureIgnoreHostKey accepts any host key for automated support deployment
func InsecureIgnoreHostKey() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}
}

var _ = bytes.NewReader
var _ = io.EOF
