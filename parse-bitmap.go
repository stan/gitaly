package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
)

var (
	idxFileRegex = regexp.MustCompile(`\A(.*/pack-)([0-9a-f]{40})\.idx\z`)
	printMap     = flag.Bool("map", false, "output map visualization of packfile")
)

func main() {
	flag.Parse()

	if len(flag.Args()) != 1 {
		log.Fatal("usage: parse-bitmap [-map] PACK_IDX")
	}

	if err := _main(flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}

func _main(packIdx string) error {
	reMatches := idxFileRegex.FindStringSubmatch(packIdx)
	if len(reMatches) == 0 {
		return fmt.Errorf("invalid idx filename: %q", packIdx)
	}

	packBase := reMatches[1] + reMatches[2]
	packID := reMatches[2]

	log.Print("parsing idx")
	idxObjects, err := readIndex(packBase, packID)
	if err != nil {
		return err
	}
	log.Printf("found %d objects", len(idxObjects))

	f, err := os.Open(packBase + ".bitmap")
	if err != nil {
		return err
	}
	defer f.Close()

	tr := NewTrailerReader(f, sumSize)
	sum := sha1.New()
	r := bufio.NewReader(io.TeeReader(tr, sum))

	nBitmapCommits, err := parseBitmapHeader(r, packID)
	if err != nil {
		return err
	}

	// Sort objects by pack offset, because the object type bitmaps use
	// packfile order.
	packObjects := make([]*packObject, len(idxObjects))
	copy(packObjects, idxObjects)

	log.Print("sorting object list")
	sort.Sort(packObjectList(packObjects))

	// The type bitmaps come in this specific order: commit, tree, blob, tag.
	log.Print("labeling objects")
	for _, t := range []objectType{tCommit, tTree, tBlob, tTag} {
		setFunc := func(i uint32) error {
			obj := packObjects[i]
			if obj.objectType != tUnknown {
				return fmt.Errorf("type already set for object %d", i)
			}

			obj.objectType = t

			return nil
		}

		if err := parseEWAH(r, setFunc); err != nil {
			return err
		}
	}

	for i := uint32(0); i < nBitmapCommits; i++ {
		const entryHeaderLen = 6
		if _, err := r.Discard(entryHeaderLen); err != nil {
			return err
		}

		if err := skipEWAH(r); err != nil {
			return err
		}
	}

	if _, err := r.Peek(1); err != io.EOF {
		return fmt.Errorf("expected EOF, got %v", err)
	}

	expectedSum, err := tr.Trailer()
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedSum, sum.Sum(nil)) {
		return fmt.Errorf("bitmap checksum mismatch")
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	fmt.Fprintf(out, "# pack-%s\n", packID)
	for _, o := range packObjects {
		if *printMap {
			c := ""
			switch o.objectType {
			case tBlob:
				c = "b"
			case tCommit:
				c = "C"
			case tTree:
				c = "e"
			case tTag:
				c = "T"
			}

			fmt.Fprint(out, c+" ")
		} else {
			fmt.Fprintln(out, o)
		}
	}

	return nil
}

type packObjectList []*packObject

func (ol packObjectList) Len() int           { return len(ol) }
func (ol packObjectList) Less(i, j int) bool { return ol[i].offset < ol[j].offset }
func (ol packObjectList) Swap(i, j int)      { ol[i], ol[j] = ol[j], ol[i] }

func skipEWAH(r io.Reader) error {
	// discard bit count
	if _, err := readUint32(r); err != nil {
		return err
	}

	words, err := readUint32(r)
	if err != nil {
		return err
	}

	if _, err := io.CopyN(ioutil.Discard, r, int64(words)*8); err != nil {
		return err
	}

	// discard RLW pointer
	if _, err := readUint32(r); err != nil {
		return nil
	}

	return nil
}

func parseEWAH(r io.Reader, f func(uint32) error) error {
	bits, err := readUint32(r)
	if err != nil {
		return err
	}

	words, err := readUint32(r)
	if err != nil {
		return err
	}

	log.Printf(" ... EWAH with %d bits, %d words", bits, words)

	offset := uint32(0)
	wordReader := io.LimitReader(r, int64(words)*8)
	for {
		header, err := readUint64(wordReader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanBit := int(header & 1)
		nClean := uint32(header >> 1)
		nDirty := uint32(header >> 33)

		for i := uint32(0); i < nClean; i++ {
			for j := 0; j < 64; j++ {
				if cleanBit == 1 {
					if err := f(offset); err != nil {
						return err
					}
				}

				offset++
			}
		}

		for i := uint32(0); i < nDirty; i++ {
			word, err := readUint64(wordReader)
			if err != nil {
				return err
			}

			for j := 0; j < 64; j++ {
				if mask := uint64(1 << j); word&mask >= mask {
					if err := f(offset); err != nil {
						return err
					}
				}

				offset++
			}
		}
	}

	// The EWAH trailer is there to make it easier to append new entries. We
	// are not modifying the EWAH so we don't need the trailer.
	const trailerLen = 4
	if _, err := io.CopyN(ioutil.Discard, r, trailerLen); err != nil {
		return err
	}

	return nil
}

func parseBitmapHeader(r io.Reader, packID string) (uint32, error) {
	const headerLen = 32
	header, err := readN(r, headerLen)
	if err != nil {
		return 0, err
	}

	const sig = "BITM\x00\x01"
	if actualSig := string(header[:len(sig)]); actualSig != sig {
		return 0, fmt.Errorf("unexpected signature %q", actualSig)
	}
	header = header[len(sig):]

	const flagLen = 2
	flags := binary.BigEndian.Uint16(header[:flagLen])
	const minFlags = 1
	if flags&minFlags < minFlags {
		return 0, fmt.Errorf("invalid flags %x", flags)
	}
	header = header[flagLen:]

	count := binary.BigEndian.Uint32(header[:4])
	header = header[4:]

	if s := hex.EncodeToString(header); s != packID {
		return 0, fmt.Errorf("unexpected pack ID in bitmap header: %s", s)
	}

	return count, nil
}

type objectType int

const (
	tUnknown objectType = iota
	tBlob
	tCommit
	tTree
	tTag
)

type packObject struct {
	oid string
	objectType
	offset uint64
}

func (po packObject) String() string {
	t := "unknown"
	switch po.objectType {
	case tBlob:
		t = "blob"
	case tCommit:
		t = "commit"
	case tTree:
		t = "tree"
	case tTag:
		t = "tag"
	}

	return fmt.Sprintf("%s %s\t%d", po.oid, t, po.offset)
}

const sumSize = 20

func readIndex(packBase, packID string) ([]*packObject, error) {
	f, err := os.Open(packBase + ".idx")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tr := NewTrailerReader(f, sumSize)
	sum := sha1.New()
	r := bufio.NewReader(io.TeeReader(tr, sum))

	const sig = "\377tOc\x00\x00\x00\x02"
	actualSig, err := readN(r, len(sig))
	if s := string(actualSig); s != sig {
		return nil, fmt.Errorf("unexpected idx signature %q", s)
	}

	count, err := nPackObjects(packBase, packID)
	if err != nil {
		return nil, err
	}

	// Fanout is used to speed up random access to the index. We are doing a
	// sequential read so we don't need the fanout table.
	const fanOutLen = 4 * 256
	if _, err := r.Discard(fanOutLen); err != nil {
		return nil, err
	}

	// TODO use a data structure other than a Go slice to hold the index
	// entries? Go slices use int as their index type, and int may not be
	// able to hold MaxUint32.
	if count > math.MaxInt32 {
		return nil, fmt.Errorf("too many objects in packfile to fit in Go slice: %d", count)
	}
	objects := make([]*packObject, count)

	buf := make([]byte, sumSize)
	for i := 0; i < len(objects); i++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		objects[i] = &packObject{oid: hex.EncodeToString(buf)}
	}

	// Discard CRC32 values (one for each object)
	for i := 0; i < len(objects); i++ {
		if _, err := r.Discard(4); err != nil {
			return nil, err
		}
	}

	// Read 4-byte offsets
	has8ByteOffsets := false
	for i := 0; i < len(objects); i++ {
		offset, err := readUint32(r)
		if err != nil {
			return nil, err
		}

		const mask = 1 << 31
		if offset&mask == mask {
			has8ByteOffsets = true
			continue
		}

		objects[i].offset = uint64(offset)
	}

	if has8ByteOffsets {
		for i := 0; i < len(objects); i++ {
			offset, err := readUint64(r)
			if err != nil {
				return nil, err
			}

			// TODO Not clear if all 8-byte offsets are populated, or only those that
			// don't fit into 4 bytes.
			if offset > 0 {
				objects[i].offset = offset
			}
		}
	}

	idxPackID, err := readN(r, sumSize)
	if err != nil {
		return nil, err
	}

	if s := hex.EncodeToString(idxPackID); s != packID {
		return nil, fmt.Errorf("unexpected pack ID in idx: %s", s)
	}

	if _, err := r.Peek(1); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected trailing data, expected EOF")
		}
		return nil, err
	}

	expectedSum, err := tr.Trailer()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(expectedSum, sum.Sum(nil)) {
		return nil, fmt.Errorf("idx file checksum mismatch")
	}

	return objects, nil
}

