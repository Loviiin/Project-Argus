package tiktok

import (
	"regexp"
	"strings"
)

var (
	// Captura discord.gg/xxx, discord.com/invite/xxx, discordapp.com/invite/xxx
	discordLinkRe = regexp.MustCompile(
		`(?i)discord(?:\.gg|(?:app)?\.com/invite)/([A-Za-z0-9\-_]{2,20})`,
	)

	// Captura linktree, beacons.ai, linktr.ee, bio.link, etc.
	aggregatorRe = regexp.MustCompile(
		`(?i)(?:linktr\.ee|linktree\.com|beacons\.ai|bio\.link|linkbio\.co)/[\w\-\.]+`,
	)
)

// ExtractDiscordLinks procura links do Discord num texto livre.
// Retorna uma lista de links normalizados (discord.gg/CODE).
func ExtractDiscordLinks(text string) []string {
	matches := discordLinkRe.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var links []string
	for _, m := range matches {
		link := "discord.gg/" + m[1]
		if !seen[link] {
			seen[link] = true
			links = append(links, link)
		}
	}
	return links
}

// ExtractAggregatorURLs encontra URLs de agregadores de links (Linktree, etc.) no texto.
func ExtractAggregatorURLs(text string) []string {
	matches := aggregatorRe.FindAllString(text, -1)
	var urls []string
	for _, m := range matches {
		if !strings.HasPrefix(m, "https://") {
			m = "https://" + m
		}
		urls = append(urls, m)
	}
	return urls
}
