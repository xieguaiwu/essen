// Package tui provides terminal UI helpers for interactive input.
package tui

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Linux termios constants (architecture-independent, from asm-generic/termbits.h)
// ---------------------------------------------------------------------------

const (
	// ioctl commands
	tcgets  = 0x5401
	tcsets  = 0x5402
	tcsanow = 0

	// c_iflag bits
	ignbrk = 0x0001
	brkint = 0x0002
	parmrk = 0x0008
	istrip = 0x0020
	inlcr  = 0x0040
	igncr  = 0x0080
	icrnl  = 0x0100
	ixon   = 0x0400

	// c_oflag bits
	opost = 0x0001

	// c_cflag bits
	csize  = 0x0030
	cs8    = 0x0030
	parenb = 0x0004

	// c_lflag bits
	isig   = 0x0001
	icanon = 0x0002
	echo   = 0x0008
	echonl = 0x0040
	iexten = 0x8000

	// c_cc indices (Linux NCCS = 19, VMIN/VTIME at indices 6/5)
	vmin  = 6
	vtime = 5
)

// termios matches the Linux kernel struct termios layout (36 bytes).
type termios struct {
	Iflag uint32
	Oflag uint32
	Cflag uint32
	Lflag uint32
	Line  uint8
	Cc    [19]uint8
}

func getTermios(fd int) (*termios, error) {
	t := &termios{}
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), tcgets, uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	return t, nil
}

func setTermios(fd int, t *termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), tcsets, uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// makeRaw puts the terminal referenced by fd into raw mode and returns the
// original termios state to be restored later.
func makeRaw(fd int) (*termios, error) {
	old, err := getTermios(fd)
	if err != nil {
		return nil, fmt.Errorf("tcgetattr: %w", err)
	}

	raw := *old
	raw.Iflag &^= ignbrk | brkint | parmrk | istrip | inlcr | igncr | icrnl | ixon
	raw.Cflag &^= csize | parenb
	raw.Cflag |= cs8
	raw.Lflag &^= echo | echonl | icanon | isig | iexten
	raw.Cc[vmin] = 1
	raw.Cc[vtime] = 0

	if err := setTermios(fd, &raw); err != nil {
		return nil, fmt.Errorf("tcsetattr: %w", err)
	}
	return old, nil
}

func restoreTermios(fd int, state *termios) error {
	return setTermios(fd, state)
}

// ---------------------------------------------------------------------------
// TTY helpers
// ---------------------------------------------------------------------------

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------------------------------------------------------------------------
// Unicode display width (simplified; handles CJK and common wide chars)
// ---------------------------------------------------------------------------

func runeDisplayWidth(r rune) int {
	if r < 32 || r == 0x7f {
		return 0 // control characters
	}
	// zero-width joiners and combining marks
	if r == 0x200d || (r >= 0x0300 && r <= 0x036f) ||
		(r >= 0x0483 && r <= 0x0489) ||
		(r >= 0x0591 && r <= 0x05bd) ||
		(r >= 0x0610 && r <= 0x061a) ||
		(r >= 0x064b && r <= 0x065f) {
		return 0
	}
	// CJK and wide characters
	if (r >= 0x1100 && r <= 0x115f) || // Hangul Jamo
		(r >= 0x2329 && r <= 0x232a) || // angle brackets
		(r >= 0x2e80 && r <= 0xa4cf) || // CJK Radicals .. Yi
		(r >= 0xa960 && r <= 0xa97c) || // Hangul Jamo Extended-A
		(r >= 0xac00 && r <= 0xd7a3) || // Hangul Syllables
		(r >= 0xf900 && r <= 0xfaff) || // CJK Compatibility Ideographs
		(r >= 0xfe10 && r <= 0xfe19) || // Vertical forms
		(r >= 0xfe30 && r <= 0xfe6f) || // CJK Compatibility Forms
		(r >= 0xff01 && r <= 0xff60) || // Fullwidth Forms
		(r >= 0xffe0 && r <= 0xffe6) || // Fullwidth Signs
		(r >= 0x1f300 && r <= 0x1f64f) || // Emoticons
		(r >= 0x1f680 && r <= 0x1f6ff) || // Transport
		(r >= 0x20000 && r <= 0x2fffd) || // CJK Extension B+
		(r >= 0x30000 && r <= 0x3fffd) { // CJK Extension G+
		return 2
	}
	return 1
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

func displayWidthRunes(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runeDisplayWidth(r)
	}
	return w
}

