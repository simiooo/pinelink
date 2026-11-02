package lan

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type StatusError struct {
	Status        int
	Code, Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

type Client struct {
	identity   *secure.Identity
	self, peer Peer
	address    string
	http       *http.Client
	root       *stateDirectory
	mu         sync.Mutex
	token      string
	expires    time.Time
	uses       int
}

func NewClient(identity *secure.Identity, peer Peer, directory string) (*Client, error) {
	if err := peer.validate(); err != nil {
		return nil, err
	}
	address, err := endpoint(peer.Address)
	if err != nil {
		return nil, err
	}
	_, expected, err := keyID(peer.SigningKey)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ResponseHeaderTimeout: 20 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConnsPerHost: 4,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}, VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) != 1 {
				return fmt.Errorf("unexpected local certificate chain")
			}
			key, ok := state.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
			if !ok || !bytes.Equal(key, expected) {
				return fmt.Errorf("local device public key mismatch")
			}
			return nil
		}}}
	root, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return nil, err
	}
	c := &Client{identity: identity, self: Describe(identity, "This device", ""), peer: peer, address: address.String(), root: root, http: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var ready struct {
		Peer Peer `json:"peer"`
	}
	if readJSON(root, "receiver.json", &ready) == nil && ready.Peer.ID == identity.LocalID() && ready.Peer.validate() == nil {
		c.self = ready.Peer
	}
	return c, nil
}
func (c *Client) Close()     { c.http.CloseIdleConnections(); _ = c.root.Close() }
func (c *Client) Peer() Peer { return c.peer }

func (c *Client) raw(ctx context.Context, method, path string, body []byte, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.address+"/local/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		nonce := randomToken()
		req.Header.Set("X-Pinelink-Local-Session", token)
		req.Header.Set("X-Pinelink-Local-Nonce", nonce)
		req.Header.Set("X-Pinelink-Local-Signature", c.identity.Sign(requestProof(token, nonce, method, req.URL.RequestURI(), body)))
	}
	// The request signature already binds each binary chunk's complete bytes.
	if method == "PUT" {
		sum := sha256Bytes(body)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Chunk-SHA256", sum)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 8<<10)).Decode(&e)
		return &StatusError{Status: response.StatusCode, Code: e.Code, Message: e.Message}
	}
	if out != nil {
		limit := int64(2 << 20)
		if strings.HasPrefix(path, "replication/") {
			limit = 12 << 20
		}
		return json.NewDecoder(io.LimitReader(response.Body, limit)).Decode(out)
	}
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 2<<20))
	return err
}

func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) && c.uses < 2048 {
		c.uses++
		return c.token, nil
	}
	var challenge struct {
		Nonce  string `json:"nonce"`
		Boot   string `json:"boot"`
		Target string `json:"target_id"`
	}
	if err := c.raw(ctx, "GET", "challenge?peer_id="+url.QueryEscape(c.self.ID), nil, "", &challenge); err != nil {
		return "", err
	}
	if challenge.Target != c.peer.ID || len(challenge.Nonce) != 43 || len(challenge.Boot) != 43 {
		return "", ErrUnauthorized
	}
	body, _ := json.Marshal(map[string]string{"peer_id": c.self.ID, "nonce": challenge.Nonce, "signature": c.identity.Sign(sessionProof(c.peer.ID, challenge.Boot, challenge.Nonce, c.self.ID))})
	var result struct {
		Token   string `json:"token"`
		Expires int    `json:"expires_in"`
		Peer    Peer   `json:"peer"`
	}
	if err := c.raw(ctx, "POST", "session", body, "", &result); err != nil {
		return "", err
	}
	if result.Peer.ID != c.peer.ID || result.Peer.SigningKey != c.peer.SigningKey || result.Peer.EncryptionKey != c.peer.EncryptionKey || len(result.Token) != 43 || result.Expires < 1 || result.Expires > 600 {
		return "", ErrUnauthorized
	}
	c.token, c.expires, c.uses = result.Token, time.Now().Add(time.Duration(result.Expires)*time.Second), 1
	return c.token, nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.session(ctx)
		if err != nil {
			return err
		}
		err = c.raw(ctx, method, path, body, token, out)
		var status *StatusError
		if !errors.As(err, &status) || status.Status != 401 {
			return err
		}
		c.mu.Lock()
		if c.token == token {
			c.token = ""
		}
		c.mu.Unlock()
	}
	return ErrUnauthorized
}

func (c *Client) Info(ctx context.Context) (Peer, error) {
	var p Peer
	err := c.raw(ctx, "GET", "info", nil, "", &p)
	if err == nil {
		if p.ID != c.peer.ID || p.SigningKey != c.peer.SigningKey || p.EncryptionKey != c.peer.EncryptionKey {
			err = ErrUnauthorized
		}
	}
	return p, err
}

