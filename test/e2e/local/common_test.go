// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	kubernetesclient "github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/utils/access"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
)

var _ = BeforeEach(func() {
	parentCtx = context.Background()
})

const projectNamespace = "garden-local"

func defaultShootCreationFramework() *framework.ShootCreationFramework {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	return framework.NewShootCreationFramework(&framework.ShootCreationConfig{
		GardenerConfig: &framework.GardenerConfig{
			ProjectNamespace:   projectNamespace,
			GardenerKubeconfig: kubeconfigPath,
			SkipAccessingShoot: true,
			CommonConfig:       &framework.CommonConfig{},
		},
	})
}

func defaultShoot(generateName string, blackholing bool, blockAddress string) *gardencorev1beta1.Shoot {
	return createShoot(generateName, blackholing, blockAddress, false, false, nil)
}

func workerSpecificShoot(generateName string, blackholing bool, blockAddress string, wgBlackholing bool, groups []string) *gardencorev1beta1.Shoot {
	return createShoot(generateName, blackholing, blockAddress, true, wgBlackholing, groups)
}

func createShoot(generateName string, blackholing bool, blockAddress string, useWgBlackholing, wgBlackholing bool, groups []string) *gardencorev1beta1.Shoot {
	efc := &v1alpha1.Configuration{
		EgressFilter: &v1alpha1.EgressFilter{
			BlackholingEnabled: blackholing,
			StaticFilterList: []v1alpha1.Filter{
				{
					Network: blockAddress + "/32",
					Policy:  "BLOCK_ACCESS",
				},
				{
					Network: "130.214.229.163/32",
					Policy:  "BLOCK_ACCESS",
				},
			},
		},
	}

	if useWgBlackholing {
		efc.EgressFilter.Workers = &v1alpha1.Workers{
			BlackholingEnabled: wgBlackholing,
			Names:              groups,
		}
	}

	filterConfig, err := json.Marshal(efc)
	Expect(err).NotTo(HaveOccurred())

	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateName,
			Annotations: map[string]string{
				v1beta1constants.AnnotationShootCloudConfigExecutionMaxDelaySeconds: "0",
			},
		},
		Spec: gardencorev1beta1.ShootSpec{
			Region:            "local",
			SecretBindingName: ptr.To("local"),
			CloudProfileName:  ptr.To("local"),
			Kubernetes: gardencorev1beta1.Kubernetes{
				Version: "1.30.0",
				Kubelet: &gardencorev1beta1.KubeletConfig{
					SerializeImagePulls: ptr.To(false),
					RegistryPullQPS:     ptr.To(int32(10)),
					RegistryBurst:       ptr.To(int32(20)),
				},
			},
			Networking: &gardencorev1beta1.Networking{
				Type:           ptr.To("calico"),
				Pods:           ptr.To("10.3.0.0/16"),
				Services:       ptr.To("10.4.0.0/16"),
				Nodes:          ptr.To("10.0.0.0/16"),
				ProviderConfig: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"calico.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig","typha":{"enabled":false},"backend":"none"}`)},
			},
			Extensions: []gardencorev1beta1.Extension{
				{Type: "shoot-networking-filter",
					ProviderConfig: &runtime.RawExtension{Raw: filterConfig},
				},
			},
			Provider: gardencorev1beta1.Provider{
				Type: "local",
				Workers: []gardencorev1beta1.Worker{{
					Name: "local",
					Machine: gardencorev1beta1.Machine{
						Type: "local",
					},
					CRI: &gardencorev1beta1.CRI{
						Name: gardencorev1beta1.CRINameContainerD,
					},
					Minimum: 1,
					Maximum: 1,
				}},
			},
		},
	}
}

func setupShootClient(ctx context.Context, f *framework.ShootCreationFramework) {
	var err error
	f.GardenClient, err = kubernetesclient.NewClientFromFile("", f.ShootFramework.Config.GardenerConfig.GardenerKubeconfig,
		kubernetesclient.WithClientOptions(client.Options{Scheme: kubernetesclient.GardenScheme}),
		kubernetesclient.WithAllowedUserFields([]string{kubernetesclient.AuthTokenFile}),
		kubernetesclient.WithDisabledCachedClient(),
	)
	Expect(err).NotTo(HaveOccurred())

	f.ShootFramework.ShootClient, err = access.CreateShootClientFromAdminKubeconfig(ctx, f.GardenClient, f.Shoot)
	Expect(err).NotTo(HaveOccurred())

	resourceDir, err := filepath.Abs(filepath.Join("..", ".."))
	Expect(err).NotTo(HaveOccurred())
	f.TemplatesDir = filepath.Join(resourceDir, "templates")
}

var (
	parentCtx     context.Context
	runtimeClient client.Client
)

var _ = BeforeSuite(func() {
	Expect(os.Getenv("KUBECONFIG")).NotTo(BeEmpty(), "KUBECONFIG must be set")
	Expect(os.Getenv("REPO_ROOT")).NotTo(BeEmpty(), "REPO_ROOT must be set")

	logf.SetLogger(logger.MustNewZapLogger(logger.InfoLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))

	restConfig, err := kubernetesclient.RESTConfigFromClientConnectionConfiguration(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, nil, kubernetesclient.AuthTokenFile, kubernetesclient.AuthClientCertificate)
	Expect(err).NotTo(HaveOccurred())

	scheme := runtime.NewScheme()
	Expect(kubernetesscheme.AddToScheme(scheme)).To(Succeed())
	Expect(operatorv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(extensionsv1alpha1.AddToScheme(scheme)).To(Succeed())
	runtimeClient, err = client.New(restConfig, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = BeforeEach(func() {
	parentCtx = context.Background()
})
