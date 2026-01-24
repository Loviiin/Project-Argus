package logic

import (
	"regexp"
	"strings"
)

type DiscordFinder struct {
	ggRegex      *regexp.Regexp
	inviteRegex  *regexp.Regexp
	channelRegex *regexp.Regexp
}

func NewDiscordFinder() *DiscordFinder {
	ggPattern := `(?i)(?:https?://)?discord(?:\.|\s*dot\s*)gg\s*/\s*([a-zA-Z0-9-]{2,})`
	invitePattern := `(?i)(?:https?://)?discord(?:\.|\s*dot\s*)com\s*/\s*invite\s*/\s*([a-zA-Z0-9-]{2,})`
	channelPattern := `(?i)(?:https?://)?discord(?:\.|\s*dot\s*)com\s*/\s*channels\s*/\s*([0-9]{5,})\s*/\s*([0-9]{5,})(?:\s*/\s*([0-9]{5,}))?`
	return &DiscordFinder{
		ggRegex:      regexp.MustCompile(ggPattern),
		inviteRegex:  regexp.MustCompile(invitePattern),
		channelRegex: regexp.MustCompile(channelPattern),
	}
}

func (f *DiscordFinder) FindInvites(text string) []string {
	clean := normalizeText(text)

	var results []string
	seen := make(map[string]bool)

	ggMatches := f.ggRegex.FindAllStringSubmatch(clean, -1)
	for _, match := range ggMatches {
		if len(match) > 1 {
			code := match[1]
			value := "discord.gg/" + code
			if !seen[value] {
				results = append(results, value)
				seen[value] = true
			}
		}
	}

	inviteMatches := f.inviteRegex.FindAllStringSubmatch(clean, -1)
	for _, match := range inviteMatches {
		if len(match) > 1 {
			code := match[1]
			value := "discord.com/invite/" + code
			if !seen[value] {
				results = append(results, value)
				seen[value] = true
			}
		}
	}

	channelMatches := f.channelRegex.FindAllStringSubmatch(clean, -1)
	for _, m := range channelMatches {
		if len(m) > 2 {
			guild := m[1]
			channel := m[2]
			link := "discord.com/channels/" + guild + "/" + channel
			if len(m) > 3 && m[3] != "" {
				link += "/" + m[3]
			}
			if !seen[link] {
				results = append(results, link)
				seen[link] = true
			}
		}
	}

	return results
}

func normalizeText(text string) string {
	replacer := strings.NewReplacer("\u200b", "", "\uFEFF", "")
	text = replacer.Replace(text)

	text = regexp.MustCompile(`(?i)discord\s*\.\s*gg`).ReplaceAllString(text, "discord.gg")
	text = regexp.MustCompile(`(?i)discord\s*\.\s*com\s*/\s*invite`).ReplaceAllString(text, "discord.com/invite")
	text = regexp.MustCompile(`(?i)discord\s*\.\s*com\s*/\s*channels`).ReplaceAllString(text, "discord.com/channels")

	text = regexp.MustCompile(`(?i)discord\.gg\s*/\s*`).ReplaceAllString(text, "discord.gg/")
	text = regexp.MustCompile(`(?i)discord\.com/invite\s*/\s*`).ReplaceAllString(text, "discord.com/invite/")
	text = regexp.MustCompile(`(?i)discord\.com/channels\s*/\s*`).ReplaceAllString(text, "discord.com/channels/")
	text = regexp.MustCompile(`(?i)discord\.com/channels/([0-9]{5,})\s*/\s*([0-9]{5,})`).ReplaceAllString(text, "discord.com/channels/$1/$2")
	text = regexp.MustCompile(`(?i)discord\.com/channels/([0-9]{5,})/([0-9]{5,})\s*/\s*([0-9]{5,})`).ReplaceAllString(text, "discord.com/channels/$1/$2/$3")

	return text
}
