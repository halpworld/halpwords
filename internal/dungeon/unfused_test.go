//go:build !(arm64 || ppc64 || ppc64le || s390x || riscv64 || loong64 || amd64.v3)

package dungeon

// fusedMultiplyAdd is true where Go fuses x*y+z into one instruction,
// which rounds once instead of twice.
const fusedMultiplyAdd = false
