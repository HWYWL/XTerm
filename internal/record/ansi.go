package record

// StripANSI drops the terminal's control sequences and the control
// characters that are not a newline, a carriage return or a tab: what is
// left is the text as the screen shows it, near enough to read.
func StripANSI(p []byte) []byte {
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c != 0x1b {
			out = append(out, c)
			continue
		}
		if i+1 >= len(p) {
			break
		}
		switch p[i+1] {
		case '[': // CSI: ESC [ parameters, then a final byte
			j := i + 2
			for j < len(p) && (p[j] < 0x40 || p[j] > 0x7e) {
				j++
			}
			i = j
		case ']': // OSC: ESC ] ... BEL or ESC \
			j := i + 2
			for j < len(p) {
				if p[j] == 0x07 {
					break
				}
				if p[j] == 0x1b && j+1 < len(p) && p[j+1] == '\\' {
					j++
					break
				}
				j++
			}
			i = j
		default: // ESC and one more byte, as ESC ( B
			i++
		}
	}
	kept := out[:0]
	for _, c := range out {
		// A backspace stays: the recorder takes back the character it
		// erased, as the screen did.
		if c == '\n' || c == '\r' || c == '\t' || c == '\b' || c >= 0x20 {
			kept = append(kept, c)
		}
	}
	return kept
}
