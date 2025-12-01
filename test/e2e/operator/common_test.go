// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e_operator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/logger"
	. "github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/gardener/gardener/test/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

var (
	parentCtx     context.Context
	runtimeClient client.Client
)

var _ = BeforeSuite(func() {
	Expect(os.Getenv("KUBECONFIG")).NotTo(BeEmpty(), "KUBECONFIG must be set")
	Expect(os.Getenv("REPO_ROOT")).NotTo(BeEmpty(), "REPO_ROOT must be set")

	logf.SetLogger(logger.MustNewZapLogger(logger.InfoLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))

	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, nil, kubernetes.AuthTokenFile, kubernetes.AuthClientCertificate)
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

func waitForGardenToBeReconciled(ctx context.Context, garden *operatorv1alpha1.Garden) {
	CEventually(ctx, func(g Gomega) gardencorev1beta1.LastOperationState {
		g.Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(garden), garden)).To(Succeed())
		if garden.Status.LastOperation == nil || garden.Status.ObservedGeneration != garden.Generation {
			return ""
		}
		return garden.Status.LastOperation.State
	}).WithPolling(2 * time.Second).Should(Equal(gardencorev1beta1.LastOperationStateSucceeded))
}

func waitForOperatorExtensionToBeReconciled(
	ctx context.Context,
	extension *operatorv1alpha1.Extension,
	expectedRuntimeStatus, expectedVirtualStatus gardencorev1beta1.ConditionStatus,
) {
	CEventually(ctx, func(g Gomega) []gardencorev1beta1.Condition {
		g.Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(extension), extension)).To(Succeed())
		if extension.Status.ObservedGeneration != extension.Generation {
			return nil
		}

		return extension.Status.Conditions
	}).WithPolling(2 * time.Second).Should(ContainElements(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionInstalled),
		"Status": Equal(gardencorev1beta1.ConditionTrue),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionRequiredRuntime),
		"Status": Equal(expectedRuntimeStatus),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionRequiredVirtual),
		"Status": Equal(expectedVirtualStatus),
	})))
}

func waitForDaemonSetToBeRunning(ctx context.Context, namespace, name string) {
	CEventually(ctx, func(g Gomega) *appsv1.DaemonSet {
		ds := &appsv1.DaemonSet{}
		g.Expect(runtimeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ds)).To(Succeed())
		return ds
	}).WithPolling(2 * time.Second).Should(HaveField("Status.NumberReady", Equal(int32(1))))
}

func waitForOperatorExtensionToBeDeleted(ctx context.Context, extension *operatorv1alpha1.Extension) {
	CEventually(ctx, func() error {
		return runtimeClient.Get(ctx, client.ObjectKeyFromObject(extension), extension)
	}).WithPolling(2 * time.Second).Should(BeNotFoundError())
}

func waitForExtensionToBeReconciled(ctx context.Context, extension *extensionsv1alpha1.Extension) {
	CEventually(ctx, func(g Gomega) gardencorev1beta1.LastOperationState {
		g.Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(extension), extension)).To(Succeed())
		if extension.Status.LastOperation == nil || extension.Status.ObservedGeneration != extension.Generation {
			return ""
		}
		return extension.Status.LastOperation.State
	}).WithPolling(2 * time.Second).Should(Equal(gardencorev1beta1.LastOperationStateSucceeded))
}

func getExtensionNamespace(ctx context.Context, controllerRegistrationName string) string {
	namespaces := &corev1.NamespaceList{}
	Expect(runtimeClient.List(ctx, namespaces, client.MatchingLabels{
		v1beta1constants.GardenRole:                      v1beta1constants.GardenRoleExtension,
		v1beta1constants.LabelControllerRegistrationName: controllerRegistrationName,
	})).To(Succeed())
	Expect(namespaces.Items).To(HaveLen(1))
	return namespaces.Items[0].Name
}

// ExecMake executes one or multiple make targets.
func execMake(ctx context.Context, targets ...string) error {
	cmd := exec.CommandContext(ctx, "make", targets...)
	cmd.Dir = os.Getenv("REPO_ROOT")
	for _, key := range []string{"PATH", "GOPATH", "HOME", "KUBECONFIG"} {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, os.Getenv(key)))
	}
	cmdString := fmt.Sprintf("running make %s", strings.Join(targets, " "))
	logf.Log.Info(cmdString)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s\n%s", cmdString, err, string(output))
	}
	return nil
}
