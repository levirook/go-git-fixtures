package fixtures

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
)

// ObjectType represents the type of a Git object as stored in a packfile.
type ObjectType int

const (
	// ObjectTypeCommit represents a commit object.
	ObjectTypeCommit ObjectType = 1
	// ObjectTypeTree represents a tree object.
	ObjectTypeTree ObjectType = 2
	// ObjectTypeBlob represents a blob object.
	ObjectTypeBlob ObjectType = 3
	// ObjectTypeTag represents a tag object.
	ObjectTypeTag ObjectType = 4
	// ObjectTypeOFSDelta represents an offset-delta object.
	ObjectTypeOFSDelta ObjectType = 6
	// ObjectTypeREFDelta represents a reference-delta object.
	ObjectTypeREFDelta ObjectType = 7
)

// String returns a human-readable name for the object type.
func (t ObjectType) String() string {
	switch t {
	case ObjectTypeCommit:
		return "commit"
	case ObjectTypeTree:
		return "tree"
	case ObjectTypeBlob:
		return "blob"
	case ObjectTypeTag:
		return "tag"
	case ObjectTypeOFSDelta:
		return "ofs-delta"
	case ObjectTypeREFDelta:
		return "ref-delta"
	default:
		return "unknown"
	}
}

// PackfileObject holds the metadata for a single object stored in a packfile.
type PackfileObject struct {
	// Hash is the hex-encoded SHA1 hash of the object.
	Hash string
	// Type is the object type (commit, tree, blob, tag, ofs-delta, ref-delta).
	Type ObjectType
	// Size is the uncompressed size of the object in bytes.
	Size int64
	// Offset is the byte offset of the object within the packfile.
	Offset int64
	// CRC is the CRC32 checksum of the packed object data.
	CRC uint32
}

// idxEntry holds a single entry as parsed from an idx v2 file.
type idxEntry struct {
	hash   string
	crc    uint32
	offset int64
}

// errLargeOffsetOverflow is returned when a large-offset value exceeds the
// int64 range. In practice, packfiles this large (> 9 exabytes) do not exist.
var errLargeOffsetOverflow = errors.New("fixtures: large offset exceeds int64 range")

// Objects returns the metadata for each object stored in the fixture's
// packfile, derived from the packfile index (.idx). The returned slice is
// ordered by object hash (the natural sort order of the index file).
//
// Returns nil when the fixture has no packfile hash or when no index file
// exists for the packfile (e.g. thin packs).
func (f *Fixture) Objects() []PackfileObject {
	if f.PackfileHash == "" {
		return nil
	}

	entries := f.parsePackfileIdx()
	if entries == nil {
		return nil
	}

	packFile, err := Filesystem.Open(fmt.Sprintf("data/pack-%s.pack", f.PackfileHash))
	if err != nil {
		panic(fmt.Errorf("fixtures: opening packfile: %w", err))
	}

	defer packFile.Close()

	return buildPackfileObjects(packFile, entries)
}

// parsePackfileIdx opens and parses the idx file for the fixture's packfile.
// Returns nil when no idx file exists (e.g. thin packs) or the format is
// unsupported. Panics on unexpected read errors.
func (f *Fixture) parsePackfileIdx() []idxEntry {
	idxFile, err := Filesystem.Open(fmt.Sprintf("data/pack-%s.idx", f.PackfileHash))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		panic(fmt.Errorf("fixtures: opening idx file: %w", err))
	}

	defer idxFile.Close()

	entries, err := parseIdxV2(idxFile)
	if err != nil {
		panic(fmt.Errorf("fixtures: parsing idx: %w", err))
	}

	return entries
}

// buildPackfileObjects reads each object header from packFile using the offsets
// in entries and returns the combined PackfileObject slice.
func buildPackfileObjects(packFile interface {
	io.ReadSeeker
}, entries []idxEntry,
) []PackfileObject {
	objects := make([]PackfileObject, len(entries))

	for i, e := range entries {
		_, err := packFile.Seek(e.offset, io.SeekStart)
		if err != nil {
			panic(fmt.Errorf("fixtures: seeking to object %d at offset %d: %w", i, e.offset, err))
		}

		objType, size, err := readPackfileObjectHeader(packFile)
		if err != nil {
			panic(fmt.Errorf("fixtures: reading object header %d: %w", i, err))
		}

		objects[i] = PackfileObject{
			Hash:   e.hash,
			Type:   objType,
			Size:   size,
			Offset: e.offset,
			CRC:    e.crc,
		}
	}

	return objects
}

