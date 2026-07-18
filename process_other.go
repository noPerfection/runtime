//go:build !unix

package topology

import "os/exec"

func configureChildProcess(cmd *exec.Cmd) {}
