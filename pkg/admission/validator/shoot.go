// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"context"
	"fmt"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencorehelper "github.com/gardener/gardener/pkg/api/core/helper"
	"github.com/gardener/gardener/pkg/apis/core"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/install"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/validation"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
)

// NewShootValidator returns a new instance of a shootValidator.
func NewShootValidator() extensionswebhook.Validator {

	scheme := runtime.NewScheme()
	install.Install(scheme)

	decoder := serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder()
	return &shootValidator{
		scheme:  scheme,
		decoder: decoder,
	}
}

type shootValidator struct {
	decoder runtime.Decoder
	scheme  *runtime.Scheme
}

// Validate validates the given shoot object.
func (s *shootValidator) Validate(ctx context.Context, new, old client.Object) error {
	shoot, ok := new.(*core.Shoot)
	if !ok {
		return fmt.Errorf("wrong object type %T", new)
	}
	// Skip if it's a workerless Shoot
	if gardencorehelper.IsWorkerless(shoot) {
		return nil
	}

	return s.validateShoot(ctx, shoot)
}

func (s *shootValidator) validateShoot(_ context.Context, shoot *core.Shoot) error {
	networkFilterExtension, extensionIndex := s.findExtension(shoot)
	if networkFilterExtension == nil {
		return nil
	}

	shootConfig := &v1alpha1.Configuration{}
	if networkFilterExtension.ProviderConfig != nil {
		if _, _, err := s.decoder.Decode(networkFilterExtension.ProviderConfig.Raw, nil, shootConfig); err != nil {
			return fmt.Errorf("failed to decode provider config: %w", err)
		}
	}

	internalShootConfig := &config.Configuration{}
	if err := s.scheme.Convert(shootConfig, internalShootConfig, nil); err != nil {
		return fmt.Errorf("failed to convert shoot config: %w", err)
	}

	fldPath := field.NewPath("spec", "extensions").Index(extensionIndex).Child("providerConfig")
	validationErrors := validation.ValidateProviderConfig(internalShootConfig, fldPath)
	if len(validationErrors) > 0 {
		return field.Invalid(fldPath, networkFilterExtension.ProviderConfig, validationErrors.ToAggregate().Error())
	}
	return nil
}

func (s *shootValidator) findExtension(shoot *core.Shoot) (*core.Extension, int) {
	for i, ext := range shoot.Spec.Extensions {
		if ext.Type == constants.ExtensionType {
			return &shoot.Spec.Extensions[i], i
		}
	}
	return nil, 0
}
