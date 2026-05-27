package state

import "os"

func os_WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
