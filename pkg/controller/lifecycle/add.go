// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"time"

	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	controllerconfig "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/config"
)

const (
	// Type is the type of Extension resource.
	Type = constants.ExtensionType
	// Name is the name of the lifecycle controller.
	Name = "shoot_networking_filter_lifecycle_controller"
	// FinalizerSuffix is the finalizer suffix for the Networking Policy Filter controller.
	FinalizerSuffix = constants.ExtensionType
)

// DefaultAddOptions contains configuration for the policy filter.
var DefaultAddOptions = AddOptions{}

// AddOptions are options to apply when adding the policy filter controller to the manager.
type AddOptions struct {
	// ControllerOptions contains options for the controller.
	ControllerOptions controller.Options
	// ServiceConfig contains configuration for the policy filter.
	ServiceConfig controllerconfig.Config
	// IgnoreOperationAnnotation specifies whether to ignore the operation annotation or not.
	IgnoreOperationAnnotation bool
	// ExtensionClass defines the main extension class this extension is responsible for.
	ExtensionClass extensionsv1alpha1.ExtensionClass
}

// AddToManager adds a Networking Policy Filter Lifecycle controller to the given Controller Manager.
func AddToManager(ctx context.Context, mgr manager.Manager) error {
	extensionClasses := []extensionsv1alpha1.ExtensionClass{extensionsv1alpha1.ExtensionClassShoot, extensionsv1alpha1.ExtensionClassSeed}
	if DefaultAddOptions.ExtensionClass == extensionsv1alpha1.ExtensionClassGarden {
		extensionClasses = []extensionsv1alpha1.ExtensionClass{extensionsv1alpha1.ExtensionClassGarden}
	}

	actuator, err := NewActuator(mgr, DefaultAddOptions.ServiceConfig.Configuration, DefaultAddOptions.ServiceConfig.OAuth2Secret, extensionClasses)
	if err != nil {
		return err
	}

	return extension.Add(mgr, extension.AddArgs{
		Actuator:          actuator,
		ControllerOptions: DefaultAddOptions.ControllerOptions,
		Name:              Name,
		FinalizerSuffix:   FinalizerSuffix,
		Resync:            60 * time.Minute,
		Predicates:        extension.DefaultPredicates(ctx, mgr, DefaultAddOptions.IgnoreOperationAnnotation),
		Type:              constants.ExtensionType,
		ExtensionClasses:  extensionClasses,
	})
}
