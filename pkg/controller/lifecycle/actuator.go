// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	extensionsv1alpha1helper "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1/helper"
	kubernetesclient "github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/extensions"
	"github.com/gardener/gardener/pkg/utils"
	gardenletutils "github.com/gardener/gardener/pkg/utils/gardener/gardenlet"
	"github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-shoot-networking-filter/imagevector"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
)

const (
	// ActuatorName is the name of the Networking Policy Filter actuator.
	ActuatorName = constants.ServiceName + "-actuator"
)

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(mgr manager.Manager, serviceConfig config.Configuration, oauth2secret *config.OAuth2Secret, extensionClasses []extensionsv1alpha1.ExtensionClass) (extension.Actuator, error) {
	a := &actuator{
		client:           mgr.GetClient(),
		config:           mgr.GetConfig(),
		scheme:           mgr.GetScheme(),
		decoder:          serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
		logger:           log.Log.WithName(ActuatorName),
		serviceConfig:    serviceConfig,
		oauth2secret:     oauth2secret,
		extensionClasses: extensionClasses,
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
	client           client.Client
	config           *rest.Config
	decoder          runtime.Decoder
	extensionClasses []extensionsv1alpha1.ExtensionClass
	serviceConfig    config.Configuration
	oauth2secret     *config.OAuth2Secret
	provider         FilterListProvider
	logger           logr.Logger
	scheme           *runtime.Scheme
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, _ logr.Logger, ex *extensionsv1alpha1.Extension) error {
	var (
		blackholingEnabled = true
		sleepDuration      = "1h"
		staticFilterList   = []config.Filter{}
		secretData         = map[string][]byte{
			constants.KeyIPV4List: []byte("[]"),
			constants.KeyIPV6List: []byte("[]"),
		}
		blackholingEnabledByWorker map[string]bool
		namespace                  = ex.GetNamespace()
		isShootDeployment          = isShootDeployment(ex)
		cluster                    *extensions.Cluster
		err                        error
	)

	if isShootDeployment {
		cluster, err = controller.GetCluster(ctx, a.client, namespace)
		if err != nil {
			return fmt.Errorf("failed to get cluster config: %w", err)
		}
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

	if err := ValidateProviderConfig(internalShootConfig); err != nil {
		return fmt.Errorf("failed to validate provider config: %w", err)
	}

	if a.serviceConfig.EgressFilter != nil {
		blackholingEnabled = a.serviceConfig.EgressFilter.BlackholingEnabled
		tagFilters := a.serviceConfig.EgressFilter.TagFilters

		if a.serviceConfig.EgressFilter.SleepDuration != nil {
			sleepDuration = a.serviceConfig.EgressFilter.SleepDuration.Duration.String()
		}

		if internalShootConfig.EgressFilter != nil {
			blackholingEnabled = internalShootConfig.EgressFilter.BlackholingEnabled
			staticFilterList = internalShootConfig.EgressFilter.StaticFilterList

			if len(internalShootConfig.EgressFilter.TagFilters) > 0 {
				// Append shoot-specific tag filters to service-level filters
				tagFilters = append(tagFilters, internalShootConfig.EgressFilter.TagFilters...)
			}

			if isShootDeployment {
				if internalShootConfig.EgressFilter.Workers != nil {
					blackholingEnabledByWorker = make(map[string]bool)
					workerSet := sets.New[string](internalShootConfig.EgressFilter.Workers.Names...)
					for _, worker := range cluster.Shoot.Spec.Provider.Workers {
						if workerSet.Has(worker.Name) {
							blackholingEnabledByWorker[worker.Name] = internalShootConfig.EgressFilter.Workers.BlackholingEnabled
						} else {
							blackholingEnabledByWorker[worker.Name] = blackholingEnabled
						}
					}
				}
			}
		}

		staticProvider, ok := a.provider.(*StaticFilterListProvider)
		if ok {
			staticProvider.filterList = a.serviceConfig.EgressFilter.StaticFilterList
			err := a.provider.Setup()
			if err != nil {
				return err
			}
		}

		var err error
		var projectFilterListSource *config.SecretRef
		if internalShootConfig.EgressFilter != nil && isShootDeployment {
			projectFilterListSource = internalShootConfig.EgressFilter.ProjectFilterListSource
		}
		secretData, err = a.readAndRestrictFilterListSecretData(ctx, namespace, staticFilterList, tagFilters, projectFilterListSource)
		if err != nil {
			return err
		}
	}

	shootResources, err := getShootResources(blackholingEnabled, sleepDuration, constants.NamespaceKubeSystem, secretData, blackholingEnabledByWorker)
	if err != nil {
		return err
	}

	if isShootDeployment {
		return managedresources.CreateForShoot(ctx, a.client, namespace, constants.ManagedResourceNamesShoot, "gardener-extension-shoot-networking-filter", false, shootResources)
	} else {
		name, err := a.getRuntimeOrSeedManagedResourceName()
		if err != nil {
			return err
		}
		if name == constants.ManagedResourceNamesSeed {
			seedIsGarden, err := gardenletutils.SeedIsGarden(ctx, a.client)
			if err != nil {
				return fmt.Errorf("failed to check if seed is the Garden runtime cluster: %w", err)
			}
			if seedIsGarden {
				// nothing to do, the resources are already deployed for the runtime cluster.
				shootResources = map[string][]byte{}
			}
		}
		return managedresources.CreateForSeed(ctx, a.client, namespace, name, false, shootResources)
	}
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, _ logr.Logger, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.GetNamespace()
	twoMinutes := 2 * time.Minute

	timeoutShootCtx, cancelShootCtx := context.WithTimeout(ctx, twoMinutes)
	defer cancelShootCtx()

	if isShootDeployment(ex) {
		if err := managedresources.DeleteForShoot(ctx, a.client, namespace, constants.ManagedResourceNamesShoot); err != nil {
			return err
		}

		if err := managedresources.WaitUntilDeleted(timeoutShootCtx, a.client, namespace, constants.ManagedResourceNamesShoot); err != nil {
			return err
		}
	} else {
		name, err := a.getRuntimeOrSeedManagedResourceName()
		if err != nil {
			return err
		}

		if err := managedresources.DeleteForSeed(ctx, a.client, namespace, name); err != nil {
			return err
		}

		if err := managedresources.WaitUntilDeleted(timeoutShootCtx, a.client, namespace, name); err != nil {
			return err
		}
	}

	return nil
}

// ForceDelete implements Network.Actuator.
func (a *actuator) ForceDelete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	return a.Delete(ctx, log, ex)
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

func (a *actuator) readAndRestrictFilterListSecretData(ctx context.Context, namespace string, staticFilterList []config.Filter, tagFilters []config.TagFilter, projectFilterListSource *config.SecretRef) (map[string][]byte, error) {
	var combinedFilterList []config.Filter

	// If project filter list is configured, use it instead of downloaded data
	if projectFilterListSource != nil {
		projectFilters, err := a.readProjectFilterList(ctx, namespace, projectFilterListSource)
		if err != nil {
			a.logger.Error(err, "failed to read project filter list, falling back to downloaded data")
			// Fall back to downloaded data on error
			combinedFilterList = a.combineDownloadedAndStaticFilters(staticFilterList, tagFilters)
		} else {
			a.logger.Info("using project filter list instead of downloaded data", "projectEntries", len(projectFilters), "staticEntries", len(staticFilterList))
			// Apply tag filters to project filters if configured
			if len(tagFilters) > 0 {
				projectFilters = filterByTags(projectFilters, tagFilters, a.logger)
			}
			// Combine static filters with project filters
			combinedFilterList = append(staticFilterList, projectFilters...)
		}
	} else {
		// No project filter list, use downloaded data
		combinedFilterList = a.combineDownloadedAndStaticFilters(staticFilterList, tagFilters)
	}

	// Generate IPv4/IPv6 lists from combined filter list
	ipv4List, ipv6List, err := generateEgressFilterValues(combinedFilterList, a.logger)
	if err != nil {
		return nil, err
	}

	a.logger.Info("filter lists generated", constants.KeyIPV4List, len(ipv4List), constants.KeyIPV6List, len(ipv6List))

	secretData := map[string][]byte{
		constants.KeyIPV4List: []byte(convertToPlainYamlList(ipv4List)),
		constants.KeyIPV6List: []byte(convertToPlainYamlList(ipv6List)),
	}

	// Apply seed load balancer filtering if configured
	if a.serviceConfig.EgressFilter.EnsureConnectivity != nil && len(a.serviceConfig.EgressFilter.EnsureConnectivity.SeedNamespaces) > 0 {
		seedLoadBalancerIPs, err := a.collectSeedLoadBalancersIPs(ctx, a.serviceConfig.EgressFilter.EnsureConnectivity.SeedNamespaces)
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
		return filteredSecretData, nil
	}

	return secretData, nil
}

func (a *actuator) getRuntimeOrSeedManagedResourceName() (string, error) {
	for _, class := range a.extensionClasses {
		if class == extensionsv1alpha1.ExtensionClassSeed {
			return constants.ManagedResourceNamesSeed, nil
		}
		if class == extensionsv1alpha1.ExtensionClassGarden {
			return constants.ManagedResourceNamesGarden, nil
		}
	}
	return "", fmt.Errorf("no managed resource name as extension classes unexpected")
}

// combineDownloadedAndStaticFilters applies tag filters to downloaded data and combines with static filters
func (a *actuator) combineDownloadedAndStaticFilters(staticFilterList []config.Filter, tagFilters []config.TagFilter) []config.Filter {
	downloadedFilterList := a.provider.GetFilterList()
	if len(tagFilters) > 0 {
		downloadedFilterList = filterByTags(downloadedFilterList, tagFilters, a.logger)
	}
	return append(downloadedFilterList, staticFilterList...)
}

// readProjectFilterList reads filter list from a Secret synced to the shoot namespace.
// The Secret must be referenced in Shoot.spec.resources for automatic syncing.
func (a *actuator) readProjectFilterList(ctx context.Context, namespace string, ref *config.SecretRef) ([]config.Filter, error) {
	key := client.ObjectKey{
		Namespace: namespace,         // Shoot namespace in seed
		Name:      "ref-" + ref.Name, // Gardener adds "ref-" prefix to synced resources
	}

	// Get the key, default to "filterList"
	dataKey := ref.Key
	if dataKey == "" {
		dataKey = "filterList"
	}

	// Read from Secret
	secret := &corev1.Secret{}
	if err := a.client.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s (ensure it's listed in Shoot.spec.resources): %w", key.Namespace, key.Name, err)
	}

	data, ok := secret.Data[dataKey]
	if !ok {
		return nil, fmt.Errorf("key %q not found in Secret %s/%s", dataKey, key.Namespace, key.Name)
	}

	// Try to decompress if gzip-encoded
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err == nil {
		defer gr.Close()
		decompressed, err := io.ReadAll(gr)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip data: %w", err)
		}
		a.logger.Info("decompressed project filter list", "compressed", len(data), "decompressed", len(decompressed))
		data = decompressed
	} else if err != gzip.ErrHeader {
		// ErrHeader means it's not gzipped (plain text), which is fine
		// Any other error is a real problem
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}

	// Parse filter list (supports both v1 and v2 formats)
	filters, err := parseFilterList(data)
	if err != nil {
		return nil, err
	}

	return filters, nil
}

// filterByTags applies tag-based policy overrides to the filter list.
// All entries are included, but entries matching tag filters get their policies overridden.
func filterByTags(filterList []config.Filter, tagFilters []config.TagFilter, logger logr.Logger) []config.Filter {
	if len(tagFilters) == 0 {
		return filterList
	}

	result := make([]config.Filter, len(filterList))
	overrideCount := 0

	for i, filter := range filterList {
		result[i] = filter
		if matchingFilter, matches := getMatchingTagFilterWithPolicy(filter, tagFilters); matches && matchingFilter != nil && matchingFilter.Policy != nil {
			// Override policy if tag filter specifies one
			result[i].Policy = *matchingFilter.Policy
			overrideCount++
		}
		// Keep original policy if no matching tag filter or no policy specified
	}

	logger.Info("applied tag-based policy overrides", "total", len(filterList), "overridden", overrideCount)
	return result
}

// getMatchingTagFilterWithPolicy checks if a filter matches any of the tag filters
// and returns the matching tag filter with the highest priority (last in list).
// Returns (matchingFilter, true) if matched, (nil, false) if not matched.
func getMatchingTagFilterWithPolicy(filter config.Filter, tagFilters []config.TagFilter) (*config.TagFilter, bool) {
	// If no tags on filter, it doesn't match any tag filter (keeps original policy)
	if len(filter.Tags) == 0 {
		return nil, false
	}

	// Track all matching tag filters (later ones have higher priority)
	var lastMatchingFilter *config.TagFilter

	// Check if filter has any of the requested tags with matching values
	for i := range tagFilters {
		tagFilter := &tagFilters[i]
		for _, filterTag := range filter.Tags {
			if filterTag.Name == tagFilter.Name {
				// Check if any value matches
				for _, filterValue := range filterTag.Values {
					for _, allowedValue := range tagFilter.Values {
						if filterValue == allowedValue {
							// This tag filter matches, remember it
							lastMatchingFilter = tagFilter
							goto nextTagFilter
						}
					}
				}
			}
		}
	nextTagFilter:
	}

	if lastMatchingFilter != nil {
		return lastMatchingFilter, true
	}
	return nil, false
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

// GetShootResources creates resources needed for the egress filter daemonset.
func GetShootResources(blackholingEnabled bool, sleepDuration, namespace string, secretData map[string][]byte) (map[string][]byte, error) {
	return getShootResources(blackholingEnabled, sleepDuration, namespace, secretData, nil)
}

func getShootResources(blackholingEnabled bool, sleepDuration, namespace string, secretData map[string][]byte, workerGroupBlackholingEnabled map[string]bool) (map[string][]byte, error) {
	shootRegistry := managedresources.NewRegistry(kubernetesclient.ShootScheme, kubernetesclient.ShootCodec, kubernetesclient.ShootSerializer)

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
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}
	objects = append(objects, secret)

	// Two cases:
	// Case A: No worker group-specific blocking => Only one DS for everyone
	if workerGroupBlackholingEnabled == nil {
		daemonset, err := buildDaemonset(checksumEgressFilter, blackholingEnabled, sleepDuration, namespace, "")
		if err != nil {
			return nil, err
		}
		objects = append(objects, daemonset)
	} else {
		// Case B: Worker group-specific blocking => One DS per worker group
		for workerGroup, blackholingEnabled := range workerGroupBlackholingEnabled {
			daemonset, err := buildDaemonset(checksumEgressFilter, blackholingEnabled, sleepDuration, namespace, workerGroup)
			if err != nil {
				return nil, err
			}
			objects = append(objects, daemonset)
		}
	}

	shootResources, err := shootRegistry.AddAllAndSerialize(objects...)
	if err != nil {
		return nil, err
	}
	return shootResources, nil
}

func buildDaemonset(checksumEgressFilter string, blackholingEnabled bool, sleepDuration, namespace string, workerGroup string) (client.Object, error) {
	var (
		requestCPU, _          = resource.ParseQuantity("5m")
		requestMemory, _       = resource.ParseQuantity("20Mi")
		limitMemory, _         = resource.ParseQuantity("256Mi")
		defaultMode      int32 = 0400
		zero             int64 = 0
		hostPathType           = corev1.HostPathFileOrCreate
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
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			RevisionHistoryLimit: ptr.To(int32(5)),
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
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: labels,
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						},
					},
					AutomountServiceAccountToken: ptr.To(false),
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
							AllowPrivilegeEscalation: ptr.To(false),
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
							{
								Name:      constants.XtablesLockName,
								ReadOnly:  false,
								MountPath: constants.XtablesLockPath,
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
						{
							Name: constants.XtablesLockName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: constants.XtablesLockPath,
									Type: &hostPathType,
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

	if workerGroup != "" {
		ds.Spec.Template.Spec.NodeSelector = map[string]string{
			v1beta1constants.LabelWorkerPool: workerGroup,
		}
		ds.Name = fmt.Sprintf("%s-%s", ds.Name, workerGroup)
	}

	return ds, nil
}

func isShootDeployment(ex *extensionsv1alpha1.Extension) bool {
	return extensionsv1alpha1helper.GetExtensionClassOrDefault(ex.Spec.Class) == extensionsv1alpha1.ExtensionClassShoot
}
