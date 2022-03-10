# Gardener Networking Policy Filter for Shoots

## Introduction
Gardener allows shoot clusters to filter egress traffic on node level. To support this the Gardener must be installed with the `shoot-networking-filter` extension.

## Configuration

To generally enable the networking filter for shoot objects the `shoot-networking-filter` extension must be registered by providing an appropriate [extension registration](https://github.com/gardener/gardener-extension-shoot-networking-filter/blob/master/example/controller-registration.yaml) in the garden cluster.

Here it is possible to decide whether the extension should be always available for all shoots or whether the extension must be separately enabled per shoot.

If the extension should be used for all shoots the `globallyEnabled` flag should be set to `true`.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerRegistration
...
spec:
  resources:
    - kind: Extension
      type: shoot-networking-filter
      globallyEnabled: true
```

### ControllerRegistration
An example of a `ControllerRegistration` for the `shoot-networking-filter` can be found here: https://github.com/gardener/gardener-extension-shoot-networking-filter/blob/master/example/controller-registration.yaml

The `ControllerRegistration` contains a Helm chart which eventually deploys the `shoot-networking-filter` to seed clusters. It offers some configuration options, mainly to set up a static filter list or provide the configuration for downloading the filter list from a service endpoint.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
...
  values:
    egressFilter:
      blackholingEnabled: true

      filterListProviderType: static
      staticFilterList:
        - network: 1.2.3.4/31
          policy: BLOCK_ACCESS
        - network: 5.6.7.8/32
          policy: BLOCK_ACCESS
        - network: ::2/128
          policy: BLOCK_ACCESS

      #filterListProviderType: download
      #downloaderConfig:
      #  endpoint: https://my.filter.list.server/lists/policy
      #  oauth2Endpoint: https://my.auth.server/oauth2/token
      #  refreshPeriod: 1h

      ## if the downloader needs an OAuth2 access token, client credentials can be provided with oauth2Secret
      #oauth2Secret:
      # clientID: 1-2-3-4
      # clientSecret: secret!!
      ## either clientSecret of client certificate is required
      # client.crt.pem: |
      #   -----BEGIN CERTIFICATE-----
      #   ...
      #   -----END CERTIFICATE-----
      # client.key.pem: |
      #   -----BEGIN PRIVATE KEY-----
      #   ...
      #   -----END PRIVATE KEY-----
```

### Enablement for a Shoot

If the shoot networking filter is not globally enabled by default (depends on the extension registration on the garden cluster), it can be enabled per shoot. To enable the service for a shoot, the shoot manifest must explicitly add the `shoot-networking-filter` extension.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
...
```

If the shoot networking filter is globally enabled by default, it can be disabled per shoot. To disable the service for a shoot, the shoot manifest must explicitly state it.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
      disabled: true
...
```
