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
	kubernetesclient "github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/utils/access"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
)

var (
	parentCtx context.Context
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
					Policy:  "BLOCK_ADDRESS",
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
				Version:                     "1.26.0",
				EnableStaticTokenKubeconfig: ptr.To(true),
				Kubelet: &gardencorev1beta1.KubeletConfig{
					SerializeImagePulls: ptr.To(false),
					RegistryPullQPS:     ptr.To(int32(10)),
					RegistryBurst:       ptr.To(int32(20)),
				},
				KubeAPIServer: &gardencorev1beta1.KubeAPIServerConfig{},
			},
			Networking: &gardencorev1beta1.Networking{
				Type:           ptr.To("calico"),
				Pods:           ptr.To("10.3.0.0/16"),
				Services:       ptr.To("10.4.0.0/16"),
				Nodes:          ptr.To("10.5.0.0/16"),
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

	resourceDir, err := filepath.Abs(filepath.Join(".."))
	Expect(err).NotTo(HaveOccurred())
	f.TemplatesDir = filepath.Join(resourceDir, "templates")
}