func (c *Client) Pair(ctx context.Context, invitation Invitation, name string) (PairRequest, error) {
	if invitation.Peer.ID != c.peer.ID || invitation.Peer.SigningKey != c.peer.SigningKey || invitation.Peer.EncryptionKey != c.peer.EncryptionKey {
		return PairRequest{}, ErrUnauthorized
	}
	if name == "" {
		name = c.self.Name
	}
	self := Describe(c.identity, name, c.self.Address)
	if err := self.validate(); err != nil {
		return PairRequest{}, err
	}
	input := pairInput{Token: invitation.Token, Peer: self}
	input.Signature = c.identity.Sign(input.proof(c.peer.ID))
	body, _ := json.Marshal(input)
	var pair PairRequest
	err := c.raw(ctx, "POST", "pair", body, "", &pair)
	if err == nil && (!validTransferID(pair.ID) || pair.Peer.ID != self.ID || pair.Peer.SigningKey != self.SigningKey || pair.Peer.EncryptionKey != self.EncryptionKey || pair.Code != comparisonCode(input.Token, c.peer.ID, self.ID)) {
		return PairRequest{}, ErrUnauthorized
	}
	return pair, err
}
func (c *Client) PairStatus(ctx context.Context, id string) (PairRequest, error) {
	input := map[string]string{"id": id, "signature": c.identity.Sign([]byte("pinelink-pair-status-v1\n" + c.peer.ID + "\n" + id))}
	body, _ := json.Marshal(input)
	var pair PairRequest
	err := c.raw(ctx, "POST", "pair/status", body, "", &pair)
	return pair, err
}
func (c *Client) Remember() error { return writeJSON(c.root, "peer-"+c.peer.ID+".json", c.peer) }
func ReadContact(directory, id string) (Peer, error) {
	if len(id) != 64 {
		return Peer{}, fmt.Errorf("invalid peer id")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return Peer{}, err
	}
	root, err := openStateDirectory(directory, "")
	if err != nil {
		return Peer{}, err
	}
	defer root.Close()
	var peer Peer
	err = readJSON(root, "peer-"+id+".json", &peer)
	if err == nil {
		err = peer.validate()
		if peer.ID != id {
			err = ErrUnauthorized
		}
		if peer.Revoked {
			err = ErrForbidden
		}
	}
	return peer, err
}

func (c *Client) Transfer(ctx context.Context, id string) (Transfer, error) {
	if !validTransferID(id) {
		return Transfer{}, fmt.Errorf("invalid transfer id")
	}
	var t Transfer
	err := c.request(ctx, "GET", "transfers/"+id, nil, &t)
	return t, err
}
func (c *Client) Transfers(ctx context.Context) ([]Transfer, error) {
	list := []Transfer{}
	cursor := ""
	for len(list) <= 10_000 {
		var page transferPage
		if err := c.request(ctx, "GET", "transfers?cursor="+url.QueryEscape(cursor), nil, &page); err != nil {
			return nil, err
		}
		list = append(list, page.Items...)
		if page.Next == "" {
			return list, nil
		}
		if !validTransferID(page.Next) || page.Next <= cursor {
			return nil, ErrConflict
		}
		cursor = page.Next
	}
	return nil, fmt.Errorf("local history exceeds the transfer limit")
}
func (c *Client) Action(ctx context.Context, id, action string, version int64) (Transfer, error) {
	if !validTransferID(id) {
		return Transfer{}, fmt.Errorf("invalid transfer id")
	}
	body, _ := json.Marshal(map[string]int64{"expected_version": version})
	var t Transfer
	err := c.request(ctx, "POST", "transfers/"+id+"/"+url.PathEscape(action), body, &t)
	if err == nil && t.PeerID == c.self.ID && c.peer.ID != c.self.ID {
		var sent sentRecord
		if readErr := readJSON(c.root, sentName(id), &sent); readErr == nil && sent.Peer.ID == c.peer.ID && sent.Peer.SigningKey == c.peer.SigningKey {
			t = sendSnapshot(t, c.peer.ID)
			sent.Status = t
			if writeErr := writeJSON(c.root, sentName(id), sent); writeErr != nil && !errors.Is(writeErr, ErrConflict) {
				return t, writeErr
			}
			if t.terminal() {
				_ = c.root.Remove(sent.CipherName)
			}
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return t, readErr
		}
	}
	return t, err
}
func (c *Client) PairRequests(ctx context.Context) ([]PairRequest, error) {
	var p []PairRequest
	err := c.request(ctx, "GET", "pair-requests", nil, &p)
	return p, err
}
func (c *Client) PairAction(ctx context.Context, id, action string) (PairRequest, error) {
	if !validTransferID(id) {
		return PairRequest{}, fmt.Errorf("invalid pairing request")
	}
	var p PairRequest
	err := c.request(ctx, "POST", "pair-requests/"+id+"/"+url.PathEscape(action), nil, &p)
	return p, err
}
func (c *Client) Invite(ctx context.Context) (Invitation, error) {
	var i Invitation
	err := c.request(ctx, "POST", "invites", nil, &i)
	return i, err
}
func (c *Client) Forget(ctx context.Context, id string) error {
	if len(id) != 64 {
		return ErrForbidden
	}
	return c.request(ctx, "POST", "peers/"+url.PathEscape(id)+"/forget", nil, nil)
}

func sha256Bytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
