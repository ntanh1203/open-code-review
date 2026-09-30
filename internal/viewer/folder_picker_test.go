// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"context"
	"testing"
)

func TestMacOSFolderPickerRejectsUnsupportedOS(t *testing.T) {
	_, err := pickFolder(context.Background(), "linux")
	if err == nil {
		t.Fatal("pickFolder on unsupported OS returned nil error")
	}
}
