// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/utils/access"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-networking-filter/test/templates"
)

const (
	// This is the ip address of example.org
	// It should be stable enough. In case it get's unreachable the test will not fail and we still have
	// the iptables rules count proving that the connection was blocked.
	blockAddress = "93.184.216.34"
)

var _ = Describe("Network Filter Tests", Label("Network"), func() {

	testCases := []struct {
		name               string
		blackholingEnabled bool
		shootName          string
	}{
		{"blackholing enabled", true, "e2e-default"},
		{"blackholing disabled", false, "e2e-blackholing"},
	}

	for _, tc := range testCases {

		f := defaultShootCreationFramework()

		f.Shoot = defaultShoot(tc.shootName, tc.blackholingEnabled, blockAddress)

		It("Create Shoot, Test Policy Filter, Delete Shoot", Label(tc.shootName), func() {
			By("Create Shoot")
			ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.CreateShootAndWaitForCreation(ctx, false)).To(Succeed())
			f.Verify()

			By("Test Policy Filter")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			values := struct {
				HelmDeployNamespace string
				KubeVersion         string
				BlackholingEnabled  bool
				BlockAddress        string
			}{
				templates.NetworkTestNamespace,
				f.Shoot.Spec.Kubernetes.Version,
				tc.blackholingEnabled,
				blockAddress,
			}

			var err error
			f.GardenClient, err = kubernetes.NewClientFromFile("", f.ShootFramework.Config.GardenerConfig.GardenerKubeconfig,
				kubernetes.WithClientOptions(client.Options{Scheme: kubernetes.GardenScheme}),
				kubernetes.WithAllowedUserFields([]string{kubernetes.AuthTokenFile}),
				kubernetes.WithDisabledCachedClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			shootKubeconfigSecret := &corev1.Secret{}
			gardenClient := f.GardenClient.Client()
			err = gardenClient.Get(ctx, kubernetesutils.Key(f.Shoot.Namespace, gardenerutils.ComputeShootProjectSecretName(f.Shoot.Name, gardenerutils.ShootProjectSecretSuffixKubeconfig)), shootKubeconfigSecret)
			Expect(err).NotTo(HaveOccurred())

			f.ShootFramework.ShootClient, err = access.CreateShootClientFromAdminKubeconfig(ctx, f.GardenClient, f.Shoot)
			Expect(err).NotTo(HaveOccurred())

			newShootKubeconfigSecret := &corev1.Secret{ObjectMeta: v1.ObjectMeta{
				Name:      "kubeconfig",
				Namespace: values.HelmDeployNamespace},
				Data: map[string][]byte{"kubeconfig": shootKubeconfigSecret.Data["kubeconfig"]},
			}
			err = f.ShootFramework.ShootClient.Client().Create(ctx, newShootKubeconfigSecret)
			Expect(err).NotTo(HaveOccurred())

			resourceDir, err := filepath.Abs(filepath.Join(".."))
			Expect(err).NotTo(HaveOccurred())
			f.TemplatesDir = filepath.Join(resourceDir, "templates")

			err = f.RenderAndDeployTemplate(ctx, f.ShootFramework.ShootClient, templates.NetworkTestName, values)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(30 * time.Second)

			err = f.ShootFramework.WaitUntilDaemonSetIsRunning(
				ctx,
				f.ShootFramework.ShootClient.Client(),
				"filter-test",
				values.HelmDeployNamespace,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Network-test daemonset is deployed successfully!")

			By("Check if filter-test fails or succeeds!")

			out, err := framework.PodExecByLabel(ctx, labels.SelectorFromSet(map[string]string{
				v1beta1constants.LabelApp: "filter-test",
			}),
				"filter-block-test",
				"/script/network-filter-test.sh",
				values.HelmDeployNamespace,
				f.ShootFramework.ShootClient,
			)
			outBytes, _ := io.ReadAll(out)
			fmt.Println(string(outBytes))

			Expect(string(outBytes)).To(ContainSubstring("SUCCESS: Egress is blocked."))
			if tc.blackholingEnabled {
				Expect(string(outBytes)).To(ContainSubstring("SUCCESS: Ingress is blocked."))
			}
			Expect(err).To(BeNil())

			By("Delete Shoot")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.DeleteShootAndWaitForDeletion(ctx, f.Shoot)).To(Succeed())

		})
	}
})
