//go:build unix

package convert

import (
	"os/exec"
	"syscall"
)

// killProcessGroupOnCancel lanza el comando en un grupo de procesos propio y, al cancelarse el
// contexto, mata el grupo entero. soffice es un script que arranca soffice.bin: matar solo al
// hijo directo dejaría a soffice.bin huérfano y consumiendo CPU.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
