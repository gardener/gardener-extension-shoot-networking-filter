// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"fmt"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"io"
	"path/filepath"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/utils/access"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-networking-filter/test/templates"
)

const (
	// This is the ip address of example.org
	// It should be stable enough. In case it gets unreachable the test will not fail, and we still have
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

			var err error
			f.GardenClient, err = kubernetes.NewClientFromFile("", f.ShootFramework.Config.GardenerConfig.GardenerKubeconfig,
				kubernetes.WithClientOptions(client.Options{Scheme: kubernetes.GardenScheme}),
				kubernetes.WithAllowedUserFields([]string{kubernetes.AuthTokenFile}),
				kubernetes.WithDisabledCachedClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			f.ShootFramework.ShootClient, err = access.CreateShootClientFromAdminKubeconfig(ctx, f.GardenClient, f.Shoot)
			Expect(err).NotTo(HaveOccurred())

			resourceDir, err := filepath.Abs(filepath.Join(".."))
			Expect(err).NotTo(HaveOccurred())
			f.TemplatesDir = filepath.Join(resourceDir, "templates")

			out := runNetworkFilterTest(ctx, f, tc.blackholingEnabled)
			fmt.Println(out)

			Expect(out).To(ContainSubstring("SUCCESS: Egress is blocked."))
			if tc.blackholingEnabled {
				Expect(out).To(ContainSubstring("SUCCESS: Ingress is blocked."))
			}
			Expect(err).To(BeNil())

			By(fmt.Sprintf("Switching to blackholingEnabled = %t", !tc.blackholingEnabled))
			updatedShoot := defaultShoot(tc.shootName, !tc.blackholingEnabled, blockAddress)
			err = f.UpdateShoot(ctx, f.Shoot, func(shoot *gardencorev1beta1.Shoot) error {
				copy(shoot.Spec.Extensions, updatedShoot.Spec.Extensions)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("Check if filter-test fails or succeeds after switch!")
			out = runNetworkFilterTest(ctx, f, !tc.blackholingEnabled)
			fmt.Println(out)

			if tc.blackholingEnabled {
				Expect(out).To(ContainSubstring("SUCCESS: No blackhole blocking mode artifacts remain."))
			} else {
				Expect(out).To(ContainSubstring("SUCCESS: No iptables blocking mode artifacts remain."))
			}
			Expect(err).To(BeNil())

			By("Delete Shoot")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.DeleteShootAndWaitForDeletion(ctx, f.Shoot)).To(Succeed())

		})
	}
})

func runNetworkFilterTest(ctx context.Context, f *framework.ShootCreationFramework, blackholingEnabled bool) string {
	values := struct {
		HelmDeployNamespace string
		KubeVersion         string
		BlackholingEnabled  bool
		BlockAddress        string
	}{
		templates.NetworkTestNamespace,
		f.Shoot.Spec.Kubernetes.Version,
		blackholingEnabled,
		blockAddress,
	}

	err := f.RenderAndDeployTemplate(ctx, f.ShootFramework.ShootClient, templates.NetworkTestName, values)
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

	out, err := framework.PodExecByLabel(ctx, labels.SelectorFromSet(map[string]string{
		v1beta1constants.LabelApp: "filter-test",
	}),
		"filter-block-test",
		"/script/network-filter-test.sh",
		values.HelmDeployNamespace,
		f.ShootFramework.ShootClient,
	)
	Expect(err).NotTo(HaveOccurred())
	outBytes, _ := io.ReadAll(out)
	return string(outBytes)
}
