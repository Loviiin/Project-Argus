package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/redis/go-redis/v9"
)

type RodDiscordClient struct {
	browser     *rod.Browser
	token       string
	clientRedis *redis.Client
}

func NewRodDiscordClient(token string, rdb *redis.Client) *RodDiscordClient {
	path, _ := launcher.LookPath()

	u := launcher.New().
		Bin(path).
		Leakless(false).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		MustLaunch()

	browser := rod.New().ControlURL(u).MustConnect()
	log.Println("Discord Browser Scraper Inicializado com sucesso")

	return &RodDiscordClient{
		browser:     browser,
		token:       token,
		clientRedis: rdb,
	}
}

func (c *RodDiscordClient) GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error) {
	cacheKey := fmt.Sprintf("discord:invite:%s", inviteCode)
	val, err := c.clientRedis.Get(ctx, cacheKey).Result()
	if err == nil {
		var cachedInvite DiscordInviteResponse
		if err := json.Unmarshal([]byte(val), &cachedInvite); err == nil {
			fmt.Printf("[CACHE] Convite %s encontrado no Redis\n", inviteCode)
			return &cachedInvite, nil
		}
	}

	url := fmt.Sprintf("https://discord.com/invite/%s", inviteCode)

	time.Sleep(3 * time.Second)

	page := stealth.MustPage(c.browser)
	defer page.MustClose()

	err = page.Navigate(url)
	if err != nil {
		return nil, fmt.Errorf("erro de navegação rod: %w", err)
	}

	router := page.HijackRequests()
	defer router.MustStop()

	router.MustAdd("*", func(ctx *rod.Hijack) {
		reqUrl := ctx.Request.URL().String()
		if strings.HasPrefix(reqUrl, "discord://") {
			ctx.Response.Fail(proto.NetworkErrorReasonAborted)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	go router.Run()

	err = page.WaitLoad()
	if err != nil {
		return nil, fmt.Errorf("timeout loading discord page")
	}
	time.Sleep(1 * time.Second)

	res, err := page.Eval(`() => {
		let guildName = "";
		let guildIcon = "";
		let memberCount = 0;
		let guildId = "unknown";
		let isInvalid = false;

		let h1 = document.querySelector('h1');
		if (h1 && h1.innerText && h1.innerText.trim() !== "") {
			guildName = h1.innerText.trim();
		}

		let allTextElements = Array.from(document.querySelectorAll('div, span, strong'));
		for (let el of allTextElements) {
			if (el.innerText) {
				let match = el.innerText.match(/([\d.,]+)\s+(Members|Membros)/i);
				if (match) {
					memberCount = parseInt(match[1].replace(/[,.]/g, ''));
					break;
				}
			}
		}

		let metaTitle = document.querySelector('meta[property="og:title"]');
		if (metaTitle && metaTitle.content) {
			let fallbackName = metaTitle.content.replace("Join the ", "").replace(" Discord Server!", "").trim();
			fallbackName = fallbackName.replace("Participe do servidor do Discord ", "").replace(/!$/, "").trim();
			
			if (guildName === "") guildName = fallbackName;
		}
		
		let metaDesc = document.querySelector('meta[property="og:description"]');
		if (metaDesc && metaDesc.content && memberCount === 0) {
			let match = metaDesc.content.match(/(?:with|com)\s+([\d.,]+)\s+(?:other members|outros membros)/i);
			if (match) {
				memberCount = parseInt(match[1].replace(/[,.]/g, ''));
			}
		}

		let metaImg = document.querySelector('meta[property="og:image"]');
		if (metaImg && metaImg.content) {
			let m = metaImg.content.match(/icons\/(\d+)\/([a-zA-Z0-9_]+)\./);
			if (m) {
				guildId = m[1];
				guildIcon = m[2];
			}
		}

		let genericTitles = [
			"Discord - Group Chat That’s All Fun & Games", 
			"Discord - Um lugar para conversar", 
			"Discord", 
			"Invite Invalid",
			"Opening Discord App."
		];
		if (genericTitles.includes(guildName) || guildName === "") {
			isInvalid = true;
		}

		return {
			name: guildName,
			icon: guildIcon,
			memberCount: memberCount,
			guildId: guildId,
			invalid: isInvalid
		};
	}`)

	if err != nil {
		return nil, fmt.Errorf("falha ao injetar js: %w", err)
	}

	if !res.Value.Nil() {
		invalid := res.Value.Get("invalid").Bool()
		if invalid {
			return nil, fmt.Errorf("convite inválido ou expirado (detectado por fallback genérico)")
		}

		name := res.Value.Get("name").String()
		icon := res.Value.Get("icon").String()
		count := int(res.Value.Get("memberCount").Int())
		gId := res.Value.Get("guildId").String()

		inviteData := DiscordInviteResponse{
			Code:                   inviteCode,
			ApproximateMemberCount: count,
		}
		inviteData.Guild.Name = name
		inviteData.Guild.Icon = icon
		inviteData.Guild.ID = gId

		if jsonData, err := json.Marshal(inviteData); err == nil {
			c.clientRedis.Set(ctx, cacheKey, jsonData, 24*time.Hour)
		}
		return &inviteData, nil
	} else if strings.Contains(page.MustHTML(), "Invite Invalid") || strings.Contains(page.MustHTML(), "inválido ou expirou") {
		return nil, fmt.Errorf("convite inválido ou expirado")
	}

	return nil, fmt.Errorf("rate limited (muitas requisições)")
}
