package distill

import (
	"fmt"
	"sort"
	"strings"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

type Cluster struct {
	Key     string
	Signal  string
	Records []backtestdoc.Experience
}

func BuildClusters(records []backtestdoc.Experience) []Cluster {
	byKey := map[string]*Cluster{}
	for _, record := range records {
		if record.Record.Attribution.ActorKind == "uncertain" {
			continue
		}
		keys := clusterKeys(record.Record)
		if len(keys) == 0 {
			continue
		}
		var cluster *Cluster
		var matchedKeys []string
		for _, key := range keys {
			existing := byKey[key]
			if existing == nil {
				continue
			}
			matchedKeys = append(matchedKeys, key)
			if cluster == nil {
				cluster = existing
				continue
			}
			if cluster != existing {
				mergeClusters(byKey, cluster, existing)
			}
		}
		if cluster == nil {
			cluster = &Cluster{Key: keys[0]}
		} else {
			promoteClusterKeyForMatches(cluster, matchedKeys)
		}
		for _, key := range keys {
			byKey[key] = cluster
		}
		cluster.Records = append(cluster.Records, record)
		if cluster.Signal == "" {
			cluster.Signal = RecordSignal(record.Record)
		}
	}
	clusters := make([]Cluster, 0, len(byKey))
	seen := map[*Cluster]bool{}
	for _, cluster := range byKey {
		if seen[cluster] {
			continue
		}
		seen[cluster] = true
		sort.Slice(cluster.Records, func(i, j int) bool {
			if cluster.Records[i].RecordedAt.Equal(cluster.Records[j].RecordedAt) {
				return cluster.Records[i].Record.ExperienceID < cluster.Records[j].Record.ExperienceID
			}
			return cluster.Records[i].RecordedAt.Before(cluster.Records[j].RecordedAt)
		})
		clusters = append(clusters, *cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Key < clusters[j].Key
	})
	return clusters
}

func ClusterKey(record ingestdoc.Record) string {
	keys := clusterKeys(record)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

func RecordSignal(record ingestdoc.Record) string {
	signatureMetadata, _ := record.Metadata["signature"].(map[string]any)
	for _, value := range []string{
		stringFromAny(signatureMetadata["check_name"]),
		stringFromAny(signatureMetadata["key"]),
		record.Outcome.Signature.SignatureID,
		record.Outcome.Kind,
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "signature"
}

func promoteClusterKeyForMatches(cluster *Cluster, matchedKeys []string) {
	var messageKey string
	for _, key := range matchedKeys {
		if !isMessageKey(key) {
			return
		}
		if messageKey == "" {
			messageKey = key
		}
	}
	if messageKey != "" {
		cluster.Key = messageKey
	}
}

func isMessageKey(key string) bool {
	return strings.HasPrefix(key, "message\x00")
}

func mergeClusters(byKey map[string]*Cluster, target *Cluster, source *Cluster) {
	target.Records = append(target.Records, source.Records...)
	if target.Signal == "" {
		target.Signal = source.Signal
	}
	for key, cluster := range byKey {
		if cluster == source {
			byKey[key] = target
		}
	}
}

func clusterKeys(record ingestdoc.Record) []string {
	keys := []string{}
	if key := stableSignatureKey(record); key != "" {
		keys = append(keys, key)
	}
	keys = append(keys, canonicalSignatureKeys(record)...)
	return keys
}

func stableSignatureKey(record ingestdoc.Record) string {
	signatureID := strings.TrimSpace(record.Outcome.Signature.SignatureID)
	if signatureID == "" || strings.HasPrefix(signatureID, "sig_generated") {
		return ""
	}
	signatureMetadata, _ := record.Metadata["signature"].(map[string]any)
	checkName := strings.TrimSpace(stringFromAny(signatureMetadata["check_name"]))
	signatureKey := strings.TrimSpace(stringFromAny(signatureMetadata["key"]))
	if checkName == "" || signatureKey == "" {
		return ""
	}
	return strings.Join([]string{"id_check_key", signatureID, checkName, signatureKey}, "\x00")
}

func canonicalSignatureKeys(record ingestdoc.Record) []string {
	signatureMetadata, _ := record.Metadata["signature"].(map[string]any)
	signatureClass := strings.TrimSpace(stringFromAny(signatureMetadata["class"]))
	checkName := strings.TrimSpace(stringFromAny(signatureMetadata["check_name"]))
	signatureKey := strings.TrimSpace(stringFromAny(signatureMetadata["key"]))
	messageFingerprint := strings.TrimSpace(stringFromAny(signatureMetadata["message_fingerprint"]))
	keys := []string{}
	if signatureClass != "" && checkName != "" && signatureKey != "" {
		keys = append(keys, strings.Join([]string{"class_check_key", signatureClass, checkName, signatureKey}, "\x00"))
	}
	if messageFingerprint != "" {
		keys = append(keys, strings.Join([]string{"message", messageFingerprint}, "\x00"))
	}
	if len(keys) == 0 {
		keys = append(keys, strings.Join([]string{"id", record.Outcome.Signature.SignatureID}, "\x00"))
	}
	return keys
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
