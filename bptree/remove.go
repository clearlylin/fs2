package bptree

import ()

import (
	"github.com/timtadh/fs2/errors"
)

// Remove one or more key/value pairs at the given key. The callback
// `where` will be called for each pair encountered and the value will
// be passed into the callback. If `where` returns true the item is
// removed otherwise it is left unchanged. To remove all items with a
// particular key simply:
//
// 	err = bpt.Remove(key, func(value []byte) bool { return true })
// 	if err != nil {
// 		panic(err)
// 	}
//
func (self *BpTree) Remove(key []byte, where func([]byte) bool) (err error) {
	a, err := self.remove(self.meta.root, 0, key, where)
	if err != nil {
		return err
	}
	if a == 0 {
		a, err = self.newLeaf()
		if err != nil {
			return err
		}
	}
	self.meta.itemCount -= 1
	self.meta.root = a
	return self.writeMeta()
}

func (self *BpTree) remove(n, sibling uint64, key []byte, where func([]byte) bool) (a uint64, err error) {
	var flags Flag
	err = self.bf.Do(n, 1, func(bytes []byte) error {
		flags = Flag(bytes[0])
		return nil
	})
	if err != nil {
		return 0, err
	}
	if flags&iNTERNAL != 0 {
		return self.internalRemove(n, sibling, key, where)
	} else if flags&lEAF != 0 {
		return self.leafRemove(n, sibling, key, where)
	} else {
		return 0, errors.Errorf("Unknown block type")
	}
}

func (self *BpTree) internalRemove(n, sibling uint64, key []byte, where func([]byte) bool) (a uint64, err error) {
	var i int
	var kid uint64
	err = self.doInternal(n, func(n *internal) error {
		var has bool
		i, has = find(n, key)
		if !has && i > 0 {
			// if it doesn't have it and the index > 0 then we have the
			// next block so we have to subtract one from the index.
			i--
		}
		kid = *n.ptr(i)
		if i+1 < int(n.meta.keyCount) {
			sibling = *n.ptr(i + 1)
		} else if sibling != 0 {
			return self.doInternal(sibling, func(m *internal) error {
				sibling = *m.ptr(0)
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	kid, err = self.remove(kid, sibling, key, where)
	if err != nil {
		return 0, err
	}
	if kid == 0 {
		err = self.doInternal(n, func(n *internal) error {
			return n.delItemAt(i)
		})
		if err != nil {
			return 0, err
		}
	} else {
		err = self.doInternal(n, func(n *internal) error {
			*n.ptr(i) = kid
			return self.firstKey(kid, func(kid_key []byte) error {
				copy(n.key(i), kid_key)
				return nil
			})
		})
		if err != nil {
			return 0, err
		}
	}
	var keyCount uint16
	err = self.doInternal(n, func(n *internal) error {
		keyCount = n.meta.keyCount
		return nil
	})
	if err != nil {
		return 0, err
	}
	if keyCount == 0 {
		return 0, nil
	}
	return n, nil
}

func (self *BpTree) leafRemove(a, sibling uint64, key []byte, where func([]byte) bool) (b uint64, err error) {
	var i int
	err = self.doLeaf(a, func(n *leaf) error {
		var has bool
		i, has = find(n, key)
		if !has {
			return errors.Errorf("key was not in tree")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	next, err := self.forwardFrom(a, i, key)
	if err != nil {
		return 0, err
	}
	b = a
	type loc struct {
		a uint64
		i int
	}
	locs := make([]*loc, 0, 10)
	for a, i, err, next = next(); next != nil; a, i, err, next = next() {
		locs = append(locs, &loc{a, i})
	}
	if err != nil {
		return 0, err
	}
	for x := len(locs) - 1; x >= 0; x-- {
		a := locs[x].a
		i := locs[x].i
		err = self.doLeaf(a, func(n *leaf) error {
			var remove bool = false
			err = n.doValueAt(self.bf, i, func(value []byte) error {
				remove = where(value)
				return nil
			})
			if err != nil {
				return err
			}
			if remove {
				// TODO: Add logic to deref and remove big values and keys
				err = n.delItemAt(i)
				if err != nil {
					return err
				}
			}
			if int(n.meta.keyCount) <= 0 {
				err = self.delListNode(a)
				if err != nil {
					return err
				}
				if n.meta.next == 0 {
					b = 0
				} else if sibling == 0 {
					b = 0
				} else if n.meta.next != sibling {
					b = n.meta.next
				} else {
					b = 0
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return b, nil
}

func (self *BpTree) removeBigValue(a uint64, size uint32) (err error) {
	blksize := uint64(self.bf.BlockSize())
	blks := uint64(blksNeeded(self.bf, int(size)))
	for i := uint64(0); i < blks; i++ {
		err = self.bf.Free(a + (blksize * i))
		if err != nil {
			return err
		}
	}
	return nil
}
