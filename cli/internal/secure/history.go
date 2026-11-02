package secure

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
)

func SealHistory(plaintext []byte, publicKeys []string) ([]byte, error) {
	if len(plaintext) > 32<<10 || len(publicKeys) < 1 || len(publicKeys) > 2 {
		return nil, fmt.Errorf("invalid history envelope")
	}
	var recipients []age.Recipient
	for _, key := range publicKeys {
		recipient, e := age.ParseX25519Recipient(key)
		if e != nil {
			return nil, e
		}
		recipients = append(recipients, recipient)
	}
	var buffer bytes.Buffer
	writer, e := age.Encrypt(&buffer, recipients...)
	if e != nil {
		return nil, e
	}
	if _, e = writer.Write(plaintext); e != nil {
		return nil, e
	}
	if e = writer.Close(); e != nil {
		return nil, e
	}
	return buffer.Bytes(), nil
}
func (i *Identity) OpenHistory(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) > 64<<10 {
		return nil, fmt.Errorf("invalid history envelope")
	}
	reader, e := age.Decrypt(bytes.NewReader(ciphertext), i.private)
	if e != nil {
		return nil, e
	}
	body, e := io.ReadAll(io.LimitReader(reader, 32<<10+1))
	if e != nil {
		return nil, e
	}
	if len(body) > 32<<10 {
		return nil, fmt.Errorf("history metadata exceeds limit")
	}
	return body, nil
}
