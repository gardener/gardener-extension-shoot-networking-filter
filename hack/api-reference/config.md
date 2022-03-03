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
</tbody>
</table>
<hr/>
<p><em>
Generated with <a href="https://github.com/ahmetb/gen-crd-api-reference-docs">gen-crd-api-reference-docs</a>
</em></p>
