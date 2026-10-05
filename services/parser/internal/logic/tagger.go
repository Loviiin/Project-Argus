package logic

import (
	"strings"
)

var tagKeywords = map[string]string{
	"vazamento": "vazamento",
	"pack":      "pack",
	"onlyfans":  "onlyfans",
	"privacy":   "privacy",
	"cp":        "cp",
	"child":     "cp",
	"lolita":    "cp",
	"loli":      "cp",
	"menor":     "cp",
	"conteúdo":  "conteúdo",
	"vendas":    "vendas",
	"comércio":  "comércio",
	"hack":      "hack",
	"cheat":     "cheat",
	"robux":     "gaming",
	"valorant":  "gaming",
	"minecraft": "gaming",
	"robô":      "bot",
	"cassino":   "cassino",
	"tigrinho":  "cassino",
}

func AutoTag(serverName, ocrText string) string {
	combined := strings.ToLower(serverName + " " + ocrText)
	tags := make(map[string]bool)

	for keyword, tag := range tagKeywords {
		if strings.Contains(combined, keyword) {
			tags[tag] = true
		}
	}

	var tagList []string
	for tag := range tags {
		tagList = append(tagList, tag)
	}
	return strings.Join(tagList, ",")
}
