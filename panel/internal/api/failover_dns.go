package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// DNSUpdater is the interface for programmatic DNS record updates.
type DNSUpdater interface {
	// UpdateARecord changes the A record for a domain to point to a new IP.
	UpdateARecord(ctx context.Context, domain string, newIP string, ttl int) error
	// GetCurrentIP returns the IP currently set for the A record.
	GetCurrentIP(ctx context.Context, domain string) (string, error)
	// VerifyPropagation checks if DNS resolvers return the expected IP.
	VerifyPropagation(ctx context.Context, domain string, expectedIP string) (bool, error)
}

// CloudflareUpdater implements DNSUpdater using the Cloudflare API.
type CloudflareUpdater struct {
	apiToken string
	zoneID   string
	recordID string
	client   *http.Client
}

// cloudflareBaseURL is the base URL for the Cloudflare API v4.
const cloudflareBaseURL = "https://api.cloudflare.com/client/v4"

// maxRetries is the maximum number of retry attempts for retryable errors.
const maxRetries = 3

// cloudflareAPIResponse represents the standard Cloudflare API response structure.
type cloudflareAPIResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  json.RawMessage   `json:"result"`
}

// cloudflareError represents a single error from the Cloudflare API.
type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cloudflareDNSRecord represents a DNS record returned by the Cloudflare API.
type cloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// NewCloudflareUpdater creates a new CloudflareUpdater with the given credentials.
func NewCloudflareUpdater(apiToken, zoneID, recordID string) *CloudflareUpdater {
	return &CloudflareUpdater{
		apiToken: apiToken,
		zoneID:   zoneID,
		recordID: recordID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UpdateARecord updates the DNS A record via the Cloudflare API.
// PUT https://api.cloudflare.com/client/v4/zones/{zone}/dns_records/{record}
func (c *CloudflareUpdater) UpdateARecord(ctx context.Context, domain string, newIP string, ttl int) error {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, c.zoneID, c.recordID)

	body := fmt.Sprintf(`{"type":"A","name":%q,"content":%q,"ttl":%d,"proxied":false}`,
		domain, newIP, ttl)

	return c.doWithRetry(ctx, http.MethodPut, url, body)
}

// GetCurrentIP retrieves the current IP address from the Cloudflare DNS record.
func (c *CloudflareUpdater) GetCurrentIP(ctx context.Context, domain string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, c.zoneID, c.recordID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("cloudflare: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cloudflare: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", errors.New("invalid_token")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cloudflare: failed to read response: %w", err)
	}

	var apiResp cloudflareAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("cloudflare: failed to parse response: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return "", fmt.Errorf("cloudflare: API error: %s", apiResp.Errors[0].Message)
		}
		return "", errors.New("cloudflare: unknown API error")
	}

	var record cloudflareDNSRecord
	if err := json.Unmarshal(apiResp.Result, &record); err != nil {
		return "", fmt.Errorf("cloudflare: failed to parse record: %w", err)
	}

	return record.Content, nil
}

// VerifyPropagation checks if DNS resolvers return the expected IP for the domain.
func (c *CloudflareUpdater) VerifyPropagation(ctx context.Context, domain string, expectedIP string) (bool, error) {
	return verifyDNSPropagation(ctx, domain, expectedIP)
}

// doWithRetry executes an HTTP request with retry logic for 429 and 5xx errors.
// 401 → returns "invalid_token" error immediately
// 429 → exponential backoff (1s, 2s, 4s) up to 3 retries
// 5xx → retry up to 3 times with exponential backoff
func (c *CloudflareUpdater) doWithRetry(ctx context.Context, method, url, body string) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("cloudflare: failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("cloudflare: request failed: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("cloudflare: failed to read response: %w", err)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			// 401: invalid token — do not retry
			return errors.New("invalid_token")

		case resp.StatusCode == http.StatusTooManyRequests:
			// 429: rate limited — retry with exponential backoff
			lastErr = errors.New("cloudflare: rate limited (429)")
			log.Printf("[cloudflare] rate limited (429), attempt %d/%d", attempt+1, maxRetries+1)
			continue

		case resp.StatusCode >= 500:
			// 5xx: server error — retry with exponential backoff
			lastErr = fmt.Errorf("cloudflare: server error (%d)", resp.StatusCode)
			log.Printf("[cloudflare] server error %d, attempt %d/%d", resp.StatusCode, attempt+1, maxRetries+1)
			continue

		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			// Success — verify the API response
			var apiResp cloudflareAPIResponse
			if err := json.Unmarshal(respBody, &apiResp); err != nil {
				return fmt.Errorf("cloudflare: failed to parse response: %w", err)
			}
			if !apiResp.Success {
				if len(apiResp.Errors) > 0 {
					return fmt.Errorf("cloudflare: API error: %s", apiResp.Errors[0].Message)
				}
				return errors.New("cloudflare: unknown API error")
			}
			return nil

		default:
			// Other 4xx errors — do not retry
			var apiResp cloudflareAPIResponse
			if err := json.Unmarshal(respBody, &apiResp); err == nil && len(apiResp.Errors) > 0 {
				return fmt.Errorf("cloudflare: API error (%d): %s", resp.StatusCode, apiResp.Errors[0].Message)
			}
			return fmt.Errorf("cloudflare: unexpected status %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("cloudflare: max retries exceeded: %w", lastErr)
}

// ManualUpdater is a no-op implementation of DNSUpdater for manual DNS management.
// UpdateARecord returns nil (no-op); GetCurrentIP and VerifyPropagation use net.LookupHost.
type ManualUpdater struct{}

// UpdateARecord is a no-op for manual DNS management.
// It returns nil since the admin manages DNS records manually.
func (m *ManualUpdater) UpdateARecord(ctx context.Context, domain string, newIP string, ttl int) error {
	log.Printf("[dns-manual] Manual DNS update required: set A record for %s to %s (TTL %d)", domain, newIP, ttl)
	return nil
}

// GetCurrentIP resolves the domain using system DNS and returns the first IP found.
func (m *ManualUpdater) GetCurrentIP(ctx context.Context, domain string) (string, error) {
	ips, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err != nil {
		return "", fmt.Errorf("dns lookup failed for %s: %w", domain, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for %s", domain)
	}
	return ips[0], nil
}

// VerifyPropagation checks if DNS resolvers return the expected IP for the domain.
func (m *ManualUpdater) VerifyPropagation(ctx context.Context, domain string, expectedIP string) (bool, error) {
	return verifyDNSPropagation(ctx, domain, expectedIP)
}

// verifyDNSPropagation is a shared helper that checks if the domain resolves to the expected IP.
func verifyDNSPropagation(ctx context.Context, domain string, expectedIP string) (bool, error) {
	ips, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err != nil {
		return false, fmt.Errorf("dns lookup failed for %s: %w", domain, err)
	}
	for _, ip := range ips {
		if ip == expectedIP {
			return true, nil
		}
	}
	return false, nil
}
