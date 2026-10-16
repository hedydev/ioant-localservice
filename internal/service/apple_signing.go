package service

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
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

type codeSigningIdentity struct {
	Hash     string
	Identity string
	TeamID   string
}

var appleTeamIDRE = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var codeSigningIdentityLineRE = regexp.MustCompile(`^\s*\d+\)\s+([A-Fa-f0-9]{40})\s+"([^"]+)"`)

func parseCodeSigningIdentityRefs(raw []byte) []codeSigningIdentity {
	out := []codeSigningIdentity{}
	for _, line := range strings.Split(string(raw), "\n") {
		match := codeSigningIdentityLineRE.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		out = append(out, codeSigningIdentity{
			Hash:     strings.ToUpper(match[1]),
			Identity: strings.TrimSpace(match[2]),
		})
	}
	return out
}

func appleCertificateTeamID(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	for _, value := range cert.Subject.OrganizationalUnit {
		candidate := strings.ToUpper(strings.TrimSpace(value))
		if appleTeamIDRE.MatchString(candidate) {
			return candidate
		}
	}
	return ""
}

func certificateTeamsBySHA1(raw []byte) map[string]string {
	out := map[string]string{}
	for len(raw) > 0 {
		block, rest := pem.Decode(raw)
		raw = rest
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		teamID := appleCertificateTeamID(cert)
		if teamID == "" {
			continue
		}
		sum := sha1.Sum(cert.Raw)
		out[strings.ToUpper(hex.EncodeToString(sum[:]))] = teamID
	}
	return out
}

func loadCodeSigningIdentities(ctx context.Context) ([]codeSigningIdentity, error) {
	identityRaw, err := exec.CommandContext(ctx, "/usr/bin/security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return nil, fmt.Errorf("无法读取这台 Mac 的代码签名身份")
	}
	certRaw, err := exec.CommandContext(ctx, "/usr/bin/security", "find-certificate", "-a", "-p").Output()
	if err != nil {
		return nil, fmt.Errorf("无法读取这台 Mac 的代码签名证书")
	}
	teams := certificateTeamsBySHA1(certRaw)
	refs := parseCodeSigningIdentityRefs(identityRaw)
	out := make([]codeSigningIdentity, 0, len(refs))
	for _, ref := range refs {
		ref.TeamID = teams[ref.Hash]
		if ref.TeamID == "" {
			continue
		}
		out = append(out, ref)
	}
	return out, nil
}

func (a *App) appleSigningTeams(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	identities, err := loadCodeSigningIdentities(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	byTeam := map[string]map[string]bool{}
	for _, identity := range identities {
		if byTeam[identity.TeamID] == nil {
			byTeam[identity.TeamID] = map[string]bool{}
		}
		byTeam[identity.TeamID][identity.Identity] = true
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
