// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC

package testfixtures

import (
	"embed"
	"fmt"
	"os"
	"strings"
)

//go:embed mock_smd_fixture.json
var fixtureFS embed.FS

// LoadMockSMDFixture returns mock SMD fixture bytes from a file path or embedded default.
func LoadMockSMDFixture(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed != "" {
		data, err := os.ReadFile(trimmed)
		if err != nil {
			return nil, fmt.Errorf("read mock fixture from %q: %w", trimmed, err)
		}
		return data, nil
	}

	data, err := fixtureFS.ReadFile("mock_smd_fixture.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded mock fixture: %w", err)
	}
	return data, nil
}
