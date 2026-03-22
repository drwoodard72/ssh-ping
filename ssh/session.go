package ssh

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"
	"golang.org/x/term"
)

type Config struct {
	Host        string
	Port        int
	User        string
	Password    string
	IdentFile   string
	Timeout     time.Duration
	SOCKS5Proxy string // optional, e.g. "127.0.0.1:9050" for Tor
	Insecure    bool   // skip host key verification
}

// connectResult holds the SSH client and any resources that must be closed
// after the client is done (e.g. the SSH agent connection).
type connectResult struct {
	client   *ssh.Client
	agentConn net.Conn
}

func Connect(cfg Config) (*connectResult, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	hostKeyCb, err := hostKeyCallback(cfg.Insecure)
	if err != nil {
		return nil, err
	}

	// Build auth methods in order: pubkey, keyboard-interactive, password
	var authMethods []ssh.AuthMethod
	var agentConn net.Conn

	// Try public key authentication
	if methods, conn, err := publicKeyAuth(cfg.IdentFile); err == nil {
		authMethods = append(authMethods, methods...)
		agentConn = conn
	}

	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	clientConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCb,
		Timeout:         cfg.Timeout,
	}

	result, err := dial(addr, cfg.SOCKS5Proxy, cfg.Timeout, clientConfig)
	if err == nil {
		result.agentConn = agentConn
		return result, nil
	}

	// If auth failed and we skipped a passphrase-protected -i key because
	// the agent was available, retry with the key loaded (prompt for passphrase).
	if agentConn != nil && cfg.IdentFile != "" && isAuthError(err) {
		fmt.Fprintf(os.Stderr, "Agent authentication failed, trying key file %s\n", cfg.IdentFile)
		method, keyErr := loadPrivateKey(cfg.IdentFile, true)
		if keyErr == nil {
			clientConfig.Auth = []ssh.AuthMethod{method}
			result, retryErr := dial(addr, cfg.SOCKS5Proxy, cfg.Timeout, clientConfig)
			if retryErr == nil {
				result.agentConn = agentConn
				return result, nil
			}
			err = retryErr
		}
	}

	if agentConn != nil {
		agentConn.Close()
	}
	return nil, err
}

func dial(addr, socks5Proxy string, timeout time.Duration, clientConfig *ssh.ClientConfig) (*connectResult, error) {
	if socks5Proxy != "" {
		dialer, err := proxy.SOCKS5("tcp", socks5Proxy, nil, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("socks5 dial failed: %w", err)
		}
		c, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ssh handshake failed: %w", err)
		}
		return &connectResult{client: ssh.NewClient(c, chans, reqs)}, nil
	}

	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	return &connectResult{client: client}, nil
}

func isAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain")
}

// publicKeyAuth returns auth methods and optionally the SSH agent connection
// (which must remain open for signing and be closed by the caller after use).
func publicKeyAuth(identFile string) ([]ssh.AuthMethod, net.Conn, error) {
	var methods []ssh.AuthMethod
	var agentConn net.Conn

	// Try SSH agent
	if agentSock := os.Getenv("SSH_AUTH_SOCK"); agentSock != "" {
		conn, err := net.Dial("unix", agentSock)
		if err == nil {
			ag := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
			agentConn = conn
		}
	}

	// If a specific identity file is provided, try loading it.
	// Only prompt for passphrase if no agent is available to fall back on.
	if identFile != "" {
		if method, err := loadPrivateKey(identFile, agentConn == nil); err == nil {
			methods = append(methods, method)
		}
	}

	// Try default SSH keys only if no agent and no explicit identity file
	if len(methods) == 0 {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			for _, name := range []string{"id_rsa", "id_ecdsa", "id_ed25519"} {
				keyPath := filepath.Join(homeDir, ".ssh", name)
				method, err := loadPrivateKey(keyPath, true)
				if err == nil {
					methods = append(methods, method)
					break
				}
			}
		}
	}

	if len(methods) == 0 {
		return nil, nil, fmt.Errorf("no public keys found")
	}

	return methods, agentConn, nil
}

func loadPrivateKey(path string, promptForPassphrase bool) (ssh.AuthMethod, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err == nil {
		return ssh.PublicKeys(signer), nil
	}

	if !promptForPassphrase {
		return nil, fmt.Errorf("key %s is passphrase-protected, skipping (agent available)", path)
	}

	passphrase, err := promptPassphrase(path)
	if err != nil {
		return nil, err
	}

	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("failed to unlock key %s: %w", path, err)
	}

	return ssh.PublicKeys(signer), nil
}

func promptPassphrase(keyPath string) (string, error) {
	fmt.Fprintf(os.Stderr, "Enter passphrase for %s: ", keyPath)

	// Read password without echo
	fd := int(syscall.Stdin)
	oldState, err := term.GetState(fd)
	if err != nil {
		// Fall back to unbuffered line reading if terminal mode unavailable.
		// Avoid bufio.NewReader here — it would consume extra bytes from
		// stdin that the TOFU host key prompt needs later.
		var passphrase []byte
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && buf[0] == '\n' {
				break
			}
			if n > 0 {
				passphrase = append(passphrase, buf[0])
			}
			if err != nil {
				break
			}
		}
		return string(passphrase), nil
	}
	defer term.Restore(fd, oldState)

	password, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "\n")
	return string(password), nil
}


func hostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home directory: %w", err)
	}
	knownHostsFile := filepath.Join(homeDir, ".ssh", "known_hosts")

	// Create known_hosts if it doesn't exist.
	if _, err := os.Stat(knownHostsFile); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(knownHostsFile), 0700); err != nil {
			return nil, fmt.Errorf("cannot create .ssh directory: %w", err)
		}
		f, err := os.OpenFile(knownHostsFile, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("cannot create known_hosts: %w", err)
		}
		f.Close()
	}

	cb, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read known_hosts (%s): %w", knownHostsFile, err)
	}

	// TOFU wrapper: prompt to accept unknown keys, reject changed keys.
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}

		if len(keyErr.Want) > 0 {
			// Check if the mismatch is a genuine key change (same algorithm,
			// different key) vs. an algorithm negotiation difference (server
			// presented a key type we haven't stored yet).
			presentedType := key.Type()
			for _, w := range keyErr.Want {
				if w.Key.Type() == presentedType {
					// Same key type, different key — genuine change, reject.
					return fmt.Errorf("WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED for %s. "+
						"This could mean someone is doing something nasty. "+
						"Host key verification failed", hostname)
				}
			}
			// Different key type — fall through to TOFU prompt to add it.
		}

		// Unknown host — prompt for trust on first use.
		keyType := keyTypeName(key.Type())
		fingerprint := ssh.FingerprintSHA256(key)
		fmt.Fprintf(os.Stderr, "The authenticity of host '%s (%s)' can't be established.\n",
			hostname, remote.String())
		fmt.Fprintf(os.Stderr, "%s key fingerprint is %s.\n", keyType, fingerprint)
		fmt.Fprintf(os.Stderr, "Are you sure you want to continue connecting (yes/no)? ")

		var answerBuf []byte
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && buf[0] == '\n' {
				break
			}
			if n > 0 {
				answerBuf = append(answerBuf, buf[0])
			}
			if err != nil {
				break
			}
		}
		answer := strings.TrimSpace(strings.ToLower(string(answerBuf)))

		if answer != "yes" {
			return fmt.Errorf("host key verification failed")
		}

		// Append to known_hosts.
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to update known_hosts: %w", err)
		}
		defer f.Close()

		if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
			return fmt.Errorf("failed to write to known_hosts: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Warning: Permanently added '%s' (%s) to the list of known hosts.\n",
			hostname, keyType)
		return nil
	}, nil
}

func keyTypeName(keyType string) string {
	switch keyType {
	case "ssh-rsa":
		return "RSA"
	case "ssh-ed25519":
		return "ED25519"
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return "ECDSA"
	case "ssh-dss":
		return "DSA"
	default:
		return strings.ToUpper(keyType)
	}
}

type Session struct {
	Client    *ssh.Client
	Sess      *ssh.Session
	agentConn net.Conn
}

func NewSession(cfg Config) (*Session, error) {
	if cfg.User == "" {
		currentUser, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to get current user: %w", err)
		}
		cfg.User = currentUser.Username
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	cr, err := Connect(cfg)
	if err != nil {
		return nil, err
	}

	sess, err := cr.client.NewSession()
	if err != nil {
		cr.client.Close()
		if cr.agentConn != nil {
			cr.agentConn.Close()
		}
		return nil, fmt.Errorf("new session failed: %w", err)
	}

	return &Session{
		Client:    cr.client,
		Sess:      sess,
		agentConn: cr.agentConn,
	}, nil
}

func (s *Session) OpenShell() (io.ReadWriter, error) {
	in, err := s.Sess.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe failed: %w", err)
	}

	out, err := s.Sess.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, fmt.Errorf("stdout pipe failed: %w", err)
	}

	// Request PTY for interactive shell
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := s.Sess.RequestPty("xterm", 24, 80, modes); err != nil {
		in.Close()
		return nil, fmt.Errorf("pty request failed: %w", err)
	}

	if err := s.Sess.Shell(); err != nil {
		in.Close()
		return nil, fmt.Errorf("shell start failed: %w", err)
	}

	return &readWriter{in: in, out: out}, nil
}

func (s *Session) Close() error {
	var err1, err2 error
	if s.Sess != nil {
		err1 = s.Sess.Close()
	}
	if s.Client != nil {
		err2 = s.Client.Close()
	}
	if s.agentConn != nil {
		s.agentConn.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

type readWriter struct {
	in  io.WriteCloser
	out io.Reader
}

func (rw *readWriter) Read(b []byte) (int, error) {
	return rw.out.Read(b)
}

func (rw *readWriter) Write(b []byte) (int, error) {
	return rw.in.Write(b)
}

func (rw *readWriter) Close() error {
	return rw.in.Close()
}
