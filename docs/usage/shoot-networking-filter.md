# Register Shoot Networking Filter Extension in Shoot Clusters

## Introduction
Within a shoot cluster, it is possible to enable the networking filter. It is necessary that the Gardener installation your shoot cluster runs in is equipped with a `shoot-networking-filter` extension. Please ask your Gardener operator if the extension is available in your environment.

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

## Opt-out

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

## Ingress Filtering

By default, the networking filter only filters egress traffic. However, if you enable blackholing, incoming traffic will also be blocked.
You can enable blackholing on a per-shoot basis.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
      providerConfig:
        egressFilter:
          blackholingEnabled: true
...
```
Ingress traffic can only be blocked by blackhole routing, if the source IP address is preserved. On Azure, GCP and AliCloud this works by default.
The default on AWS is a classic load balancer that replaces the source IP by it's own IP address. Here, a network load balancer has to be
configured adding the annotation `service.beta.kubernetes.io/aws-load-balancer-type: "nlb"` to the service.
On OpenStack, load balancers don't preserve the source address.

When you disable `blackholing` in an existing shoot, the associated blackhole routes will be removed automatically. 
Conversely, when you re-enable `blackholing` again, the iptables-based filter rules will be removed and replaced by blackhole routes.

## Ingress Filtering per Worker Group

You can optionally enable or disable ingress filtering for specified worker groups.
For example, you may want to disable blackholing in general but enable it for a worker group hosting an external API.
You can do so by using an optional `workers` field:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
      providerConfig:
        egressFilter:
          blackholingEnabled: false
          workers:
            blackholingEnabled: true
            names:
              - external-api
...
```

Please note that only blackholing can be changed per worker group. You may not define different IPs to block or
disable blocking altogether.

## Custom IP 

It is possible to add custom IP addresses to the network filter. This can be useful for testing purposes.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
      providerConfig:
        egressFilter:
          staticFilterList:
          - network: 1.2.3.4/31
            policy: BLOCK_ACCESS
          - network: 5.6.7.8/32
            policy: BLOCK_ACCESS
          - network: ::2/128
            policy: BLOCK_ACCESS
...
```

