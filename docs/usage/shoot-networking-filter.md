# Register OpenID Connect provider in Shoot Clusters

## Introduction
Within a shoot cluster, it is possible to dynamically register OpenID Connect providers. It is necessary that the Gardener installation your shoot cluster runs in is equipped with a `shoot-networking-filter` extension. Please ask your Gardener operator if the extension is available in your environment.

## Shoot Feature Gate

In most of the Gardener setups the `shoot-networking-filter` extension is not enabled globally and thus must be configured per shoot cluster. Please adapt the shoot specification by the configuration shown below to activate the extension individually.

```yaml
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
...
```
