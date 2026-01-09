[![REUSE status](https://api.reuse.software/badge/github.com/openmcp-project/service-provider-template)](https://api.reuse.software/info/github.com/openmcp-project/service-provider-template)

# service-provider-velero

## About this project

Managed [velero](https://velero.io) offering for [openmcp](https://github.com/openmcp-project).

## Requirements and Setup

Run End-to-End tests:

```shell
task test-e2e
```

## End-User facing API

- version: velero version to install
- plugins: currently unrestricted

## Platform Operator facing API

- PollInterval for drift detection
- ImagePullSecretRef allows to pull velero (plugin) images from private registries

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/openmcp-project/service-provider-template/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure

If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/openmcp-project/service-provider-template/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2025 SAP SE or an SAP affiliate company and service-provider-template contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmcp-project/service-provider-template).
