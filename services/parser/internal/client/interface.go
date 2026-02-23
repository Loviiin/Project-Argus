package client

import (
	"context"
)

type DiscordInviteResponse struct {
	Code  string `json:"code"`
	Guild struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Icon string `json:"icon"`
	} `json:"guild"`
	ApproximateMemberCount int     `json:"approximate_member_count"`
	ExpiresAt              *string `json:"expires_at"`
}

type DiscordProvider interface {
	GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error)
}
