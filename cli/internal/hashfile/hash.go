package hashfile

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

type Result struct {
	Size        int64
	ContentRoot string
	ChunkHashes []string
}

func Compute(path string, chunkSize int) (Result, error) {
	if chunkSize <= 0 {
		return Result{}, fmt.Errorf("chunk size must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()

	whole := sha256.New()
	buffer := make([]byte, chunkSize)
	var result Result
	for {
		read, readErr := io.ReadFull(file, buffer)
		if read > 0 {
			chunk := buffer[:read]
			chunkDigest := sha256.Sum256(chunk)
			result.ChunkHashes = append(result.ChunkHashes, hex.EncodeToString(chunkDigest[:]))
			_, _ = whole.Write(chunk)
			result.Size += int64(read)
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return Result{}, readErr
		}
	}
	result.ContentRoot = hex.EncodeToString(whole.Sum(nil))
	return result, nil
}

func DedupKey(secret []byte, size int64, contentRoot string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "sha256-v1\x00%d\x00%s", size, contentRoot)
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(path, expected string) error {
	digest := sha256.New()
	if err := digestFile(path, digest); err != nil {
		return err
	}
	if hex.EncodeToString(digest.Sum(nil)) != expected {
		return fmt.Errorf("content checksum mismatch")
	}
	return nil
}

func digestFile(path string, destination hash.Hash) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}
