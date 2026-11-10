package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Memory struct {
	Available      bool   `json:"available"`
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
	SampledAt      string `json:"sampled_at"`
	Error          string `json:"error,omitempty"`
}

func parseMemory(text string) (Memory, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Memory{}, err
		}
		if len(fields) > 2 && fields[2] == "kB" {
			n *= 1024
		}
		values[strings.TrimSuffix(fields[0], ":")] = n
	}
	if err := scanner.Err(); err != nil {
		return Memory{}, err
	}
	total := values["MemTotal"]
	available, ok := values["MemAvailable"]
	if !ok || total == 0 || available > total {
		return Memory{}, fmt.Errorf("нет достоверных MemTotal/MemAvailable")
	}
	swapTotal, swapFree := values["SwapTotal"], values["SwapFree"]
	if swapFree > swapTotal {
		return Memory{}, fmt.Errorf("некорректный размер swap")
	}
	return Memory{Available: true, TotalBytes: total, AvailableBytes: available, UsedBytes: total - available, SwapTotalBytes: swapTotal, SwapUsedBytes: swapTotal - swapFree, SampledAt: time.Now().Format(time.RFC3339)}, nil
}

func HostMemory() Memory {
	text, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Memory{Error: "RAM сервера недоступна: требуется Linux /proc/meminfo"}
	}
	result, err := parseMemory(string(text))
	if err != nil {
		return Memory{Error: err.Error()}
	}
	return result
}
