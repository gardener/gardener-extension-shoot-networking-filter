<p>Packages:</p>
<ul>
<li>
<a href="#shoot-networking-filter.extensions.config.gardener.cloud%2fv1alpha1">shoot-networking-filter.extensions.config.gardener.cloud/v1alpha1</a>
</li>
</ul>

<h2 id="shoot-networking-filter.extensions.config.gardener.cloud/v1alpha1">shoot-networking-filter.extensions.config.gardener.cloud/v1alpha1</h2>
<p>

</p>

<h3 id="configuration">Configuration
</h3>


<p>
Configuration contains information about the policy filter configuration.
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
<code>egressFilter</code></br>
<em>
<a href="#egressfilter">EgressFilter</a>
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
<a href="#healthcheckconfig">HealthCheckConfig</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HealthCheckConfig is the config for the health check controller.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="downloaderconfig">DownloaderConfig
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
DownloaderConfig contains the configuration for the filter list downloader.
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
<p>Endpoint is the endpoint URL for downloading the filter list.</p>
</td>
</tr>
<tr>
<td>
<code>oauth2Endpoint</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>OAuth2Endpoint contains the optional OAuth endpoint for fetching the access token.<br />If specified, the OAuth2Secret must be provided, too.</p>
</td>
</tr>
<tr>
<td>
<code>refreshPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshPeriod is interval for refreshing the filter list.<br />If unset, the filter list is only fetched on startup.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="egressfilter">EgressFilter
</h3>


<p>
(<em>Appears on:</em><a href="#configuration">Configuration</a>)
</p>

<p>
EgressFilter contains the configuration for the egress filter.
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
boolean
</em>
</td>
<td>
<p>BlackholingEnabled is a flag to set blackholing or firewall approach.</p>
</td>
</tr>
<tr>
<td>
<code>workers</code></br>
<em>
<a href="#workers">Workers</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Workers contains worker-specific block modes</p>
</td>
</tr>
<tr>
<td>
<code>sleepDuration</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<p>SleepDuration is the time interval between policy updates.</p>
</td>
</tr>
<tr>
<td>
<code>filterListProviderType</code></br>
<em>
<a href="#filterlistprovidertype">FilterListProviderType</a>
</em>
</td>
<td>
<p>FilterListProviderType specifies how the filter list is retrieved.<br />Supported types are `static` and `download`.</p>
</td>
</tr>
<tr>
<td>
<code>staticFilterList</code></br>
<em>
<a href="#filter">Filter</a> array
</em>
</td>
<td>
<em>(Optional)</em>
<p>StaticFilterList contains the static filter list.<br />Only used for provider type `static`.</p>
</td>
</tr>
<tr>
<td>
<code>downloaderConfig</code></br>
<em>
<a href="#downloaderconfig">DownloaderConfig</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DownloaderConfig contains the configuration for the filter list downloader.<br />Only used for provider type `download`.</p>
</td>
</tr>
<tr>
<td>
<code>ensureConnectivity</code></br>
<em>
<a href="#ensureconnectivity">EnsureConnectivity</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.</p>
</td>
</tr>
<tr>
<td>
<code>tagFilters</code></br>
<em>
<a href="#tagfilter">TagFilter</a> array
</em>
</td>
<td>
<em>(Optional)</em>
<p>TagFilters contains filters to select entries based on tags.<br />Only used with v2 format filter lists.</p>
</td>
</tr>
<tr>
<td>
<code>projectFilterListSource</code></br>
<em>
<a href="#secretref">SecretRef</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ProjectFilterListSource references a Secret containing additional filter entries.<br />The Secret must be listed in Shoot.spec.resources for Gardener to sync it automatically.</p>
</td>
</tr>
<tr>
<td>
<code>shootFilterListSource</code></br>
<em>
<a href="#secretref">SecretRef</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ShootFilterListSource references a Secret in the shoot cluster containing additional filter entries.<br />Mutually exclusive with ProjectFilterListSource.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="ensureconnectivity">EnsureConnectivity
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.
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
<code>seedNamespaces</code></br>
<em>
string array
</em>
</td>
<td>
<em>(Optional)</em>
<p>SeedNamespaces contains the seed namespaces to check for load balancers.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="filter">Filter
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
Filter specifies a network-CIDR policy pair.
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
<a href="#policy">Policy</a>
</em>
</td>
<td>
<p>Policy is the access policy (`BLOCK_ACCESS` or `ALLOW_ACCESS`).</p>
</td>
</tr>
<tr>
<td>
<code>tags</code></br>
<em>
<a href="#tag">Tag</a> array
</em>
</td>
<td>
<em>(Optional)</em>
<p>Tags contains metadata tags for the entry (preserved from v2 format).</p>
</td>
</tr>

</tbody>
</table>


<h3 id="filterlistprovidertype">FilterListProviderType
</h3>
<p><em>Underlying type: string</em></p>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>

</p>


<h3 id="policy">Policy
</h3>
<p><em>Underlying type: string</em></p>


<p>
(<em>Appears on:</em><a href="#filter">Filter</a>, <a href="#tagfilter">TagFilter</a>)
</p>

<p>
Policy is the access policy
</p>


<h3 id="secretref">SecretRef
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
SecretRef references a Secret containing filter list data.
- For ProjectFilterListSource: Secret in seed cluster (synced by Gardener from Shoot.spec.resources)
- For ShootFilterListSource: Secret directly in the shoot cluster (not synced)
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
<code>name</code></br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the Secret.</p>
</td>
</tr>
<tr>
<td>
<code>key</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Key is the data key containing the filter list in JSON format.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace is the namespace of the Secret in the shoot cluster.<br />Only used for ShootFilterListSource.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="tag">Tag
</h3>


<p>
(<em>Appears on:</em><a href="#filter">Filter</a>)
</p>

<p>
Tag represents a metadata tag with a name and values.
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
<code>name</code></br>
<em>
string
</em>
</td>
<td>
<p>Name is the tag name.</p>
</td>
</tr>
<tr>
<td>
<code>values</code></br>
<em>
string array
</em>
</td>
<td>
<p>Values is the list of tag values.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="tagfilter">TagFilter
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
TagFilter specifies a tag-based filter criterion.
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
<code>name</code></br>
<em>
string
</em>
</td>
<td>
<p>Name is the tag name to filter on.</p>
</td>
</tr>
<tr>
<td>
<code>values</code></br>
<em>
string array
</em>
</td>
<td>
<p>Values is the list of allowed tag values.<br />An entry matches if it has this tag with any of these values.</p>
</td>
</tr>
<tr>
<td>
<code>policy</code></br>
<em>
<a href="#policy">Policy</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Policy is an optional access policy to override for matching entries.<br />If specified, matching entries will have their policy changed to this value.<br />If omitted, entries keep their original policy from the source filter list.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="workers">Workers
</h3>


<p>
(<em>Appears on:</em><a href="#egressfilter">EgressFilter</a>)
</p>

<p>
Workers allows to set the blocking mode for specific worker groups which may differ from the default.
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
boolean
</em>
</td>
<td>
<p>BlackholingEnabled is a flag to set blackholing or firewall approach.</p>
</td>
</tr>
<tr>
<td>
<code>names</code></br>
<em>
string array
</em>
</td>
<td>
<p>Names is a list of worker groups to use the specified blocking mode.</p>
</td>
</tr>

</tbody>
</table>


