package nginx

import (
	"os"
	"runtime"
	"syscall"
)

// processAlive проверяет, существует ли процесс с заданным pid,
// выбирая реализацию в зависимости от целевой ОС.
func processAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		return processAliveWindows(pid)
	}
	return processAliveUnix(pid)
}

func processAliveWindows(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

func processAliveUnix(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}