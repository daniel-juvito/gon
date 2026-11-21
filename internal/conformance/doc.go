// Package conformance holds the conformance harness of the explicit non-nil
// conversion (Part A of docs/rfc-explicit-nonnil-conversion.md). It has no
// production code: the tests read testdata/conformance, where a manifest lists
// every fixture with its RFC clause, its status and the diagnostic it expects,
// so that the behaviour is defined before the implementation and is not
// defined by it. See docs/plan-explicit-nonnil-conversion-part-a.md, section 4.
package conformance
