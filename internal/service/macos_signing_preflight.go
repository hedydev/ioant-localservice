package service

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type macOSDevelopmentSigning struct {
	TeamID   string
	Identity string
}

func isMacOSPackageOnlyLane(lane string) bool {
	return lane == "macos-test" || lane == "macos-portable-adhoc"
}

func parseAppleDevelopmentIdentities(raw []byte) []macOSDevelopmentSigning {
	out := []macOSDevelopmentSigning{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		identity := strings.TrimSpace(line)
		firstQuote := strings.IndexByte(identity, '"')
		lastQuote := strings.LastIndexByte(identity, '"')
		if firstQuote >= 0 && lastQuote > firstQuote {
			identity = identity[firstQuote+1 : lastQuote]
		}
		if !strings.HasPrefix(identity, "Apple Development:") {
			continue
		}
		match := signingIdentityTeamRE.FindStringSubmatch(identity)
		if len(match) != 2 {
			continue
		}
		key := match[1] + "\x00" + identity
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, macOSDevelopmentSigning{TeamID: match[1], Identity: identity})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TeamID != out[j].TeamID {
			return out[i].TeamID < out[j].TeamID
		}
		return out[i].Identity < out[j].Identity
	})
	return out
}

func selectMacOSDevelopmentSigning(identities []macOSDevelopmentSigning, preferredTeam string) (macOSDevelopmentSigning, error) {
	preferredTeam = strings.ToUpper(strings.TrimSpace(preferredTeam))
	if preferredTeam != "" {
		for _, identity := range identities {
			if identity.TeamID == preferredTeam {
				return identity, nil
			}
		}
		return macOSDevelopmentSigning{}, fmt.Errorf("macOS Internal Test 需要 Apple Development 证书；未找到 Team %s 的有效开发签名身份", preferredTeam)
	}
	if len(identities) == 0 {
		return macOSDevelopmentSigning{}, fmt.Errorf("macOS Internal Test 需要 Apple Development 证书；请先在 Xcode / Keychain 安装有效开发签名身份")
	}
	team := identities[0].TeamID
	for _, identity := range identities[1:] {
		if identity.TeamID != team {
			return macOSDevelopmentSigning{}, fmt.Errorf("检测到多个 Apple Development Team；请在 macos-test Profile 中明确选择 Apple Team")
		}
	}
	return identities[0], nil
}

func resolveMacOSDevelopmentSigning(ctx context.Context, preferredTeam string) (macOSDevelopmentSigning, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-identity", "-v", "-p", "codesigning")
	raw, err := cmd.Output()
	if err != nil {
		return macOSDevelopmentSigning{}, fmt.Errorf("无法读取这台 Mac 的 Apple Development 签名身份")
	}
	return selectMacOSDevelopmentSigning(parseAppleDevelopmentIdentities(raw), preferredTeam)
}
