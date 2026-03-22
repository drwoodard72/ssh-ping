package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/drwoodard72/ssh-ping/ssh"
	"github.com/drwoodard72/ssh-ping/stats"
)

const version = "1.0.0"

func main() {
	// Define flags
	versionFlag := flag.Bool("V", false, "print version and exit")
	countFlag := flag.Int("c", 100, "number of echo characters to send")
	timeFlag := flag.Duration("t", 0, "time limit for echo test")
	echoTimeoutFlag := flag.Duration("w", 10*time.Second, "per-echo timeout")
	sizeFlag := flag.Int("s", 8, "size in MB for speed test")
	remoteFlag := flag.String("r", "/tmp/sshping-test", "remote test file path")
	identFlag := flag.String("i", "", "identity/key file")
	portFlag := flag.Int("p", 22, "SSH port")
	userFlag := flag.String("u", "", "SSH user")
	proxyFlag := flag.String("proxy", "", "SOCKS5 proxy address (e.g. 127.0.0.1:9050 for Tor)")
	insecureFlag := flag.Bool("insecure", false, "skip host key verification")
	echoOnlyFlag := flag.Bool("e", false, "echo test only")
	speedOnlyFlag := flag.Bool("b", false, "speed test only")
	humanFlag := flag.Bool("H", false, "human-readable output")
	delimFlag := flag.Bool("d", false, "delimited numbers")
	verboseFlag := flag.Int("v", 0, "verbosity level (1 or 2)")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("ssh-ping %s\n", version)
		os.Exit(0)
	}

	verbosity := *verboseFlag

	// Get target from remaining args
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: ssh-ping [options] [user@]host[:port]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	target := flag.Arg(0)

	// Parse [user@]host[:port]
	host, port, username := parseTarget(target, *userFlag, *portFlag)

	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			log.Fatalf("failed to get current user: %v", err)
		}
		username = currentUser.Username
	}

	// Connect
	cfg := ssh.Config{
		Host:        host,
		Port:        port,
		User:        username,
		IdentFile:   *identFlag,
		Timeout:     10 * time.Second,
		SOCKS5Proxy: *proxyFlag,
		Insecure:    *insecureFlag,
	}

	sess, err := ssh.NewSession(cfg)
	if err != nil {
		log.Fatalf("connection failed: %v", err)
	}
	defer sess.Close()

	// Run tests
	if !*speedOnlyFlag {
		runEchoTest(sess, *countFlag, *timeFlag, *echoTimeoutFlag, verbosity, *humanFlag, *delimFlag)
	}

	if !*echoOnlyFlag {
		runSpeedTest(sess.Client, *remoteFlag, *sizeFlag, *humanFlag, *delimFlag)
	}
}

func parseTarget(target, userFlag string, portFlag int) (string, int, string) {
	var host string
	var port int = portFlag
	var user string = userFlag

	// Check for user@host
	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		user = parts[0]
		target = parts[1]
	}

	// Check for host:port (handles IPv6 bracket notation via net.SplitHostPort)
	if h, p, err := net.SplitHostPort(target); err == nil {
		host = h
		if pn, err := strconv.Atoi(p); err == nil {
			port = pn
		}
	} else {
		host = target
	}

	return host, port, user
}

func runEchoTest(sess *ssh.Session, count int, limit, echoTimeout time.Duration, verbosity int, human, delim bool) {
	shell, err := sess.OpenShell()
	if err != nil {
		log.Fatalf("failed to open shell: %v", err)
	}

	result, err := ssh.RunEchoTest(shell, count, limit, echoTimeout, verbosity)
	if err != nil {
		log.Fatalf("echo test failed: %v", err)
	}

	fmt.Println("\nEcho results:")
	fmt.Printf("  Sent:     %d\n", result.Sent)
	fmt.Printf("  Received: %d\n", result.Received)

	if len(result.Latencies) > 0 {
		min := stats.Min(result.Latencies)
		max := stats.Max(result.Latencies)
		mean := stats.Mean(result.Latencies)
		median := stats.Median(result.Latencies)
		stddev := stats.StdDev(result.Latencies)

		fmt.Printf("  Min:      %s\n", fmtDuration(min, human, delim))
		fmt.Printf("  Max:      %s\n", fmtDuration(max, human, delim))
		fmt.Printf("  Mean:     %s\n", fmtDuration(mean, human, delim))
		fmt.Printf("  Median:   %s\n", fmtDuration(median, human, delim))
		fmt.Printf("  StdDev:   %s\n", fmtDuration(stddev, human, delim))
	}
}

func runSpeedTest(client *gossh.Client, remotePath string, sizeMB int, human, delim bool) {
	fmt.Println("\nSpeed test:")

	upload, err := ssh.RunUploadTest(client, remotePath, sizeMB)
	if err != nil {
		log.Printf("upload test failed: %v", err)
	} else {
		fmt.Printf("  Upload:   %s\n", fmtThroughput(upload, human, delim))
	}

	download, err := ssh.RunDownloadTest(client, remotePath, sizeMB)
	if err != nil {
		log.Printf("download test failed: %v", err)
	} else {
		fmt.Printf("  Download: %s\n", fmtThroughput(download, human, delim))
	}
}

func fmtDuration(d time.Duration, human, delim bool) string {
	if delim {
		return fmt.Sprintf("%d", d.Nanoseconds())
	}
	if human {
		ns := d.Nanoseconds()
		if ns < 1000 {
			return fmt.Sprintf("%d ns", ns)
		} else if ns < 1000000 {
			return fmt.Sprintf("%.2f µs", float64(ns)/1000)
		} else if ns < 1000000000 {
			return fmt.Sprintf("%.2f ms", float64(ns)/1000000)
		} else {
			return fmt.Sprintf("%.2f s", float64(ns)/1000000000)
		}
	}
	return d.String()
}

func fmtThroughput(mbps float64, human, delim bool) string {
	if delim {
		return fmt.Sprintf("%.6f", mbps)
	}
	if human {
		if mbps >= 1024 {
			return fmt.Sprintf("%.2f GB/s", mbps/1024)
		} else if mbps >= 1 {
			return fmt.Sprintf("%.2f MB/s", mbps)
		}
		return fmt.Sprintf("%.2f KB/s", mbps*1024)
	}
	return fmt.Sprintf("%.2f MB/s", mbps)
}
