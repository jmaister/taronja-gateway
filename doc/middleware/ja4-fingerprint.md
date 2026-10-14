# ja4_fingerprint

Fingerprints each request and stores it in the `X-Taronja-Fingerprint`
header as `<type>:<value>`, overwriting any value the client sent. The
header is forwarded to proxied backends.

```yaml
middleware:
  - name: ja4_fingerprint
```

**Options:** none. **Depends on:** nothing. Needed by
[`session_extraction`](session-extraction.md) and
[`traffic_metrics`](traffic-metrics.md).

Two fingerprint types, the first one available wins:

| Type | Built from | Notes |
|---|---|---|
| `stable` | `User-Agent`, `Accept-Encoding`, `Accept-Language`, `Sec-Ch-Ua*` | Same value across a browser's page, asset and API requests. Not part of the JA4 spec. |
| `ja4h` | [JA4H](https://github.com/FoxIO-LLC/ja4) (method, header names, cookies, referer) | Changes between page loads, assets and XHR from the same browser. |

Neither identifies a visitor reliably; treat them as a grouping hint.
