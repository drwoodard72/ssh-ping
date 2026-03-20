# ssh-ping

A Go port of [sshping](https://github.com/spook/sshping) by Jim Scarborough — measures SSH echo latency and SFTP transfer throughput.

This project was built entirely using [Claude Code](https://claude.ai/claude-code) (Anthropic's CLI coding agent).

## Features

- **Echo latency test** — sends characters through an SSH shell and measures round-trip time
- **SFTP speed test** — measures upload and download throughput via SFTP
- **SOCKS5 proxy support** — route connections through a SOCKS5 proxy (e.g. Tor)
- **TOFU host key verification** — matches OpenSSH behavior: prompts for unknown hosts, appends to `~/.ssh/known_hosts`
- **Works over Tor** — tested against `.onion` hidden services

## Installation

```bash
go install github.com/drwoodard72/ssh-ping@latest
```

Or build from source:

```bash
git clone https://github.com/drwoodard72/ssh-ping.git
cd ssh-ping
go build -o ssh-ping .
```

## Usage

```
ssh-ping [options] [user@]host[:port]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-c` | 1000 | Number of echo characters to send |
| `-t` | 0 | Time limit for echo test (e.g. `30s`, `5m`) |
| `-w` | 10s | Per-echo timeout |
| `-s` | 8 | Size in MB for speed test |
| `-r` | `/tmp/sshping-test` | Remote test file path |
| `-i` | | Identity/key file |
| `-p` | 22 | SSH port |
| `-u` | current user | SSH user |
| `-proxy` | | SOCKS5 proxy address (e.g. `127.0.0.1:9050` for Tor) |
| `-insecure` | false | Skip host key verification |
| `-e` | false | Echo test only |
| `-b` | false | Speed test only |
| `-H` | false | Human-readable output |
| `-d` | false | Delimited (machine-parseable) numbers |
| `-v` | 0 | Verbosity level (1 = progress, 2 = per-char latency) |

### Examples

Basic echo + speed test:
```bash
ssh-ping myserver
```

Echo test only, 50 characters, human-readable output:
```bash
ssh-ping -e -c 50 -H user@myserver
```

Speed test with 16 MB file:
```bash
ssh-ping -b -s 16 myserver
```

Connect through Tor to a .onion address:
```bash
ssh-ping -proxy 127.0.0.1:9050 -p 2222 user@abcdef.onion
```

Verbose per-character latency:
```bash
ssh-ping -e -c 20 -v 2 myserver
```

### Sample Output

```
Echo: 50 sent, 50 received

Echo results:
  Sent:     50
  Received: 50
  Min:      10.65µs
  Max:      120.33µs
  Mean:     38.42µs
  Median:   21.33µs
  StdDev:   28.17µs

Speed test:
  Upload:   14.76 MB/s
  Download: 60.95 MB/s
```

## How It Works

**Echo test**: Opens an SSH shell, sets the remote terminal to raw mode (`stty raw -echo`), then runs `cat` to echo back characters one at a time. A sentinel byte (ACK, 0x06) synchronizes the start of measurement. Each character's round-trip latency is recorded.

**Speed test**: Uses SFTP to upload a random data file to the remote host, then downloads it back. Measures throughput in MB/s for each direction.

## Authentication

Authentication methods are tried in this order (additive — multiple methods can be used):

1. SSH agent (`SSH_AUTH_SOCK`)
2. Explicit key file (`-i`)
3. Default keys (`~/.ssh/id_rsa`, `id_ecdsa`, `id_ed25519`) — only if no agent or explicit key

## Credits

- **Original project**: [sshping](https://github.com/spook/sshping) by Jim Scarborough (C++)
- **Go port**: Built with [Claude Code](https://claude.ai/claude-code) by Anthropic

## License

See the original [sshping](https://github.com/spook/sshping) project for license terms.
