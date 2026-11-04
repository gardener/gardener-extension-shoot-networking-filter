package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
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

		It("should fail if response body exceeds maximum allowed size", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				oversized := make([]byte, constants.FilterListMaxDecompressedSize+1)
				_, _ = w.Write(oversized)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			_, err := provider.download()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum allowed size"))
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

		It("should download and parse v2 format filter list", func() {
			filterListV2 := []config.FilterListV2{
				{
					Entries: []config.FilterEntryV2{
						{Target: "10.0.0.0/8", Policy: config.PolicyBlock},
						{Target: "192.168.1.0/24", Policy: config.PolicyAllow},
					},
				},
			}
			b, _ := json.Marshal(filterListV2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			result, err := provider.download()
			Expect(err).To(BeNil())
			Expect(result).To(HaveLen(2))
			Expect(result[0].Network).To(Equal("10.0.0.0/8"))
			Expect(result[0].Policy).To(Equal(config.PolicyBlockAccess))
			Expect(result[1].Network).To(Equal("192.168.1.0/24"))
			Expect(result[1].Policy).To(Equal(config.PolicyAllowAccess))
		})

		It("should correctly detect v1 format when both fields exist", func() {
			// Ensure v1 format is detected even if JSON happens to have similar field names
			filters := []config.Filter{
				{Network: "172.16.0.0/12", Policy: config.PolicyBlockAccess},
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
			Expect(result).To(HaveLen(1))
			Expect(result[0].Network).To(Equal("172.16.0.0/12"))
			Expect(result[0].Policy).To(Equal(config.PolicyBlockAccess))
		})

		It("should preserve tags when converting v2 to v1 format", func() {
			v2List := []config.FilterListV2{
				{
					Entries: []config.FilterEntryV2{
						{
							Target: "10.0.0.0/8",
							Policy: config.PolicyBlock,
							Tags: []config.Tag{
								{Name: "S", Values: []string{"1"}},
								{Name: "Region", Values: []string{"EU"}},
							},
						},
						{
							Target: "192.168.1.0/24",
							Policy: config.PolicyAllow,
							Tags: []config.Tag{
								{Name: "S", Values: []string{"2"}},
							},
						},
					},
				},
			}
			b, _ := json.Marshal(v2List)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			result, err := provider.download()
			Expect(err).To(BeNil())
			Expect(result).To(HaveLen(2))

			// Verify first entry with tags
			Expect(result[0].Network).To(Equal("10.0.0.0/8"))
			Expect(result[0].Policy).To(Equal(config.PolicyBlockAccess))
			Expect(result[0].Tags).To(HaveLen(2))
			Expect(result[0].Tags[0].Name).To(Equal("S"))
			Expect(result[0].Tags[0].Values).To(ConsistOf("1"))
			Expect(result[0].Tags[1].Name).To(Equal("Region"))
			Expect(result[0].Tags[1].Values).To(ConsistOf("EU"))

			// Verify second entry with tags
			Expect(result[1].Network).To(Equal("192.168.1.0/24"))
			Expect(result[1].Policy).To(Equal(config.PolicyAllowAccess))
			Expect(result[1].Tags).To(HaveLen(1))
			Expect(result[1].Tags[0].Name).To(Equal("S"))
			Expect(result[1].Tags[0].Values).To(ConsistOf("2"))
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

	Describe("#GetFilterList", func() {
		It("should return an independent copy so mutations do not affect stored list", func() {
			filters := []config.Filter{
				{Network: "1.2.3.4/32", Policy: config.PolicyBlockAccess},
			}
			b, _ := json.Marshal(filters)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL

			Expect(provider.downloadAndStore()).To(Succeed())

			got := provider.GetFilterList()
			Expect(got).To(HaveLen(1))

			// mutate the returned slice
			got[0].Policy = config.PolicyAllowAccess

			// stored list must be unaffected
			got2 := provider.GetFilterList()
			Expect(got2[0].Policy).To(Equal(config.PolicyBlockAccess))
		})

		It("should not race on concurrent reads and writes", func() {
			filters := []config.Filter{
				{Network: "10.0.0.0/8", Policy: config.PolicyBlockAccess},
			}
			b, _ := json.Marshal(filters)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			}))
			defer server.Close()
			provider.downloaderConfig.Endpoint = server.URL
			Expect(provider.downloadAndStore()).To(Succeed())

			const goroutines = 5
			const iterations = 50
			var wg sync.WaitGroup
			wg.Add(goroutines * 2)

			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						_ = provider.GetFilterList()
					}
				}()
				go func() {
					defer wg.Done()
					for range iterations {
						_ = provider.downloadAndStore()
					}
				}()
			}
			wg.Wait()
		})
	})
})

var _ = Describe("StaticFilterListProvider", func() {
	var (
		ctx      context.Context
		logger   logr.Logger
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
		fakeClient = fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	})

	Describe("#GetFilterList", func() {
		It("should return an independent copy so mutations do not affect stored list", func() {
			filters := []config.Filter{
				{Network: "1.2.3.4/32", Policy: config.PolicyBlockAccess},
			}
			provider := NewStaticFilterListProvider(ctx, fakeClient, logger, filters)

			got := provider.GetFilterList()
			Expect(got).To(HaveLen(1))

			got[0].Policy = config.PolicyAllowAccess

			got2 := provider.GetFilterList()
			Expect(got2[0].Policy).To(Equal(config.PolicyBlockAccess))
		})

		It("should not race on concurrent SetFilterList and GetFilterList", func() {
			filters := []config.Filter{
				{Network: "10.0.0.0/8", Policy: config.PolicyBlockAccess},
			}
			provider := NewStaticFilterListProvider(ctx, fakeClient, logger, filters)

			const goroutines = 5
			const iterations = 50
			var wg sync.WaitGroup
			wg.Add(goroutines * 2)

			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						_ = provider.GetFilterList()
					}
				}()
				go func() {
					defer wg.Done()
					for range iterations {
						provider.SetFilterList([]config.Filter{
							{Network: "192.168.0.0/16", Policy: config.PolicyAllowAccess},
						})
					}
				}()
			}
			wg.Wait()
		})
	})
})
