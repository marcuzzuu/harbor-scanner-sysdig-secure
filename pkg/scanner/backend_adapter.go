package scanner

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/sysdiglabs/harbor-scanner-sysdig-secure/pkg/harbor"
	"github.com/sysdiglabs/harbor-scanner-sysdig-secure/pkg/secure"
)

var (
	scanner = &harbor.Scanner{
		Name:    "Sysdig Secure",
		Vendor:  "Sysdig",
		Version: secure.BackendVersion,
	}

	scannerAdapterMetadata = harbor.ScannerAdapterMetadata{
		Scanner: scanner,
		Capabilities: []harbor.ScannerCapability{
			{
				ConsumesMimeTypes: []string{
					harbor.OCIImageManifestMimeType,
					harbor.DockerDistributionManifestMimeType,
				},
				ProducesMimeTypes: []string{
					harbor.ScanReportMimeType,
				},
			},
		},
		Properties: map[string]string{
			"harbor.scanner-adapter/scanner-type": "os-package-vulnerability",
		},
	}

	severities = map[harbor.Severity]int{
		harbor.UNKNOWN:    0,
		harbor.NEGLIGIBLE: 1,
		harbor.LOW:        2,
		harbor.MEDIUM:     3,
		harbor.HIGH:       4,
		harbor.CRITICAL:   5,
	}
)

type backendAdapter struct {
	secureClient secure.Client
}

func NewBackendAdapter(client secure.Client) Adapter {
	return &backendAdapter{
		secureClient: client,
	}
}

func (s *backendAdapter) GetMetadata() harbor.ScannerAdapterMetadata {
	return scannerAdapterMetadata
}

func (s *backendAdapter) Scan(req harbor.ScanRequest) (harbor.ScanResponse, error) {
	if err := s.setupCredentials(req); err != nil {
		return harbor.ScanResponse{}, err
	}

	response, err := s.secureClient.AddImage(getImageFrom(req), false)
	if err != nil {
		return harbor.ScanResponse{}, err
	}

	return harbor.ScanResponse{
		ID: createScanResponseID(req.Artifact.Repository, response.ImageDigest),
	}, nil
}

func (s *backendAdapter) setupCredentials(req harbor.ScanRequest) error {
	registry := getRegistryFrom(req)
	user, password := getUserAndPasswordFrom(req.Registry.Authorization)

	if err := s.secureClient.AddRegistry(registry, user, password); err != nil {
		if err != secure.ErrRegistryAlreadyExists {
			return err
		}

		if err = s.secureClient.UpdateRegistry(registry, user, password); err != nil {
			return err
		}
	}
	return nil
}

func getRegistryFrom(req harbor.ScanRequest) string {
	return strings.ReplaceAll(req.Registry.URL, "https://", "")
}

func getUserAndPasswordFrom(authorization string) (string, string) {
	payload := strings.ReplaceAll(authorization, "Basic ", "")
	plain, _ := base64.StdEncoding.DecodeString(payload)
	splitted := strings.Split(string(plain), ":")

	return splitted[0], splitted[1]
}

func getImageFrom(req harbor.ScanRequest) string {
	return fmt.Sprintf("%s/%s:%s", getRegistryFrom(req), req.Artifact.Repository, req.Artifact.Tag)
}

func createScanResponseID(repository string, shaDigest string) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%s", repository, shaDigest)))
}

func (s *backendAdapter) GetVulnerabilityReport(scanResponseID string) (harbor.VulnerabilityReport, error) {
	result := harbor.VulnerabilityReport{
		Scanner:  scanner,
		Severity: harbor.UNKNOWN,
	}

	repository, shaDigest := parseScanResponseID(scanResponseID)

	if err := s.fillVulnerabilities(shaDigest, &result); err != nil {
		return harbor.VulnerabilityReport{}, err
	}

	s.fillArtifact(repository, shaDigest, &result)

	return result, nil
}

func parseScanResponseID(scanResponseID string) (string, string) {
	plain, _ := base64.URLEncoding.DecodeString(scanResponseID)
	splitted := strings.Split(string(plain), "|")

	return splitted[0], splitted[1]
}

func (s *backendAdapter) fillVulnerabilities(shaDigest string, result *harbor.VulnerabilityReport) error {
	vulnerabilityReport, err := s.secureClient.GetVulnerabilities(shaDigest)
	if err != nil {
		switch err {
		case secure.ErrImageNotFound:
			return ErrScanRequestIDNotFound
		case secure.ErrVulnerabiltyReportNotReady:
			return ErrVulnerabiltyReportNotReady
		}
		return err
	}

	for _, vulnerability := range vulnerabilityReport.Vulnerabilities {
		vulnerabilityItem := toHarborVulnerabilityItem(vulnerability)
		result.Vulnerabilities = append(result.Vulnerabilities, vulnerabilityItem)

		if severities[result.Severity] < severities[vulnerabilityItem.Severity] {
			result.Severity = vulnerabilityItem.Severity
		}
	}
	return nil
}

func toHarborVulnerabilityItem(vulnerability *secure.Vulnerability) harbor.VulnerabilityItem {
	return harbor.VulnerabilityItem{
		ID:         vulnerability.Vuln,
		Package:    vulnerability.PackageName,
		Version:    vulnerability.PackageVersion,
		FixVersion: vulnerability.Fix,
		Severity:   harbor.Severity(vulnerability.Severity),
		Links:      []string{vulnerability.URL},
	}
}

func (s *backendAdapter) fillArtifact(repository string, shaDigest string, result *harbor.VulnerabilityReport) {
	scanResponse, _ := s.secureClient.GetImage(shaDigest)

	for _, imageDetail := range scanResponse.ImageDetail {
		if imageDetail.Repository == repository {
			result.GeneratedAt = imageDetail.CreatedAt
			result.Artifact = &harbor.Artifact{
				Repository: imageDetail.Repository,
				Digest:     imageDetail.Digest,
				Tag:        imageDetail.Tag,
				MimeType:   harbor.DockerDistributionManifestMimeType,
			}
			return
		}
	}
}
