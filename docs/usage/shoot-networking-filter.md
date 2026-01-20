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

## Event Logging

Block events are logged automatically into the linux kernel log of the node where the event occurred.

For example, consider a shoot cluster configured using the following configuration:

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
...
```

If a pod tries to access the IP address `1.2.3.4`, e.g. by running the command `curl https://1.2.3.4`, the following log message will be generated in the kernel log of the node where the pod is running:

```
Policy-Filter-Dropped:IN=califb3eb82ef50 OUT=ens5 MAC=ee:ee:ee:ee:ee:ee:8a:7f:1f:f9:a0:ca:08:00 SRC=100.64.0.7 DST=1.2.3.4 LEN=60 TOS=0x00 PREC=0x00 TTL=63 ID=33784 DF PROTO=TCP SPT=55012 DPT=443 WINDOW=65535 RES=0x00 SYN URGP=0 MARK=0x10000
```

Please note that the log message includes the source (`SRC`) and destination (`DST`) IP addresses and the port numbers (`SPT` & `DPT`).

The block events can be viewed using the `dmesg` command or various other tools displaying linux kernel logs. They are also available via the Gardener observability tools.

## Tag-Based Filtering

The extension supports two filter list formats:

**Basic Format (v1)** - Simple list without tags:
```json
[
  {"network": "10.0.0.0/8", "policy": "BLOCK_ACCESS"},
  {"network": "192.168.1.0/24", "policy": "ALLOW_ACCESS"}
]
```

**Tag-Based Format (v2)** - Entries with metadata tags for selective filtering:
```json
[
  {
    "entries": [
      {
        "target": "10.0.0.0/8",
        "policy": "BLOCK",
        "tags": [{"name": "Fruit", "values": ["Apple"]}]
      },
      {
        "target": "172.16.0.0/12",
        "policy": "BLOCK",
        "tags": [{"name": "Fruit", "values": ["Banana"]}]
      }
    ]
  }
]
```

When using the tag-based format, you can configure tag filters to selectively apply only entries matching specific tag criteria. This is useful when a centrally-managed filter list contains entries for multiple environments or severity levels, and you want to apply only a subset.

**Important:** Tag filtering uses **OR logic** - an entry is included if it matches **ANY** of the configured tag filters. Within a tag filter, multiple values are also ORed together.

Tag filters are configured in the shoot specification under `egressFilter.tagFilters`:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
...
spec:
  extensions:
    - type: shoot-networking-filter
      providerConfig:
        egressFilter:
          tagFilters:
          - name: Fruit
            values:
            - "Apple"
            - "Banana"
```

In this example, only entries tagged with `Fruit=Apple` **OR** `Fruit=Banana` will be applied.

The extension can read filter entries from a Secret that Gardener automatically syncs from the garden cluster to the seed cluster. This is useful when you have a large project-specific list of policies.

**Important:** The project Secret filters **replace** (not append to) the centrally downloaded filter list. This allows you to completely override the default filters while still being able to add shoot-specific static filters. See [Merge Behavior](#merge-behavior) below for details.

### 1. Create Secret in Garden Cluster

Create a Secret in your project namespace (garden cluster). You can use either the basic format (v1) or tag-based format (v2):

**Example using basic format (v1):**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: blocked-ips
  namespace: garden-myproject  # Your project namespace
type: Opaque
stringData:
  filterList: |
    [
      {"network": "10.250.0.0/16", "policy": "BLOCK_ACCESS"},
      {"network": "172.31.0.0/16", "policy": "BLOCK_ACCESS"}
    ]
```

**Example using tag-based format (v2):**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: blocked-ips
  namespace: garden-myproject  # Your project namespace
type: Opaque
stringData:
  filterList: |
    [
      {
        "entries": [
          {"target": "10.250.0.0/16", "policy": "BLOCK", "tags": [{"name": "Fruit", "values": ["Apple"]}]},
          {"target": "172.31.0.0/16", "policy": "BLOCK", "tags": [{"name": "Fruit", "values": ["Banana"]}]}
        ]
      }
    ]
