// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	_ "embed"
	"fmt"
	"net"
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
func NewActuator(config config.Configuration, oauth2secret *config.OAuth2Secret) extension.Actuator {
	return &actuator{
		logger:        log.Log.WithName(ActuatorName),
		serviceConfig: config,
		oauth2secret:  oauth2secret,
	}
}

type actuator struct {
	client        client.Client
	config        *rest.Config
	decoder       runtime.Decoder
	serviceConfig config.Configuration
	oauth2secret  *config.OAuth2Secret
	provider      filterListProvider
	logger        logr.Logger
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	blackholingEnabled := true
	secretData := map[string][]byte{
		constants.KeyIPV4List: []byte("[]"),
		constants.KeyIPV6List: []byte("[]"),
	}
	if a.serviceConfig.EgressFilter != nil {
		blackholingEnabled = a.serviceConfig.EgressFilter.BlackholingEnabled
		var err error
		secretData, err = a.readAndRestrictFilterListSecretData(ctx)
		if err != nil {
			return err
		}
	}

	shootResources, err := getShootResources(blackholingEnabled, secretData)
	if err != nil {
		return err
	}

	namespace := ex.GetNamespace()
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

func (a *actuator) readAndRestrictFilterListSecretData(ctx context.Context) (map[string][]byte, error) {
	secretData, err := a.provider.ReadSecretData(ctx)
	if err != nil {
		return nil, err
	}
	if a.serviceConfig.EgressFilter.EnsureConnectivity == nil || len(a.serviceConfig.EgressFilter.EnsureConnectivity.SeedNamespaces) == 0 {
		return secretData, err
	}

	seedLoadBalancerIPs, err := a.collectSeedLoadBalancersIPs(ctx, a.serviceConfig.EgressFilter.EnsureConnectivity.SeedNamespaces)
	if err != nil {
		return nil, err
	}

	filteredSecretData, err := filterSecretDataForIPs(secretData, seedLoadBalancerIPs)
	if err != nil {
		return nil, err
	}

	modified := false
	for _, key := range []string{constants.KeyIPV4List, constants.KeyIPV6List} {
		if len(secretData[key]) != len(filteredSecretData[key]) {
			modified = true
			a.logger.Info(fmt.Sprintf("modified filterList %s: len changed from %d to %d", key, len(secretData[key]), len(filteredSecretData[key])))
		}
	}
	if !modified {
		a.logger.Info("filterList unmodified by seed load balancers")
	}
	return filteredSecretData, err
}

func (a *actuator) collectSeedLoadBalancersIPs(ctx context.Context, namespaces []string) ([]net.IP, error) {
	var result []net.IP
	var countLBs int
	for _, ns := range namespaces {
		list := &corev1.ServiceList{}
		nsclient := client.NewNamespacedClient(a.client, ns)
		err := nsclient.List(ctx, list)
		if err != nil {
			return nil, err
		}
		for _, svc := range list.Items {
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				for _, ingress := range svc.Status.LoadBalancer.Ingress {
					countLBs++
					if ingress.IP != "" {
						if ip := net.ParseIP(ingress.IP); ip != nil {
							result = append(result, ip)
						}
					} else if ingress.Hostname != "" {
						if ips, err := net.LookupIP(ingress.Hostname); err == nil {
							result = append(result, ips...)
						} else {
							a.logger.Info("cannot lookup svc loadbalancer", "err", err)
						}
					}
				}
			}
		}
	}
	a.logger.Info(fmt.Sprintf("found %d seed load balancers with %d IP addresses in %d namespaces", countLBs, len(result), len(namespaces)))
	return result, nil
}

func (a *actuator) setupFilterListProvider() error {
	switch a.serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		a.provider = newStaticFilterListProvider(context.Background(), a.client, a.logger, a.serviceConfig.EgressFilter.StaticFilterList)
	case config.FilterListProviderTypeDownload:
		a.provider = newDownloaderFilterListProvider(context.Background(), a.client, a.logger,
			a.serviceConfig.EgressFilter.DownloaderConfig, a.oauth2secret)
	default:
		return fmt.Errorf("unexpected FilterListProviderType: %s", a.serviceConfig.EgressFilter.FilterListProviderType)
	}
	return a.provider.Setup()
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
