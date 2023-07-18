// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils"
	managedresources "github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/imagevector"
)

const (
	// ActuatorName is the name of the Networking Policy Filter actuator.
	ActuatorName = constants.ServiceName + "-actuator"
)

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(mgr manager.Manager, serviceConfig config.Configuration, oauth2secret *config.OAuth2Secret) (extension.Actuator, error) {
	a := &actuator{
		client:        mgr.GetClient(),
		config:        mgr.GetConfig(),
		scheme:        mgr.GetScheme(),
		decoder:       serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
		logger:        log.Log.WithName(ActuatorName),
		serviceConfig: serviceConfig,
		oauth2secret:  oauth2secret,
	}

	switch a.serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		a.provider = newStaticFilterListProvider(context.Background(), a.client, a.logger, a.serviceConfig.EgressFilter.StaticFilterList)
	case config.FilterListProviderTypeDownload:
		a.provider = newDownloaderFilterListProvider(context.Background(), a.client, a.logger,
			a.serviceConfig.EgressFilter.DownloaderConfig, a.oauth2secret)
	default:
		return nil, fmt.Errorf("unexpected FilterListProviderType: %s", a.serviceConfig.EgressFilter.FilterListProviderType)
	}
	a.logger.Info("Update filter list")
	return a, a.provider.Setup()
}

type actuator struct {
	client        client.Client
	config        *rest.Config
	decoder       runtime.Decoder
	serviceConfig config.Configuration
	oauth2secret  *config.OAuth2Secret
	provider      filterListProvider
	logger        logr.Logger
	scheme        *runtime.Scheme
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, _ logr.Logger, ex *extensionsv1alpha1.Extension) error {
	blackholingEnabled := true
	sleepDuration := "1h"
	staticFilterList := []config.Filter{}
	pspEnabled := true
	secretData := map[string][]byte{
		constants.KeyIPV4List: []byte("[]"),
		constants.KeyIPV6List: []byte("[]"),
	}

	shootConfig := &v1alpha1.Configuration{}
	if ex.Spec.ProviderConfig != nil {
		if _, _, err := a.decoder.Decode(ex.Spec.ProviderConfig.Raw, nil, shootConfig); err != nil {
			return fmt.Errorf("failed to decode provider config: %w", err)
		}
	}

	internalShootConfig := &config.Configuration{}
	if err := a.scheme.Convert(shootConfig, internalShootConfig, nil); err != nil {
		return fmt.Errorf("failed to convert shoot config: %w", err)
	}

	if a.serviceConfig.EgressFilter != nil {
		blackholingEnabled = a.serviceConfig.EgressFilter.BlackholingEnabled
		if a.serviceConfig.EgressFilter.PSPDisabled != nil {
			pspEnabled = !*a.serviceConfig.EgressFilter.PSPDisabled
		}
		if a.serviceConfig.EgressFilter.SleepDuration != nil {
			sleepDuration = a.serviceConfig.EgressFilter.SleepDuration.Duration.String()
		}

		if internalShootConfig.EgressFilter != nil {
			blackholingEnabled = internalShootConfig.EgressFilter.BlackholingEnabled
			staticFilterList = internalShootConfig.EgressFilter.StaticFilterList
		}

		staticProvider, ok := a.provider.(*staticFilterListProvider)
		if ok {
			staticProvider.filterList = a.serviceConfig.EgressFilter.StaticFilterList
			err := a.provider.Setup()
			if err != nil {
				return err
			}
		}

		var err error
		secretData, err = a.readAndRestrictFilterListSecretData(ctx, staticFilterList)
		if err != nil {
			return err
		}
	}

	cluster, err := controller.GetCluster(ctx, a.client, ex.Namespace)
	if err != nil {
		return err
	}

	if gardencorev1beta1helper.IsPSPDisabled(cluster.Shoot) {
		pspEnabled = false
	}

	shootResources, err := getShootResources(blackholingEnabled, pspEnabled, sleepDuration, secretData)
	if err != nil {
		return err
	}

	namespace := ex.GetNamespace()
	return managedresources.CreateForShoot(ctx, a.client, namespace, constants.ManagedResourceNamesShoot, "gardener-extension-shoot-networking-filter", false, shootResources)
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, _ logr.Logger, ex *extensionsv1alpha1.Extension) error {
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
func (a *actuator) Restore(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, log, ex)
}

// Migrate the Extension resource.
func (a *actuator) Migrate(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.client, ex.GetNamespace(), constants.ManagedResourceNamesShoot, true); err != nil {
		return err
	}

	return a.Delete(ctx, log, ex)
}

