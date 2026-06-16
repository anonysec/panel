package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// AgentVersionResponse is the response from the Panel API version endpoint.
type AgentVersionResponse struct {
	OK       bool   `json:"ok"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	Checksum string `json:"checksum_sha256"`
}

// Updater handles checking for and applying node agent binary updates.
type Updater struct {
	panelURL       string
	nodeToken      string
	currentVersion string
	client         *http.Client
}

// New creates a new Updater instance.
func New(panelURL, nodeToken, currentVersion string, client *http.Client) *Updater {
	return &Updater{
		panelURL:       strings.TrimRight(panelURL, "/"),
		nodeToken:      nodeToken,
		currentVersion: currentVersion,
		client:         client,
	}
}

// CheckAndUpdate queries the panel for the latest agent version. If a newer
// version is available, it downloads the binary, verifies its SHA-256 checksum,
// replaces the current binary, and restarts via systemctl.
func (u *Updater) CheckAndUpdate() error {
	// 1. Query version endpoint
	versionURL := u.panelURL + "/api/node/agent/version"
	req, err := http.NewRequest(http.MethodGet, versionURL, nil)
	if err != nil {
		return fmt.Errorf("updater: create version request: %w", err)
	}
	req.Header.Set("X-Node-Token", u.nodeToken)

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("updater: version request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("updater: version endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var versionResp AgentVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionResp); err != nil {
		return fmt.Errorf("updater: decode version response: %w", err)
	}

	if !versionResp.OK {
		return fmt.Errorf("updater: version response not ok")
	}

	// 2. Compare versions — only update if remote is newer
	if !CompareVersions(u.currentVersion, versionResp.Version) {
		return nil // already up to date
	}

	// 3. Download new binary
	downloadURL := versionResp.URL
	if downloadURL == "" {
		downloadURL = u.panelURL + "/api/node/agent/download"
	}

	dlReq, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("updater: create download request: %w", err)
	}
	dlReq.Header.Set("X-Node-Token", u.nodeToken)

	dlResp, err := u.client.Do(dlReq)
	if err != nil {
		return fmt.Errorf("updater: download request failed: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(dlResp.Body, 512))
		return fmt.Errorf("updater: download endpoint returned %d: %s", dlResp.StatusCode, string(body))
	}

	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return fmt.Errorf("updater: read download body: %w", err)
	}

	// 4. Verify SHA-256 checksum
	if !VerifyChecksum(data, versionResp.Checksum) {
		actualHash := sha256.Sum256(data)
		return fmt.Errorf("updater: checksum mismatch: expected %s, got %s",
			versionResp.Checksum, hex.EncodeToString(actualHash[:]))
	}

	// 5. Write to temp file
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: get executable path: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "node-agent-update-*")
	if err != nil {
		return fmt.Errorf("updater: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("updater: write temp file: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("updater: chmod temp file: %w", err)
	}

	// 6. Rename over current binary
	if err := os.Rename(tmpPath, binaryPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("updater: rename binary: %w", err)
	}

	// 7. Restart via systemctl
	cmd := exec.Command("systemctl", "restart", "node-agent")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updater: systemctl restart: %w", err)
	}

	return nil
}

// VerifyChecksum computes the SHA-256 hash of data and compares it to the
// expected hex-encoded checksum string.
func VerifyChecksum(data []byte, expected string) bool {
	h := sha256.Sum256(data)
	actual := hex.EncodeToString(h[:])
	return strings.EqualFold(actual, strings.TrimSpace(expected))
}

// CompareVersions returns true if remote version is greater than current version.
// Supports formats "v1.2.3" and "1.2.3". Comparison is numeric on major.minor.patch.
func CompareVersions(current, remote string) bool {
	curMajor, curMinor, curPatch, okCur := parseVersion(current)
	remMajor, remMinor, remPatch, okRem := parseVersion(remote)

	if !okCur || !okRem {
		return false
	}

	if remMajor != curMajor {
		return remMajor > curMajor
	}
	if remMinor != curMinor {
		return remMinor > curMinor
	}
	return remPatch > curPatch
}

// parseVersion parses a semver string like "v1.2.3" or "1.2.3" into major, minor, patch.
func parseVersion(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, false
	}

	return major, minor, patch, true
}
