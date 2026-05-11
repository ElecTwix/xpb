// Package common holds constants and helpers shared by every language
// codegen. Keeping the per-language emitters dependent on a single source
// of truth for things like the default array-max budget prevents drift —
// when one codegen tightens the policy, the others get the same value
// without a follow-up patch.
package common

// DefaultMaxElements is the budget the generated decoders pass to the
// runtime's array-count gate when the schema doesn't carry a per-field
// cap. Picked at 1 << 24 (~16 M elements) — large enough for any
// legitimate payload, small enough that a 4-byte adversarial count
// can't OOM a host. Application code may override per call site by
// editing the generated source; the constant is the floor.
const DefaultMaxElements = 1 << 24
