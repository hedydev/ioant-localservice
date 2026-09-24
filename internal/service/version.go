package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
var numericRE = regexp.MustCompile(`^[0-9]+$`)

func validVersion(v string) bool {
	m := versionRE.FindStringSubmatch(v)
	if m == nil || len(v) > 100 {
		return false
	}
	for _, p := range strings.Split(m[4], ".") {
		if numericRE.MatchString(p) && len(p) > 1 && p[0] == '0' {
			return false
		}
	}
	return true
}

// Compare numeric strings without overflow, then SemVer pre-release identifiers.
func numericCompare(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func compareVersion(a, b string) int {
	x, y := versionRE.FindStringSubmatch(a), versionRE.FindStringSubmatch(b)
	for i := 1; i <= 3; i++ {
		if c := numericCompare(x[i], y[i]); c != 0 {
			return c
		}
	}
	if x[4] == y[4] {
		return 0
	}
	if x[4] == "" {
		return 1
	}
	if y[4] == "" {
		return -1
	}
	p, q := strings.Split(x[4], "."), strings.Split(y[4], ".")
	for i := 0; i < len(p) && i < len(q); i++ {
		an, bn := numericRE.MatchString(p[i]), numericRE.MatchString(q[i])
		c := 0
		if an && bn {
			c = numericCompare(p[i], q[i])
		} else if an {
			c = -1
		} else if bn {
			c = 1
		} else {
			c = strings.Compare(p[i], q[i])
		}
		if c != 0 {
			return c
		}
	}
	if len(p) < len(q) {
		return -1
	}
	return 1
}
func parseBuild(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("build 必须为正整数")
	}
	return n, nil
}
func compareRelease(a, b Release) int {
	if c := compareVersion(a.Version, b.Version); c != 0 {
		return c
	}
	if a.Build < b.Build {
		return -1
	}
	if a.Build > b.Build {
		return 1
	}
	return 0
}
