// Deliberately incompatible support, compiled only by the ABI negative test
// under the real support import path. Never a normal program dependency.
package probesupport

//go:noinline
func ABI_v0() {}
