package lan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

// History reads local journals without contacting a device or cloud service.
// Completed sends remain verifiable after their ciphertext cache is removed.
func History(identity *secure.Identity, directory string) ([]Transfer, error) {
	root, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := root.keys("sent", "transfer")
	if err != nil {
		return nil, err
	}
	list := []Transfer{}
	for _, name := range entries {
		var current Transfer
		if strings.HasPrefix(name, "sent-") {
			var sent sentRecord
			if err = readJSON(root, name, &sent); err != nil {
				return nil, err
			}
			current = sent.Status
			if current.State == "COMPLETED" {
				if current.Receipt == nil || current.Receipt.Verify(sent.Peer, sent.Offer, identity.LocalID()) != nil || current.Receipt.PlainRoot != sent.PlainRoot || current.Receipt.PlainSize != sent.PlainSize {
					return nil, fmt.Errorf("invalid local delivery receipt")
				}
			}
			if name != sentName(current.ID) {
				return nil, ErrUnauthorized
			}
		} else if strings.HasPrefix(name, "transfer-") {
			if err = readJSON(root, name, &current); err != nil {
				return nil, err
			}
			if name != recordName(current.ID) {
				return nil, ErrUnauthorized
			}
			if current.State == "COMPLETED" && (current.Receipt == nil || current.Receipt.Verify(Describe(identity, "This device", ""), current.Offer, current.PeerID) != nil) {
				return nil, fmt.Errorf("invalid local delivery receipt")
			}
		} else {
			continue
		}
		if !validTransferID(current.ID) {
			return nil, ErrUnauthorized
		}
		record := newRecord(current)
		list = append(list, record.snapshot())
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ExpiresAt.After(list[j].ExpiresAt) })
	return list, nil
}
