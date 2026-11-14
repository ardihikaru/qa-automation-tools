package gaossync

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// ProductIDRe is the shape of a GAOS product id.
var ProductIDRe = regexp.MustCompile(`^p-[0-9]+$`)

// MaxProductIDLen is the width of product_settings.gaos_product_id.
const MaxProductIDLen = 40

// ValidProductID reports whether s is a GAOS product id (p-6, p-12, ...).
func ValidProductID(s string) bool { return len(s) <= MaxProductIDLen && ProductIDRe.MatchString(s) }

var vpnNet = netip.MustParsePrefix("10.10.0.0/24")

// ValidateConfig checks GAOS_API_URL and GAOS_API_TOKEN and returns the normalised base URL
// (trailing slashes trimmed). Both empty is "not configured" and returns ("", nil) so the caller
// can tell it from a bad value. The error text never contains the token.
func ValidateConfig(apiURL, token string) (string, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" && token == "" {
		return "", nil
	}
	if apiURL == "" {
		return "", errors.New("GAOS_API_URL is empty")
	}
	if token == "" {
		return "", errors.New("GAOS_API_TOKEN is empty")
	}
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", errors.New("GAOS_API_URL is not a valid URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("GAOS_API_URL must not carry userinfo, a query or a fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		ip, err := netip.ParseAddr(u.Hostname())
		if err != nil || !(ip.IsLoopback() || vpnNet.Contains(ip)) {
			return "", errors.New("GAOS_API_URL must be https (http is allowed only to a loopback or 10.10.0.0/24 IP address)")
		}
	default:
		return "", errors.New("GAOS_API_URL must be an https URL")
	}
	if !strings.HasPrefix(token, "gaos_") || len(token) < 37 {
		return "", errors.New("GAOS_API_TOKEN must start with gaos_ and have at least 37 characters")
	}
	for _, c := range token {
		if c <= ' ' || c > '~' {
			return "", errors.New("GAOS_API_TOKEN contains a space or a non-printable character")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}
