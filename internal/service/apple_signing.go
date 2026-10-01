package service

import (
	"context"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type AppleSigningTeam struct {
	ID         string   `json:"id"`
	Identities []string `json:"identities"`
}

var appleTeamIDRE = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var signingIdentityTeamRE = regexp.MustCompile(`\(([A-Z0-9]{10})\)"?\s*$`)

func (a *App) appleSigningTeams(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-identity", "-v", "-p", "codesigning")
	raw, err := cmd.Output()
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取这台 Mac 的代码签名身份")
		return
	}

	byTeam := map[string]map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		match := signingIdentityTeamRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 2 {
			continue
		}
		id := match[1]
		identity := strings.TrimSpace(line)
		firstQuote := strings.IndexByte(identity, '"')
		lastQuote := strings.LastIndexByte(identity, '"')
		if firstQuote >= 0 && lastQuote > firstQuote {
			identity = identity[firstQuote+1 : lastQuote]
		}
		if byTeam[id] == nil {
			byTeam[id] = map[string]bool{}
		}
		byTeam[id][identity] = true
	}

	ids := make([]string, 0, len(byTeam))
	for id := range byTeam {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	teams := make([]AppleSigningTeam, 0, len(ids))
	for _, id := range ids {
		identities := make([]string, 0, len(byTeam[id]))
		for identity := range byTeam[id] {
			identities = append(identities, identity)
		}
		sort.Strings(identities)
		teams = append(teams, AppleSigningTeam{ID: id, Identities: identities})
	}
	respond(w, http.StatusOK, teams)
}
