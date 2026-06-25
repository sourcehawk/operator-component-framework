# Version Matrix

This example demonstrates the `pkg/testing/goldengen` helper: sweeping a version universe over a single fixture,
classifying the swept versions into behaviorally-distinct gating regimes, generating one golden per regime, asserting
the gating, and proving every registered mutation is accounted for.

## What it shows

- **One build, swept across versions**: `resources.NewStatefulSetResource` builds a StatefulSet with three mutations.
  The owner's `Spec.Version` drives every gate, so wiring that build through `goldengen.Config.Build` and listing a
  version universe produces a distinct golden per gating regime instead of one golden per version.
- **Version-gated mutations**:
  - `ContainerImage` has no gate, so it fires at every version and anchors the always-on part of the firing set.
  - `PeerDiscovery/PreV2` fires for versions `< 2.0.0` (legacy peer-discovery format).
  - `PeerDiscovery/V2` fires for versions `>= 2.0.0` (peer-discovery format introduced in 2.0.0).
- **Firing-set classification**: The version universe `1.0.0`, `1.5.0`, `2.0.0` collapses to two regimes:
  `{ContainerImage, PeerDiscovery/PreV2}` covering `1.0.0` and `1.5.0`, and `{ContainerImage, PeerDiscovery/V2}`
  covering `2.0.0`. Only two goldens are written, one per regime, named by the regime's representative version.
- **Ascending version order**: Listing `Versions` ascending puts each regime's representative on the lower inclusive
  boundary of its gating range, so the golden's filename marks exactly where the regime begins.
- **Gating assertions**: `Requires`/`Forbids` pin which mutation fires (or does not) at which version. The boundary is
  asserted from both sides: `PeerDiscovery/V2` is required at `2.0.0` and forbidden at `1.5.0`.
- **Completeness accounting**: `TestMain` calls `gen.AssertComplete(m.Run())`, which fails the package if any registered
  mutation is neither required by a fixture nor listed in `Exclude`. Adding a fourth mutation without asserting it would
  break this test.

## Generated artifacts

```
testdata/version_matrix/
  manifest.yaml              # per-fixture regimes: representative, versions, firing-set
  default/1.0.0.yaml         # regime representative for { ContainerImage, PeerDiscovery/PreV2 }
  default/2.0.0.yaml         # regime representative for { ContainerImage, PeerDiscovery/V2 }
```

## Running

Generate or refresh the goldens and the manifest:

```bash
go test ./examples/version-matrix/ -run TestVersionMatrix -update
```

Verify against the committed goldens:

```bash
go test ./examples/version-matrix/
```

See [docs/testing.md](../../docs/testing.md) for the full goldengen reference.
