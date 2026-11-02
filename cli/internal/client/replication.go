package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	rp "github.com/simiooo/pinelink/protocol/replication"
)

func (c *Client) BindReplication(ctx context.Context, b rp.Binding) (rp.Profile, error) {
	var p rp.Profile
	err := c.doJSON(ctx, "POST", "/v1/replication/bind", b, &p)
	return p, err
}
func (c *Client) ReplicationProfile(ctx context.Context, stream string) (rp.Profile, error) {
	var p rp.Profile
	err := c.doJSON(ctx, "GET", "/v1/replication/profile?stream="+url.QueryEscape(stream), nil, &p)
	return p, err
}
func (c *Client) ReplicationRoster(ctx context.Context, r rp.Roster) (rp.Profile, error) {
	var p rp.Profile
	err := c.doJSON(ctx, "PUT", "/v1/replication/peers", r, &p)
	return p, err
}
func (c *Client) UploadReplication(ctx context.Context, b rp.Upload) (rp.Ack, error) {
	var p rp.Ack
	err := c.doJSON(ctx, "POST", "/v1/replication/events", b, &p)
	return p, err
}
func (c *Client) ReadReplication(ctx context.Context, epoch string, after int64, snapshot bool, upper int64, key string) (rp.Page, error) {
	path := "/v1/replication/events"
	if snapshot {
		path = "/v1/replication/snapshot"
	}
	query := url.Values{"epoch": []string{epoch}, "after": []string{strconv.FormatInt(after, 10)}, "upper": []string{strconv.FormatInt(upper, 10)}, "limit": []string{"100"}, "key": []string{key}}
	var p rp.Page
	err := c.doJSON(ctx, "GET", path+"?"+query.Encode(), nil, &p)
	return p, err
}
func CheckReplicationProfile(p rp.Profile, pin, root string) error {
	if p.Root != root || !rp.ValidID(p.Epoch) || p.Uploaded < 0 || p.RosterVersion < 0 || !rp.Verify(pin, p.Proof(), p.Signature) {
		return fmt.Errorf("invalid signed replication profile")
	}
	return nil
}
