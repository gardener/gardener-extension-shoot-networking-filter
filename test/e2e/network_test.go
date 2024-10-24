// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"fmt"
	"io"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/test/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

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

		f.Shoot = defaultShoot(tc.shootName, tc.blackholingEnabled, blockAddress, false, false, nil)

		It("Create Shoot, Test Policy Filter, Delete Shoot", Label(tc.shootName), func() {
			By("Create Shoot")
			ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.CreateShootAndWaitForCreation(ctx, false)).To(Succeed())
			f.Verify()

			By("Test Policy Filter")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()

			setupShootClient(ctx, f)

			out := runNetworkFilterTest(ctx, f, tc.blackholingEnabled)

			Expect(out).To(ContainSubstring("SUCCESS: Egress is blocked."))
			if tc.blackholingEnabled {
				Expect(out).To(ContainSubstring("SUCCESS: Ingress is blocked."))
			}

			By(fmt.Sprintf("Switching to blackholingEnabled = %t", !tc.blackholingEnabled))
			updatedShoot := defaultShoot(tc.shootName, !tc.blackholingEnabled, blockAddress, false, false, nil)
			err := f.UpdateShoot(ctx, f.Shoot, func(shoot *gardencorev1beta1.Shoot) error {
				copy(shoot.Spec.Extensions, updatedShoot.Spec.Extensions)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("Test Policy Filter after switch")
			out = runNetworkFilterTest(ctx, f, !tc.blackholingEnabled)

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

	err = f.ShootFramework.WaitUntilDaemonSetIsRunning(
		ctx,
		f.ShootFramework.ShootClient.Client(),
		"filter-test",
		values.HelmDeployNamespace,
	)
	Expect(err).NotTo(HaveOccurred())

	defer func() {
		By("Deleting filter-test daemonset")
		err := f.ShootFramework.ShootClient.Kubernetes().AppsV1().DaemonSets(templates.NetworkTestNamespace).Delete(ctx, "filter-test", v1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}()

	By("filter-test daemonset is deployed successfully!")

	out, err := framework.PodExecByLabel(ctx, labels.SelectorFromSet(map[string]string{
		v1beta1constants.LabelApp: "filter-test",
	}),
		"filter-block-test",
		"/script/network-filter-test.sh",
		values.HelmDeployNamespace,
		f.ShootFramework.ShootClient,
	)
	Expect(out).ToNot(BeNil())
	outBytes, _ := io.ReadAll(out)
	fmt.Println(string(outBytes))
	Expect(err).NotTo(HaveOccurred())
	return string(outBytes)
}