func readN(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func nPackObjects(packBase, packID string) (uint32, error) {
	f, err := openPack(packBase, packID)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	const headerLen = 12
	header, err := readN(f, headerLen)
	if err != nil {
		return 0, err
	}

	const sig = "PACK\x00\x00\x00\x02"
	if s := string(header[:len(sig)]); s != sig {
		return 0, fmt.Errorf("unexpected pack signature %q", s)
	}
	header = header[len(sig):]

	return binary.BigEndian.Uint32(header), nil
}

func openPack(packBase, packID string) (f *os.File, err error) {
	packPath := packBase + ".pack"
	f, err = os.Open(packPath)
	if err != nil {
		return nil, err
	}

	defer func(f *os.File) {
		if err != nil {
			f.Close()
		}
	}(f) // Bind f early so that we can do "return nil, err".

	if _, err := f.Seek(-sumSize, io.SeekEnd); err != nil {
		return nil, err
	}

	sum, err := readN(f, sumSize)
	if err != nil {
		return nil, err
	}

	if s := hex.EncodeToString(sum); s != packID {
		return nil, fmt.Errorf("unexpected trailing checksum in .pack: %s", s)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return f, nil
}

func readUint32(r io.Reader) (uint32, error) {
	buf, err := readN(r, 4)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(buf), nil
}

func readUint64(r io.Reader) (uint64, error) {
	buf, err := readN(r, 8)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint64(buf), nil
}

type trailerReader struct {
	r           io.Reader
	left, right int
	trailerSize int
	buf         []byte
	atEOF       bool
}

func NewTrailerReader(r io.Reader, trailerSize int) *trailerReader {
	return &trailerReader{
		r:           r,
		trailerSize: trailerSize,
		buf:         make([]byte, 4096),
	}
}

func (tr *trailerReader) Trailer() ([]byte, error) {
	bufLen := tr.right - tr.left
	if !tr.atEOF || bufLen > tr.trailerSize {
		return nil, fmt.Errorf("cannot get trailer before reader has reached EOF")
	}

	if bufLen < tr.trailerSize {
		return nil, fmt.Errorf("not enough bytes to yield trailer")
	}

	return tr.buf[tr.right-tr.trailerSize : tr.right], nil
}

func (tr *trailerReader) Read(p []byte) (int, error) {
	if bufLen := tr.right - tr.left; !tr.atEOF && bufLen <= tr.trailerSize {
		copy(tr.buf, tr.buf[tr.left:tr.right])
		tr.left = 0
		tr.right = bufLen

		m, err := tr.r.Read(tr.buf[tr.right:])
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
			tr.atEOF = true
		}
		tr.right += m
	}

	if tr.right-tr.left <= tr.trailerSize {
		if tr.atEOF {
			return 0, io.EOF
		}
		return 0, nil
	}

	chunk := tr.right - tr.left - tr.trailerSize
	if chunk > len(p) {
		chunk = len(p)
	}

	copy(p, tr.buf[tr.left:tr.left+chunk])
	tr.left += chunk
	return chunk, nil
}
