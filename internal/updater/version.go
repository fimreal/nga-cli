package updater

import (
	"strconv"
	"strings"
)

// version parses a release tag such as "v0.2.0", "0.2.0-rc.1" or "dev".
type version struct {
	nums    []int
	pre     string
	unknown bool
}

func parseVersion(s string) version {
	v := version{unknown: true}
	s = strings.TrimSpace(s)
	if s == "" {
		return v
	}

	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	// Build stamps such as "0.2.0-12-gabcdef" or "v0.2.0+dirty" stay unknown.
	if i := strings.IndexAny(s, "+"); i >= 0 {
		s = s[:i]
	}
	core := s
	if i := strings.IndexAny(s, "-"); i >= 0 {
		core, v.pre = s[:i], s[i+1:]
	}

	if core == "" {
		return v
	}

	parts := strings.Split(core, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		// A numeric segment may still carry a trailing identifier ("1a").
		digits := p
		if i := strings.IndexFunc(digits, func(r rune) bool {
			return r < '0' || r > '9'
		}); i >= 0 {
			digits = digits[:i]
			v.pre = strings.TrimPrefix(p, digits) + "-" + v.pre
		}
		if digits == "" {
			return v
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return v
		}
		nums = append(nums, n)
	}

	v.nums = nums
	v.unknown = false
	return v
}

// compareVersion orders two tags. Unknown versions (dev builds) are treated as
// older than any released version so that they can be upgraded to a release.
func compareVersion(a, b string) int {
	va, vb := parseVersion(a), parseVersion(b)
	return compareVersions(va, vb)
}

func compareVersions(va, vb version) int {
	if va.unknown && vb.unknown {
		return 0
	}
	if va.unknown {
		return -1
	}
	if vb.unknown {
		return 1
	}

	for i := 0; i < len(va.nums) || i < len(vb.nums); i++ {
		x, y := 0, 0
		if i < len(va.nums) {
			x = va.nums[i]
		}
		if i < len(vb.nums) {
			y = vb.nums[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}

	// Equal cores: a pre-release sorts before its stable counterpart.
	switch {
	case va.pre == vb.pre:
		return 0
	case va.pre == "":
		return 1
	case vb.pre == "":
		return -1
	case va.pre < vb.pre:
		return -1
	default:
		return 1
	}
}
