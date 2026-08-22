package postgres

import "fmt"

// fmtSscanf isolates the single fmt.Sscanf use so the interval parser reads
// cleanly above.
func fmtSscanf(s, format string, args ...any) (int, error) {
	return fmt.Sscanf(s, format, args...)
}
