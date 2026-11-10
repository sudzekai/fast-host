//go:build !linux

package metrics

import "os"

func fileIdentity(info os.FileInfo) string { return "" }
