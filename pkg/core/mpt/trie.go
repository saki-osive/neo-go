package mpt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

// Trie is an MPT trie storing all key-value pairs.
type Trie struct {
	Store *storage.MemCachedStore

	updated map[util.Uint256]int
	root    Node
}

// ErrNotFound is returned when requested trie item is missing.
var ErrNotFound = errors.New("item not found")

// NewTrie returns new MPT trie. It accepts a MemCachedStore to decouple storage errors from logic errors
// so that all storage errors are processed during `store.Persist()` at the caller.
// This also has the benefit, that every `Put` can be considered an atomic operation.
func NewTrie(root Node, store *storage.MemCachedStore) *Trie {
	if root == nil {
		root = new(HashNode)
	}

	return &Trie{
		Store: store,
		root:  root,

		updated: make(map[util.Uint256]int),
	}
}

// Get returns value for the provided key in t.
func (t *Trie) Get(key []byte) ([]byte, error) {
	path := toNibbles(key)
	r, bs, err := t.getWithPath(t.root, path)
	if err != nil {
		return nil, err
	}
	t.root = r
	return bs, nil
}

// getWithPath returns value the provided path in a subtrie rooting in curr.
// It also returns a current node with all hash nodes along the path
// replaced to their "unhashed" counterparts.
func (t *Trie) getWithPath(curr Node, path []byte) (Node, []byte, error) {
	switch n := curr.(type) {
	case *LeafNode:
		if len(path) == 0 {
			return curr, copySlice(n.value), nil
		}
	case *BranchNode:
		i, path := splitPath(path)
		r, bs, err := t.getWithPath(n.Children[i], path)
		if err != nil {
			return nil, nil, err
		}
		n.Children[i] = r
		return n, bs, nil
	case *HashNode:
		if !n.IsEmpty() {
			if r, err := t.getFromStore(n.hash); err == nil {
				return t.getWithPath(r, path)
			}
		}
	case *ExtensionNode:
		if bytes.HasPrefix(path, n.key) {
			r, bs, err := t.getWithPath(n.next, path[len(n.key):])
			if err != nil {
				return nil, nil, err
			}
			n.next = r
			return curr, bs, err
		}
	default:
		panic("invalid MPT node type")
	}
	return curr, nil, ErrNotFound
}

// Put puts key-value pair in t.
func (t *Trie) Put(key, value []byte) error {
	if len(key) > MaxKeyLength {
		return errors.New("key is too big")
	} else if len(value) > MaxValueLength {
		return errors.New("value is too big")
	}
	if len(value) == 0 {
		return t.Delete(key)
	}
	path := toNibbles(key)
	n := NewLeafNode(value)
	r, err := t.putIntoNode(t.root, path, n)
	if err != nil {
		return err
	}
	t.root = r
	return nil
}

// putIntoLeaf puts val to trie if current node is a Leaf.
// It returns Node if curr needs to be replaced and error if any.
func (t *Trie) putIntoLeaf(curr *LeafNode, path []byte, val Node) (Node, error) {
	v := val.(*LeafNode)
	if len(path) == 0 {
		t.removeRef(curr.Hash())
		t.addRef(val.Hash())
		return v, nil
	}

	b := NewBranchNode()
	b.Children[path[0]] = t.newSubTrie(path[1:], v, true)
	b.Children[lastChild] = curr
	t.addRef(b.Hash())
	return b, nil
}

// putIntoBranch puts val to trie if current node is a Branch.
// It returns Node if curr needs to be replaced and error if any.
func (t *Trie) putIntoBranch(curr *BranchNode, path []byte, val Node) (Node, error) {
	i, path := splitPath(path)
	t.removeRef(curr.Hash())
	r, err := t.putIntoNode(curr.Children[i], path, val)
	if err != nil {
		return nil, err
	}
	curr.Children[i] = r
	curr.invalidateCache()
	t.addRef(curr.Hash())
	return curr, nil
}

// putIntoExtension puts val to trie if current node is an Extension.
// It returns Node if curr needs to be replaced and error if any.
func (t *Trie) putIntoExtension(curr *ExtensionNode, path []byte, val Node) (Node, error) {
	t.removeRef(curr.Hash())
	if bytes.HasPrefix(path, curr.key) {
		r, err := t.putIntoNode(curr.next, path[len(curr.key):], val)
		if err != nil {
			return nil, err
		}
		curr.next = r
		curr.invalidateCache()
		t.addRef(curr.Hash())
		return curr, nil
	}

	pref := lcp(curr.key, path)
	lp := len(pref)
	keyTail := curr.key[lp:]
	pathTail := path[lp:]

	s1 := t.newSubTrie(keyTail[1:], curr.next, false)
	b := NewBranchNode()
	b.Children[keyTail[0]] = s1

	i, pathTail := splitPath(pathTail)
	s2 := t.newSubTrie(pathTail, val, true)
	b.Children[i] = s2

	t.addRef(b.Hash())
	if lp > 0 {
		e := NewExtensionNode(copySlice(pref), b)
		t.addRef(e.Hash())
		return e, nil
	}
	return b, nil
}

