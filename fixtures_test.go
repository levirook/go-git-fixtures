package fixtures_test

import (
	"strconv"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	fixtures "github.com/go-git/go-git-fixtures/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDotGit(t *testing.T) {
	t.Parallel()

	fs := fixtures.Basic().One().DotGit(fixtures.WithTargetDir(t.TempDir))
	files, err := fs.ReadDir("/")
	require.NoError(t, err)
	assert.Greater(t, len(files), 1)

	fs = fixtures.Basic().One().DotGit(fixtures.WithMemFS())
	files, err = fs.ReadDir("/")
	require.NoError(t, err)
	assert.Greater(t, len(files), 1)
}

//nolint:cyclop
func TestEmbeddedFiles(t *testing.T) {
	t.Parallel()

	for i, f := range fixtures.All() {
		if f.PackfileHash != "" {
			if f.Packfile() == nil {
				assert.Fail(t, "failed to get pack file", i)
			}
			// skip pack file ee4fef0 as it does not have an idx file.
			if f.PackfileHash != "ee4fef0ef8be5053ebae4ce75acf062ddf3031fb" && f.Idx() == nil {
				assert.Fail(t, "failed to get idx file", i)
			}
		}

		if f.WorktreeHash != "" {
			if f.Worktree(fixtures.WithMemFS()) == nil {
				assert.Fail(t, "[mem] failed to get worktree", i)
			}

			if f.Worktree(fixtures.WithTargetDir(t.TempDir)) == nil {
				assert.Fail(t, "[tempdir] failed to get worktree", i)
			}
		}

		if f.DotGitHash != "" {
			if f.DotGit(fixtures.WithMemFS()) == nil {
				assert.Fail(t, "[mem] failed to get dotgit", i)
			}

			if f.DotGit(fixtures.WithTargetDir(t.TempDir)) == nil {
				assert.Fail(t, "[tempdir] failed to get dotgit", i)
			}
		}
	}
}

func TestRevFiles(t *testing.T) {
	t.Parallel()

	f := fixtures.ByTag("packfile-sha256").One()

	assert.NotNil(t, f)
	assert.NotNil(t, f.Rev(), "failed to get rev file")
}

func TestAll(t *testing.T) {
	t.Parallel()

	fs := fixtures.All()

	assert.Len(t, fs, 39)
}

func TestByTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag string
		len int
	}{
		{tag: "packfile", len: 20},
		{tag: "ofs-delta", len: 3},
		{tag: ".git", len: 12},
		{tag: "merge-conflict", len: 1},
		{tag: "worktree", len: 6},
		{tag: "submodule", len: 1},
		{tag: "tags", len: 1},
		{tag: "notes", len: 1},
		{tag: "multi-packfile", len: 1},
		{tag: "diff-tree", len: 7},
		{tag: "packfile-sha256", len: 1},
	}

	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()

			f := fixtures.ByTag(tc.tag)
			assert.Len(t, f, tc.len)
		})
	}
}

func TestByURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		URL string
		len int
	}{
		{URL: "https://github.com/git-fixtures/root-references.git", len: 1},
		{URL: "https://github.com/git-fixtures/basic.git", len: 9},
		{URL: "https://github.com/git-fixtures/submodule.git", len: 1},
		{URL: "https://github.com/src-d/go-git.git", len: 1},
		{URL: "https://github.com/git-fixtures/tags.git", len: 1},
		{URL: "https://github.com/spinnaker/spinnaker.git", len: 1},
		{URL: "https://github.com/jamesob/desk.git", len: 1},
		{URL: "https://github.com/cpcs499/Final_Pres_P.git", len: 1},
		{URL: "https://github.com/github/gem-builder.git", len: 1},
		{URL: "https://github.com/githubtraining/example-branches.git", len: 1},
		{URL: "https://github.com/rumpkernel/rumprun-xen.git", len: 1},
		{URL: "https://github.com/mcuadros/skeetr.git", len: 1},
		{URL: "https://github.com/dezfowler/LiteMock.git", len: 1},
		{URL: "https://github.com/tyba/storable.git", len: 1},
		{URL: "https://github.com/toqueteos/ts3.git", len: 1},
		{URL: "https://github.com/git-fixtures/empty.git", len: 1},
	}

	for _, tc := range tests {
		t.Run(tc.URL, func(t *testing.T) {
			t.Parallel()

			f := fixtures.ByURL(tc.URL)
			assert.Len(t, f, tc.len)
		})
	}
}

func TestIdx(t *testing.T) {
	t.Parallel()

	for i, f := range fixtures.ByTag("packfile") {
		t.Run("#"+strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			index := f.Idx()
			assert.NotNil(t, index)

			err := index.Close()
			assert.NoError(t, err)
		})
	}
}

