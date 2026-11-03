package accounts

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Only five hash characters leave the service. Padding reduces disclosure via
// response size; passwords and complete hashes are never sent to this API.
func (s *Service) checkBreached(ctx context.Context, value string) error {
	if !s.config.BreachCheck {
		return nil
	}
	sum := sha1.Sum([]byte(value))
	digest := fmt.Sprintf("%X", sum)
	request, err := http.NewRequestWithContext(ctx, "GET", "https://api.pwnedpasswords.com/range/"+digest[:5], nil)
	if err != nil {
		return err
	}
	request.Header.Set("Add-Padding", "true")
	request.Header.Set("User-Agent", "Pinelink-password-check")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return fault(503, "PASSWORD_CHECK_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fault(503, "PASSWORD_CHECK_UNAVAILABLE")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
	if err != nil || len(body) > 1048576 {
		return fault(503, "PASSWORD_CHECK_UNAVAILABLE")
	}
	for _, line := range strings.Split(string(body), "\n") {
		suffix, count, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && suffix == digest[5:] && count != "0" {
			return fault(400, "PASSWORD_COMMON")
		}
	}
	return nil
}
