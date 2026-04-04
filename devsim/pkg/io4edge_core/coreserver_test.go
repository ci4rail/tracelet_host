package io4edgecore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirmwareFromFileHybridPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, firmwareFile)

	payload := append([]byte(`{"name":"hybrid-fw","version":"1.2.3"}`), 0x00, 0xff, 0x10, 'B')
	require.NoError(t, os.WriteFile(path, payload, 0644))

	fw, ok := firmwareFromFile(path)
	require.True(t, ok)
	require.NotNil(t, fw)
	require.Equal(t, "hybrid-fw", fw.Name)
	require.Equal(t, "1.2.3", fw.Version)
}
