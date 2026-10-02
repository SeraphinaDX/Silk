// SPDX-License-Identifier: GPL-3.0-or-later
package terminal

import (
	"strings"
	"testing"
)

func TestInlineSixelMode(t *testing.T) {
	if !strings.Contains(Enter, "\x1b[?80l") || strings.Contains(Enter, "\x1b[?80h") {
		t.Fatal("inline graphics require DECSDM reset")
	}
}
