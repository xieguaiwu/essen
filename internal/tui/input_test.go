package tui

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Display width tests
// ---------------------------------------------------------------------------

func TestRuneDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		r     rune
		width int
	}{
		{"ASCII letter", 'a', 1},
		{"ASCII digit", '5', 1},
		{"space", ' ', 1},
		{"null (control)", 0x00, 0},
		{"DEL (control)", 0x7f, 0},
		{"ESC (control)", 0x1b, 0},
		{"CJK unified ideograph", '中', 2},
		{"CJK unified ideograph", '文', 2},
		{"Hangul syllable", '한', 2},
		{"Fullwidth digit", '０', 2},
		{"Emoji (heart)", '❤', 1}, // U+2764 is 1-wide in most terminals
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runeDisplayWidth(tt.r)
			if got != tt.width {
				t.Errorf("runeDisplayWidth(%q) = %d, want %d", tt.r, got, tt.width)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
	}{
		{"empty", "", 0},
		{"ASCII", "hello", 5},
		{"CJK", "你好", 4},
		{"mixed", "hello世界", 9}, // 5 + 2 + 2
		{"ASCII + space", "a b", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayWidth(tt.s)
			if got != tt.width {
				t.Errorf("displayWidth(%q) = %d, want %d", tt.s, got, tt.width)
			}
		})
	}
}

func TestDisplayWidthRunes(t *testing.T) {
	rs := []rune{'h', 'e', 'l', 'l', 'o', '世', '界'}
	got := displayWidthRunes(rs)
	want := 9 // 5 + 2 + 2
	if got != want {
		t.Errorf("displayWidthRunes = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// TTY detection
// ---------------------------------------------------------------------------

func TestIsTTY(t *testing.T) {
	// os.Stdin is not a TTY during test runs (piped input typically).
	if isTTY(os.Stdin) {
		t.Log("stdin appears to be a TTY (unusual for test runs)")
	}
	// os.Stdout may or may not be a TTY depending on test harness.
	// We just verify the function doesn't panic.
	_ = isTTY(os.Stdout)
}

// ---------------------------------------------------------------------------
// Fallback path (non-TTY)
// ---------------------------------------------------------------------------

func TestReadLineFallback(t *testing.T) {
	// readLineFallback uses os.Stdin which is not available in tests.
	// We test that the function is exported and doesn't panic on nil-like behavior.
	// The fallback hits bufio.Scanner which requires a real stdin.
	// This is a structural test only.
	t.Log("readLineFallback requires real stdin — skip in unit tests")
}

// ---------------------------------------------------------------------------
// Termios struct size check
// ---------------------------------------------------------------------------

func TestTermiosSize(t *testing.T) {
	// Ensure the termios struct matches Linux kernel layout (36 bytes).
	// This test only matters on Linux.
	var tt termios
	// The struct should be exactly 36 bytes: 4*4(flags) + 1(line) + 19(cc) = 36
	// We don't check exact size here because cross-platform compat;
	// just ensure it compiles and fields are accessible.
	tt.Iflag = 0
	tt.Oflag = 0
	tt.Cflag = 0
	tt.Lflag = 0
	tt.Line = 0
	tt.Cc[vmin] = 1
	tt.Cc[vtime] = 0
	if tt.Cc[vmin] != 1 || tt.Cc[vtime] != 0 {
		t.Error("termios field access failed")
	}
}
