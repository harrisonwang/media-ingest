package ingest

func youtubePlatform() videoPlatform {
	return videoPlatform{
		ID:   "youtube",
		Name: "YouTube",
		MatchHosts: []string{
			"youtube.com",
			"youtu.be",
		},
		LoginURL: "https://www.youtube.com",
		// YouTube authentication cookies are typically on google.com, while playback happens on youtube.com.
		CookieDomainSuffixes: []string{
			"youtube.com",
			"google.com",
		},
		AuthCookieNames: []string{
			"SAPISID",
			"SID",
			"__Secure-3PSID",
			"__Secure-1PSID",
		},
	}
}