// ---------------------------------------------------------------------------
// readEscSeq reads the rest of an escape sequence after the initial ESC.
// It returns the full sequence string (including the leading ESC) and the
// interpreted command key or 0 if unrecognised.
// ---------------------------------------------------------------------------

type escResult int

const (
	escNone   escResult = iota
	escUp                // [A
	escDown              // [B
	escRight             // [C
	escLeft              // [D
	escHome              // [H or [1~
	escEnd               // [F or [4~
	escDelete            // [3~
)

func readEscSeq() (escResult, error) {
	// After ESC (already consumed), read the next byte.
	b := make([]byte, 1)
	n, err := os.Stdin.Read(b)
	if err != nil {
		return escNone, err
	}
	if n == 0 || b[0] != '[' {
		return escNone, nil
	}

	// Read the command byte.
	n, err = os.Stdin.Read(b)
	if err != nil {
		return escNone, err
	}
	if n == 0 {
		return escNone, nil
	}

	switch b[0] {
	case 'A':
		return escUp, nil
	case 'B':
		return escDown, nil
	case 'C':
		return escRight, nil
	case 'D':
		return escLeft, nil
	case 'H':
		return escHome, nil
	case 'F':
		return escEnd, nil
	case '1':
		// Home (VT100): [1~
		n, err = os.Stdin.Read(b)
		if err != nil || n == 0 || b[0] != '~' {
			return escNone, nil
		}
		return escHome, nil
	case '3':
		// Delete: [3~
		n, err = os.Stdin.Read(b)
		if err != nil || n == 0 || b[0] != '~' {
			return escNone, nil
		}
		return escDelete, nil
	case '4':
		// End (VT100): [4~
		n, err = os.Stdin.Read(b)
		if err != nil || n == 0 || b[0] != '~' {
			return escNone, nil
		}
		return escEnd, nil
	default:
		return escNone, nil
	}
}

// ---------------------------------------------------------------------------
// ReadLine — public API
// ---------------------------------------------------------------------------

