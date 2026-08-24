package xlsx

// ColumnName converts a zero-based column index into its spreadsheet
// letters: 0 becomes "A", 25 "Z", 26 "AA".
func ColumnName(i int) string {
	if i < 0 {
		return "A"
	}
	var buf [4]byte
	n := len(buf)
	for {
		n--
		buf[n] = byte('A' + i%26)
		i = i/26 - 1
		if i < 0 || n == 0 {
			break
		}
	}
	return string(buf[n:])
}

// columnIndex extracts the zero-based column from a cell reference such as
// "BC12". Excel omits empty cells entirely rather than padding them, so a
// cell's reference is the only thing that says which column it belongs to
// — reading positionally would shift every value after the first gap.
// Returns -1 when ref carries no column letters.
func columnIndex(ref string) int {
	n, seen := 0, false
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		switch {
		case c >= 'A' && c <= 'Z':
			n = n*26 + int(c-'A') + 1
		case c >= 'a' && c <= 'z':
			n = n*26 + int(c-'a') + 1
		default:
			// Letters always precede the row digits in a reference, so
			// the first non-letter ends the column part.
			if seen {
				return n - 1
			}
			return -1
		}
		seen = true
		if n > maxColumns {
			return -1
		}
	}
	if !seen {
		return -1
	}
	return n - 1
}
