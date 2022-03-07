// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/imagevector"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils"
	managedresources "github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ActuatorName is the name of the Networking Policy Filter actuator.
	ActuatorName = constants.ServiceName + "-actuator"
)

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(config config.Configuration) extension.Actuator {
	return &actuator{
		logger:        log.Log.WithName(ActuatorName),
		serviceConfig: config,
	}
}

type actuator struct {
	client        client.Client
	config        *rest.Config
	decoder       runtime.Decoder
	serviceConfig config.Configuration
	logger        logr.Logger
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.GetNamespace()
	/*
		cluster, err := controller.GetCluster(ctx, a.client, namespace)
		if err != nil {
			return err
		}
	*/
	blackholingEnabled := true
	secretData := map[string][]byte{
		constants.KeyIPV4List: []byte("[]"),
		constants.KeyIPV6List: []byte("[]"),
	}
	if a.serviceConfig.EgressFilter != nil {
		blackholingEnabled = a.serviceConfig.EgressFilter.BlackholingEnabled
		var err error
		secretData, err = a.readFilterListSecretData(ctx)
		if err != nil {
			return err
		}
	}

	shootResources, err := getShootResources(blackholingEnabled, secretData)
	if err != nil {
		return err
	}

	return managedresources.CreateForShoot(ctx, a.client, namespace, constants.ManagedResourceNamesShoot, false, shootResources)
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.GetNamespace()
	twoMinutes := 2 * time.Minute

	timeoutShootCtx, cancelShootCtx := context.WithTimeout(ctx, twoMinutes)
	defer cancelShootCtx()

	if err := managedresources.DeleteForShoot(ctx, a.client, namespace, constants.ManagedResourceNamesShoot); err != nil {
		return err
	}

	if err := managedresources.WaitUntilDeleted(timeoutShootCtx, a.client, namespace, constants.ManagedResourceNamesShoot); err != nil {
		return err
	}

	return nil
}

// Restore the Extension resource.
func (a *actuator) Restore(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, ex)
}

// Migrate the Extension resource.
func (a *actuator) Migrate(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.client, ex.GetNamespace(), constants.ManagedResourceNamesShoot, true); err != nil {
		return err
	}

	return a.Delete(ctx, ex)
}

// InjectConfig injects the rest config to this actuator.
func (a *actuator) InjectConfig(config *rest.Config) error {
	a.config = config
	return nil
}

// InjectClient injects the controller runtime client into the reconciler.
func (a *actuator) InjectClient(client client.Client) error {
	a.client = client
	return a.setupFilterListProvider()
}

// InjectScheme injects the given scheme into the reconciler.
func (a *actuator) InjectScheme(scheme *runtime.Scheme) error {
	a.decoder = serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder()
	return nil
}

func (a *actuator) readFilterListSecretData(ctx context.Context) (map[string][]byte, error) {
	namespace, err := getExtensionDeploymentNamespace()
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: constants.FilterListSecretName, Namespace: namespace}
	err = a.client.Get(ctx, key, secret)
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}

func (a *actuator) setupFilterListProvider() error {
	switch a.serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		p := newStaticFilterListProvider(context.Background(), a.client, a.logger, a.serviceConfig.EgressFilter.StaticFilterList)
		return p.setup()
	case config.FilterListProviderTypeDownload:
		p := newDownloaderFilterListProvider(context.Background(), a.client, a.logger, a.serviceConfig.EgressFilter.DownloaderConfig)
		return p.setup()
	default:
		return fmt.Errorf("unexpected FilterListProviderType: %s", a.serviceConfig.EgressFilter.FilterListProviderType)
	}
}

func getExtensionDeploymentNamespace() (string, error) {
	namespace := os.Getenv(constants.ExtensionNamespaceEnvName)
	if namespace == "" {
		return "", fmt.Errorf("missing env variable %q", constants.ExtensionNamespaceEnvName)
	}
	return namespace, nil
}

func getShootResources(blackholingEnabled bool, secretData map[string][]byte) (map[string][]byte, error) {
	shootRegistry := managedresources.NewRegistry(kubernetes.ShootScheme, kubernetes.ShootCodec, kubernetes.ShootSerializer)

	if secretData == nil {
		return nil, fmt.Errorf("missing filter list secret data")
	}
	for _, key := range []string{constants.KeyIPV4List, constants.KeyIPV6List} {
		if _, ok := secretData[key]; !ok {
			return nil, fmt.Errorf("missing key %q in filter list secret data", key)
		}
	}

	checksumEgressFilter := utils.ComputeSecretChecksum(secretData)

	daemonset, err := buildDaemonset(checksumEgressFilter, blackholingEnabled)
	if err != nil {
		return nil, err
	}
	shootResources, err := shootRegistry.AddAllAndSerialize(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.EgressFilterSecretName,
				Namespace: constants.NamespaceKubeSystem,
			},
			Type: corev1.SecretTypeOpaque,
			Data: secretData,
		},
		daemonset,
	)

	if err != nil {
		return nil, err
	}
	return shootResources, nil
}

func buildDaemonset(checksumEgressFilter string, blackholingEnabled bool) (client.Object, error) {
	var (
		requestCPU, _          = resource.ParseQuantity("50m")
		limitCPU, _            = resource.ParseQuantity("100m")
		requestMemory, _       = resource.ParseQuantity("64Mi")
		limitMemory, _         = resource.ParseQuantity("256Mi")
		defaultMode      int32 = 0400
		zero             int64 = 0
	)

	labels := map[string]string{
		"k8s-app":             "egress-filter-applier",
		"gardener.cloud/role": "system-component",
	}

	imageName := constants.ImageEgressFilterBlackholer
	if !blackholingEnabled {
		imageName = constants.ImageEgressFilterFirwaller
	}
	image, err := imagevector.ImageVector().FindImage(imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to find image version for %s: %v", imageName, err)
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.ApplicationName,
			Namespace: constants.NamespaceKubeSystem,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: pointer.Int32Ptr(5),
			Selector:             &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "100%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"checksum/" + constants.EgressFilterSecretName: checksumEgressFilter,
					},
				},
				Spec: corev1.PodSpec{
					HostNetwork:                   true,
					PriorityClassName:             "system-node-critical",
					TerminationGracePeriodSeconds: &zero,
					Tolerations: []corev1.Toleration{
						{
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "CriticalAddonsOnly",
							Operator: corev1.TolerationOpExists,
						},
						{
							Effect:   corev1.TaintEffectNoExecute,
							Operator: corev1.TolerationOpExists,
						},
					},
					AutomountServiceAccountToken: pointer.Bool(false),
					Containers: []corev1.Container{{
						Name:            constants.ApplicationName,
						Image:           image.String(),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    requestCPU,
								corev1.ResourceMemory: requestMemory,
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    limitCPU,
								corev1.ResourceMemory: limitMemory,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{"NET_ADMIN"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "lists",
								ReadOnly:  true,
								MountPath: "/lists",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "lists",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  constants.EgressFilterSecretName,
									DefaultMode: &defaultMode,
								},
							},
						},
					},
				},
			},
		},
	}, nil
}
