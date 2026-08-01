package registry

// testIndexYAML mirrors the real mass-registry index.yml shape, extended with
// extra versions and a second worker backend to exercise newest-first
// resolution and platform-key misses. Digests are stubbed except where a test
// needs a real one.
const testIndexYAML = `
schema_version: 1
packages:
  - name: mass-runtime-gateway-llama-cpp
    kind: runtime
    runtime_name: llama-cpp
    display_name: llama.cpp
    description: |
      Runtime gateway for llama.cpp-family inference workers.
    versions:
      - version: 0.1.0
        mass: ">=0.1 <0.2"
        artifacts:
          linux/amd64:
            url: https://example.test/runtime/0.1.0/linux_amd64.mass
            sha256: aaaa
          darwin/arm64:
            url: https://example.test/runtime/0.1.0/darwin_arm64.mass
            sha256: bbbb
      - version: 0.2.0
        mass: ">=0.2"
        artifacts:
          linux/amd64:
            url: https://example.test/runtime/0.2.0/linux_amd64.mass
            sha256: cccc
  - name: mass-worker-llama-cpp
    kind: worker
    runtime_name: llama-cpp
    display_name: llama.cpp worker
    description: |
      Native llama.cpp inference worker with a Vulkan backend.
    versions:
      - version: 0.1.0
        runtime: ">=0.1 <0.2"
        mass: ">=0.1"
        artifacts:
          linux/amd64/vulkan:
            url: https://example.test/worker/0.1.0/linux_amd64_vulkan
            sha256: dddd
      - version: 0.1.5
        runtime: ">=0.1 <0.2"
        mass: ">=0.3"
        artifacts:
          linux/amd64/vulkan:
            url: https://example.test/worker/0.1.5/linux_amd64_vulkan
            sha256: eeee
  - name: mass-worker-llama-cpp-extra
    kind: worker
    runtime_name: llama-cpp
    display_name: llama.cpp extra worker
    description: |
      A second worker package for the same runtime, with an unconstrained
      mass range to exercise the empty-mass path.
    versions:
      - version: 0.2.0
        runtime: ">=0.1 <0.2"
        artifacts:
          linux/amd64/vulkan:
            url: https://example.test/worker-extra/0.2.0/linux_amd64_vulkan
            sha256: ffff
`
