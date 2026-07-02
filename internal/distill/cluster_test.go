package distill

import (
	"testing"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestBuildClustersMergesMatchingMessageFingerprints(t *testing.T) {
	first := testClusterExperience("exp_2", "pytest", "tests/billing_test.py::TestInvoice", "fingerprint-1", "2026-01-02T00:00:00Z")
	second := testClusterExperience("exp_1", "go test", "./internal/billing", "fingerprint-1", "2026-01-01T00:00:00Z")

	clusters := BuildClusters([]backtestdoc.Experience{first, second})

	if len(clusters) != 1 {
		t.Fatalf("len(clusters) = %d, want 1", len(clusters))
	}
	if clusters[0].Key != "message\x00fingerprint-1" {
		t.Fatalf("cluster key = %q, want message fingerprint key", clusters[0].Key)
	}
	if got := []string{clusters[0].Records[0].Record.ExperienceID, clusters[0].Records[1].Record.ExperienceID}; got[0] != "exp_1" || got[1] != "exp_2" {
		t.Fatalf("records not sorted by recorded_at: %v", got)
	}
}

func TestBuildClustersSkipsUncertainAttribution(t *testing.T) {
	record := testClusterExperience("exp_1", "pytest", "tests/example.py::test_case", "fingerprint-1", "2026-01-01T00:00:00Z")
	record.Record.Attribution.ActorKind = "uncertain"

	if clusters := BuildClusters([]backtestdoc.Experience{record}); len(clusters) != 0 {
		t.Fatalf("len(clusters) = %d, want 0", len(clusters))
	}
}

func TestClusterKeyPrefersStableSignatureIDWithCheckAndKey(t *testing.T) {
	record := testClusterRecord("exp_1", "pytest", "tests/example.py::test_case", "fingerprint-1")
	record.Outcome.Signature.SignatureID = "sig_hand_authored"

	key := ClusterKey(record)

	if key != "id_check_key\x00sig_hand_authored\x00pytest\x00tests/example.py::test_case" {
		t.Fatalf("cluster key = %q, want stable signature key", key)
	}
}

func testClusterExperience(id string, checkName string, signatureKey string, fingerprint string, recordedAt string) backtestdoc.Experience {
	parsed, err := time.Parse(time.RFC3339, recordedAt)
	if err != nil {
		panic(err)
	}
	return backtestdoc.Experience{
		RecordedAt: parsed,
		Record:     testClusterRecord(id, checkName, signatureKey, fingerprint),
	}
}

func testClusterRecord(id string, checkName string, signatureKey string, fingerprint string) ingestdoc.Record {
	return ingestdoc.Record{
		ExperienceID: id,
		Attribution:  ingestdoc.Attribution{ActorKind: "agent"},
		Outcome: ingestdoc.Outcome{
			Kind: "ci_failure",
			Signature: ingestdoc.Signature{
				SignatureID: "sig_generated_" + id,
			},
		},
		Metadata: map[string]any{
			"signature": map[string]any{
				"class":               "test_failure",
				"check_name":          checkName,
				"key":                 signatureKey,
				"message_fingerprint": fingerprint,
			},
		},
	}
}