// putIntoHash puts val to trie if current node is a HashNode.
// It returns Node if curr needs to be replaced and error if any.
func (t *Trie) putIntoHash(curr *HashNode, path []byte, val Node) (Node, error) {
	if curr.IsEmpty() {
		hn := t.newSubTrie(path, val, true)
		return hn, nil
	}

	result, err := t.getFromStore(curr.hash)
	if err != nil {
		return nil, err
	}
	return t.putIntoNode(result, path, val)
}

// newSubTrie create new trie containing node at provided path.
func (t *Trie) newSubTrie(path []byte, val Node, newVal bool) Node {
	if newVal {
		t.addRef(val.Hash())
	}
	if len(path) == 0 {
		return val
	}
	e := NewExtensionNode(path, val)
	t.addRef(e.Hash())
	return e
}

// putIntoNode puts val with provided path inside curr and returns updated node.
// Reference counters are updated for both curr and returned value.
func (t *Trie) putIntoNode(curr Node, path []byte, val Node) (Node, error) {
	switch n := curr.(type) {
	case *LeafNode:
		return t.putIntoLeaf(n, path, val)
	case *BranchNode:
		return t.putIntoBranch(n, path, val)
	case *ExtensionNode:
		return t.putIntoExtension(n, path, val)
	case *HashNode:
		return t.putIntoHash(n, path, val)
	default:
		panic("invalid MPT node type")
	}
}

// Delete removes key from trie.
// It returns no error on missing key.
func (t *Trie) Delete(key []byte) error {
	path := toNibbles(key)
	r, err := t.deleteFromNode(t.root, path)
	if err != nil {
		return err
	}
	t.root = r
	return nil
}

func (t *Trie) deleteFromBranch(b *BranchNode, path []byte) (Node, error) {
	i, path := splitPath(path)
	h := b.Hash()
	r, err := t.deleteFromNode(b.Children[i], path)
	if err != nil {
		return nil, err
	}
	t.removeRef(h)
	b.Children[i] = r
	b.invalidateCache()
	var count, index int
	for i := range b.Children {
		h, ok := b.Children[i].(*HashNode)
		if !ok || !h.IsEmpty() {
			index = i
			count++
		}
	}
	// count is >= 1 because branch node had at least 2 children before deletion.
	if count > 1 {
		t.addRef(b.Hash())
		return b, nil
	}
	c := b.Children[index]
	if index == lastChild {
		return c, nil
	}
	if h, ok := c.(*HashNode); ok {
		c, err = t.getFromStore(h.Hash())
		if err != nil {
			return nil, err
		}
	}
	if e, ok := c.(*ExtensionNode); ok {
		t.removeRef(e.Hash())
		e.key = append([]byte{byte(index)}, e.key...)
		e.invalidateCache()
		t.addRef(e.Hash())
		return e, nil
	}

	e := NewExtensionNode([]byte{byte(index)}, c)
	t.addRef(e.Hash())
	return e, nil
}

func (t *Trie) deleteFromExtension(n *ExtensionNode, path []byte) (Node, error) {
	if !bytes.HasPrefix(path, n.key) {
		return nil, ErrNotFound
	}
	h := n.Hash()
	r, err := t.deleteFromNode(n.next, path[len(n.key):])
	if err != nil {
		return nil, err
	}
	t.removeRef(h)
	switch nxt := r.(type) {
	case *ExtensionNode:
		t.removeRef(nxt.Hash())
		n.key = append(n.key, nxt.key...)
		n.next = nxt.next
	case *HashNode:
		if nxt.IsEmpty() {
			return nxt, nil
		}
	default:
		n.next = r
	}
	n.invalidateCache()
	t.addRef(n.Hash())
	return n, nil
}

// deleteFromNode removes value with provided path from curr and returns an updated node.
// Reference counters are updated for both curr and returned value.
func (t *Trie) deleteFromNode(curr Node, path []byte) (Node, error) {
	switch n := curr.(type) {
	case *LeafNode:
		if len(path) == 0 {
			t.removeRef(curr.Hash())
			return new(HashNode), nil
		}
		return nil, ErrNotFound
	case *BranchNode:
		return t.deleteFromBranch(n, path)
	case *ExtensionNode:
		return t.deleteFromExtension(n, path)
	case *HashNode:
		if n.IsEmpty() {
			return nil, ErrNotFound
		}
		newNode, err := t.getFromStore(n.Hash())
		if err != nil {
			return nil, err
		}
		return t.deleteFromNode(newNode, path)
	default:
		panic("invalid MPT node type")
	}
}

