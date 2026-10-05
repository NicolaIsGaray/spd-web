//go:build !unix

package convert

import "os/exec"

// killProcessGroupOnCancel: fuera de Unix se usa el comportamiento por defecto de
// exec.CommandContext (mata solo el proceso hijo).
func killProcessGroupOnCancel(*exec.Cmd) {}
