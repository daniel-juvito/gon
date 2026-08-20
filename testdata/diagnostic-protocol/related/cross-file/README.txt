Reserved fixture area for a future producer that projects ContractTrace
(or equivalent) into Diagnostic.relatedInformation.

Protocol v1 does not require a live producer for relatedInformation.
Schema coverage is provided by the unit-level JSON round-trip test in
cmd/gon (construct → marshal → unmarshal), not by running the checker
against sources in this directory.

Do not add .gon placeholders here that imply required diagnostic output
under the current harness.
