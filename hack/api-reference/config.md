<p>Packages:</p>
<ul>
<li>
<a href="#networking-policy-filter.extensions.config.gardener.cloud%2fv1alpha1">networking-policy-filter.extensions.config.gardener.cloud/v1alpha1</a>
</li>
</ul>
<h2 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1">networking-policy-filter.extensions.config.gardener.cloud/v1alpha1</h2>
<p>
<p>Package v1alpha1 contains the OIDC Shoot Service extension configuration.</p>
</p>
Resource Types:
<ul><li>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Configuration">Configuration</a>
</li></ul>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Configuration">Configuration
</h3>
<p>
<p>Configuration contains information about the policy filter configuration.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code></br>
string</td>
<td>
<code>
networking-policy-filter.extensions.config.gardener.cloud/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code></br>
string
</td>
<td><code>Configuration</code></td>
</tr>
<tr>
<td>
<code>egressFilter</code></br>
<em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.EgressFilter">
EgressFilter
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>EgressFilter contains the configuration for the egress filter</p>
</td>
</tr>
<tr>
<td>
<code>healthCheckConfig</code></br>
<em>
<a href="https://github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config">
github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config/v1alpha1.HealthCheckConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HealthCheckConfig is the config for the health check controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.DownloaderConfig">DownloaderConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.EgressFilter">EgressFilter</a>)
</p>
<p>
<p>DownloaderConfig contains the configuration for the filter set downloader.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>endpoint</code></br>
<em>
string
</em>
</td>
<td>
<p>Endpoint is the endpoint URL for downloading the filter set.</p>
</td>
</tr>
<tr>
<td>
<code>refreshPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.15/#duration-v1-meta">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshPeriod is interval for refreshing the filter set.
If unset, the filter set is only fetched on startup.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.EgressFilter">EgressFilter
</h3>
<p>
(<em>Appears on:</em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Configuration">Configuration</a>)
</p>
<p>
<p>EgressFilter contains the configuration for the egress filter.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>blackholingEnabled</code></br>
<em>
bool
</em>
</td>
<td>
<p>BlackholingEnabled is a flag to set blackholing or firewall approach.</p>
</td>
</tr>
<tr>
<td>
<code>filterSetProviderType</code></br>
<em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.FilterSetProviderType">
FilterSetProviderType
</a>
</em>
</td>
<td>
<p>FilterSetProviderType specifies how the filter set is retrieved.
Supported types are <code>static</code> and <code>download</code>.</p>
</td>
</tr>
<tr>
<td>
<code>staticFilterSet</code></br>
<em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Filter">
[]Filter
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>StaticFilterSet contains the static filter set.
Only used for provider type <code>static</code>.</p>
</td>
</tr>
<tr>
<td>
<code>downloaderConfig</code></br>
<em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.DownloaderConfig">
DownloaderConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DownloaderConfig contains the configuration for the filter set downloader.
Only used for provider type <code>download</code>.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Filter">Filter
</h3>
<p>
(<em>Appears on:</em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.EgressFilter">EgressFilter</a>)
</p>
<p>
<p>Filter specifies a network-CIDR policy pair.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>network</code></br>
<em>
string
</em>
</td>
<td>
<p>Network is the network CIDR of the filter.</p>
</td>
</tr>
<tr>
<td>
<code>policy</code></br>
<em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Policy">
Policy
</a>
</em>
</td>
<td>
<p>Policy is the access policy (<code>BLOCK_ACCESS</code> or <code>ALLOW_ACCESS</code>).</p>
</td>
</tr>
</tbody>
</table>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.FilterSetProviderType">FilterSetProviderType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.EgressFilter">EgressFilter</a>)
</p>
<p>
<p>FilterSetProviderType</p>
</p>
<h3 id="networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Policy">Policy
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#networking-policy-filter.extensions.config.gardener.cloud/v1alpha1.Filter">Filter</a>)
</p>
<p>
<p>Policy is the access policy</p>
</p>
<hr/>
<p><em>
Generated with <a href="https://github.com/ahmetb/gen-crd-api-reference-docs">gen-crd-api-reference-docs</a>
</em></p>
