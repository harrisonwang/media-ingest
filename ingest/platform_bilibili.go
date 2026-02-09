package ingest

func bilibiliPlatform() videoPlatform {
	return videoPlatform{
		ID:   "bilibili",
		Name: "Bilibili",
		MatchHosts: []string{
			"bilibili.com",
			"b23.tv",
		},
		LoginURL: "https://www.bilibili.com",
		CookieDomainSuffixes: []string{
			"bilibili.com",
		},
		// Signal cookie: SESSDATA is the session token used for logged-in access.
		AuthCookieNames: []string{
			"SESSDATA",
		},
	}
}

