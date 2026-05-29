package logic

import (
	"reflect"
	"testing"
)

// assertSlicesEqual is a test helper that compares two string slices with clear error messages.
func assertSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) == 0 {
		got = []string{}
	}
	if len(want) == 0 {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resultado incorreto\n  got:  %v\n  want: %v", got, want)
	}
}

func TestFindInvites(t *testing.T) {
	t.Parallel()
	finder := NewDiscordFinder()

	tests := []struct {
		name string
		text string
		want []string
	}{
		// === discord.gg links ===
		{
			name: "discord.gg simples",
			text: "entra no discord.gg/abc123",
			want: []string{"discord.gg/abc123"},
		},
		{
			name: "discord.gg com https",
			text: "https://discord.gg/myServer",
			want: []string{"discord.gg/myServer"},
		},
		{
			name: "discord.gg com http",
			text: "http://discord.gg/test-server",
			want: []string{"discord.gg/test-server"},
		},

		// === discord.com/invite links ===
		{
			name: "discord.com/invite simples",
			text: "discord.com/invite/xyz789",
			want: []string{"discord.com/invite/xyz789"},
		},
		{
			name: "discord.com/invite com https",
			text: "https://discord.com/invite/mycode",
			want: []string{"discord.com/invite/mycode"},
		},

		// === discord.com/channels links ===
		{
			name: "channels sem message ID",
			text: "discord.com/channels/123456/789012",
			want: []string{"discord.com/channels/123456/789012"},
		},
		{
			name: "channels com message ID",
			text: "discord.com/channels/123456/789012/111111",
			want: []string{"discord.com/channels/123456/789012/111111"},
		},

		// === Case insensitivity ===
		{
			name: "case insensitive discord.gg",
			text: "DISCORD.GG/TestCode",
			want: []string{"discord.gg/TestCode"},
		},
		{
			name: "case mixed DiScOrD",
			text: "DiScOrD.gG/CaSe",
			want: []string{"discord.gg/CaSe"},
		},

		// === Obfuscated links ===
		{
			name: "discord . gg com espacos",
			text: "discord . gg / abc123",
			want: []string{"discord.gg/abc123"},
		},
		{
			name: "discord dot gg (dot notation)",
			text: "discord dot gg/server1",
			want: []string{"discord.gg/server1"},
		},
		{
			name: "discord.com / invite com espacos",
			text: "discord . com / invite / secret",
			want: []string{"discord.com/invite/secret"},
		},

		// === Multiple links ===
		{
			name: "multiplos links diferentes",
			text: "vem pro discord.gg/server1 e discord.gg/server2",
			want: []string{"discord.gg/server1", "discord.gg/server2"},
		},
		{
			name: "mix de formatos",
			text: "discord.gg/aaa e discord.com/invite/bbb",
			want: []string{"discord.gg/aaa", "discord.com/invite/bbb"},
		},

		// === Deduplication ===
		{
			name: "dedup mesmo link repetido",
			text: "discord.gg/same discord.gg/same discord.gg/same",
			want: []string{"discord.gg/same"},
		},

		// === Edge cases ===
		{
			name: "texto vazio",
			text: "",
			want: []string{},
		},
		{
			name: "texto sem links",
			text: "nenhum link de discord aqui, só texto normal sobre jogos",
			want: []string{},
		},
		{
			name: "link com zero-width chars",
			text: "discord\u200b.\u200bgg/hid\u200bden",
			want: []string{"discord.gg/hidden"},
		},
		{
			name: "link com BOM unicode",
			text: "\uFEFFdiscord.gg/bomtest",
			want: []string{"discord.gg/bomtest"},
		},
		{
			name: "link dentro de texto longo",
			text: "olha gente eu tava pensando em criar um server de minecraft e fiz esse discord.gg/WQ5UU7UzCc pra galera entrar dai vocês podem ver os mods",
			want: []string{"discord.gg/WQ5UU7UzCc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := finder.FindInvites(tt.text)
			assertSlicesEqual(t, got, tt.want)
		})
	}
}

func TestNormalizeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "remove zero-width space",
			input: "discord\u200b.gg/test",
			want:  "discord.gg/test",
		},
		{
			name:  "remove BOM",
			input: "\uFEFFdiscord.gg/test",
			want:  "discord.gg/test",
		},
		{
			name:  "normaliza espacos discord.gg",
			input: "discord . gg / code",
			want:  "discord.gg/code",
		},
		{
			name:  "normaliza discord.com/invite com espacos",
			input: "discord . com / invite / code",
			want:  "discord.com/invite/code",
		},
		{
			name:  "normaliza discord.com/channels",
			input: "discord . com / channels / 12345 / 67890",
			want:  "discord.com/channels/12345/67890",
		},
		{
			name:  "texto sem mudancas",
			input: "texto normal sem discord links",
			want:  "texto normal sem discord links",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeText(tt.input)
			if got != tt.want {
				t.Errorf("normalizeText()\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestNewDiscordFinder(t *testing.T) {
	t.Parallel()
	finder := NewDiscordFinder()

	if finder == nil {
		t.Fatal("NewDiscordFinder() retornou nil")
	}
	if finder.ggRegex == nil {
		t.Error("ggRegex é nil")
	}
	if finder.inviteRegex == nil {
		t.Error("inviteRegex é nil")
	}
	if finder.channelRegex == nil {
		t.Error("channelRegex é nil")
	}
}
