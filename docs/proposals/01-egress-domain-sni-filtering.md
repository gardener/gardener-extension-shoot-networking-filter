# GEP-143: Domain and SNI Based Egress Filtering for Regulated Workloads

* **Issue**: [gardener/gardener-extension-shoot-networking-filter#143](https://github.com/gardener/gardener-extension-shoot-networking-filter/issues/143)
* **Author**: Alijohn Ghassemlouei (`@aghassemlouei`) / Gardener Extension Contributors
* **Status**: Proposed / Accepted for Analysis
* **Created**: 2026-08-01

## 1. Summary

This Gardener Enhancement Proposal (GEP) addresses [Issue #143](https://github.com/gardener/gardener-extension-shoot-networking-filter/issues/143), which requests support for domain-based and Server Name Indication (SNI) based egress network filtering on Gardener Shoot clusters. 

Currently, `gardener-extension-shoot-networking-filter` operates exclusively at Layer 3 (Network) and Layer 4 (Transport) by deploying static Linux kernel routing table blackholes (`ip route add blackhole ...`) and `iptables` drop rules on Shoot worker nodes via the `egress-filter-applier` DaemonSet.

This document presents:
1. The compliance requirements driving domain and SNI filtering (specifically **US NIST 800-53 R5 SC-7(8)**).
2. A technical analysis explaining why static L3/L4 IP blackholing is architecturally incompatible with FQDN, wildcard domain, and CDN filtering.
3. Three Kubernetes-native architectural solutions for Gardener to satisfy domain/SNI permit lists without collateral service disruption.
4. An API schema evolution proposal extending `EgressFilter` while preserving backwards compatibility.

## 2. Motivation & Compliance Requirements

### 2.1 Regulatory Context: US NIST 800-53 R5 SC-7 (8)
Regulated workloads (e.g., FedRAMP, GovCloud, PCI-DSS) must comply with [US NIST 800-53 R5 SC-7 (8) System and Communications Protection; Boundary Protection | Route Traffic to Authenticated Proxy Servers](https://csrc.nist.gov/projects/cprt/catalog#/cprt/framework/version/SP_800_53_5_1_0/home?element=SC-7):

> *"The information system routes internal communications to external networks through authenticated proxy servers."*

Compliance requires an explicit permit-list (allowlist) boundary control that inspects application-layer destinations before authoring connection egress.

### 2.2 Use Case Examples
Workloads operating in secure Shoot clusters typically require egress access to explicit domain lists such as:

```
# Ubuntu Package Repositories
archive.ubuntu.com
security.ubuntu.com
esm.ubuntu.com
.canonical.com
api.snapcraft.io
.cdn.snapcraftcontent.com

# PKI Certificate & OCSP Validation
cacerts.digicert.com
ocsp.digicert.com
crl3.digicert.com
crl4.digicert.com
ocsp.pki.goog
crl.pki.goog
crls.pki.goog
.amazontrust.com
```

Without native domain/SNI filtering in Gardener, platform operators are forced to provision and maintain third-party cloud-native firewalls (such as AWS Network Firewall or Google Cloud Secure Web Proxy), significantly increasing infrastructure costs and multi-cloud operational overhead.

## 3. Current Architecture & Technical Limitations

### 3.1 Overview of `shoot-networking-filter`
The `gardener-extension-shoot-networking-filter` extension reconciles `ShootNetworkingFilter` / `EgressFilter` resources and deploys the following architecture:

1. **Policy Calculation**: The controller (`pkg/controller/lifecycle/filter.go`) processes static and downloaded filter lists, carving out `ALLOW_ACCESS` IP CIDRs from `BLOCK_ACCESS` IP CIDRs (`generateEgressFilterValues`), and writes `ipv4-list` and `ipv6-list` to a Kubernetes Secret (`egress-filter-list`) in the Shoot's `kube-system` namespace.
2. **Node Enforcement**: The controller deploys the `egress-filter-applier` DaemonSet (running the `gardener/egress-filter-refresher` container image) to Shoot worker nodes.
3. **Kernel Filtering**: The DaemonSet reads the IP CIDR lists and applies Linux kernel network filtering:
   - **Blackhole Mode (`blackholingEnabled: true`)**: Adds static kernel routing entries (`ip route add blackhole <CIDR>`).
   - **Firewall Mode (`blackholingEnabled: false`)**: Inserts `iptables -A ... -j DROP` and `ip6tables` drop rules.

```
+-------------------------------------------------------------------------------+
| Shoot Worker Node                                                             |
|                                                                               |
|   +--------------------------+       +------------------------------------+   |
|   | Pod Workload             |       | egress-filter-applier (DaemonSet)  |   |
|   +------------+-------------+       +-----------------+------------------+   |
|                |                                       |                      |
|                v                                       v                      |
|   +--------------------------+       +------------------------------------+   |
|   | Linux Kernel Network     |<------| Configures: ip route add blackhole |   |
|   | Stack (L3/L4 Only)       |       |          or iptables -A ... -j DROP|   |
|   +--------------------------+       +------------------------------------+   |
+-------------------------------------------------------------------------------+
```

### 3.2 Why L3/L4 Static IP Filtering Cannot Perform FQDN / SNI Filtering
Attempting to implement domain or SNI filtering via DNS-to-IP resolution within `shoot-networking-filter` fails due to fundamental TCP/IP and cloud architecture constraints:

1. **Short TTLs & Dynamic CDN IP Rotation**:
   - High-availability endpoints (`archive.ubuntu.com`, `api.snapcraft.io`, `ocsp.pki.goog`) are served via global Content Delivery Networks (Akamai, CloudFront, Fastly, Google Cloud CDN) with short DNS TTLs (often 60 seconds or less).
   - Resolving a domain to A/AAAA IP records at policy generation time will capture only a subset of CDN IPs. Subsequent pod connections will resolve to different CDN edge IPs, causing legitimate traffic to be dropped.

2. **Multi-Tenant IP Sharing & Collateral Damage**:
   - CDN and cloud provider IPs are multi-tenant. A single IP address block (e.g., CloudFront or Akamai edge IP) hosts thousands of distinct domains.
   - If an operator allows an IP CIDR because `archive.ubuntu.com` resolved to it, all other tenant domains hosted on that edge IP become reachable.
   - Conversely, if an operator blocks an IP CIDR because a blocked domain resolved to it, legitimate services sharing that IP address are disrupted.

3. **Wildcard Domains (`*.canonical.com`)**:
   - DNS resolution cannot query wildcard expressions (`.canonical.com` or `.amazontrust.com`). Wildcards require matching alphanumeric hostname strings during connection initiation.

4. **Layer 7 SNI vs. Layer 3/4 Headers**:
   - The Server Name Indication (SNI) extension is transmitted inside the TLS ClientHello handshake at Layer 7.
   - Standard Linux IP routing tables and `iptables` drop rules inspect only Layer 3 (Source/Destination IP) and Layer 4 (Protocol, TCP/UDP port), making TLS ClientHello SNI inspection impossible at the L3/L4 kernel routing layer.

## 4. Proposed Architectural Solutions

To satisfy Issue #143 while adhering to Kubernetes and Gardener design patterns, we propose three distinct architectural options.

### 4.1 Option 1 (Recommended): Layer 7 Egress Gateway / Web Proxy Extension (`shoot-egress-proxy`)
Create a new Gardener extension controller (or add an L7 mode to `shoot-networking-filter`) that deploys a containerized Layer 7 Egress Proxy on Shoot worker nodes.

```
+-----------------------------------------------------------------------------------+
| Shoot Cluster                                                                     |
|                                                                                   |
|  +---------------------+         +---------------------------------------------+  |
|  | Pod Workload        |         | egress-proxy (DaemonSet / Envoy or Squid)   |  |
|  |                     |         |                                             |  |
|  |  HTTP_PROXY=...     |-------->|  1. Inspects TLS ClientHello SNI / Host     |  |
|  |  HTTPS_PROXY=...    |         |  2. Evaluates Domain Allow/Block Policy     |  |
|  |  (or L4 TPROXY)     |         |  3. Enforces Explicit Permit List           |  |
|  +---------------------+         +---------------------+-----------------------+  |
|                                                        |                          |
+--------------------------------------------------------|--------------------------+
                                                         v
                                           Allowed Internet Domain / SNI
                                         (e.g., archive.ubuntu.com:443)
```

* **Technology**: **Envoy Proxy** (configured with TLS SNI routing and RBAC HTTP filters) or **Squid Web Proxy** (configured for HTTPS CONNECT method SNI filtering).
* **Traffic Interception**:
  - **Explicit Proxying**: Workloads use standard `HTTP_PROXY` and `HTTPS_PROXY` environment variables (injected via a mutating admission webhook for namespaces with `egress-proxy: enabled`).
  - **Transparent Proxying**: Pod network interfaces use `iptables TPROXY` / `REDIRECT` rules to transparently redirect outbound TCP port 80/443 traffic to the local L7 proxy DaemonSet without modifying application code.
* **Compliance**: Directly complies with US NIST 800-53 R5 SC-7(8) by providing an authenticated L7 proxy boundary.

### 4.2 Option 2: eBPF & DNS-Aware CNI Policy Integration (Cilium `toFQDNs`)
For Shoot clusters utilizing **Cilium** as their Container Network Interface (CNI), integrate domain filtering directly into Cilium's eBPF DNS proxy.

* **Mechanism**:
  - Cilium deploys an eBPF-based DNS proxy that intercepts DNS queries (`A`/`AAAA`) from pods on port 53.
  - When a pod resolves a permitted domain name (`matchName: "archive.ubuntu.com"` or wildcard `matchPattern: "*.canonical.com"`), Cilium dynamically adds the returned IP addresses to an eBPF allowlist specifically mapped to that pod's identity.
* **Extension Integration**:
  - `shoot-networking-filter` converts user-configured domain permit lists into `CiliumClusterwideNetworkPolicy` (CCNP) Custom Resources when `networking.type: cilium` is detected on the Shoot.

```yaml
apiVersion: "cilium.io/v2"
kind: CiliumClusterwideNetworkPolicy
metadata:
  name: "egress-domain-filter"
spec:
  endpointSelector:
    matchLabels: {}
  egress:
  - toFQDNs:
    - matchName: "archive.ubuntu.com"
    - matchName: "security.ubuntu.com"
    - matchPattern: "*.canonical.com"
    - matchPattern: "*.digicert.com"
    - matchPattern: "*.pki.goog"
  - toEndpoints:
    - matchLabels:
        "k8s:io.kubernetes.pod.namespace": "kube-system"
        "k8s:k8s-app": "kube-dns"
    toPorts:
    - ports:
      - port: "53"
        protocol: ANY
      rules:
        dns:
        - matchPattern: "*"
```

### 4.3 Option 3: Cloud Provider Managed Proxy Integration
For environments requiring managed cloud-provider boundary protection:
* **AWS**: Provision an **AWS Network Firewall** endpoint with TLS SNI stateless/stateful domain rule groups.
* **Google Cloud**: Provision a **Google Cloud Secure Web Proxy (SWP)** instance with TLS inspection / SNI permit-list policies.
* **Trade-offs**: Highest operational simplicity, but incurs cloud provider hourly and per-GB processing costs.

## 5. API Schema Evolution Proposal

To support domain and SNI filtering in Gardener without breaking existing L3/L4 `StaticFilterList` or `TagFilters` definitions, we propose extending the `EgressFilter` configuration in `pkg/apis/config/types.go` and `pkg/apis/config/v1alpha1/types.go`:

### 5.1 Go Type Definitions (`pkg/apis/config/types.go`)

```go
type EgressFilter struct {
    // BlackholingEnabled is a flag to set blackholing or firewall approach for IP CIDRs.
    BlackholingEnabled bool

    // Workers contains worker-specific block modes.
    Workers *Workers

    // StaticFilterList contains the static IP CIDR filter list.
    StaticFilterList []Filter

    // TagFilters contains filters to select entries based on tags.
    TagFilters []TagFilter

    // DomainFilter contains explicit domain and SNI permit/block rules (Issue #143).
    // Optional: Only enforced when an L7 Proxy or Cilium CNI mode is enabled.
    DomainFilter *DomainFilterConfig
}

// DomainFilterConfig specifies L7 domain and SNI egress filtering rules.
type DomainFilterConfig struct {
    // Mode specifies the enforcement mechanism ("L7Proxy", "CiliumFQDN", or "CloudManaged").
    Mode DomainFilterMode

    // DefaultPolicy specifies the baseline egress policy ("ALLOW" or "BLOCK").
    // Regulated workloads under NIST 800-53 R5 SC-7(8) should use "BLOCK".
    DefaultPolicy Policy

    // PermittedDomains contains explicit allowed domain names and wildcards.
    PermittedDomains []DomainRule

    // BlockedDomains contains explicit blocked domain names and wildcards.
    BlockedDomains []DomainRule
}

type DomainFilterMode string

const (
    // DomainFilterModeL7Proxy uses the containerized L7 Egress Proxy DaemonSet.
    DomainFilterModeL7Proxy DomainFilterMode = "L7Proxy"
    // DomainFilterModeCiliumFQDN uses Cilium eBPF toFQDNs policies.
    DomainFilterModeCiliumFQDN DomainFilterMode = "CiliumFQDN"
)

// DomainRule represents a single domain or wildcard SNI rule.
type DomainRule struct {
    // Name is the exact FQDN (e.g., "archive.ubuntu.com") or wildcard (e.g., "*.canonical.com").
    Name string
    // Ports specifies allowed destination ports (default: 80, 443).
    Ports []int
}
```

### 5.2 Example Shoot Specification (`Shoot.spec.extensions`)

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
metadata:
  name: regulated-shoot
  namespace: garden-project
spec:
  extensions:
    - type: shoot-networking-filter
      providerConfig:
        apiVersion: config.shoot-networking-filter.extensions.gardener.cloud/v1alpha1
        kind: Configuration
        egressFilter:
          blackholingEnabled: true
          staticFilterList:
            - network: 169.254.169.254/32
              policy: BLOCK_ACCESS
          domainFilter:
            mode: L7Proxy
            defaultPolicy: BLOCK_ACCESS
            permittedDomains:
              - name: "archive.ubuntu.com"
                ports: [80, 443]
              - name: "security.ubuntu.com"
                ports: [80, 443]
              - name: "*.canonical.com"
                ports: [443]
              - name: "*.digicert.com"
                ports: [80, 443]
              - name: "*.pki.goog"
                ports: [80, 443]
              - name: "*.amazontrust.com"
                ports: [443]
```

## 6. Evaluation Matrix

| Criterion | L3/L4 IP Blackholing (Current) | Option 1: L7 Egress Web Proxy (`shoot-egress-proxy`) | Option 2: Cilium eBPF FQDN (`toFQDNs`) | Option 3: Managed Cloud Proxy (AWS/GCP) |
| :--- | :--- | :--- | :--- | :--- |
| **NIST 800-53 R5 SC-7(8) Compliance** | No (L3/L4 IP only) | **Yes** (Authenticated L7 Proxy) | **Yes** (eBPF DNS/SNI inspection) | **Yes** (Cloud SWP / Firewall) |
| **Wildcard Support (`*.canonical.com`)** | Impossible | **Yes** (SNI / Host regex) | **Yes** (`matchPattern`) | **Yes** |
| **CDN & Multi-Tenant IP Churn Immunity** | Highly vulnerable | **Immune** (Evaluates hostname) | **Immune** (Tracks DNS response) | **Immune** |
| **Infrastructure & Licensing Cost** | None | **Low** (DaemonSet CPU/Mem) | **None** (Included with Cilium CNI) | **High** (Hourly + Data fees) |
| **CNI Independence** | Independent | **Independent** | Requires Cilium CNI | Independent |
| **Workload Modifications Required** | None | Optional (`HTTP_PROXY` or TPROXY) | None (Transparent eBPF) | VPC Route / Proxy setup |

## 7. Recommendation & Next Steps

1. **Short-Term Recommendation**:
   - For Shoots deployed with **Cilium** (`networking.type: cilium`), implement **Option 2** (`CiliumClusterwideNetworkPolicy` integration) as it provides immediate eBPF-based FQDN filtering with zero proxy overhead.
2. **Medium-Term Recommendation**:
   - For universal CNI compatibility across all Gardener Shoots, implement **Option 1** by introducing an optional L7 Web Proxy DaemonSet mode (`DomainFilterModeL7Proxy`) managed by the extension controller.
3. **Documentation Update**:
   - Reference this proposal in `docs/usage/shoot-networking-filter.md` to guide platform users and compliance officers evaluating domain-based egress filtering for Issue #143.
