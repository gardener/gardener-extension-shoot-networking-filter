# Gardener Networking Policy Filter for Shoots

## Introduction
Gardener allows Shoot clusters to dynamically register OpenID Connect providers. To support this the Gardener must be installed with the `shoot-networking-filter` extension.

## Configuration

To generally enable the OIDC service for shoot objects the `shoot-networking-filter` extension must be registered by providing an appropriate [extension registration](https://github.com/gardener/gardener-extension-shoot-networking-filter/blob/master/example/controller-registration.yaml) in the garden cluster.

Here it is possible to decide whether the extension should be always available for all shoots or whether the extension must be separately enabled per shoot.

If the extension should be used for all shoots the `globallyEnabled` flag should be set to `true`.

```yaml
spec:
  resources:
    - kind: Extension
      type: shoot-networking-filter
      globallyEnabled: true
```

### Shoot Feature Gate

If the shoot OIDC service is not globally enabled by default (depends on the extension registration on the garden cluster), it can be enabled per shoot. To enable the service for a shoot, the shoot manifest must explicitly add the `shoot-networking-filter` extension.

```yaml
...
spec:
  extensions:
    - type: shoot-networking-filter
...
```

If the shoot OIDC service is globally enabled by default, it can be disabled per shoot. To disable the service for a shoot, the shoot manifest must explicitly state it.

```yaml
...
spec:
  extensions:
    - type: shoot-networking-filter
      disabled: true
...
```