// parseIdxV2 parses an idx v2 file and returns one entry per stored object.
// Returns nil when the format is not a supported idx v2.
func parseIdxV2(r io.Reader) ([]idxEntry, error) {
	n, err := readIdxV2Header(r)
	if err != nil {
		return nil, err
	}

	if n < 0 {
		return nil, nil // Unsupported idx format.
	}

	hashes, err := readIdxHashes(r, n)
	if err != nil {
		return nil, err
	}

	crcs := make([]uint32, n)

	err = binary.Read(r, binary.BigEndian, crcs)
	if err != nil {
		return nil, fmt.Errorf("reading idx CRC table: %w", err)
	}

	rawOffsets := make([]uint32, n)

	err = binary.Read(r, binary.BigEndian, rawOffsets)
	if err != nil {
		return nil, fmt.Errorf("reading idx offset table: %w", err)
	}

	largeOffsets, err := readLargeOffsets(r, rawOffsets)
	if err != nil {
		return nil, err
	}

	return buildIdxEntries(hashes, crcs, rawOffsets, largeOffsets)
}

// readIdxV2Header reads the magic number, version, and fan-out table from an
// idx v2 file. Returns the total object count (fan_out[255]) on success, or
// -1 if the file is not a supported idx v2.
func readIdxV2Header(r io.Reader) (int, error) {
	var magic [4]byte

	_, err := io.ReadFull(r, magic[:])
	if err != nil {
		return 0, fmt.Errorf("reading idx magic: %w", err)
	}

	if magic != [4]byte{0xff, 0x74, 0x4f, 0x63} {
		return -1, nil // Not an idx v2 file.
	}

	var version uint32

	err = binary.Read(r, binary.BigEndian, &version)
	if err != nil {
		return 0, fmt.Errorf("reading idx version: %w", err)
	}

	if version != 2 {
		return -1, nil // Only idx v2 is supported.
	}

	// Fan-out table: 256 big-endian uint32 values. Entry [255] holds the
	// total object count.
	var fanOut [256]uint32

	err = binary.Read(r, binary.BigEndian, &fanOut)
	if err != nil {
		return 0, fmt.Errorf("reading idx fan-out table: %w", err)
	}

	return int(fanOut[255]), nil
}

// readIdxHashes reads n × 20-byte SHA1 hashes from r and returns them as
// hex-encoded strings.
func readIdxHashes(r io.Reader, n int) ([]string, error) {
	hashes := make([]string, n)

	for i := range n {
		h := make([]byte, 20)

		_, err := io.ReadFull(r, h)
		if err != nil {
			return nil, fmt.Errorf("reading idx hash %d: %w", i, err)
		}

		hashes[i] = hex.EncodeToString(h)
	}

	return hashes, nil
}

// buildIdxEntries combines the parsed idx arrays into a single entry slice.
func buildIdxEntries(hashes []string, crcs, rawOffsets []uint32, largeOffsets []uint64) ([]idxEntry, error) {
	entries := make([]idxEntry, len(hashes))

	for i := range hashes {
		offset, err := resolveOffset(rawOffsets[i], largeOffsets)
		if err != nil {
			return nil, err
		}

		entries[i] = idxEntry{hash: hashes[i], crc: crcs[i], offset: offset}
	}

	return entries, nil
}

// resolveOffset converts a raw 4-byte idx offset to an int64, following large
// offset table references when the MSB is set.
func resolveOffset(raw uint32, large []uint64) (int64, error) {
	if raw&0x80000000 == 0 {
		return int64(raw), nil
	}

	lo := large[raw&0x7fffffff]
	if lo > math.MaxInt64 {
		return 0, errLargeOffsetOverflow
	}

	return int64(lo), nil
}

// readLargeOffsets reads the 8-byte large-offset table from r if any entries
// in rawOffsets reference it (MSB set).
func readLargeOffsets(r io.Reader, rawOffsets []uint32) ([]uint64, error) {
	maxLargeIdx := -1

	for _, o := range rawOffsets {
		if o&0x80000000 != 0 {
			if idx := int(o & 0x7fffffff); idx > maxLargeIdx {
				maxLargeIdx = idx
			}
		}
	}

	if maxLargeIdx < 0 {
		return nil, nil
	}

	largeOffsets := make([]uint64, maxLargeIdx+1)

	err := binary.Read(r, binary.BigEndian, largeOffsets)
	if err != nil {
		return nil, fmt.Errorf("reading idx large-offset table: %w", err)
	}

	return largeOffsets, nil
}

// readPackfileObjectHeader reads the variable-length header at the current
// position in a packfile reader and returns the object type and uncompressed
// size.
func readPackfileObjectHeader(r io.Reader) (ObjectType, int64, error) {
	buf := make([]byte, 1)

	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, 0, err
	}

	objType := ObjectType((buf[0] >> 4) & 0x7)
	size := int64(buf[0] & 0xf)

	for shift := uint(4); buf[0]&0x80 != 0; shift += 7 {
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, 0, err
		}

		size |= int64(buf[0]&0x7f) << shift
	}

	return objType, size, nil
}
