package secure_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/sysdiglabs/harbor-scanner-sysdig-secure/pkg/secure"
)

var _ = Describe("Sysdig Secure Client", func() {
	var (
		client secure.Client
	)

	BeforeEach(func() {
		client = secure.NewClient(os.Getenv("SECURE_API_TOKEN"), os.Getenv("SECURE_URL"))
	})

	Context("when adding an image to scanning queue", func() {
		It("adds image to scanning queue", func() {
			response, _ := client.AddImage("sysdig/agent:9.8.0", false)

			Expect(response).NotTo(Equal(secure.ScanResponse{}))
			Expect(response.ImageContent).NotTo(BeNil())
			Expect(response.ImageContent.Metadata).NotTo(BeNil())
			Expect(len(response.ImageDetail)).To(BeNumerically(">", 0))
		})

		Context("when an error happens", func() {
			It("returns the error", func() {
				_, err := client.AddImage("sysdiglabs/non-existent", false)

				Expect(err).To(MatchError("cannot fetch image digest/manifest from registry"))
			})
		})
	})

	Context("when retrieving vulnerabilities for an image", func() {
		It("gets the report for a SHA", func() {
			response, _ := client.GetVulnerabilities("sha256:fda6b046981f5dab88aad84c6cebed4e47a0d6ad1c8ff7f58b5f0e6a95a5b2c1")

			Expect(response).NotTo(Equal(secure.VulnerabilityReport{}))
			Expect(len(response.Vulnerabilities)).To(BeNumerically(">", 0))
			Expect(len(response.Vulnerabilities[0].NVDData)).To(BeNumerically(">", 0))
		})

		Context("when an error happens", func() {
			It("returns a ImageNotFoundErr if the image does not exists on Secure", func() {
				_, err := client.GetVulnerabilities("non-existent")

				Expect(err).To(MatchError(secure.ErrImageNotFound))
			})

			It("returns a ReportNotReadyErr if the image is being analyzed", func() {
				response, _ := client.AddImage("sysdig/agent:9.9.0", true)

				_, err := client.GetVulnerabilities(response.ImageDigest)

				Expect(err).To(MatchError(secure.ErrVulnerabiltyReportNotReady))
			})
		})
	})
})
