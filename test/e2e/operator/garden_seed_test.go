// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_operator_test

import (
	"context"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	operatorclient "github.com/gardener/gardener/pkg/operator/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Network-Filter Tests", func() {
	var (
		garden            = &operatorv1alpha1.Garden{ObjectMeta: metav1.ObjectMeta{Name: "local"}}
		seed              = &gardencorev1beta1.Seed{ObjectMeta: metav1.ObjectMeta{Name: "local"}}
		operatorExtension = &operatorv1alpha1.Extension{ObjectMeta: metav1.ObjectMeta{Name: "extension-shoot-networking-filter"}}
		runtimeExtension  = &extensionsv1alpha1.Extension{ObjectMeta: metav1.ObjectMeta{Namespace: "garden", Name: "garden-shoot-networking-filter"}}
		seedExtension     = &extensionsv1alpha1.Extension{ObjectMeta: metav1.ObjectMeta{Namespace: "garden", Name: "shoot-networking-filter"}}

		rawExtension = &runtime.RawExtension{}
	)

	It("Create, Delete", Label("simple"), func() {
		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()

		By("Deploy Extension")
		Expect(execMake(ctx, "extension-operator-up")).To(Succeed())

		By("Get Virtual Garden Client")
		virtualClusterClient, err := kubernetes.NewClientFromSecret(ctx, runtimeClient, v1beta1constants.GardenNamespace, "gardener",
			kubernetes.WithDisabledCachedClient(),
			kubernetes.WithClientOptions(client.Options{Scheme: operatorclient.VirtualScheme}),
		)
		Expect(err).NotTo(HaveOccurred())

		By("Patch Seed: Add extension")
		Expect(virtualClusterClient.Client().Get(ctx, client.ObjectKeyFromObject(seed), seed)).To(Succeed())
		seedPatch := client.MergeFrom(seed.DeepCopy())
		seed.Spec.Extensions = []gardencorev1beta1.Extension{
			{
				Type:           "shoot-networking-filter",
				ProviderConfig: rawExtension,
			},
		}
		Expect(virtualClusterClient.Client().Patch(ctx, seed, seedPatch)).To(Succeed())

		By("Patch Garden: Add garden extension")
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(garden), garden)).To(Succeed())
		patch := client.MergeFrom(garden.DeepCopy())
		garden.Spec.Extensions = []operatorv1alpha1.GardenExtension{
			{
				Type:           "shoot-networking-filter",
				ProviderConfig: rawExtension,
			},
		}
		Expect(runtimeClient.Patch(ctx, garden, patch)).To(Succeed())

		waitForGardenToBeReconciled(ctx, garden)

		By("Check Operator Extension required status")
		waitForOperatorExtensionToBeReconciled(ctx, operatorExtension, gardencorev1beta1.ConditionTrue, gardencorev1beta1.ConditionTrue)

		By("Check Garden Runtime Extension")
		waitForExtensionToBeReconciled(ctx, runtimeExtension)

		By("Check Seed Extension")
		// seedExtension.Namespace = getExtensionNamespace(ctx, "extension-shoot-networking-filter")
		waitForExtensionToBeReconciled(ctx, seedExtension)

		By("Check DaemonSet")
		waitForDaemonSetToBeRunning(ctx, "kube-system", "egress-filter-applier")

		By("Patch Garden: Remove garden extension")
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(garden), garden)).To(Succeed())
		patch = client.MergeFrom(garden.DeepCopy())
		garden.Spec.Extensions = nil
		Expect(runtimeClient.Patch(ctx, garden, patch)).To(Succeed())
		waitForGardenToBeReconciled(ctx, garden)

		By("Patch Seed: Remove extension")
		Expect(virtualClusterClient.Client().Get(ctx, client.ObjectKeyFromObject(seed), seed)).To(Succeed())
		seedPatch = client.MergeFrom(seed.DeepCopy())
		seed.Spec.Extensions = nil
		Expect(virtualClusterClient.Client().Patch(ctx, seed, seedPatch)).To(Succeed())

		By("Check Operator Extension required status")
		waitForOperatorExtensionToBeReconciled(ctx, operatorExtension, gardencorev1beta1.ConditionFalse, gardencorev1beta1.ConditionFalse)

		By("Delete Extension")
		Expect(runtimeClient.Delete(ctx, operatorExtension)).To(Succeed())
		waitForOperatorExtensionToBeDeleted(ctx, operatorExtension)
	})
})
