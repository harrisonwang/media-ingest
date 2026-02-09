package ingest

import (
	"net/url"
	"strings"
)

// videoPlatform describes per-site behavior (cookies, auth signals, etc.).
// For now we only ship YouTube, but the goal is to make adding new platforms
// (e.g. bilibili) mostly a config change instead of a giant if/else.
type videoPlatform struct {
	ID   string
	Name string

	// MatchHosts are hostname suffixes used to detect the platform from a URL
	// (e.g. "youtube.com", "youtu.be").
	MatchHosts []string

	// LoginURL is the URL opened during `mingest auth <platform>`.
	LoginURL string

	// CookieDomainSuffixes define which cookie domains we will keep when persisting
	// cookie jars for this platform.
	CookieDomainSuffixes []string

	// AuthCookieNames are used as a heuristic to detect whether a cookie jar is
	// likely authenticated for this platform.
	AuthCookieNames []string
}

func (p videoPlatform) MatchesURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return false
	}
	for _, s := range p.MatchHosts {
		ss := strings.ToLower(strings.TrimSpace(s))
		if ss == "" {
			continue
		}
		if host == ss || strings.HasSuffix(host, "."+ss) {
			return true
		}
	}
	return false
}

func (p videoPlatform) AllowsCookieDomain(domain string) bool {
	// If not configured, don't filter.
	if len(p.CookieDomainSuffixes) == 0 {
		return true
	}

	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if d == "" {
		return false
	}
	for _, s := range p.CookieDomainSuffixes {
		ss := strings.ToLower(strings.TrimSpace(s))
		if ss == "" {
			continue
		}
		if d == ss || strings.HasSuffix(d, "."+ss) {
			return true
		}
	}
	return false
}

func (p videoPlatform) HasAuthSignals() bool {
	return len(p.AuthCookieNames) > 0
}

func supportedPlatforms() []videoPlatform {
	return []videoPlatform{
		youtubePlatform(),
	}
}

func platformByID(id string) (videoPlatform, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range supportedPlatforms() {
		if p.ID == id {
			return p, true
		}
	}
	return videoPlatform{}, false
}

// platformForURL returns the best matching platform for the given URL.
// The boolean indicates whether the platform is a known/built-in one.
func platformForURL(u *url.URL) (videoPlatform, bool) {
	for _, p := range supportedPlatforms() {
		if p.MatchesURL(u) {
			return p, true
		}
	}
	return videoPlatform{}, false
}
