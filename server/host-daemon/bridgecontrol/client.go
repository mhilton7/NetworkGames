// Package bridgecontrol coordinates the small, fixed set of privileged
// transitions exposed by a WiiBridge Raspberry Pi controller.
package bridgecontrol

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const csrfPrefix = "wiibridge-setup-csrf\x00"

var allowedActions = map[string]bool{
	"detach": true, "disconnect": true, "connect-wii": true,
	"connect-gamecube-physical": true, "connect-gamecube-emulated": true,
	"attach": true,
}

// Client authenticates the Pi by an exact certificate pin and authenticates
// each request with the Pi's independently provisioned management token.
type Client struct {
	baseURL string
	token   string
	csrf    string
	http    *http.Client
}

func New(baseURL, token, certificatePath string) (*Client, error) {
	if baseURL == "" && token == "" && certificatePath == "" {
		return nil, nil
	}
	if baseURL == "" || token == "" || certificatePath == "" {
		return nil, errors.New("automatic Pi switching requires URL, token, and pinned certificate")
	}
	if len(token) < 12 {
		return nil, errors.New("Pi management token is too short")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Pi management URL must be a plain https URL")
	}
	certificatePEM, err := os.ReadFile(certificatePath)
	if err != nil {
		return nil, fmt.Errorf("read Pi certificate: %w", err)
	}
	block, _ := decodeCertificate(certificatePEM)
	if block == nil {
		return nil, errors.New("Pi certificate contains no X.509 certificate")
	}
	certificate, err := x509.ParseCertificate(block)
	if err != nil {
		return nil, fmt.Errorf("parse Pi certificate: %w", err)
	}
	pin := sha256.Sum256(certificate.Raw)
	if now := time.Now(); now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
		return nil, errors.New("Pi certificate is not currently valid")
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		// Hostname verification cannot be used with the Pi's device-unique,
		// self-signed setup certificate. VerifyPeerCertificate below performs
		// exact certificate pinning instead.
		InsecureSkipVerify: true, //nolint:gosec
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) != 1 {
				return errors.New("Pi returned an unexpected certificate chain")
			}
			got := sha256.Sum256(rawCerts[0])
			if subtle.ConstantTimeCompare(got[:], pin[:]) != 1 {
				return errors.New("Pi certificate pin mismatch")
			}
			if now := time.Now(); now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
				return errors.New("Pi certificate is not currently valid")
			}
			return nil
		},
	}
	csrf := sha256.Sum256([]byte(csrfPrefix + token))
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		csrf:    hex.EncodeToString(csrf[:]),
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("Pi controller redirects are forbidden")
			},
		},
	}, nil
}

func (c *Client) Action(ctx context.Context, action string) error {
	if c == nil {
		return errors.New("Pi switching is not configured")
	}
	if !allowedActions[action] {
		return errors.New("unsupported Pi action")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/action/"+action, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth("admin", c.token)
	request.Header.Set("X-WiiBridge-CSRF", c.csrf)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("Pi %s request: %w", action, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Pi %s returned HTTP %d", action, response.StatusCode)
	}
	return nil
}
