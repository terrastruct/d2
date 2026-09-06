# Real-world diagrams

These fixtures document real projects. `TestE2E/real_world` runs each through the existing graph serialization, layout, SVG and board-golden checks for both Dagre and ELK. The corpus preserves the authors’ structure, labels and styling. It does not assert that a documented target design is deployed.

Third-party icons in the Jupyter and Queue fixtures have been replaced with built-in labeled D2 shapes. The fixtures have no external image dependencies. ROSS diagram links use URLs pinned to the collected upstream revision so they resolve outside the original repository.

Source revisions and repository license files are recorded below. Fixture headers describe adaptations. The diagram sources use MIT or Apache-2.0; the original third-party icon artwork is not included.

## Jupyter AWS EKS infrastructure

Fixture: [jupyter_aws_eks.d2](jupyter_aws_eks.d2)

Source: [jupyter-infra/jupyter-deploy: diagrams/aws-eks-oidc-template/infrastructure.d2](https://github.com/jupyter-infra/jupyter-deploy/blob/7283c07396c3d0126346217089e2527d1ce652f6/diagrams/aws-eks-oidc-template/infrastructure.d2)

Revision: `7283c07396c3d0126346217089e2527d1ce652f6`

Original source SHA-256: `18fb06d9c18519c9ce12e71de6717e7854f15882e7c2e79c091b5338b940b245`

License: MIT — [LICENSE](real_world_licenses/jupyter_aws_eks_LICENSE)

AWS account, region, VPC, EKS control plane, platform nodes, routing nodes and workspace node pools, with DNS and OIDC.

## ROSS package overview

Fixture: [ross_overview.d2](ross_overview.d2)

Source: [petrobras/ross: docs/_static/diagrams/ross-overview.d2](https://github.com/petrobras/ross/blob/631a249adbae414d5f5f986479b58f1a4c47935e/docs/_static/diagrams/ross-overview.d2)

Revision: `631a249adbae414d5f5f986479b58f1a4c47935e`

Original source SHA-256: `9fd759ba20f4693824eba0d502f1d990af335c36a015da38354a79ac48533bcd`

License: Apache-2.0 — [LICENSE.md](real_world_licenses/ross_overview_LICENSE.md)

The rotor-dynamics package’s elements, rotor assembly, analyses and results.

## Go Queue multi-service worker architecture

Fixture: [queue_workers.d2](queue_workers.d2)

Source: [golang-queue/queue: images/flow-03.d2](https://github.com/golang-queue/queue/blob/2a79b8aac36eaca758ff4f5412fcb485b027dd58/images/flow-03.d2)

Revision: `2a79b8aac36eaca758ff4f5412fcb485b027dd58`

Original source SHA-256: `f6f604a937a0db36fc78cea511a05e9e9f15b45e3bf0d6b1ed0d56eb27102500`

License: MIT — [LICENSE](real_world_licenses/queue_workers_LICENSE)

Multiple producers and worker services around the queue/ring buffer, with Redis, RabbitMQ and NATS.

Redis uses D2's cylinder shape; RabbitMQ and NATS use its queue shape. Decorative icons are replaced by the existing labeled shapes.

## Spyre encoder inference target design

Fixture: [spyre_encoder.d2](spyre_encoder.d2)

Source: [torch-spyre/spyre-inference: docs/architecture/encoder-ideal-state.d2](https://github.com/torch-spyre/spyre-inference/blob/c81a50ca25c5faf42a5deeeb80654c4b853625a6/docs/architecture/encoder-ideal-state.d2)

Revision: `c81a50ca25c5faf42a5deeeb80654c4b853625a6`

Original source SHA-256: `02c966aafd4c9f640ee5765cc161d74fc94e7341db4189d85819e1502b60f613`

License: Apache-2.0 — [LICENSE](real_world_licenses/spyre_encoder_LICENSE)

Target architecture for encoder/embedding inference: vLLM scheduling, compiled model body, attention, pooling and dense/flash strategies. Explicitly a target design.

## Mocha secure-enclave SoC

Fixture: [mocha_soc.d2](mocha_soc.d2)

Source: [lowRISC/mocha: doc/img/mocha.d2](https://github.com/lowRISC/mocha/blob/b5973217f704923917e7761f73df7dfcb8d0c345/doc/img/mocha.d2)

Revision: `b5973217f704923917e7761f73df7dfcb8d0c345`

Original source SHA-256: `74ffb5140e8dd482362a0d785f3d11cafe79ea5ec78838667a736607e13d5f56`

License: Apache-2.0 (REUSE.toml explicitly covers doc/**) — [REUSE.toml](real_world_licenses/mocha_soc_REUSE.toml); [Apache-2.0.txt](real_world_licenses/mocha_soc_Apache-2.0.txt)

CVA6-CHERI processor, cache, AXI/TileLink crossbars, tag control, SRAM, DRAM and grouped peripherals, with cached/tag-awareness legend.

## TPMJS platform architecture v1

Fixture: [tpmjs_architecture.d2](tpmjs_architecture.d2)

Source: [tpmjs/tpmjs: docs/architecture-v1.d2](https://github.com/tpmjs/tpmjs/blob/0564b4b9d1248b4a112508869ff7b17b14f04d7f/docs/architecture-v1.d2)

Revision: `0564b4b9d1248b4a112508869ff7b17b14f04d7f`

Original source SHA-256: `81afba25886b60260668411d3c93e8a1d8cbf53c6c9854771d127d2f2d5b4763`

License: MIT — [LICENSE](real_world_licenses/tpmjs_architecture_LICENSE)

Large agent-tool package manager graph: browser/CLI/MCP clients, Next.js registry/API, Postgres schema, npm ingestion, package authoring, sandbox executor and local bridge.

This is a historical v1 architecture snapshot. Its source explicitly identifies the Vercel hosting nodes as retired; some other components are marked work in progress upstream.

## Jupyter Kubernetes OIDC access flow

Fixture: [jupyter_k8s_oidc.d2](jupyter_k8s_oidc.d2)

Source: [jupyter-infra/jupyter-k8s: diagrams/web-access-oidc.d2](https://github.com/jupyter-infra/jupyter-k8s/blob/92e2d8e43c94355f999f4d4a46c7a73dfc395f93/diagrams/web-access-oidc.d2)

Revision: `92e2d8e43c94355f999f4d4a46c7a73dfc395f93`

Original source SHA-256: `301d5d2847078680cb903cf045835cbf62c9be468a36979f78346c2dc0911c63`

License: MIT — [LICENSE](real_world_licenses/jupyter_k8s_oidc_LICENSE)

Browser login, router, OIDC provider, auth middleware, Extension API and workspace; shows ten numbered handoffs across namespaces.

## Ouroboros Leios simulator components

Fixture: [leios_simulator.d2](leios_simulator.d2)

Source: [input-output-hk/ouroboros-leios: simulation/docs/component.d2](https://github.com/input-output-hk/ouroboros-leios/blob/c5c86a28414319eabbda7cf1b27507753cc5a7a1/simulation/docs/component.d2)

Revision: `c5c86a28414319eabbda7cf1b27507753cc5a7a1`

Original source SHA-256: `7849a0924dc44deb0287a17be071fede0dd803c06b3dd744c1a54c560b9bd6fb`

License: Apache-2.0 (simulation/LICENSE) — [LICENSE](real_world_licenses/leios_simulator_LICENSE)

Protocol engine, nested Leios relays, Praos mini-protocols, modeled TCP/channel layer, event tracing and visualization engine.

## Fulcro RAD architecture

Fixture: [fulcro_rad.d2](fulcro_rad.d2)

Source: [fulcrologic/fulcro-rad: docs/architecture.d2](https://github.com/fulcrologic/fulcro-rad/blob/db68b2c8df345284e40d1125cd002a7c74be968b/docs/architecture.d2)

Revision: `db68b2c8df345284e40d1125cd002a7c74be968b`

Original source SHA-256: `9560570c88e1752be7ee8492f83f43dc17dfc52017caa57fb4f8338552773823`

License: MIT — [LICENSE](real_world_licenses/fulcro_rad_LICENSE)

Attributes drive forms, reports, resolvers and a multi-step save middleware pipeline ending at the database.

## Lion Reader frontend data flow

Fixture: [lion_reader_frontend.d2](lion_reader_frontend.d2)

Source: [brendanlong/lion-reader: docs/diagrams/frontend-data-flow.d2](https://github.com/brendanlong/lion-reader/blob/34a5f1fac3503e0df19e6904da44eeb048b76700/docs/diagrams/frontend-data-flow.d2)

Revision: `34a5f1fac3503e0df19e6904da44eeb048b76700`

Original source SHA-256: `934d5b2fd0f0cab2a4e1de75502f28629533395f27cdc52cf307edbfdb3d6099`

License: MIT — [LICENSE](real_world_licenses/lion_reader_frontend_LICENSE)

React page components, custom hooks, TanStack Query, tRPC client, SSE manager and backend services; detailed end-to-end browser data path.
