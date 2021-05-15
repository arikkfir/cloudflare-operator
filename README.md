# cloudflare-operator

[![Build](https://github.com/arikkfir/cloudflare-operator/actions/workflows/build.yml/badge.svg)](https://github.com/arikkfir/cloudflare-operator/actions/workflows/build.yml)

Operate Cloudflare resources as Kubernetes objects.

## Roadmap

- [x] Add APIKey object
- [ ] Add tests that use a local cluster
- [ ] Detect deletion of `DNSRecord` and delete the associated DNS record
- [ ] Update `Status` of `DNSRecord` objects
