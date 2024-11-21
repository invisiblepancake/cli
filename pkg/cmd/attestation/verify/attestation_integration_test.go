//go:build integration

package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/stretchr/testify/require"
)

func getAttestationsFor(t *testing.T, bundlePath string) []*api.Attestation {
	t.Helper()

	attestations, err := verification.GetLocalAttestations(bundlePath)
	require.NoError(t, err)

	return attestations
}

func TestVerifyAttestations(t *testing.T) {
	sgVerifier := verification.NewLiveSigstoreVerifier(verification.SigstoreConfig{
		Logger: io.NewTestHandler(),
	})

	certSummary := certificate.Summary{}
	certSummary.SourceRepositoryOwnerURI = "https://github.com/sigstore"
	certSummary.SourceRepositoryURI = "https://github.com/sigstore/sigstore-js"
	certSummary.Issuer = verification.GitHubOIDCIssuer

	ec := verification.EnforcementCriteria{
		Certificate:   certSummary,
		PredicateType: verification.SLSAPredicateV1,
		SANRegex:      "^https://github.com/sigstore/",
	}
	require.NoError(t, ec.Valid())

	artifactPath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	a, err := artifact.NewDigestedArtifact(nil, artifactPath, "sha512")
	require.NoError(t, err)

	t.Run("all attestations pass verification", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		require.Len(t, attestations, 2)
		results, errMsg, err := verifyAttestations(*a, attestations, sgVerifier, ec)
		require.NoError(t, err)
		require.Zero(t, errMsg)
		require.Len(t, results, 2)
	})

	t.Run("passes verification with 2/3 attestations passing Sigstore verification", func(t *testing.T) {
		invalidBundle := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		attestations = append(attestations, invalidBundle[0])
		require.Len(t, attestations, 3)

		results, errMsg, err := verifyAttestations(*a, attestations, sgVerifier, ec)
		require.NoError(t, err)
		require.Zero(t, errMsg)
		require.Len(t, results, 2)
	})

	t.Run("fails verification when Sigstore verification fails", func(t *testing.T) {
		invalidBundle := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		invalidBundle2 := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		attestations := append(invalidBundle, invalidBundle2...)
		require.Len(t, attestations, 2)

		results, errMsg, err := verifyAttestations(*a, attestations, sgVerifier, ec)
		require.Error(t, err)
		require.Contains(t, errMsg, "✗ Sigstore verification failed")
		require.Nil(t, results)
	})

	t.Run("passes verification with 2/3 attestations passing cert extension verification", func(t *testing.T) {
		sgjAttestation := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		reusableWorkflowAttestations := getAttestationsFor(t, "../test/data/reusable-workflow-attestation.sigstore.json")
		attestations := []*api.Attestation{sgjAttestation[0], reusableWorkflowAttestations[0], sgjAttestation[1]}
		require.Len(t, attestations, 3)

		rwfResult := verification.BuildMockResult(reusableWorkflowAttestations[0].Bundle, "", "https://github.com/malancas", "", verification.GitHubOIDCIssuer)
		sgjResult := verification.BuildDefaultMockResult(t)
		mockResults := []*verification.AttestationProcessingResult{&sgjResult, &rwfResult, &sgjResult}

		mockSgVerifier := verification.NewMockSigstoreVerifier(t, mockResults)
		results, errMsg, err := verifyAttestations(*a, attestations, mockSgVerifier, ec)
		require.NoError(t, err)
		require.Zero(t, errMsg)
		require.Len(t, results, 2)
	})

	t.Run("fails verification when cert extension verification fails", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		require.Len(t, attestations, 2)

		expectedCriteria := ec
		expectedCriteria.Certificate.SourceRepositoryOwnerURI = "https://github.com/wrong"

		results, errMsg, err := verifyAttestations(*a, attestations, sgVerifier, expectedCriteria)
		require.Error(t, err)
		require.Contains(t, errMsg, "✗ Policy verification failed")
		require.Nil(t, results)
	})
}
