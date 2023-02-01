package datastore

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	f := pflag.FlagSet{}
	expected := Config{}
	err := RegisterDatastoreFlagsWithPrefix(&f, "", &expected)
	require.NoError(t, err)
	received := DefaultDatastoreConfig()
	require.Equal(t, expected, *received)
}

func TestLoadDatastoreFromFileContents(t *testing.T) {
	ctx := context.Background()
	ds, err := NewDatastore(ctx,
		SetBootstrapFileContents(map[string][]byte{"test": []byte("schema: definition user{}")}),
		WithEngine(MemoryEngine))
	require.NoError(t, err)

	revision, err := ds.HeadRevision(ctx)
	require.NoError(t, err)
	namespaces, err := ds.SnapshotReader(revision).ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, namespaces, 1)
	require.Equal(t, "user", namespaces[0].Name)
}

func TestLoadDatastoreFromFile(t *testing.T) {
	file, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = file.Write([]byte("schema: definition user{}"))
	require.NoError(t, err)

	ctx := context.Background()
	ds, err := NewDatastore(ctx,
		SetBootstrapFiles([]string{file.Name()}),
		WithEngine(MemoryEngine))
	require.NoError(t, err)

	revision, err := ds.HeadRevision(ctx)
	require.NoError(t, err)
	namespaces, err := ds.SnapshotReader(revision).ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, namespaces, 1)
	require.Equal(t, "user", namespaces[0].Name)
}

func TestLoadDatastoreFromFileAndContents(t *testing.T) {
	file, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = file.Write([]byte("schema: definition repository{}"))
	require.NoError(t, err)

	ctx := context.Background()
	ds, err := NewDatastore(ctx,
		SetBootstrapFiles([]string{file.Name()}),
		SetBootstrapFileContents(map[string][]byte{"test": []byte("schema: definition user{}")}),
		WithEngine(MemoryEngine))
	require.NoError(t, err)

	revision, err := ds.HeadRevision(ctx)
	require.NoError(t, err)
	namespaces, err := ds.SnapshotReader(revision).ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, namespaces, 2)
	namespaceNames := []string{namespaces[0].Name, namespaces[1].Name}
	require.Contains(t, namespaceNames, "user")
	require.Contains(t, namespaceNames, "repository")
}
