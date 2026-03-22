package ssh

import (
	"bufio"
	"fmt"
	"io"
	"time"
)

type EchoResult struct {
	Sent      int
	Received  int
	Latencies []time.Duration
}

type readResult struct {
	b   byte
	err error
}

func RunEchoTest(shell io.ReadWriter, count int, limit, echoTimeout time.Duration, verbose int) (EchoResult, error) {
	result := EchoResult{
		Latencies: make([]time.Duration, 0, count),
	}

	// Set up raw mode on remote (no buffering, no echo), print a sentinel
	// byte, then start cat. The sentinel (0x06 ACK) is printed by printf
	// as part of the command chain, so it appears on stdout only after stty
	// has run. Cat starts immediately after, so any stdin we send after
	// seeing the sentinel is guaranteed to be read by cat.
	const sentinel = 0x06
	fmt.Fprintf(shell, "stty raw -echo && printf '\\006' && cat\n")

	// Single persistent reader goroutine. The done channel lets it exit once
	// ReadByte unblocks (which happens when the SSH session is closed by the
	// caller). Between RunEchoTest returning and session close, the goroutine
	// remains blocked in ReadByte — this is expected and harmless.
	done := make(chan struct{})
	defer close(done)
	byteCh := make(chan readResult, 1)
	reader := bufio.NewReader(shell)
	go func() {
		for {
			b, err := reader.ReadByte()
			select {
			case byteCh <- readResult{b, err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the sentinel to appear, discarding any shell banner/prompt noise.
	syncWait := 3 * echoTimeout
	if syncWait < 30*time.Second {
		syncWait = 30 * time.Second
	}
	syncTimeout := time.After(syncWait)
sync:
	for {
		select {
		case r := <-byteCh:
			if r.err != nil {
				return result, fmt.Errorf("sync failed: %w", r.err)
			}
			if r.b == sentinel {
				break sync
			}
		case <-syncTimeout:
			return result, fmt.Errorf("sync timeout waiting for remote shell")
		}
	}

	chars := "abcdefghijklmnopqrstuvwxyz"
	startTime := time.Now()
	timeout := time.NewTimer(echoTimeout)
	defer timeout.Stop()

	for i := 0; i < count; i++ {
		if limit > 0 && time.Since(startTime) > limit {
			break
		}

		char := chars[i%len(chars)]

		if _, err := fmt.Fprintf(shell, "%c", char); err != nil {
			return result, fmt.Errorf("send failed: %w", err)
		}
		sendTime := time.Now()
		result.Sent++

		if !timeout.Stop() {
			select {
			case <-timeout.C:
			default:
			}
		}
		timeout.Reset(echoTimeout)

		select {
		case r := <-byteCh:
			if r.err != nil && r.err != io.EOF {
				return result, fmt.Errorf("read failed: %w", r.err)
			}
			latency := time.Since(sendTime)
			result.Latencies = append(result.Latencies, latency)
			result.Received++

			if verbose > 1 {
				fmt.Printf("%c: %.3f ms\n", char, latency.Seconds()*1000)
			}

		case <-timeout.C:
			if verbose > 1 {
				fmt.Printf("%c: timeout\n", char)
			}
			// Drain any late response to prevent desync — the stale
			// byte would otherwise be read as the next char's echo.
			// Scale drain window with echoTimeout for slow links.
			drainWait := echoTimeout / 10
			if drainWait < 500*time.Millisecond {
				drainWait = 500 * time.Millisecond
			}
			if drainWait > 2*time.Second {
				drainWait = 2 * time.Second
			}
			drainTimer := time.NewTimer(drainWait)
			select {
			case <-byteCh:
				drainTimer.Stop()
			case <-drainTimer.C:
			}
		}

		if verbose == 1 && i%10 == 0 {
			fmt.Printf("\rEcho: %d sent, %d received", result.Sent, result.Received)
		}
	}

	if verbose == 1 {
		fmt.Println()
	}

	io.WriteString(shell, "\n")
	io.WriteString(shell, "exit\n")

	return result, nil
}
