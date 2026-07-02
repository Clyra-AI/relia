package distill

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func RuleID(kind string, cluster Cluster) string {
	slug := slugifyRuleIDPart(cluster.Signal)
	if slug == "" {
		slug = "signature"
	}
	return fmt.Sprintf("%s-%s-%s", kind, slug, shortHash(kind+"\x00"+cluster.Key))
}

func RuleStatement(kind string, cluster Cluster, scopePaths []string) string {
	scope := "this scope"
	if len(scopePaths) > 0 {
		scope = strings.Join(scopePaths, ", ")
	}
	signal := cluster.Signal
	if signal == "" {
		signal = "the clustered signature"
	}
	if kind == "playbook" {
		return fmt.Sprintf("Prefer the held %s pattern in %s when this signature appears.", signal, scope)
	}
	return fmt.Sprintf("Avoid repeating %s in %s without addressing the prior failure evidence.", signal, scope)
}

func slugifyRuleIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		keep := false
		switch {
		case r >= 'a' && r <= 'z':
			keep = true
		case r >= '0' && r <= '9':
			keep = true
		}
		if keep {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)[:12]
}
