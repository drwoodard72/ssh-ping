package ssh

import (
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func RunUploadTest(client *ssh.Client, remotePath string, sizeMB int) (float64, error) {
	if sizeMB <= 0 {
		return 0, fmt.Errorf("size must be > 0, got %d", sizeMB)
	}
	sfTPClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, fmt.Errorf("sftp client creation failed: %w", err)
	}
	defer sfTPClient.Close()

	// Create remote file
	remoteFile, err := sfTPClient.Create(remotePath)
	if err != nil {
		return 0, fmt.Errorf("remote file creation failed: %w", err)
	}

	// Generate random data and write
	bufSize := 1024 * 1024 // 1 MB chunks
	totalBytes := int64(sizeMB) * 1024 * 1024
	buf := make([]byte, bufSize)

	// Fill with random data
	rand.Read(buf)

	written := int64(0)
	start := time.Now()

	for written < totalBytes {
		remaining := totalBytes - written
		if int64(bufSize) > remaining {
			buf = make([]byte, remaining)
			rand.Read(buf)
		}

		n, err := remoteFile.Write(buf)
		if err != nil {
			remoteFile.Close()
			sfTPClient.Remove(remotePath)
			return 0, fmt.Errorf("write failed: %w", err)
		}
		written += int64(n)
	}

	if err := remoteFile.Close(); err != nil {
		sfTPClient.Remove(remotePath)
		return 0, fmt.Errorf("close failed: %w", err)
	}
	elapsed := time.Since(start)
	sfTPClient.Remove(remotePath)

	bytesSec := float64(written) / elapsed.Seconds()
	return bytesSec / (1024 * 1024), nil // Convert to MB/s
}

func RunDownloadTest(client *ssh.Client, remotePath string, sizeMB int) (float64, error) {
	if sizeMB <= 0 {
		return 0, fmt.Errorf("size must be > 0, got %d", sizeMB)
	}
	sfTPClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, fmt.Errorf("sftp client creation failed: %w", err)
	}
	defer sfTPClient.Close()

	// First, create a file to download by uploading test data
	remoteFile, err := sfTPClient.Create(remotePath)
	if err != nil {
		return 0, fmt.Errorf("remote file creation failed: %w", err)
	}

	bufSize := 1024 * 1024
	totalBytes := int64(sizeMB) * 1024 * 1024
	buf := make([]byte, bufSize)
	rand.Read(buf)

	written := int64(0)
	for written < totalBytes {
		remaining := totalBytes - written
		if int64(bufSize) > remaining {
			buf = make([]byte, remaining)
			rand.Read(buf)
		}
		n, err := remoteFile.Write(buf)
		if err != nil {
			remoteFile.Close()
			sfTPClient.Remove(remotePath)
			return 0, fmt.Errorf("setup write failed: %w", err)
		}
		written += int64(n)
	}
	if err := remoteFile.Close(); err != nil {
		sfTPClient.Remove(remotePath)
		return 0, fmt.Errorf("setup close failed: %w", err)
	}

	// Ensure remote file is always cleaned up from this point on
	defer sfTPClient.Remove(remotePath)

	// Now download it
	downloadFile, err := sfTPClient.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("remote file open failed: %w", err)
	}
	defer downloadFile.Close()

	readBuf := make([]byte, bufSize)
	downloaded := int64(0)
	start := time.Now()

	for {
		n, err := downloadFile.Read(readBuf)
		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("read failed: %w", err)
		}
		downloaded += int64(n)
		if err == io.EOF {
			break
		}
	}

	elapsed := time.Since(start)

	bytesSec := float64(downloaded) / elapsed.Seconds()
	return bytesSec / (1024 * 1024), nil // Convert to MB/s
}
