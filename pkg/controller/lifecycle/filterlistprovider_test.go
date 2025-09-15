package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
)

var _ = Describe("DownloaderFilterListProvider", func() {
	var (
		ctx            context.Context
		logger         logr.Logger
		client         client.Client // Assume you have a fake client for testing
		downloaderConf *config.DownloaderConfig
		oauth2Secret   *config.OAuth2Secret
		provider       *DownloaderFilterListProvider
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
		client = fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
		downloaderConf = &config.DownloaderConfig{
			Endpoint: "http://example.com/filters",
		}
		oauth2Secret = &config.OAuth2Secret{
			ClientID:     "id",
			ClientSecret: "secret",
		}
		provider = NewDownloaderFilterListProvider(ctx, client, logger, downloaderConf, oauth2Secret)
	})

	Describe("#download", func() {
		It("should download and parse filter list", func() {
			filters := []config.Filter{
				{Network: "1.2.3.4/32", Policy: "BLOCK_ACCESS"},
			}
			b, _ := json.Marshal(filters)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			result, err := provider.download()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(filters))
		})

		It("should fail on invalid JSON", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{invalid json"))
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			_, err := provider.download()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshalling body failed"))
		})

		It("should print additional info on server outage", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("no healthy upstream"))
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			_, err := provider.download()
			Expect(err).To(HaveOccurred())
			fmt.Println(err)
			Expect(err.Error()).To(ContainSubstring("unmarshalling body failed"))
			Expect(err.Error()).To(ContainSubstring("no healthy upstream"))
		})

		It("should fail on invalid CIDR", func() {
			filters := []config.Filter{
				{Network: "invalid-cidr", Policy: "BLOCK_ACCESS"},
			}
			b, _ := json.Marshal(filters)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			_, err := provider.download()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("filterList[0].network"))
		})
	})

	Describe("#getAccessToken", func() {
		It("should fail if secret is nil", func() {
			token, err := provider.getAccessToken("http://token", nil)
			Expect(err).To(HaveOccurred())
			Expect(token).To(BeEmpty())
		})

		It("should fail if clientID is missing", func() {
			secret := &config.OAuth2Secret{ClientSecret: "secret"}
			token, err := provider.getAccessToken("http://token", secret)
			Expect(err).To(HaveOccurred())
			Expect(token).To(BeEmpty())
		})

		It("should fail if clientSecret and certs are missing", func() {
			secret := &config.OAuth2Secret{ClientID: "id"}
			token, err := provider.getAccessToken("http://token", secret)
			Expect(err).To(HaveOccurred())
			Expect(token).To(BeEmpty())
		})
	})
})
