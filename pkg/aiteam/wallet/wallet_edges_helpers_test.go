package wallet

import "os"

func readFileImpl(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeFileImpl(path string, b []byte) {
	_ = os.WriteFile(path, b, 0o600)
}
