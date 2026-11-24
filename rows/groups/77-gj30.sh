# Group gj30 of Ecd Runtime: the smoke row of the comparison benchmarks of package gj in go/enginecmp. TestGJSmoke runs
# the body of each benchmark of the gates GJ1 to GJ4, G4 and G6 once with 1,000 entities on both engines, Jacquard and
# Ark, and fails on any error. The measurement of the gates is rows/gj.bench.sh, a separate script that this group does
# not run.
# Sourced by the rows harness from /w; the harness supplies say, sel, offline, head_of, count, why_of, go_row and
# HAVE_GO.
#
#   GJ-00    TestGJSmoke passes: every comparison benchmark body runs once on both engines without an error
#
# The row holds only with package gj of go/enginecmp and its test TestGJSmoke; a run that finds no such test is CNM.

go_row GJ-00 gj30 "TestGJSmoke rc=0" /w/go/enginecmp test -run '^TestGJSmoke$' ./gj/