// StateRoot returns root hash of t.
func (t *Trie) StateRoot() util.Uint256 {
	if hn, ok := t.root.(*HashNode); ok && hn.IsEmpty() {
		return util.Uint256{}
	}
	return t.root.Hash()
}

func makeStorageKey(mptKey []byte) []byte {
	return append([]byte{byte(storage.DataMPT)}, mptKey...)
}

// Flush puts every node in the trie except Hash ones to the storage.
// Because we care only about block-level changes, there is no need to put every
// new node to storage. Normally, flush should be called with every StateRoot persist, i.e.
// after every block.
func (t *Trie) Flush() {
	flushed := make(map[util.Uint256]struct{})
	w := io.NewBufBinWriter()
	t.flush(t.root, w, flushed)
	for h, cnt := range t.updated {
		switch {
		case cnt > 0:
			// BUG: node has references but is not in trie.
			// FIXME remove after debugging
			panic(fmt.Sprintf("not all items were processed: %s cnt %d", h.StringBE(), cnt))
		case cnt == 0:
		default:
			t.updateRefCount(h, nil, w)
		}
	}
	t.updated = make(map[util.Uint256]int)
}

func (t *Trie) flush(node Node, w *io.BufBinWriter, flushed map[util.Uint256]struct{}) {
	if node.IsFlushed() {
		return
	}
	switch n := node.(type) {
	case *BranchNode:
		for i := range n.Children {
			t.flush(n.Children[i], w, flushed)
		}
	case *ExtensionNode:
		t.flush(n.next, w, flushed)
	case *HashNode:
		return
	}
	t.putToStoreWithBuf(node, w, flushed)
}

func (t *Trie) putToStoreWithBuf(n Node, w *io.BufBinWriter, flushed map[util.Uint256]struct{}) {
	if n.Type() == HashT {
		panic("can't put hash node in trie")
	}
	// n.SetFlushed() saves info about flushed nodes between consecutive flushes.
	// flushed map contains nodes which were flushed during current invocation
	// so here we mark node as flushed and perform real flush only if it wasn't done before.
	n.SetFlushed()
	if _, ok := flushed[n.Hash()]; ok {
		return
	}
	if t.updated[n.Hash()] != 0 {
		t.updateRefCount(n.Hash(), n, w)
	}
	delete(t.updated, n.Hash())
	flushed[n.Hash()] = struct{}{}
}

func (t *Trie) updateRefCount(h util.Uint256, n Node, w *io.BufBinWriter) {
	var cnt int
	key := makeStorageKey(h.BytesBE())
	data, err := t.Store.Get(key)
	if err == nil {
		cnt = int(binary.LittleEndian.Uint32(data))
	} else {
		if n == nil {
			// BUG: item must me in store.
			// FIXME remove after debugging
			panic(fmt.Sprintf("item is not in store in `updateRefCount`: %s", h.StringBE()))
		}
		w.Reset()
		no := NodeObject{Node: n}
		no.EncodeBinary(w.BinWriter)
		data = w.Bytes()
	}
	cnt += t.updated[h]
	switch {
	case cnt < 0:
		// BUG: negative reference count
		// FIXME remove after debugging
		panic(fmt.Sprintf("negative reference count: %s new %d, upd %d", h.StringBE(), cnt, t.updated[h]))
	case cnt == 0:
		_ = t.Store.Delete(key)
	default:
		binary.LittleEndian.PutUint32(data, uint32(cnt))
		_ = t.Store.Put(key, data)
	}
}

func (t *Trie) addRef(h util.Uint256) {
	t.updated[h]++
}

func (t *Trie) removeRef(h util.Uint256) {
	t.updated[h]--
}

func (t *Trie) getFromStore(h util.Uint256) (Node, error) {
	data, err := t.Store.Get(makeStorageKey(h.BytesBE()))
	if err != nil {
		return nil, err
	}

	var n NodeObject
	r := io.NewBinReaderFromBuf(data)
	n.DecodeBinary(r)
	if r.Err != nil {
		return nil, r.Err
	}
	n.Node.(flushedNode).setCache(data[4:], h)
	return n.Node, nil
}

// Collapse compresses all nodes at depth n to the hash nodes.
// Note: this function does not perform any kind of storage flushing so
// `Flush()` should be called explicitly before invoking function.
func (t *Trie) Collapse(depth int) {
	if depth < 0 {
		panic("negative depth")
	}
	t.root = collapse(depth, t.root)
}

func collapse(depth int, node Node) Node {
	if _, ok := node.(*HashNode); ok {
		return node
	} else if depth == 0 {
		return NewHashNode(node.Hash())
	}

	switch n := node.(type) {
	case *BranchNode:
		for i := range n.Children {
			n.Children[i] = collapse(depth-1, n.Children[i])
		}
	case *ExtensionNode:
		n.next = collapse(depth-1, n.next)
	case *LeafNode:
	case *HashNode:
	default:
		panic("invalid MPT node type")
	}
	return node
}