// ReadLine reads a single line of input with interactive line editing support.
//
// When stdin is a TTY the terminal is placed into raw mode so that every
// keystroke is available immediately.  Supported editing keys:
//
//	Left / Right        — move cursor
//	Home (Ctrl+A)       — jump to beginning of line
//	End  (Ctrl+E)       — jump to end of line
//	Backspace           — delete character before cursor
//	Delete (Ctrl+D)     — delete character at cursor (empty buffer → EOF)
//	Ctrl+U              — clear entire line
//	Ctrl+C              — abort (returns the interrupt as an error)
//
// When stdin is not a TTY (pipe / redirect) the function falls back to
// bufio.Scanner so the program remains usable in non-interactive contexts.
func ReadLine(prompt string) (string, error) {
	if !isTTY(os.Stdin) {
		return readLineFallback(prompt)
	}

	fd := int(os.Stdin.Fd())
	oldState, err := makeRaw(fd)
	if err != nil {
		return readLineFallback(prompt)
	}
	defer func() {
		_ = restoreTermios(fd, oldState)
	}()

	fmt.Print(prompt)

	runes := make([]rune, 0, 256)
	pos := 0 // cursor within runes (0 = before first char)

	var buf [1]byte
	for {
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			fmt.Println()
			return "", err
		}
		if n == 0 {
			continue
		}

		ch := buf[0]

		switch {
		case ch == '\n' || ch == '\r':
			// Enter — finish input.
			fmt.Println()
			return string(runes), nil

		case ch == 0x03:
			// Ctrl+C — abort (send interrupt).
			fmt.Println()
			// Re-send SIGINT to ourselves.
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Signal(syscall.SIGINT)
			return "", fmt.Errorf("interrupted")

		case ch == 0x7f || ch == '\b':
			// Backspace — delete character before cursor.
			if pos > 0 {
				copy(runes[pos-1:], runes[pos:])
				runes = runes[:len(runes)-1]
				pos--
				redrawLine(prompt, runes, pos)
			}

		case ch == 0x1b:
			// Escape — arrow keys, Home, End, Delete.
			res, _ := readEscSeq()
			switch res {
			case escLeft:
				if pos > 0 {
					pos--
					repositionCursor(prompt, runes, pos)
				}
			case escRight:
				if pos < len(runes) {
					pos++
					repositionCursor(prompt, runes, pos)
				}
			case escHome:
				pos = 0
				repositionCursor(prompt, runes, pos)
			case escEnd:
				pos = len(runes)
				repositionCursor(prompt, runes, pos)
			case escDelete:
				if pos < len(runes) {
					copy(runes[pos:], runes[pos+1:])
					runes = runes[:len(runes)-1]
					redrawLine(prompt, runes, pos)
				}
			case escUp, escDown:
				// Ignore up/down arrows.
			default:
				// Unknown escape sequence — ignore.
			}

		case ch == 0x01:
			// Ctrl+A — Home.
			pos = 0
			repositionCursor(prompt, runes, pos)

		case ch == 0x05:
			// Ctrl+E — End.
			pos = len(runes)
			repositionCursor(prompt, runes, pos)

		case ch == 0x04:
			// Ctrl+D — Delete at cursor (or EOF on empty line).
			if pos < len(runes) {
				copy(runes[pos:], runes[pos+1:])
				runes = runes[:len(runes)-1]
				redrawLine(prompt, runes, pos)
			} else if len(runes) == 0 {
				fmt.Println()
				return "", nil
			}

		case ch == 0x15:
			// Ctrl+U — clear entire line.
			runes = runes[:0]
			pos = 0
			redrawLine(prompt, runes, pos)

		case ch >= 32 && ch <= 126:
			// Printable ASCII — insert at cursor.
			runes = append(runes, 0)
			copy(runes[pos+1:], runes[pos:])
			runes[pos] = rune(ch)
			pos++
			redrawLine(prompt, runes, pos)

		default:
			// Multi-byte UTF-8 (lead byte >= 0xC0) or high ASCII / extended.
			if ch >= 0x80 {
				r := decodeUTF8ByteByByte(ch)
				if r != utf8.RuneError {
					runes = append(runes, 0)
					copy(runes[pos+1:], runes[pos:])
					runes[pos] = r
					pos++
					redrawLine(prompt, runes, pos)
				}
			}
			// Other control characters (< 32) are ignored.
		}
	}
}

// readLineFallback uses bufio.Scanner when stdin is not a TTY.
func readLineFallback(prompt string) (string, error) {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

// decodeUTF8ByteByByte reads the remaining bytes of a UTF-8 sequence from
// stdin after the lead byte has already been consumed. Returns the decoded
// rune or utf8.RuneError on failure.
func decodeUTF8ByteByByte(lead byte) rune {
	// Determine how many continuation bytes are expected.
	extra := 0
	switch {
	case lead&0xE0 == 0xC0:
		extra = 1
	case lead&0xF0 == 0xE0:
		extra = 2
	case lead&0xF8 == 0xF0:
		extra = 3
	default:
		return utf8.RuneError
	}

	buf := make([]byte, 1+extra)
	buf[0] = lead
	for i := 0; i < extra; i++ {
		n, err := os.Stdin.Read(buf[1+i : 2+i])
		if err != nil || n == 0 {
			return utf8.RuneError
		}
	}

	r, _ := utf8.DecodeRune(buf)
	return r
}

// ---------------------------------------------------------------------------
// Terminal redraw helpers
// ---------------------------------------------------------------------------

// redrawLine clears the current line and re-prints prompt + buffer, then
// positions the cursor at the correct offset.
func redrawLine(prompt string, runes []rune, pos int) {
	fmt.Print("\r\033[K")           // carriage return + clear to end of line
	fmt.Print(prompt)               // prompt
	fmt.Print(string(runes))        // current buffer

	// Move cursor back to position after prompt.
	charsAfterCursor := runes[pos:]
	back := displayWidthRunes(charsAfterCursor)
	if back > 0 {
		fmt.Printf("\033[%dD", back)
	}
}

// repositionCursor moves the cursor without re-printing the whole line.
// Used for arrow-key movements (no content change).
func repositionCursor(prompt string, runes []rune, pos int) {
	// Redraw the entire line and position the cursor correctly.
	// This is simple and correct for all CJK/wide character cases.
	redrawLine(prompt, runes, pos)
}