func (a *actuator) readAndRestrictFilterListSecretData(ctx context.Context, filterList []config.Filter) (map[string][]byte, error) {
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

	secretData, err = appendStaticIPs(a.logger, secretData, filterList)
	if err != nil {
		return nil, err
	}

	filteredSecretData, err := filterSecretDataForIPs(a.logger, secretData, seedLoadBalancerIPs)
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

func getShootResources(blackholingEnabled, pspEnabled bool, sleepDuration string, secretData map[string][]byte) (map[string][]byte, error) {
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

	var objects []client.Object
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.EgressFilterSecretName,
			Namespace: constants.NamespaceKubeSystem,
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}
	objects = append(objects, secret)
	serviceAccountName := ""
	if pspEnabled {
		serviceAccountName = constants.ApplicationName
		pspObjects, err := buildPodSecurityPolicy(serviceAccountName)
		if err != nil {
			return nil, err
		}
		objects = append(objects, pspObjects...)
	}

	daemonset, err := buildDaemonset(checksumEgressFilter, blackholingEnabled, sleepDuration, serviceAccountName)
	if err != nil {
		return nil, err
	}
	objects = append(objects, daemonset)

	shootResources, err := shootRegistry.AddAllAndSerialize(objects...)
	if err != nil {
		return nil, err
	}
	return shootResources, nil
}

func buildDaemonset(checksumEgressFilter string, blackholingEnabled bool, sleepDuration string, serviceAccountName string) (client.Object, error) {
	var (
		requestCPU, _          = resource.ParseQuantity("50m")
		requestMemory, _       = resource.ParseQuantity("64Mi")
		limitMemory, _         = resource.ParseQuantity("256Mi")
		defaultMode      int32 = 0400
		zero             int64 = 0
	)

	labels := map[string]string{
		"k8s-app":             "egress-filter-applier",
		"gardener.cloud/role": "system-component",
	}

	imageName := constants.ImageEgressFilter
	image, err := imagevector.ImageVector().FindImage(imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to find image version for %s: %v", imageName, err)
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.ApplicationName,
			Namespace: constants.NamespaceKubeSystem,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: pointer.Int32(5),
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
					ServiceAccountName:           serviceAccountName,
					Containers: []corev1.Container{{
						Name:            constants.ApplicationName,
						Image:           image.String(),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/filter-updater"},
						Args: []string{
							fmt.Sprintf("-blackholing=%s", strconv.FormatBool(blackholingEnabled)),
							fmt.Sprintf("-filter-list-dir=%s", constants.FilterListPath),
							fmt.Sprintf("-filter-list-ipv4=%s", constants.KeyIPV4List),
							fmt.Sprintf("-filter-list-ipv6=%s", constants.KeyIPV6List),
							fmt.Sprintf("-sleep-duration=%s", sleepDuration),
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    requestCPU,
								corev1.ResourceMemory: requestMemory,
							},
							Limits: corev1.ResourceList{
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
								Name:      constants.FilterListPath,
								ReadOnly:  true,
								MountPath: fmt.Sprintf("/%s", constants.FilterListPath),
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: constants.FilterListPath,
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
	}

	if ds.Spec.Template.Spec.SecurityContext == nil {
		ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}

	ds.Spec.Template.Spec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}

	return ds, nil
}

func buildPodSecurityPolicy(serviceAccountName string) ([]client.Object, error) {
	roleName := "gardener.cloud:psp:kube-system:" + constants.ApplicationName
	resourceName := "gardener.kube-system." + constants.ApplicationName
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:       []string{"policy"},
				Verbs:           []string{"use"},
				Resources:       []string{"podsecuritypolicies"},
				ResourceNames:   []string{resourceName},
				NonResourceURLs: nil,
			},
		},
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: constants.NamespaceKubeSystem,
			},
		},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: constants.NamespaceKubeSystem,
		},
		AutomountServiceAccountToken: pointer.Bool(false),
	}
	t := true
	psp := &policyv1beta1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Annotations: map[string]string{
				"seccomp.security.alpha.kubernetes.io/defaultProfileName":  "runtime/default",
				"seccomp.security.alpha.kubernetes.io/allowedProfileNames": "runtime/default",
			},
		},
		Spec: policyv1beta1.PodSecurityPolicySpec{
			Privileged:               false,
			DefaultAddCapabilities:   nil,
			RequiredDropCapabilities: nil,
			AllowedCapabilities:      []corev1.Capability{"NET_ADMIN"},
			Volumes:                  []policyv1beta1.FSType{"secret"},
			HostNetwork:              true,
			HostPorts:                nil,
			HostPID:                  false,
			HostIPC:                  false,
			SELinux: policyv1beta1.SELinuxStrategyOptions{
				Rule: policyv1beta1.SELinuxStrategyRunAsAny,
			},
			RunAsUser: policyv1beta1.RunAsUserStrategyOptions{
				Rule: policyv1beta1.RunAsUserStrategyRunAsAny,
			},
			RunAsGroup: nil,
			SupplementalGroups: policyv1beta1.SupplementalGroupsStrategyOptions{
				Rule: policyv1beta1.SupplementalGroupsStrategyRunAsAny,
			},
			FSGroup: policyv1beta1.FSGroupStrategyOptions{
				Rule: policyv1beta1.FSGroupStrategyRunAsAny,
			},
			ReadOnlyRootFilesystem:          false,
			DefaultAllowPrivilegeEscalation: nil,
			AllowPrivilegeEscalation:        &t,
			AllowedHostPaths:                nil,
			AllowedFlexVolumes:              nil,
			AllowedCSIDrivers:               nil,
			AllowedUnsafeSysctls:            nil,
			ForbiddenSysctls:                nil,
			AllowedProcMountTypes:           nil,
			RuntimeClass:                    nil,
		},
	}

	return []client.Object{clusterRole, clusterRoleBinding, serviceAccount, psp}, nil
}