```

### 1b. Create Gzipped Secret for Large Filter Lists (Optional)

For very large filter lists (thousands of entries), you can use gzip compression to stay within Kubernetes Secret size limits (1MB). The extension automatically detects and decompresses gzipped data.

**Example using basic format (v1):**

**Step 1: Create your filter list file**
```bash
cat > filterlist.json <<EOF
[
  {"network": "10.250.0.0/16", "policy": "BLOCK_ACCESS"},
  {"network": "172.31.0.0/16", "policy": "BLOCK_ACCESS"}
]
EOF
```

**Example using tag-based format (v2):**

**Step 1: Create your filter list file**
```bash
cat > filterlist.json <<EOF
[
  {
    "entries": [
      {"target": "10.250.0.0/16", "policy": "BLOCK", "tags": [{"name": "Fruit", "values": ["Apple"]}]},
      {"target": "172.31.0.0/16", "policy": "BLOCK", "tags": [{"name": "Fruit", "values": ["Banana"]}]}
    ]
  }
]
EOF
```

**Step 2: Compress with gzip**
```bash
gzip -c filterlist.json > filterlist.json.gz
```

**Step 3: Create Secret with compressed data**
```bash
kubectl create secret generic additional-blocked-ips \
  --from-file=filterList=filterlist.json.gz \
  --namespace=garden-myproject
```

Or using YAML:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: additional-blocked-ips
  namespace: garden-myproject
type: Opaque
data:
  filterList: <base64-encoded-gzipped-data>
```

To get the base64-encoded gzipped data:
```bash
base64 -i filterlist.json.gz
```

### 2. Add Resource Reference to Shoot Spec

**IMPORTANT**: You must list the Secret in `Shoot.spec.resources` for Gardener to sync it:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
metadata:
  name: my-shoot
  namespace: garden-myproject
spec:
  # This tells Gardener to sync the Secret to the seed cluster
  resources:
  - name: additional-blocked-ips
    resourceRef:
      apiVersion: v1
      kind: Secret
      name: additional-blocked-ips
  
  extensions:
  - type: shoot-networking-filter
    providerConfig:
      apiVersion: shoot-networking-filter.extensions.config.gardener.cloud/v1alpha1
      kind: Configuration
      egressFilter:
        blackholingEnabled: true
        projectFilterListSource:
          name: additional-blocked-ips
          key: filterList  # Optional, defaults to "filterList"
```

### Format Support

The ConfigMap/Secret data can contain:

**V1 Format:**
```json
[
  {"network": "10.0.0.0/8", "policy": "BLOCK_ACCESS"},
  {"network": "192.168.1.0/24", "policy": "ALLOW_ACCESS"}
]
```

**V2 Format:**
```json
[
  {
    "entries": [
      {
        "target": "10.0.0.0/8",
        "policy": "BLOCK",
        "tags": [{"name": "Fruit", "values": ["Apple"]}]
      }
    ]
  }
]
```

### Merge Behavior

The extension supports two filter sources that are mutually exclusive:

**When Project Secret is configured:**
1. **Project Secret filters** (replaces downloaded data)
2. Tag filtering (if configured) - applied to project filters
3. Static filter list (from shoot providerConfig) - merged with project filters

**When no Project Secret is configured:**
1. Downloaded filter list (from service config)
2. Tag filtering (if configured) - applied to downloaded filters
3. Static filter list (from shoot providerConfig) - merged with downloaded filters

**Key Points:**
- Project Secret filters **replace** (not append to) the downloaded filter list
- Static filters are always merged regardless of the source
- Tag filtering is applied to whichever source is active (project or downloaded)
- If reading the project Secret fails, the extension falls back to downloaded data

**Examples:**

*Example 1: Project Secret blocks, static filter allows*
- Project Secret: `{"network": "10.0.0.0/8", "policy": "BLOCK_ACCESS"}`
- Static filter: `{"network": "10.1.0.0/16", "policy": "ALLOW_ACCESS"}`
- **Result**: `10.0.0.0/8` is blocked, **except** `10.1.0.0/16` is carved out and allowed

*Example 2: Project Secret allows, static filter blocks*
- Project Secret: `{"network": "10.0.0.0/8", "policy": "ALLOW_ACCESS"}` (carves out from another block)
- Static filter: `{"network": "10.1.0.0/16", "policy": "BLOCK_ACCESS"}`
- **Result**: Both filters are applied - the ALLOW carves out from broader blocks, and the BLOCK adds `10.1.0.0/16` to the block list

*Example 3: Downloaded data vs static filters (when no Project Secret is configured)*
- Downloaded data: `{"network": "192.168.0.0/16", "policy": "BLOCK_ACCESS"}`
- Static filter: `{"network": "192.168.1.0/24", "policy": "ALLOW_ACCESS"}`
- **Result**: `192.168.0.0/16` is blocked, **except** `192.168.1.0/24` is carved out and allowed

The key principle: **ALLOW_ACCESS policies carve out exceptions from BLOCK_ACCESS policies**. All filters from the active source (project Secret OR downloaded) are merged with static filters, then ALLOW entries remove subnets from BLOCK entries.

This allows to completely override the default filter list while still being able to add shoot-specific static filters.
