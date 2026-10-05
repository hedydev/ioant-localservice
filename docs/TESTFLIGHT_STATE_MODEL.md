# TestFlight Release state model

ILS keeps three Apple-side TestFlight signals separate:

```text
Build Upload
Internal Testing
External Testing
```

The Release-level state follows the **external testing path** when that path exists. Internal readiness is still shown independently, but it must not make the whole Release externally available.

## Required semantics

| Apple state | ILS Release state |
| --- | --- |
| Build Upload `AWAITING_UPLOAD` | `submitted` |
| Build Upload `PROCESSING` | `processing` |
| Internal `READY_FOR_BETA_TESTING`, External empty | `processing` |
| Internal `READY_FOR_BETA_TESTING`, External `READY_FOR_BETA_SUBMISSION` | `processing` |
| External `WAITING_FOR_BETA_REVIEW` | `processing` |
| External `IN_BETA_REVIEW` | `processing` |
| External `BETA_APPROVED` | `processing` |
| External `READY_FOR_BETA_TESTING` | `available` |
| External `IN_BETA_TESTING` | `available` |
| External `BETA_REJECTED` / processing exception / expiry | `unavailable` |

`BETA_APPROVED` means the external beta review was approved, but ILS still waits for Apple to expose a beta-ready/testing state before promoting the Release to `available`.

## Regression evidence

On 2026-10-05 the real Sowhat TestFlight catalog showed:

```text
build 228: Internal Ready for Testing / External Approved
build 224: Internal Ready for Testing / External Ready to Submit
```

ILS correctly mirrored those independent states in the Web UI. The existing `TestTestFlightStateInternalReadyExternalReadyToSubmit` regression test still expected `available`, contradicting both the implementation and the table-driven state test. The regression test was corrected to require `processing`, and an explicit `BETA_APPROVED -> processing` case was added.

This state model must remain aligned with actual App Store Connect evidence. Upload success, internal readiness, approval, or elapsed time alone must never be promoted to externally testable availability.