func TestObjects(t *testing.T) {
	t.Parallel()

	t.Run("basic ofs-delta fixture", testObjectsBasicOFSDelta)

	t.Run("no packfile hash returns nil", func(t *testing.T) {
		t.Parallel()

		f := fixtures.ByTag("merge-conflict").One()
		require.NotNil(t, f)

		assert.Nil(t, f.Objects())
	})

	t.Run("thin pack without idx returns nil", func(t *testing.T) {
		t.Parallel()

		f := fixtures.ByTag("thinpack").One()
		require.NotNil(t, f)

		assert.Nil(t, f.Objects())
	})
}

func testObjectsBasicOFSDelta(t *testing.T) {
	t.Helper()
	t.Parallel()

	// The basic ofs-delta fixture has 31 known objects.
	f := fixtures.ByTag("ofs-delta").ByTag("packfile").
		Exclude("root-reference").One()
	require.NotNil(t, f)

	objs := f.Objects()
	require.NotNil(t, objs)
	assert.Len(t, objs, 31)

	// Build a lookup map by hash for convenient per-object assertions.
	byHash := make(map[string]fixtures.PackfileObject, len(objs))
	for _, o := range objs {
		byHash[o.Hash] = o
	}

	// Spot-check known objects. Objects stored as deltas (ofs-delta /
	// ref-delta) report the raw packfile type, not the resolved logical type.
	type entry struct {
		hash   string
		otype  fixtures.ObjectType
		size   int64
		offset int64
		crc    uint32
	}

	tests := []entry{
		{"e8d3ffab552895c19b9fcf7aa264d277cde33881", fixtures.ObjectTypeCommit, 254, 12, 0xaa07ba4b},
		{"918c48b83bd081e863dbe1b80f8998f058cd8294", fixtures.ObjectTypeCommit, 242, 286, 0x12438846},
		{"af2d6a6954d532f8ffb47615169c8fdf9d383a1a", fixtures.ObjectTypeCommit, 242, 449, 0x2905a38c},
		{"1669dce138d9b841a518c64b10914d88f5e488ea", fixtures.ObjectTypeCommit, 333, 615, 0xd9429436},
		{"32858aad3c383ed1ff0a0f9bdf231d54a00c9e88", fixtures.ObjectTypeBlob, 189, 1524, 0x1f08118a},
		{"dbd3641b371024f44d0e469a9c8f5457b0660de1", fixtures.ObjectTypeTree, 272, 84115, 0x901cce2c},
		// Stored as ofs-delta in this packfile (delta-compressed commit).
		{"6ecf0ef2c2dffb796033e5a02219af86ec6584e5", fixtures.ObjectTypeOFSDelta, 93, 186, 0xf706df58},
	}

	for _, tc := range tests {
		t.Run(tc.hash[:8], func(t *testing.T) {
			t.Parallel()

			o, ok := byHash[tc.hash]
			require.True(t, ok, "object %s not found", tc.hash)
			assert.Equal(t, tc.otype, o.Type)
			assert.Equal(t, tc.size, o.Size)
			assert.Equal(t, tc.offset, o.Offset)
			assert.Equal(t, tc.crc, o.CRC)
		})
	}
}

func TestObjectTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		otype fixtures.ObjectType
		want  string
	}{
		{fixtures.ObjectTypeCommit, "commit"},
		{fixtures.ObjectTypeTree, "tree"},
		{fixtures.ObjectTypeBlob, "blob"},
		{fixtures.ObjectTypeTag, "tag"},
		{fixtures.ObjectTypeOFSDelta, "ofs-delta"},
		{fixtures.ObjectTypeREFDelta, "ref-delta"},
		{fixtures.ObjectType(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.otype.String())
		})
	}
}

func TestWithMemFS(t *testing.T) {
	t.Parallel()

	f := fixtures.Basic().One()
	require.NotNil(t, f)

	fs := f.DotGit(fixtures.WithMemFS())
	require.NotNil(t, fs)

	files, err := fs.ReadDir("/")
	require.NoError(t, err)
	assert.NotEmpty(t, files)

	stat, err := fs.Stat("config")
	require.NoError(t, err)
	assert.NotNil(t, stat)
}

func TestWithTargetDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options []osfs.Option
	}{
		{
			name:    "no options",
			options: nil,
		},
		{
			name:    "with chroot",
			options: []osfs.Option{osfs.WithChrootOS()},
		},
		{
			name:    "with bound",
			options: []osfs.Option{osfs.WithBoundOS()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := fixtures.Basic().One()
			require.NotNil(t, f)

			fs := f.DotGit(fixtures.WithTargetDir(t.TempDir, tc.options...))
			require.NotNil(t, fs)

			files, err := fs.ReadDir("/")
			require.NoError(t, err)
			assert.NotEmpty(t, files)

			stat, err := fs.Stat("config")
			require.NoError(t, err)
			assert.NotNil(t, stat)
		})
	}
}
