# Changelog

## [0.1.2](https://github.com/hortator-ai/Hortator/compare/v0.1.1...v0.1.2) (2026-02-14)


### Bug Fixes

* correct spawn CLI flags and JSON key mismatch in agentic runtime ([b71db5c](https://github.com/hortator-ai/Hortator/commit/b71db5c5a950fc8aae9161807b971c8b91ca19ad))
* isolate test phases to prevent AgentPolicy cross-contamination ([d20b738](https://github.com/hortator-ai/Hortator/commit/d20b73857f1df3bee2154df84be7305d353e71a9))
* redact PII in runtime output via Presidio anonymize endpoint ([428f1bc](https://github.com/hortator-ai/Hortator/commit/428f1bc09039daebedff2c4512dfbbca5671ee21))
* redact PII in runtime output via Presidio anonymize endpoint ([a4c03b3](https://github.com/hortator-ai/Hortator/commit/a4c03b3d7b54e2c6b4659b6483265d5a62442b2d))
* use correct runtime entrypoint for agentic tiers in warm pool ([ad421a3](https://github.com/hortator-ai/Hortator/commit/ad421a38257d32bd38ebe6950ad5e71c04a5c042))
* use correct runtime entrypoint for agentic tiers in warm pool ([b496b54](https://github.com/hortator-ai/Hortator/commit/b496b542aaa342316a51a61052d094156fc0dffc))
* wire result cache check before pod creation and add retain CLI command ([e223593](https://github.com/hortator-ai/Hortator/commit/e2235933f2c14f672f0001dbb88ab885bb3e9e58))
* wire result cache config and add hortator retain CLI command ([bab297d](https://github.com/hortator-ai/Hortator/commit/bab297d2f23e772d0f72d31294d4e6a488a20885))

## [0.1.1](https://github.com/hortator-ai/Hortator/compare/v0.1.0...v0.1.1) (2026-02-13)


### Bug Fixes

* add cleanup steps between test phases to prevent policy pollution ([c0aa2ba](https://github.com/hortator-ai/Hortator/commit/c0aa2ba74ef08d8e7654fda03a8a7045fef3bc5f))
* default all image tags to appVersion instead of latest ([2fcc8e4](https://github.com/hortator-ai/Hortator/commit/2fcc8e4b6f5204db484960e9938e18d419309294))
* default all image tags to appVersion instead of latest ([e48184d](https://github.com/hortator-ai/Hortator/commit/e48184dfbcf5e20276d4a5405840d415a2444b84))
* improve phase 1 test manifests for reliable failure testing ([6153dc2](https://github.com/hortator-ai/Hortator/commit/6153dc2efa9345fb419a7a74c9d0ca0a836019e5))
* improve phase 1 test manifests for reliable failure testing ([7e563d0](https://github.com/hortator-ai/Hortator/commit/7e563d0930b6379b6c39303bef94555a3ced711d))
* kitchen-sink test uses self-generated code instead of external repo ([4156ca5](https://github.com/hortator-ai/Hortator/commit/4156ca582c3fb7e8607aceba5fab950f3073de28))
* make github-credentials optional in kitchen-sink test ([0a1405b](https://github.com/hortator-ai/Hortator/commit/0a1405bb27d4d79c2eb96a43b28bd8e138a4c5de))
* reduce cleanup TTL defaults (completed=1h, failed=24h, cancelled=1h) ([e2a9544](https://github.com/hortator-ai/Hortator/commit/e2a9544cda403d89c18bef4b836011ce6852987a))
* reduce cleanup TTL defaults to practical values ([c13c18c](https://github.com/hortator-ai/Hortator/commit/c13c18caf0b0747eec79671bef127c2612ac7a4e))
* remove hardcoded storageClass, use cluster default ([3c39081](https://github.com/hortator-ai/Hortator/commit/3c3908174df3e71ce8db8491281413622ea29706))

## [0.1.0](https://github.com/hortator-ai/Hortator/compare/v0.0.1...v0.1.0) (2026-02-13)


### Features

* TUI single-pane view for logs, details, describe, summary ([19cfb6b](https://github.com/hortator-ai/Hortator/commit/19cfb6b62a1bacda682d5e521bc0c7fdf9b1a14a))
* TUI single-pane views for logs, details, describe, summary ([ec4a6c9](https://github.com/hortator-ai/Hortator/commit/ec4a6c94d52a6fa576469485f987143c25799455))


### Bug Fixes

* align container image names across CI, chart, and Go code ([f299b9f](https://github.com/hortator-ai/Hortator/commit/f299b9f246ab4e38fe9ceb6ab4d1235c2258643e))
* align container image names across CI, chart, and Go code ([ef40362](https://github.com/hortator-ai/Hortator/commit/ef403627c8e6a6c055d5f52aa27283a8e3400e0a))
* align pr-check Go version to 1.25 ([7cde320](https://github.com/hortator-ai/Hortator/commit/7cde320a0c4b6941911aa9ee98c58e3a01c411f9))
* bump controller-gen to v0.18.0 in pr-check ([565b4ba](https://github.com/hortator-ai/Hortator/commit/565b4ba488e68757bde9fa605424a0ff86ea87c0))
* regenerate deepcopy, CRDs and RBAC with controller-gen v0.18.0 ([32804cf](https://github.com/hortator-ai/Hortator/commit/32804cf81864b917e6982677c05c99e3d2577039))
* update policy glob patterns to match new image paths ([467457d](https://github.com/hortator-ai/Hortator/commit/467457d8ee9ddef0d13a5455c73d8a1d136ef765))
