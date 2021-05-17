# cloudflare-operator

[![Build](https://github.com/arikkfir/cloudflare-operator/actions/workflows/build.yml/badge.svg)](https://github.com/arikkfir/cloudflare-operator/actions/workflows/build.yml)

Operate Cloudflare resources as Kubernetes objects.

## Roadmap

- [x] Add APIKey object
- [ ] Support custom sync interval for `DNSRecord` objects
- [ ] Add tests that use a local cluster
- [x] Detect deletion of `DNSRecord` and delete the associated DNS record
- [ ] Update `Status` of `DNSRecord` objects
- [ ] Verify thread-safety for DNSRecordReconciler `loops` map
