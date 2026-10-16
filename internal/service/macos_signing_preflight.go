package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type macOSDevelopmentSigning struct {
	TeamID   string
	Identity string
}

func isMacOSPackageOnlyLane(lane string) bool {
	return lane == "macos-test"
}

func appleDevelopmentSignings(identities []codeSigningIdentity) []macOSDevelopmentSigning {
	out := []macOSDevelopmentSigning{}
	seen := map[string]bool{}
	for _, item := range identities {
		if !strings.HasPrefix(item.Identity, "Apple Development:") || item.TeamID == "" {
			continue
		}
		key := item.TeamID + "\x00" + item.Identity
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, macOSDevelopmentSigning{TeamID: item.TeamID, Identity: item.Identity})
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
	identities, err := loadCodeSigningIdentities(ctx)
	if err != nil {
		return macOSDevelopmentSigning{}, err
	}
	return selectMacOSDevelopmentSigning(appleDevelopmentSignings(identities), preferredTeam)
}
