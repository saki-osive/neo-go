package server

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

type dump []interface{}

type storageOp struct {
	State string `json:"state"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// NEO has some differences of key storing.
// out format: script hash in LE + key
// neo format: script hash in BE + byte(0) + key with 0 between every 16 bytes, padded to len 16.
func toNeoStorageKey(key []byte) []byte {
	if len(key) < util.Uint160Size {
		panic("invalid key in storage")
	}

	var nkey []byte
	for i := util.Uint160Size - 1; i >= 0; i-- {
		nkey = append(nkey, key[i])
	}

	key = key[util.Uint160Size:]

	index := 0
	remain := len(key)
	for remain >= 16 {
		nkey = append(nkey, key[index:index+16]...)
		nkey = append(nkey, 0)
		index += 16
		remain -= 16
	}

	if remain > 0 {
		nkey = append(nkey, key[index:]...)
	}

	padding := 16 - remain
	for i := 0; i < padding; i++ {
		nkey = append(nkey, 0)
	}

	nkey = append(nkey, byte(padding))

	return nkey
}

// batchToMap converts batch to a map so that JSON is compatible
// with https://github.com/NeoResearch/neo-storage-audit/
func batchToMap(index uint32, batch *storage.MemBatch) map[string]interface{} {
	size := len(batch.Put) + len(batch.Deleted)
	ops := make([]storageOp, 0, size)
	for i := range batch.Put {
		key := batch.Put[i].Key
		if len(key) == 0 || key[0] != byte(storage.STStorage) {
			continue
		}

		op := "Added"
		if batch.Put[i].Exists {
			op = "Changed"
		}

		key = toNeoStorageKey(key[1:])
		ops = append(ops, storageOp{
			State: op,
			Key:   hex.EncodeToString(key),
			Value: "00" + hex.EncodeToString(batch.Put[i].Value),
		})
	}

	for i := range batch.Deleted {
		key := batch.Deleted[i].Key
		if len(key) == 0 || key[0] != byte(storage.STStorage) || !batch.Deleted[i].Exists {
			continue
		}

		key = toNeoStorageKey(key[1:])
		ops = append(ops, storageOp{
			State: "Deleted",
			Key:   hex.EncodeToString(key),
		})
	}

	return map[string]interface{}{
		"block":   index,
		"size":    len(ops),
		"storage": ops,
	}
}

func newDump() *dump {
	return new(dump)
}

func (d *dump) add(index uint32, batch *storage.MemBatch) {
	m := batchToMap(index, batch)
	*d = append(*d, m)
}

func (d *dump) tryPersist(prefix string, index uint32) error {
	if index%1000 != 0 {
		return nil
	}

	f, err := createFile(prefix, index)
	if err != nil {
		return err
	}

	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	if err := enc.Encode(*d); err != nil {
		return err
	}

	*d = (*d)[:0]

	return nil
}

// createFile creates directory and file in it for storing blocks up to index.
// Directory structure is the following:
// https://github.com/NeoResearch/neo-storage-audit#folder-organization-where-to-find-the-desired-block
// Dir `BlockStorage_$DIRNO` contains blocks up to $DIRNO (from $DIRNO-100k)
// Inside it there are files grouped by 1k blocks.
// File dump-block-$FILENO.json contains blocks from $FILENO-999, $FILENO
// Example: file `BlockStorage_100000/dump-block-6000.json` contains blocks from 5001 to 6000.
func createFile(prefix string, index uint32) (*os.File, error) {
	dirN := (index-1)/100000 + 1
	dir := fmt.Sprintf("BlockStorage_%d00000", dirN)

	path := filepath.Join(prefix, dir)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return nil, err
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("file `%s` is not a directory", path)
	}

	fileN := (index-1)/1000 + 1
	file := fmt.Sprintf("dump-block-%d000.json", fileN)
	path = filepath.Join(path, file)

	return os.Create(path)
}