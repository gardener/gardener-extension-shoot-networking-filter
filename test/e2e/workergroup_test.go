// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"strings"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Worker Group Specific Tests", Label("Network"), func() {

	f := defaultShootCreationFramework()

	f.Shoot = defaultShoot("e2e-worker-group", false, blockAddress, false, false, nil)

	It("Create Shoot, Enable per-worker-group blocking, Delete Shoot", Label("e2e-worker-group"), func() {
		By("Create Shoot")
		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()
		Expect(f.CreateShootAndWaitForCreation(ctx, false)).To(Succeed())
		f.Verify()

		ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()

		setupShootClient(ctx, f)

		By("Add worker group 'blackholed'")
		err := f.UpdateShoot(ctx, f.Shoot, func(shoot *gardencorev1beta1.Shoot) error {
			w := gardencorev1beta1.Worker{
				CRI: &gardencorev1beta1.CRI{
					Name: gardencorev1beta1.CRINameContainerD,
				},
				Name: "blackholed",
				Machine: gardencorev1beta1.Machine{
					Type: "local",
				},
				Maximum: 1,
				Minimum: 1,
			}
			shoot.Spec.Provider.Workers = append(shoot.Spec.Provider.Workers, w)
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Verify that there is only one daemon set")
		dsList, err := f.ShootFramework.ShootClient.Kubernetes().AppsV1().DaemonSets("kube-system").List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		found := 0
		for _, ds := range dsList.Items {
			if strings.Contains(ds.Name, "egress-filter-applier") {
				found++
			}
		}
		Expect(found).To(Equal(1))

		By("Enable per-worker-group blocking")
		groups := []string{"blackholed"}
		updatedShoot := defaultShoot("e2e-worker-group", false, blockAddress, true, true, groups)
		err = f.UpdateShoot(ctx, f.Shoot, func(shoot *gardencorev1beta1.Shoot) error {
			shoot.Spec.Extensions = updatedShoot.Spec.Extensions
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Verify that there are now two daemon sets")
		dsList, err = f.ShootFramework.ShootClient.Kubernetes().AppsV1().DaemonSets("kube-system").List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		var filterDS []appsv1.DaemonSet
		for _, ds := range dsList.Items {
			if strings.Contains(ds.Name, "egress-filter-applier") {
				filterDS = append(filterDS, ds)
			}
		}
		Expect(len(filterDS)).To(Equal(2))

		By("Verify that one daemon set uses blackholing and one does not")
		for _, ds := range filterDS {
			if ds.Spec.Template.Spec.NodeSelector[constants.LabelWorkerPool] == "blackholed" {
				Expect(ds.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-blackholing=true"))
			} else {
				Expect(ds.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-blackholing=false"))
			}
		}

		By("Delete Shoot")
		ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()
		Expect(f.DeleteShootAndWaitForDeletion(ctx, f.Shoot)).To(Succeed())

	})
})
